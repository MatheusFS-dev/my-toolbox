# Terminal Beside

Terminal Beside opens a separate terminal window beside VS Code at the root of a local workspace. Choose whether the terminal appears on the left or right of the current monitor.

**Author:** MatheusFS-dev

## Supported systems

- Linux with an X11 session and a default `x-terminal-emulator` that accepts `-T TITLE`
- Windows with Windows Terminal (`wt.exe`)

Remote workspaces, virtual workspaces, and Linux Wayland sessions are not supported.

## Install

The standalone installers in this directory build and install the extension through the VS Code CLI. On Linux, run `sh split_terminal.sh`. On Windows, run `powershell -ExecutionPolicy Bypass -File split_terminal.ps1`. Reload existing VS Code windows after installation.

If you previously installed `my-toolbox.split-terminal`, uninstall it before using Terminal Beside to avoid two extensions claiming the same shortcut.

## Use

Open a local workspace in VS Code, then press **Ctrl+Alt+S** or run **Terminal Beside: Open Terminal** from the Command Palette. Choose **Left** or **Right**. Cancelling the choice opens nothing.

The terminal starts at the selected workspace root and receives keyboard focus. In a workspace with several folders, choose the folder before the terminal opens.

## Smart pairing

The `terminalBeside.smart` setting is on by default. It links the terminal and VS Code as a pair. Focusing or Alt-Tabbing to either window raises both, and resizing one window resizes the other. The link ends when either window closes, is maximized or made fullscreen, or is dragged away from the other.

Smart pairing needs the Terminal Beside GNOME Shell extension, which the installer copies to `~/.local/share/gnome-shell/extensions/terminal-beside@matheusfs-dev` and enables. It supports GNOME Shell 46 on X11. The installer registers it, so after an install or update you only restart the shell (press Alt+F2, type `r`, press Enter, or log out and back in). If the extension is missing, Ctrl+Alt+S shows an error and moves no window. Set `terminalBeside.smart` to `false` to place the windows once, without the extension, as in earlier versions.

## Configure width

In VS Code Settings, search for **Terminal Beside: Terminal Width Percent**. Enter a value from 1 to 99. The default is one third of the monitor work area. The setting also works in `settings.json`:

```json
"terminalBeside.terminalWidthPercent": 40
```

This example gives the terminal 40% of the available width and VS Code the remaining 60%.

## Uninstall

Run `code --uninstall-extension MatheusFS-dev.terminal-beside`. To remove smart pairing, run `gnome-extensions disable terminal-beside@matheusfs-dev` and `gnome-extensions uninstall terminal-beside@matheusfs-dev`.
