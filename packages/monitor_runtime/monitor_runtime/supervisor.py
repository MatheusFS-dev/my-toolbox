"""Process-group supervision, logging, metrics, restart handling, and queues."""

import collections
import datetime
import json
import os
import queue
import secrets
import signal
import shutil
import subprocess
import threading
import time
from pathlib import Path
from typing import Any, Callable, Dict, List, Optional

from .reports import write_reports
from .restart import RestartPolicy
from .detection import HeartbeatClock, possible_memory_leak
from .viewer import open_log_viewer


EventSink = Callable[[Dict[str, Any]], None]


def _event(sink: EventSink, event_type: str, **values: Any) -> Dict[str, Any]:
    event = {"protocol_version": 1, "type": event_type}
    event.update(values)
    sink(event)
    return event


def _run_directory(script: str) -> Path:
    timestamp = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    root = Path(script).parent / "runs" / "monitor_logs" / Path(script).name
    directory = root / "{}-{}".format(timestamp, secrets.token_hex(3))
    (directory / "crash_logs").mkdir(mode=0o700, parents=True)
    return directory


def _reader(stream: Any, stream_name: str, messages: queue.Queue) -> None:
    while True:
        chunk = stream.readline()
        if not chunk:
            break
        messages.put((stream_name, chunk.decode("utf-8", errors="replace")))
    stream.close()


def _process_metrics(pid: int, started: float) -> Dict[str, Any]:
    sample = {
        "elapsed_seconds": round(time.monotonic() - started, 3),
        "cpu_percent": 0.0,
        "ram_bytes": 0,
        "ram_mib": 0.0,
        "system_ram_total_bytes": 0,
        "gpu_percent": None,
        "gpu_memory_mib": None,
        "gpu_memory_total_mib": None,
        "gpu_scope": None,
    }
    pids = {pid}
    try:
        import psutil
        root = psutil.Process(pid)
        processes = [root] + root.children(recursive=True)
        pids = {process.pid for process in processes}
        sample["cpu_percent"] = round(sum(process.cpu_percent(None) for process in processes if process.is_running()), 2)
        ram_bytes = sum(process.memory_info().rss for process in processes if process.is_running())
        sample["ram_bytes"] = ram_bytes
        sample["ram_mib"] = round(ram_bytes / (1024 * 1024), 2)
        sample["system_ram_total_bytes"] = int(psutil.virtual_memory().total)
    except (ImportError, OSError, RuntimeError):
        pass
    except Exception:
        pass
    sample.update(_gpu_metrics(pids))
    return sample


def _gpu_metrics(pids):
    result = {"gpu_percent": None, "gpu_memory_mib": None, "gpu_memory_total_mib": None, "gpu_scope": None}
    executable = shutil.which("nvidia-smi")
    if not executable:
        return result
    try:
        output = subprocess.check_output(
            [executable, "--query-compute-apps=pid,used_memory", "--format=csv,noheader,nounits"],
            text=True, stderr=subprocess.DEVNULL, timeout=2,
        )
        for line in output.splitlines():
            fields = [field.strip() for field in line.split(",")]
            try:
                if len(fields) == 2 and int(fields[0]) in pids:
                    result["gpu_memory_mib"] = round((result["gpu_memory_mib"] or 0) + float(fields[1]), 2)
            except ValueError:
                continue
    except (OSError, subprocess.SubprocessError):
        pass
    try:
        pmon = subprocess.check_output([executable, "pmon", "-c", "1", "-s", "u"], text=True, stderr=subprocess.DEVNULL, timeout=2)
        utilization = 0.0
        found = False
        for line in pmon.splitlines():
            fields = line.split()
            if not fields or fields[0].startswith("#") or len(fields) < 4:
                continue
            try:
                if int(fields[1]) in pids and fields[3] != "-":
                    utilization += float(fields[3])
                    found = True
            except ValueError:
                continue
        if found:
            result["gpu_percent"] = round(utilization, 2)
    except (OSError, subprocess.SubprocessError):
        pass
    if result["gpu_percent"] is not None or result["gpu_memory_mib"] is not None:
        result["gpu_scope"] = "target"
        return result
    try:
        global_output = subprocess.check_output(
            [executable, "--query-gpu=index,name,utilization.gpu,memory.used,memory.total", "--format=csv,noheader,nounits"],
            text=True, stderr=subprocess.DEVNULL, timeout=2,
        )
        utilization_values = []
        memory_used = 0.0
        memory_total = 0.0
        for line in global_output.splitlines():
            fields = [field.strip() for field in line.split(",")]
            if len(fields) != 5:
                continue
            try:
                utilization = float(fields[2])
                used = float(fields[3])
                total = float(fields[4])
            except ValueError:
                continue
            utilization_values.append(utilization)
            memory_used += used
            memory_total += total
        if utilization_values:
            result["gpu_percent"] = round(sum(utilization_values) / len(utilization_values), 2)
            result["gpu_memory_mib"] = round(memory_used, 2)
            result["gpu_memory_total_mib"] = round(memory_total, 2)
            result["gpu_scope"] = "system-wide"
    except (OSError, subprocess.SubprocessError):
        pass
    return result


def terminate_process_group(process: subprocess.Popen, timeout: float = 5.0) -> None:
    """Terminate the target process group and escalate after a bounded wait."""
    try:
        os.killpg(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        process.poll()
        return
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        process.poll()
        try:
            os.killpg(process.pid, 0)
        except ProcessLookupError:
            if process.poll() is None:
                process.wait()
            return
        time.sleep(0.02)
    try:
        os.killpg(process.pid, signal.SIGKILL)
    except ProcessLookupError:
        pass
    if process.poll() is None:
        process.wait()


def supervise_script(script: str, interpreter: str, config: Dict[str, Any], sink: EventSink, notifier=None, title=None, queue_index=1, queue_total=1) -> Dict[str, Any]:
    """Run one script until success or its crash-retry budget is exhausted.

    Args:
        script: Target Python script path.
        interpreter: Python 3 executable used for the target.
        config: Validated Monitor run configuration.
        sink: Callback that receives protocol events.
        notifier: Optional callback that delivers runtime notifications.
        title: User-selected run title, or the filename when omitted.
        queue_index: One-based position of the target in its queue.
        queue_total: Total number of targets in the queue.

    Returns:
        Final outcome, counters, elapsed time, and artifact location.
    """
    script = str(Path(script).resolve())
    title = title or Path(script).name
    run_directory = _run_directory(script)
    output_path = run_directory / "output.log"
    restart = config["restart"]
    policy = RestartPolicy(restart["crash_retries"], restart["base_delay_seconds"], restart["backoff_multiplier"], restart["max_delay_seconds"])
    lifecycle = collections.deque(maxlen=10000)
    samples = collections.deque(maxlen=100000)
    latest = collections.deque(maxlen=10)
    attempt = 0
    started = time.monotonic()
    heartbeat = HeartbeatClock(config["heartbeat_interval_minutes"], started)
    leak_reported = False
    possible_code_error = False
    possible_code_error_details = None
    _event(sink, "run_created", title=title, script=script, run_directory=str(run_directory), interpreter=os.path.realpath(interpreter), queue_index=queue_index, queue_total=queue_total)
    final_code = 1
    outcome = "failed"
    with output_path.open("w", encoding="utf-8", buffering=1) as output:
        viewer = None
        if config.get("gui_viewer"):
            viewer, warning = open_log_viewer(output_path)
            if warning:
                lifecycle.append(_event(sink, "lifecycle", state="viewer_warning", message=warning))
        while True:
            attempt += 1
            attempt_offset = output.tell()
            attempt_started = time.monotonic()
            process = subprocess.Popen(
                [interpreter, "-u", script], cwd=str(Path(script).parent),
                stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True,
            )
            item = _event(sink, "attempt_started", attempt=attempt, pid=process.pid)
            lifecycle.append(item)
            messages = queue.Queue(maxsize=1000)
            readers = [
                threading.Thread(target=_reader, args=(process.stdout, "stdout", messages), daemon=True),
                threading.Thread(target=_reader, args=(process.stderr, "stderr", messages), daemon=True),
            ]
            for reader in readers:
                reader.start()
            sample_at = 0.0
            scheduled_at = None
            scheduled_minutes = restart.get("scheduled_interval_minutes", 0)
            if scheduled_minutes and scheduled_minutes > 0:
                scheduled_at = time.monotonic() + scheduled_minutes * 60
            scheduled_restart = False
            memory_restart = False
            memory_restart_sample = None
            try:
                while process.poll() is None:
                    now = time.monotonic()
                    if now >= sample_at:
                        sample = _process_metrics(process.pid, started)
                        samples.append(sample)
                        _event(sink, "resource_sample", **sample)
                        sample_at = now + config["sampling_interval_seconds"]
                        if notifier and config["notifications"].get("heartbeat") and heartbeat.due(now):
                            if notifier("heartbeat", title, list(latest), sample):
                                heartbeat.delivered(now)
                        if notifier and config["notifications"].get("possible_leak") and not leak_reported and possible_memory_leak(samples, config["leak_detection"]):
                            notifier("possible_leak", title, list(latest), sample)
                            leak_reported = True
                        memory_limit = float(restart.get("memory_limit_gb", 0)) * 1000000000
                        if restart.get("memory_aware") and sample["ram_bytes"] >= memory_limit:
                            memory_restart = True
                            memory_restart_sample = sample
                            terminate_process_group(process)
                            break
                    if scheduled_at is not None and now >= scheduled_at:
                        scheduled_restart = True
                        terminate_process_group(process)
                        break
                    try:
                        stream_name, text = messages.get(timeout=0.05)
                    except queue.Empty:
                        continue
                    _record_output(output, sink, latest, attempt, stream_name, text)
            except KeyboardInterrupt:
                terminate_process_group(process)
                if viewer is not None and viewer.poll() is None:
                    viewer.terminate()
                _drain_output(messages, output, sink, latest, attempt)
                summary = {"outcome": "cancelled", "exit_code": 130, "attempts": attempt, "crash_restarts": policy.crash_count, "scheduled_restarts": policy.scheduled_count, "memory_restarts": policy.memory_count}
                if config.get("reports_enabled", True):
                    write_reports(run_directory, samples, lifecycle, summary)
                _event(
                    sink, "final_outcome", **summary,
                    title=title, script=script, run_directory=str(run_directory), queue_index=queue_index, queue_total=queue_total,
                    crash_count=policy.crash_count, scheduled_count=policy.scheduled_count, memory_count=policy.memory_count,
                )
                summary["run_directory"] = str(run_directory)
                return summary
            process.wait()
            for reader in readers:
                reader.join(timeout=1)
            _drain_output(messages, output, sink, latest, attempt)
            final_code = process.returncode
            attempt_duration = round(time.monotonic() - attempt_started, 3)
            item = _event(
                sink, "attempt_ended", attempt=attempt, exit_code=final_code,
                scheduled=scheduled_restart, attempt_duration_seconds=attempt_duration,
            )
            lifecycle.append(item)
            if scheduled_restart:
                policy.record_scheduled_restart()
                lifecycle.append(_event(sink, "restart_decision", reason="scheduled", restart=True, delay_seconds=0, crash_count=policy.crash_count, scheduled_count=policy.scheduled_count, memory_count=policy.memory_count))
                if notifier and config["notifications"].get("scheduled_restart"):
                    notifier("scheduled_restart", title, list(latest), samples[-1] if samples else {})
                continue
            if memory_restart:
                policy.record_memory_restart()
                lifecycle.append(_event(
                    sink, "restart_decision", reason="memory_limit", restart=True,
                    delay_seconds=0, ram_bytes=memory_restart_sample["ram_bytes"],
                    ram_mib=memory_restart_sample["ram_mib"], memory_limit_gb=restart["memory_limit_gb"],
                    crash_count=policy.crash_count, scheduled_count=policy.scheduled_count,
                    memory_count=policy.memory_count,
                ))
                continue
            if final_code == 0:
                outcome = "success"
                if notifier and policy.crash_count and config["notifications"].get("recovery"):
                    notifier("recovery", title, list(latest), samples[-1] if samples else {})
                break
            output.flush()
            crash_copy = run_directory / "crash_logs" / "attempt-{:03d}.log".format(attempt)
            with output_path.open("r", encoding="utf-8") as complete_output:
                complete_output.seek(attempt_offset)
                crash_copy.write_text(complete_output.read(), encoding="utf-8")
            rapid_crash = attempt_duration < float(restart["rapid_crash_seconds"])
            if rapid_crash:
                details = dict(samples[-1] if samples else {})
                details.update({
                    "attempt": attempt,
                    "attempt_duration_seconds": attempt_duration,
                    "exit_code": final_code,
                    "remaining_retries": max(0, policy.max_crash_retries - policy.crash_count),
                    "script": script,
                })
                possible_code_error_details = details
                if not possible_code_error:
                    possible_code_error = True
                    lifecycle.append(_event(
                        sink, "lifecycle", state="possible_code_error_warning",
                        message="Target exited with code {} after {:.3f}s. Monitor will email only if all crash retries fail.".format(final_code, attempt_duration),
                        attempt=attempt, exit_code=final_code, attempt_duration_seconds=attempt_duration,
                    ))
            should_restart, delay = policy.record_crash()
            lifecycle.append(_event(sink, "restart_decision", reason="crash", restart=should_restart, delay_seconds=delay, crash_count=policy.crash_count, scheduled_count=policy.scheduled_count, memory_count=policy.memory_count))
            if not should_restart:
                break
            if delay:
                time.sleep(delay)
    if viewer is not None and viewer.poll() is None:
        viewer.terminate()
    summary = {"outcome": outcome, "exit_code": final_code, "attempts": attempt, "crash_restarts": policy.crash_count, "scheduled_restarts": policy.scheduled_count, "memory_restarts": policy.memory_count, "elapsed_seconds": round(time.monotonic() - started, 3), "possible_code_error": possible_code_error}
    if config.get("reports_enabled", True):
        write_reports(run_directory, list(samples), list(lifecycle), summary)
    if notifier:
        graph_paths = []
        if config.get("reports_enabled", True):
            graph_paths = [str(path) for path in (run_directory / "cpu.png", run_directory / "ram.png", run_directory / "gpu.png") if path.exists()]
        if outcome == "success" and config["notifications"].get("completion"):
            notifier("completion", title, list(latest), samples[-1] if samples else summary, graph_paths)
        elif outcome != "success" and possible_code_error and config["notifications"].get("possible_code_error"):
            possible_code_error_details["remaining_retries"] = 0
            possible_code_error_details["attempts"] = attempt
            notifier("possible_code_error", title, list(latest), possible_code_error_details, graph_paths)
        elif outcome != "success" and config["notifications"].get("final_failure"):
            notifier("final_failure", title, list(latest), samples[-1] if samples else summary, graph_paths)
    _event(
        sink, "final_outcome", **summary,
        title=title, script=script, run_directory=str(run_directory), queue_index=queue_index, queue_total=queue_total,
        crash_count=policy.crash_count, scheduled_count=policy.scheduled_count, memory_count=policy.memory_count,
    )
    summary["run_directory"] = str(run_directory)
    return summary


def _record_output(output: Any, sink: EventSink, latest: collections.deque, attempt: int, stream_name: str, text: str) -> None:
    for line in text.splitlines(True):
        rendered = line.rstrip("\r\n")
        output.write("[attempt {} {}] {}\n".format(attempt, stream_name, rendered))
        display = rendered
        if len(display) > 4096:
            display = "…" + display[-4095:]
        latest.append(display)
        _event(sink, "target_output", attempt=attempt, stream=stream_name, text=display)


def _drain_output(messages: queue.Queue, output: Any, sink: EventSink, latest: collections.deque, attempt: int) -> None:
    while True:
        try:
            stream_name, text = messages.get_nowait()
        except queue.Empty:
            return
        _record_output(output, sink, latest, attempt, stream_name, text)


def run_queue(scripts: List[str], interpreter: str, config: Dict[str, Any], sink: EventSink, notifier=None, titles=None) -> int:
    """Run scripts sequentially, stopping at the first terminal failure.

    Args:
        scripts: Ordered target script paths.
        interpreter: Python 3 executable used for every target.
        config: Validated Monitor run configuration.
        sink: Callback that receives protocol events.
        notifier: Optional callback that delivers runtime notifications.
        titles: Optional user-selected title for each target.

    Returns:
        Zero on success, 130 on cancellation, or one on terminal failure.
    """
    titles = titles or [Path(script).name for script in scripts]
    for index, (script, title) in enumerate(zip(scripts, titles)):
        _event(sink, "lifecycle", state="queue_start", queue_index=index + 1, queue_total=len(scripts), title=title, script=script)
        result = supervise_script(script, interpreter, config, sink, notifier, title, index + 1, len(scripts))
        if result["outcome"] == "cancelled":
            return 130
        if result["outcome"] != "success":
            return 1
    return 0
