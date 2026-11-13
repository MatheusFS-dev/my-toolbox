"""Example scheduler entry point. Edit the target and arguments before use."""

import sys

from portable_monitor import run


if __name__ == "__main__":
    sys.exit(run("target.py", args=[]))
