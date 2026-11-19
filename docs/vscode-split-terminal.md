# Terminal Beside

On Linux X11 (Windows is covered in its own section below), run `tb macros`, select **VS Code split terminal**, then **Run**.
The foreground installer creates and installs `MatheusFS-dev.terminal-beside` using
`code --install-extension`. Reload existing VS Code windows after installation.
Running the installer again updates the same extension. **Download** saves a
self-contained shell installer that can be run with `sh split_terminal.sh` from
any directory, without the toolbox source tree.

Install VS Code, a default terminal registered as `x-terminal-emulator`, Node.js,
`zip`, `xdotool`, `wmctrl`, `xrandr`, and `xprop` first. On Debian/Ubuntu, the helper
packages can be installed with:

```sh
sudo apt install nodejs zip xdotool wmctrl x11-xserver-utils x11-utils
```

With VS Code focused in a local workspace, press **Ctrl+Alt+S**, or run
**Terminal Beside: Open Terminal** from the Command Palette. Choose **Left** or **Right**
in the VS Code prompt. A fresh default terminal opens at the workspace root with
the project folder name as its title. It occupies the selected third of the usable
monitor, while VS Code occupies the other two-thirds. The terminal gets keyboard
focus. Dismissing the prompt opens nothing and leaves both windows in place.
Set `terminalBeside.terminalWidthPercent` in VS Code Settings to choose
the terminal width as a percentage of the monitor work area (1–99). The default
is one third. For example, set it to `40` for a 40% terminal and 60% VS Code.
The workspace folder determines the directory, regardless of the active file or
integrated terminal directory. Multiple-folder workspaces always show a picker.
Canceling the picker does nothing. Repeated completed invocations create fresh
terminals. Overlapping invocations are ignored while a terminal is opening.

The extension supports local Linux X11 workspaces. Empty, remote, and virtual
workspaces and Wayland sessions show an explanatory notification. The default
terminal must accept `-T TITLE` and create a fresh window, as Kitty does. Startup
and window-identification failures leave VS Code's layout unchanged and produce
an error notification. The startup detection window is five seconds, with bounded
individual X11 command timeouts.

Existing terminal settings and user keybindings are preserved. The installer
warns if a default or named VS Code profile has a conflicting Ctrl+Alt+S user
binding, or if those bindings cannot be parsed. User bindings take precedence.
Use **Preferences: Open Keyboard Shortcuts** to assign another shortcut to
**Terminal Beside: Open Terminal** if needed. Other extensions can also contribute
this shortcut. The installer checks user bindings, not every extension binding.

To remove the extension:

```sh
code --uninstall-extension MatheusFS-dev.terminal-beside
```

Reload existing VS Code windows after removal. Already opened terminals remain
ordinary terminal windows and can be closed normally.

For a desktop smoke check, open a local project in VS Code, press Ctrl+Alt+S,
choose each side in turn, run `pwd` in the new terminal, and check the root
directory, one-third terminal width, panel clearance, and keyboard focus. Repeat
on another monitor, then cancel the side prompt and check that no window moves.
Try a multiple-folder workspace and cancel its folder picker.

Automated checks:

```sh
go test ./internal/features/macros ./internal/ui
node --test tooling/checks/vscode_split_terminal_test.cjs
sh -n assets/payload/packages/macros/vscode/split_terminal.sh
shellcheck assets/payload/packages/macros/vscode/split_terminal.sh
sh tooling/checks/output_contract_test.sh
python3 tooling/checks/output_contract_audit.py
```

## Windows

On Windows, run `tb macros`, select **VS Code split terminal**. The entry first asks whether you want the Linux
or the Windows version. For the system you are on, choose **Run** or **Download**.
For the other system only **Download** is offered, which saves that system's
installer (`split_terminal.sh` or `split_terminal.ps1`). The PowerShell installer runs with
`powershell -ExecutionPolicy Bypass -File split_terminal.ps1` from any directory.
It installs the same `MatheusFS-dev.terminal-beside` extension id with
`code --install-extension`, so reload existing VS Code windows afterwards.

Install VS Code with its `code` command on PATH and Windows Terminal (`wt`)
first. Windows PowerShell 5.1 ships with Windows and is the only other requirement.
Node.js is not needed.

With VS Code focused in a local workspace, press **Ctrl+Alt+S**, or run
**Terminal Beside: Open Terminal** from the Command Palette. Choose **Left** or
**Right** in the VS Code prompt. A fresh Windows Terminal window opens at the
workspace root and takes the selected third of the work area of the monitor that
holds VS Code. VS Code takes the other two thirds, and the terminal gets keyboard
focus. Dismissing the prompt opens nothing and leaves both windows in place.
The `terminalBeside.terminalWidthPercent` setting also applies on
Windows. It accepts 1–99 and defaults to one third.
Windows hides the taskbar area from the
work area, and invisible window borders are compensated so both windows sit flush.
The terminal window title is the project folder name.

Workspace selection, single-flight behavior, and the remote, virtual, and empty
workspace messages match Linux. The extension runs a small PowerShell helper
(`split.ps1` inside the extension) to identify the focused VS Code window and to
move windows. If Windows Terminal does not show a titled window within eight
seconds, VS Code is left in place and an error notification appears. The
helper uses per-monitor DPI awareness and re-applies the placement until
Windows accepts it, so monitors with different scaling or resolution tile correctly.

The binding is limited to Windows with `"when": "isWindows"`. The installer warns
about conflicting Ctrl+Alt+S user bindings in the default and named profiles
under `%APPDATA%\Code\User` and never edits them.

To remove the extension:

```powershell
code --uninstall-extension MatheusFS-dev.terminal-beside
```

Windows checks:

```powershell
go test ./internal/features/macros
node --test tooling/checks/vscode_split_terminal_windows_test.cjs
python tooling/checks/output_contract_audit.py
```
