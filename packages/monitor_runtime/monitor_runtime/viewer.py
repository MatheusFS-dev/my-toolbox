"""Optional allowlisted terminal viewer for the target output log."""

import os
import shutil
import subprocess


def open_log_viewer(output_path):
    """Open an allowlisted terminal running tail, or return a warning."""
    if not (os.environ.get("DISPLAY") or os.environ.get("WAYLAND_DISPLAY")):
        return None, "GUI log viewer unavailable because no display is configured"
    terminals = [
        ("x-terminal-emulator", ["-e"]),
        ("gnome-terminal", ["--"]),
        ("konsole", ["-e"]),
        ("kitty", []),
        ("alacritty", ["-e"]),
        ("xterm", ["-e"]),
    ]
    for name, prefix in terminals:
        executable = shutil.which(name)
        if executable:
            try:
                return subprocess.Popen([executable] + prefix + ["tail", "-F", "--", str(output_path)], start_new_session=True), ""
            except OSError:
                continue
    return None, "GUI log viewer unavailable because no supported terminal emulator was found"
