"""Run an endless counter to exercise Monitor's output and script arguments."""

import getopt
import math
from pathlib import Path
import sys
import time

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "assets/payload/packages"))
from toolbox_output import TerminalOutput


def main():
    """Log a counter until interrupted, accepting a name and interval.

    Returns:
        int: Zero after interruption, or two for invalid arguments.
    """
    output = TerminalOutput(sys.stdout, "monitor-test")
    name = "test"
    interval = 1.0
    try:
        options, remaining = getopt.getopt(sys.argv[1:], "", ["name=", "interval="])
        if remaining:
            raise ValueError("unexpected positional arguments")
        for option, value in options:
            if option == "--name":
                name = value
            elif option == "--interval":
                interval = float(value)
        if not math.isfinite(interval) or interval <= 0:
            raise ValueError("interval must be a positive number of seconds")
    except (getopt.GetoptError, ValueError) as error:
        output.error(f"{error}. Usage: monitor_test.py [--name NAME] [--interval SECONDS]")
        return 2

    output.info(f"Starting infinite loop: name={name!r}, interval={interval}s. Press Ctrl+C to stop.")
    started = time.monotonic()
    count = 0
    try:
        while True:
            count += 1
            elapsed = time.monotonic() - started
            output.info(f"{name}: iteration={count}, elapsed={elapsed:.1f}s")
            time.sleep(interval)
    except KeyboardInterrupt:
        output.success(f"Stopped after {count} iterations.")
        return 0


if __name__ == "__main__":
    raise SystemExit(main())
