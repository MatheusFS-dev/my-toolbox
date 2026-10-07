"""Standalone interactive application entry point."""
import json
import multiprocessing
import sys
import tempfile

from .config import prompt_configuration
from .devices import discover_devices, sample_device
from .reporting import Dashboard, format_report, prompt_save, render_dashboard, summarize_sample
from .session import run_session


def load_dependencies():
    """Import the isolated runtime dependencies with actionable diagnostics.

    Args:
        None.

    Returns:
        tuple: PyTorch module and NVIDIA management bindings.

    Raises:
        RuntimeError: If packages are missing or PyTorch lacks CUDA support.
    """
    try:
        import torch
        import pynvml
    except ImportError as error:
        raise RuntimeError("Application dependencies are unavailable. Run tb install-stress-gpu to repair the environment.") from error
    if torch.version.cuda is None:
        raise RuntimeError("PyTorch has no NVIDIA CUDA support. Run tb install-stress-gpu to repair the environment.")
    return torch, pynvml


def main():
    """Run interactive configuration, stress, report and optional log export.

    Args:
        None.

    Returns:
        int: Zero for completed/manual-stop runs, one for errors, 130 for setup cancellation.

    Raises:
        None. User cancellation and runtime errors are reported to the terminal.
    """
    if len(sys.argv) > 1:
        print("stress-gpu is interactive and does not accept command-line arguments.", file=sys.stderr)
        return 1
    nvml = None
    initialized = False
    try:
        torch, nvml = load_dependencies()
        nvml.nvmlInit()
        initialized = True
        devices = discover_devices(torch, nvml)
        print("Available NVIDIA CUDA GPUs:")
        for device in devices:
            print(f'  {device["ordinal"]}: {device["name"]} [{device["uuid"]}] | free {device["free_bytes"] / 1024**2:.0f} / {device["total_bytes"] / 1024**2:.0f} MiB')
        config = prompt_configuration(devices, input)
        print("Initializing selected GPUs. The test timer starts when all workers are ready.", flush=True)
        state = {}
        with tempfile.TemporaryFile(mode="w+", encoding="utf-8") as spool:
            with Dashboard(sys.stdout) as dashboard:
                def on_sample(sample):
                    """Stream observations to disk and refresh a bounded summary.

                    Args:
                        sample (dict): UUID-tagged resource observation.
                    Returns:
                        None.
                    Raises:
                        OSError: If spooling or terminal output fails.
                    """
                    spool.write(json.dumps(sample, allow_nan=False) + "\n")
                    summarize_sample(state, sample)
                    dashboard.draw(render_dashboard(config, state, sample["elapsed_seconds"]))
                result = run_session(config, lambda uuid: sample_device(nvml, uuid), on_sample)
            report = format_report(config, result, state)
            print(report)
            prompt_save(report, spool, input, sys.stdout)
            return 1 if result["errors"] else 0
    except (KeyboardInterrupt, EOFError):
        print("\nConfiguration cancelled.")
        return 130
    except Exception as error:
        print(f"stress-gpu failed: {error}", file=sys.stderr)
        return 1
    finally:
        if initialized:
            nvml.nvmlShutdown()


if __name__ == "__main__":
    multiprocessing.freeze_support()
    sys.exit(main())
