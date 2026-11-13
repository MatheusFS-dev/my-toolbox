"""Run a target once, preserving its streams and exit status."""

import subprocess
import sys
import time

from .emailer import send_notification
from .toolbox_output import TerminalOutput


def run(script: str, args: list[str] | None = None, config_path: str = "monitor-gmail.json") -> int:
    """Run a Python script and attempt to email its outcome.

    Args:
        script: Path to the target Python script.
        args: Arguments passed directly to the script.
        config_path: Gmail JSON configuration, relative to the working directory.

    Returns:
        int: The target's shell-compatible exit status, or 1 on startup failure.
    """
    started = time.monotonic()
    try:
        result = subprocess.run([sys.executable, script, *(args or [])], check=False)
        code = result.returncode
        if code < 0:
            status = "failed (signal {})".format(-code)
            code = 128 - code
        else:
            status = "completed" if code == 0 else "failed"
    except OSError:
        code = 1
        status = "failed to start"
    elapsed = time.monotonic() - started
    try:
        send_notification(config_path, script, status, code, elapsed)
    except Exception:
        TerminalOutput(sys.stderr, "portable-monitor").error("Notification failed. Check the Gmail configuration and network access.")
    return code
