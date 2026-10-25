"""Standalone interactive application entry point."""
import json
import multiprocessing
import sys
import tempfile

from .config import prompt_configuration
from .devices import discover_devices, sample_device
from .reporting import Dashboard, format_report, prompt_save, render_dashboard, summarize_sample
from .session import run_session
from .output import ContractWriter, TerminalOutput


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
    output = TerminalOutput(sys.stdout)
    error_output = TerminalOutput(sys.stderr)
    if len(sys.argv) > 1:
        error_output.error("stress-gpu is interactive and does not accept command-line arguments; run stress-gpu without arguments.")
        return 1
    nvml = None
    initialized = False
    try:
        output.info("Checking CUDA and NVIDIA runtime availability...")
        torch, nvml = load_dependencies()
        nvml.nvmlInit()
        initialized = True
        devices = discover_devices(torch, nvml)
        output.menu("Available NVIDIA CUDA GPUs:", [
            f'{device["ordinal"]}: {device["name"]} [{device["uuid"]}] | free {device["free_bytes"] / 1024**2:.0f} / {device["total_bytes"] / 1024**2:.0f} MiB'
            for device in devices
        ])
        ask = lambda message: output.ask(message, input)
        config = prompt_configuration(devices, ask, output.warn)
        output.info("Initializing selected GPUs. The test timer starts when all workers are ready...")
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
            output.info(report)
            prompt_save(report, spool, ask, ContractWriter(output))
            if result["errors"]:
                error_output.error("The stress test finished with worker errors; review the final report and retry after correcting the reported cause.")
                return 1
            output.success("Stress test completed successfully.")
            return 0
    except (KeyboardInterrupt, EOFError):
        output.info("Configuration cancelled; no stress test was started.")
        return 130
    except Exception as error:
        error_output.error(f"stress-gpu failed: {error}. Review the error, correct the environment or configuration, and retry.")
        return 1
    finally:
        if initialized:
            nvml.nvmlShutdown()


if __name__ == "__main__":
    multiprocessing.freeze_support()
    sys.exit(main())
