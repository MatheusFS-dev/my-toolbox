# VS Code split terminal

On Linux X11, run `tb macros`, select **VS Code split terminal**, then **Run**.
The foreground installer creates and installs `my-toolbox.split-terminal` using
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
**Toolbox: Open Split Terminal** from the Command Palette. A fresh default terminal
opens at the workspace root with the project folder name as its title, VS Code occupies the left two-thirds of the usable
monitor, and the terminal occupies the right third with keyboard focus.
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
**Toolbox: Open Split Terminal** if needed. Other extensions can also contribute
this shortcut. The installer checks user bindings, not every extension binding.

To remove the extension:

```sh
code --uninstall-extension my-toolbox.split-terminal
```

Reload existing VS Code windows after removal. Already opened terminals remain
ordinary terminal windows and can be closed normally.

For a desktop smoke check, open a local project in VS Code, press Ctrl+Alt+S,
run `pwd` in the new terminal, and check the root directory, two-thirds split,
panel clearance, and keyboard focus. Repeat on another monitor, then try a
multiple-folder workspace and cancel its picker.

Automated checks:

```sh
go test ./internal/features/macros ./internal/ui
node --test tooling/checks/vscode_split_terminal_test.cjs
sh -n assets/payload/packages/macros/vscode/split_terminal.sh
shellcheck assets/payload/packages/macros/vscode/split_terminal.sh
sh tooling/checks/output_contract_test.sh
python3 tooling/checks/output_contract_audit.py
```
