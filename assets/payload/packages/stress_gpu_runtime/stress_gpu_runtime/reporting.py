"""Bounded-memory statistics, terminal dashboard and transactional exports."""
import json
import math
import os
from pathlib import Path
import shutil
import tempfile

from .interrupts import defer_interrupts

METRICS = ("utilization_percent", "memory_used_bytes", "temperature_celsius", "power_watts", "graphics_clock_mhz", "memory_clock_mhz")


def summarize_sample(state, sample):
    """Update online statistics and retain only the latest observation.

    Args:
        state (dict): Mutable per-GPU summary dictionary.
        sample (dict): UUID-tagged observation with elapsed_seconds.

    Returns:
        None.

    Raises:
        KeyError: If identity or elapsed time is missing.
    """
    board = state.setdefault(sample["uuid"], {"metrics": {}, "latest": {}, "first_temperature": None, "last_temperature": None, "thermal_samples": 0, "thermal_observations": 0, "peak_temperature": None})
    board["latest"] = dict(sample)
    for name in METRICS:
        value = sample.get(name)
        if value is not None and math.isfinite(value):
            metric = board["metrics"].setdefault(name, {"count": 0, "sum": 0.0, "peak": value})
            metric["count"] += 1
            metric["sum"] += value
            metric["peak"] = max(metric["peak"], value)
    temperature = sample.get("temperature_celsius")
    if temperature is not None:
        point = (sample["elapsed_seconds"], temperature)
        if board["first_temperature"] is None:
            board["first_temperature"] = point
        board["last_temperature"] = point
        board["peak_temperature"] = temperature if board["peak_temperature"] is None else max(board["peak_temperature"], temperature)
    if sample.get("thermal_throttling") is not None:
        board["thermal_observations"] += 1
        board["thermal_samples"] += int(bool(sample["thermal_throttling"]))


def metric_text(value, scale=1, suffix=""):
    """Format a nullable measurement without inventing a zero.

    Args:
        value (float | None): Measured value.
        scale (float): Unit conversion divisor.
        suffix (str): Display unit.

    Returns:
        str: One-decimal measurement or N/A.

    Raises:
        None.
    """
    return "N/A" if value is None else f"{value / scale:.1f}{suffix}"


def render_dashboard(config, state, elapsed):
    """Render targets and achieved per-board load as a single frame.

    Args:
        config (dict): Device descriptions, relative load and duration.
        state (dict): Online summaries.
        elapsed (float): Elapsed measured run seconds.

    Returns:
        str: Dashboard text with nullable resources and temperature analysis.

    Raises:
        KeyError: If configuration fields are absent.
    """
    duration = config["duration_seconds"]
    remaining = "continuous" if duration is None else f"{max(0, duration - elapsed):.1f}s remaining"
    rows = [f'NVIDIA stress | target compute/free VRAM {config["load_percent"]:g}% | {elapsed:.1f}s | {remaining}', "Ctrl+C stops every selected GPU. Measurements are board-wide."]
    for device in config["devices"]:
        latest = state.get(device["uuid"], {}).get("latest", {})
        temperature = latest.get("temperature_celsius")
        threshold = device.get("slowdown_celsius")
        condition = "temperature unavailable" if temperature is None else "below reported threshold" if threshold is not None and temperature < threshold else "threshold reached" if threshold is not None else "threshold unavailable"
        if latest.get("thermal_throttling"):
            condition += ", thermal throttling observed"
        memory = metric_text(latest.get("memory_used_bytes"), 1024**2, " MiB")
        total = metric_text(latest.get("memory_total_bytes", device.get("total_bytes")), 1024**2, " MiB")
        events = latest.get("clock_event_reasons")
        rows.extend([
            f'GPU {device["ordinal"]}: {device["name"]} [{device["uuid"]}] | {"running" if latest else "waiting for sample"}',
            f'  GPU {metric_text(latest.get("utilization_percent"), suffix="%")} | VRAM {memory}/{total} | allocated {metric_text(latest.get("allocated_bytes"), 1024**2, " MiB")}',
            f'  Temp {metric_text(temperature, suffix=" C")} | power {metric_text(latest.get("power_watts"), suffix=" W")} | graphics/memory clocks {metric_text(latest.get("graphics_clock_mhz"))}/{metric_text(latest.get("memory_clock_mhz"))} MHz',
            f'  {condition} | clock event mask {"N/A" if events is None else hex(events)}',
        ])
    return "\n".join(rows)


def format_report(config, result, state):
    """Describe achieved stress and temperatures without hardware-health claims.

    Args:
        config (dict): Requested devices, load and duration.
        result (dict): Stop reason, errors and measured elapsed seconds.
        state (dict): Online per-board summaries.

    Returns:
        str: Complete readable report.

    Raises:
        KeyError: If required run fields are absent.
    """
    rows = ["Final report", f'Status: {result["stop_reason"]}', f'Elapsed: {result["elapsed_seconds"]:.2f}s', f'Requested compute/free VRAM: {config["load_percent"]:g}%', f'Requested duration: {config["duration_seconds"] if config["duration_seconds"] is not None else "continuous"}', "Metrics are sampled, board-wide values. Duty cycle is a target, not a guarantee of exact measured GPU utilization."]
    for device in config["devices"]:
        rows.append(f'\nGPU {device["ordinal"]}: {device["name"]} [{device["uuid"]}]')
        board = state.get(device["uuid"])
        if not board:
            rows.append("No observations collected.")
            continue
        for name in METRICS:
            metric = board["metrics"].get(name)
            scale = 1024**2 if name == "memory_used_bytes" else 1
            unit = " MiB" if scale != 1 else ""
            rows.append(f'  {name}: ' + (f'average {metric["sum"] / metric["count"] / scale:.1f}{unit}, peak {metric["peak"] / scale:.1f}{unit}, samples {metric["count"]}' if metric else "N/A"))
        rows.append(f'  Worker allocation: {metric_text(board["latest"].get("allocated_bytes"), 1024**2, " MiB")}')
        first, last = board["first_temperature"], board["last_temperature"]
        if first and last and last[0] > first[0]:
            rows.append(f'  Temperature endpoint trend: {(last[1] - first[1]) / (last[0] - first[0]):+.2f} C/s. This includes warm-up effects.')
        threshold = device.get("slowdown_celsius")
        peak = board["peak_temperature"]
        status = "threshold unavailable" if threshold is None else "temperature unavailable" if peak is None else "threshold reached" if peak >= threshold else "below reported threshold"
        rows.append(f'  Temperature: {status}. Slowdown threshold: {metric_text(threshold, suffix=" C")}.')
        thermal = str(board["thermal_samples"]) if board["thermal_observations"] else "N/A"
        rows.append(f"  Samples with thermal throttling: {thermal}")
    if result["errors"]:
        rows.extend(["\nErrors:", *result["errors"]])
    rows.append("\nThis measures achieved load and observed behavior. It does not certify that a GPU is free of hardware faults.")
    return "\n".join(rows) + "\n"


class Dashboard:
    """Own terminal cursor state for the duration of a stress session."""

    def __init__(self, output, interactive=None):
        """Initialize a terminal-aware renderer.

        Args:
            output (TextIO): Output stream.
            interactive (bool | None): Override TTY detection for testing.
        Returns:
            None.
        Raises:
            None.
        """
        self.output = output
        self.interactive = output.isatty() if interactive is None else interactive
        if self.interactive and os.name == "nt":
            import ctypes
            kernel = ctypes.windll.kernel32
            mode = ctypes.c_ulong()
            handle = kernel.GetStdHandle(-11)
            self.interactive = bool(kernel.GetConsoleMode(handle, ctypes.byref(mode)) and kernel.SetConsoleMode(handle, mode.value | 4))

    def __enter__(self):
        """Hide the cursor when the terminal supports refresh.

        Args:
            None.
        Returns:
            Dashboard: Active renderer.
        Raises:
            OSError: If output fails.
        """
        if self.interactive:
            self.output.write("\x1b[?25l")
            self.output.flush()
        return self

    def draw(self, frame):
        """Replace a TTY frame, keeping redirected output concise.

        Args:
            frame (str): Current resource dashboard.
        Returns:
            None.
        Raises:
            OSError: If output fails.
        """
        if self.interactive:
            self.output.write("\x1b[2J\x1b[H" + frame)
            self.output.flush()

    def __exit__(self, error_type, error, traceback):
        """Restore the cursor regardless of session outcome.

        Args:
            error_type (type | None): Exception class.
            error (Exception | None): Exception instance.
            traceback (object | None): Exception traceback.
        Returns:
            bool: False, so exceptions propagate.
        Raises:
            OSError: If terminal output fails.
        """
        if self.interactive:
            self.output.write("\x1b[?25h\n")
            self.output.flush()
        return False


def save_results(directory, report, sample_file, overwrite=False):
    """Publish report and samples, rolling back an incomplete replacement.

    Args:
        directory (str | Path): Destination directory, including relative or ~ paths.
        report (str): Readable final report.
        sample_file (TextIO): Seekable JSONL spool.
        overwrite (bool): Explicit permission to replace existing outputs.

    Returns:
        list[str]: Absolute report and samples paths.

    Raises:
        FileExistsError: If an output exists without overwrite permission.
        OSError: If staging or publication fails.
        KeyboardInterrupt: If export is interrupted after restoring old files.

    Examples:
        save_results("~/gpu-test", report, spool)
    """
    root = Path(directory).expanduser().resolve()
    if root.exists() and not root.is_dir():
        raise NotADirectoryError(f"Destination is not a directory: {root}")
    root.mkdir(parents=True, exist_ok=True)
    targets = [root / "report.txt", root / "samples.jsonl"]
    if not overwrite and any(path.exists() or path.is_symlink() for path in targets):
        raise FileExistsError("Report or samples already exist in this directory.")
    stage = Path(tempfile.mkdtemp(prefix=".stress-gpu-export-", dir=root))
    retain_backups = False
    try:
        (stage / "report.txt").write_text(report, encoding="utf-8")
        sample_file.flush()
        sample_file.seek(0)
        with (stage / "samples.jsonl").open("w", encoding="utf-8") as output:
            shutil.copyfileobj(sample_file, output)
        backups = {}
        for target in targets:
            if target.exists() or target.is_symlink():
                if not target.is_file() or target.is_symlink():
                    raise OSError(f"Refusing to replace non-regular output: {target}")
                backup = stage / (target.name + ".backup")
                shutil.copy2(target, backup)
                backups[target] = backup
        try:
            for target in targets:
                os.replace(stage / target.name, target)
        except BaseException:
            try:
                # Restore the complete original set. A successful rename may
                # have been interrupted before any subsequent bookkeeping.
                with defer_interrupts():
                    for target in targets:
                        if target in backups:
                            os.replace(backups[target], target)
                        else:
                            target.unlink(missing_ok=True)
            except BaseException as restore_error:
                retain_backups = True
                raise OSError(f"Export rollback failed. Recovery backups retained at {stage}: {restore_error}") from restore_error
            raise
    finally:
        if not retain_backups:
            shutil.rmtree(stage)
    return [str(target) for target in targets]


def prompt_save(report, sample_file, input_fn, output):
    """Ask whether and where to save, permitting retries after errors.

    Args:
        report (str): Final report text.
        sample_file (TextIO): Raw sample spool.
        input_fn (Callable[[str], str]): Interactive input callback.
        output (TextIO): Status destination.

    Returns:
        None.

    Raises:
        OSError: If status output itself fails.
    """
    try:
        if input_fn("Save report and sampled logs? [y/N]: ").strip().lower() not in ("y", "yes"):
            return
        while True:
            path = input_fn("Destination directory, or blank to cancel: ").strip()
            if not path:
                return
            try:
                try:
                    paths = save_results(path, report, sample_file)
                except FileExistsError:
                    if input_fn("Outputs exist. Replace report.txt and samples.jsonl? [y/N]: ").strip().lower() not in ("y", "yes"):
                        continue
                    paths = save_results(path, report, sample_file, overwrite=True)
                output.write("Saved:\n" + "\n".join(paths) + "\n")
                return
            except OSError as error:
                output.write(f"Could not save: {error}\nChoose another directory, or blank to cancel.\n")
    except (KeyboardInterrupt, EOFError):
        output.write("\nLog export cancelled.\n")
