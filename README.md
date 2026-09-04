![my-toolbox header](https://capsule-render.vercel.app/api?height=190&type=blur&color=7f5af0&section=header&text=my%20toolbox&fontColor=fffffe&fontSize=42)

<pre align="center">
███╗   ███╗██╗   ██╗    ████████╗ ██████╗  ██████╗ ██╗     ██████╗  ██████╗ ██╗  ██╗
████╗ ████║╚██╗ ██╔╝    ╚══██╔══╝██╔═══██╗██╔═══██╗██║     ██╔══██╗██╔═══██╗╚██╗██╔╝
██╔████╔██║ ╚████╔╝        ██║   ██║   ██║██║   ██║██║     ██████╔╝██║   ██║ ╚███╔╝
██║╚██╔╝██║  ╚██╔╝         ██║   ██║   ██║██║   ██║██║     ██╔══██╗██║   ██║ ██╔██╗
██║ ╚═╝ ██║   ██║          ██║   ╚██████╔╝╚██████╔╝███████╗██████╔╝╚██████╔╝██╔╝ ██╗
╚═╝     ╚═╝   ╚═╝          ╚═╝    ╚═════╝  ╚═════╝ ╚══════╝╚═════╝  ╚═════╝ ╚═╝  ╚═╝
</pre>

<p align="center">
  <a href="https://github.com/DenverCoder1/readme-typing-svg"><img src="https://readme-typing-svg.herokuapp.com?font=Fira+Code&color=%237F5AF0&size=22&center=true&vCenter=true&width=760&height=32&lines=Portable+terminal+tools+for+Linux+and+Windows" alt="Portable terminal tools for Linux and Windows" /></a>
</p>

<p align="center">
  <a href="https://github.com/MatheusFS-dev/my-toolbox/blob/main/LICENSE"><img src="https://img.shields.io/github/license/MatheusFS-dev/my-toolbox?style=flat-square" alt="License" /></a>
  <a href="https://github.com/MatheusFS-dev/my-toolbox/stargazers"><img src="https://img.shields.io/github/stars/MatheusFS-dev/my-toolbox?style=flat-square" alt="Stars" /></a>
  <a href="https://github.com/MatheusFS-dev/my-toolbox/network/members"><img src="https://img.shields.io/github/forks/MatheusFS-dev/my-toolbox?style=flat-square" alt="Forks" /></a>
  <a href="https://visitor-badge.laobi.icu/badge?page_id=MatheusFS-dev.my-toolbox"><img src="https://visitor-badge.laobi.icu/badge?page_id=MatheusFS-dev.my-toolbox" alt="Visitors" /></a>
</p>

`tb` is a portable terminal toolbox for Linux x64, Linux ARM64, and Windows x64. It brings supported command-line agents, base tools, Superpowers plugins, terminal utilities, and reusable agent workspace templates into one platform-aware catalog. Installing the toolbox itself does not require `sudo`.

## Table of Contents

- [Overview](#overview)
- [Installation](#installation)
- [Usage](#usage)
- [Tool Catalog](#tool-catalog)
- [Uninstallation](#uninstallation)
- [Development](#development)
- [Contributing](#contributing)
- [License](#license)
- [Collaborators](#collaborators)

## Overview

| Capability | Details |
| --- | --- |
| Supported systems | Linux x64, Linux ARM64, and Windows x64 |
| Interactive workflow | Categorized, multi-select terminal interface through `tb list` |
| Platform awareness | Native Linux, WSL, and Windows filtering before commands are shown or run |
| Catalog source | Tool names, categories, descriptions, and platform rules in `commands.json` |
| Maintenance | Built-in update, version, help, and uninstall commands |

The toolbox gathers all required answers before it runs selected tools, executes them in catalog order, and stops at the first failure. Unsupported tools return an explicit platform error, while direct-only commands remain available through `tb help`.

## Installation

### Linux

```sh
curl -fsSL https://matheusfs-dev.github.io/my-toolbox/install.sh | sh
```

or, via cutt.ly:

```sh
curl -fsSL https://cutt.ly/tblinux | sh
```

Bash, Zsh, and Python are not universal installation requirements. Bash and Zsh are independent, optional completion integrations: completion assets are always installed, while profile changes are made only for detected shell executables. Released `tb` binaries are statically compiled, so Go is not required to install, run, or update my-toolbox.

### Windows PowerShell

```powershell
irm https://matheusfs-dev.github.io/my-toolbox/install.ps1 | iex
```

or , via cutt.ly:

```powershell
irm https://cutt.ly/tbwin | iex
```

On Windows, the installer adds `%LOCALAPPDATA%\my-toolbox\bin` to the user `PATH` and activates it in the current PowerShell session, making `tb` available immediately. Running the installer again repairs the managed `PATH` entry without duplicating equivalent entries.

The installers publish top-level `tb` completion assets for Bash, Zsh, Windows PowerShell 5.1, and PowerShell 7. On Linux, marked source blocks are added only for detected Bash or Zsh executables; a missing shell is left untouched. On Windows, both PowerShell `CurrentUserAllHosts` profiles are updated. Open a new shell session after installation to activate completion.

Bootstrap installation does not replace an existing toolbox. When a newer release is available, `tb update` downloads the bootstrap installer, removes the managed toolbox installation, and runs the installer again. If installation fails after removal, rerun the installation command for your platform to restore `tb`.

## Usage

```text
tb list
tb <tool> [arguments...]
tb update
tb uninstall
tb version
tb help
```

Run `tb list` to open the categorized selector. Use Up and Down to move, Space to toggle the focused tool, Enter to continue, and Escape or Ctrl+C to cancel.

```text
SELECT TOOLS

  Agents
› ◯ install-codex
    Install Codex for the current user on Linux or Windows. Skips
    installation when `codex` is already available.
    Requires: Bash
  ◉ install-claude
    Install Claude Code for the current user on Linux or Windows. Skips
    installation when `claude` is already available.

↑/↓ move • space select • enter run • esc cancel
1 tool selected
```

The example is shortened to show the row layout. The live selector wraps to the current terminal width, capped at 72 columns. Its title and controls remain visible while tool rows scroll; selected markers and names are green, while descriptions remain gray.

`tb list` excludes direct-only commands, while `tb help` includes them. Running `tb` without arguments is invalid and directs you to `tb list`. Help output uses ANSI styling only when standard output is a terminal; redirected output remains plain text with the same hierarchy.

Tab completion suggests environment-supported built-in, listed, and direct-only command names for the first argument after `tb`. The toolbox does not add flag, path, or later-argument suggestions for commands delegated to selected tools.

## Tool Catalog

The command catalog is defined in `commands.json`. The descriptions below summarize each tool and its relevant safeguards.

### Agents

- `install-codex`: Install Codex for the current user on Linux or Windows. Skips installation when `codex` is already available.
  Requires: Bash (Linux/WSL); Windows PowerShell 5.1 or PowerShell 7 (Windows).
- `install-claude`: Install Claude Code for the current user on Linux or Windows. Skips installation when `claude` is already available.
  Requires: Bash (Linux/WSL); Windows PowerShell 5.1 or PowerShell 7 (Windows).
- `install-antigravity`: Install Antigravity for the current user on Linux or Windows. Skips installation when `agy` is already available.
  Requires: Bash (Linux/WSL); Windows PowerShell 5.1 or PowerShell 7 (Windows).

### Base Tools

- `install-uv`: Install uv for the current user on Linux or Windows without changing shell `PATH` configuration. Skips installation when `uv` is already available.
  Requires: Bash (Linux/WSL); Windows PowerShell 5.1 or PowerShell 7 (Windows).
- `install-gh`: Download, verify, and install the latest GitHub CLI for the current user. Shows `PATH` guidance when needed.

### Agent Plugins

- `install-superpowers-codex`: Add the Superpowers plugin to Codex. Requires Codex plugin management, skips an existing installation, and leaves other plugins unchanged.
  Requires: Codex with plugin management.
- `install-superpowers-claude`: Add the Superpowers plugin to Claude Code for the current user. Requires plugin management, skips an existing installation, and leaves other plugins unchanged.
  Requires: Claude Code with plugin management.
- `install-superpowers-antigravity`: Add the Superpowers plugin to Antigravity from its GitHub repository. Requires plugin management, skips an existing installation, and leaves other plugins unchanged.
  Requires: Antigravity with plugin management.

### Agent Workspace

- `setup-agents-codex`: Set up global Codex instructions, configuration, optional profiles, and packaged skills. Shows every conflict before asking whether to replace or back it up.
  Requires: Python 3.9+, or Python 2.7 with `toml==0.10.2` (Linux/WSL); Python 3.9+ (Windows).
- `setup-agents-claude`: Set up global Claude Code instructions, settings, and packaged skills. Shows every conflict before asking whether to replace or back it up.
  Requires: Python 3.9+, or Python 2.7 with `toml==0.10.2` (Linux/WSL); Python 3.9+ (Windows).
- `setup-agents-antigravity`: Set up global Antigravity instructions, settings, and packaged skills. Shows every conflict before asking whether to replace or back it up.
  Requires: Python 3.9+, or Python 2.7 with `toml==0.10.2` (Linux/WSL); Python 3.9+ (Windows).
- `setup-agents-project` (direct only): Add instruction files for selected agents to an existing project. Can update `.gitignore` and back up conflicting managed instruction files.
  Requires: Python 3.9+, or Python 2.7 with `toml==0.10.2` (Linux/WSL); Python 3.9+ (Windows).

### Terminal

- `setup-alacritty` (native Linux): Build an Alacritty-based terminal setup on Debian or Ubuntu. Choose shell tools, fonts, desktop integrations, and default-terminal options; the existing Alacritty configuration is replaced without a backup.
  Requires: Bash; sudo; Debian or Ubuntu; apt-get; `chown`; `cut`; `dirname`; `getent`.
- `setup-kitty` (native Linux): Build a Kitty-based terminal setup on Debian or Ubuntu. Choose shell tools, fonts, desktop integrations, and default-terminal options; the existing Kitty configuration is backed up before replacement.
  Requires: Bash; sudo; Debian or Ubuntu; apt-get; `chown`; `cut`; `dirname`; `getent`; `install`.
- `setup-windows` (Windows): Set up Windows Terminal, PowerShell 7, selected fonts, and terminal tools with WinGet. Backs up managed configuration when possible and reports each result.
  Requires: Windows PowerShell 5.1 or PowerShell 7; Windows 10 build 17763+ or Windows 11; WinGet.
- `set-terminal-hotkey` (Windows): Make `Ctrl+Alt+T` open the Windows default terminal application for the current user. The Start Menu shortcut persists across sign-ins and reboots; run with `-Undo` to remove it.
  Requires: Windows PowerShell 5.1 or PowerShell 7.
- `setup-wsl` (WSL): Set up selected shell and terminal tools on Ubuntu 22.04 or 24.04 under WSL. Uses `sudo` for system dependencies, backs up managed configuration when possible, and continues past optional feature failures.
  Requires: Bash; sudo; WSL Ubuntu 22.04 or 24.04; apt-get; `cut`; `dirname`; `env`; `getent`; `grep`; `sort`.
- `set-vscode-wsl-cwd` (Windows): Open a chosen WSL directory in VS Code and use it as the working directory of a managed terminal profile. Preserves JSONC comments, backs up changed settings, and supports `-Undo`.
  Requires: Windows PowerShell 5.1 or PowerShell 7; WSL; VS Code with WSL support.
- `set-default-cwd` (WSL): Make Bash and Zsh start in a chosen WSL directory when opened from home. Preserves unrelated shell configuration and backs up changed files.
  Requires: Bash; `awk`; `chmod`; `cmp`; `cp`; `date`; `grep`; `mktemp`; `mv`; `od`; `rm`; `tail`; `tr`.

### System Utilities

- `install-monitor` (Linux or WSL): Install or repair Monitor for the current Linux or WSL user with an isolated supervisor runtime.
  Requires: Python 3.9+.

  Run `monitor <script.py> [more.py ...]` after installation. Monitor keeps private state in `~/.monitor`, executes targets with the selected Python 3 interpreter, and writes run artifacts below each script's `runs/monitor_logs/` directory. Use `monitor config`, `monitor --help`, and `monitor --version` for configuration and command details.

- `change-grub-order` (native Linux): Choose the default GRUB boot entry from an interactive list. Backs up the current GRUB settings before applying the change.
  Requires: Bash; sudo; Python 3; GRUB configuration files; GRUB utilities; `awk`; `cat`; `cp`; `date`; `grep`.
- `setup-venv` (Linux or WSL): Add or remove a `venv` shell command that activates the nearest `.venv`. Keeps unrelated Bash and Zsh configuration but does not create backups.
  Requires: Bash; `awk`; `cat`; `dirname`; `grep`; `mktemp`; `rm`.
- `toggle-nopasswd-sudo` (Linux or WSL): Enable or disable passwordless `sudo` for one Linux or WSL user. Validates enabling changes and manages only the toolbox-owned sudoers file.
  Requires: Bash; sudo; visudo; `cat`; `chmod`; `grep`; `id`; `install`; `mktemp`; `rm`.

### Project Utilities

- `create-env-alias` (Linux or WSL): Create a Bash or Zsh alias that activates a chosen `.venv`. Previews changes, confirms replacements separately, and can back up conflicts.
  Requires: Python 3.9+.
- `bootstrap-python-from-venv` (Linux or WSL): Generate requirements, `pyproject.toml`, and `.python-version` from imports found in Python files and optional notebooks. Preserves unrelated TOML, stops on ambiguous input, and can run `uv lock`.
  Requires: Python 3.9+.
- `create-project-template`: Merge the packaged project template into an existing directory without deleting destination-only files. Checks every conflict before asking to overwrite and does not create backups.
  Requires: Python 3.9+.

On Python 3.9 and 3.10, project TOML parsing uses bundled Tomli 2.2.1 and requires no package installation.

Copied Bash and PowerShell tools receive direct arguments unchanged. The three Project Utilities are interactive and reject command-line arguments. Vendored Alacritty, Kitty, and WSL setup scripts target their documented Debian or Ubuntu environments. Alacritty and Kitty setup skip their optional file-manager integration when Nautilus is unavailable, and optional-step failures do not stop the remaining setup.

## Uninstallation

Run `tb uninstall` and confirm removal. The command removes the toolbox wrapper and all installed toolbox versions. On Windows, it also removes each exact managed wrapper-directory entry from the user `PATH` while preserving unrelated entries.

Uninstallation does not remove tools, plugins, agent configurations, or generated workspaces.

## Development

Development and release builds require Go 1.25.8, as declared in `go.mod`, and Python 3.9 or newer. Both are development-only dependencies; released users need neither globally. Run the core test suites and installer test with:

```sh
go test ./...
python3 -m unittest discover -s packages/agent-workspace-template/source/tests -v
python3 -m unittest discover -s packages/others/tests -v
sh scripts/install_test.sh
bash scripts/terminal-setup_test.sh
```

Create release archives by passing a canonical three-part version and an output directory:

```sh
scripts/build-release.sh 0.1.123 dist
```

The build produces Linux x64, Linux ARM64, and Windows x64 archives, corresponding SHA-256 files, and `version.txt`. Release, Pages, and submodule automation is defined under `.github/workflows`; repository secrets and settings are configured separately.

## Contributing

> [!IMPORTANT]
> Read and follow the [Code of Conduct](CODE_OF_CONDUCT.md), then run the development checks relevant to your platform before opening a pull request. CI validates Go tests and builds, Python tests and syntax, installer behavior, shell syntax, and ShellCheck where applicable.

Contributions are welcome:

1. Fork the repository.
2. Create a focused branch from `main`.
3. Commit a scoped change that preserves the existing architecture and naming.
4. Push the branch to your fork.
5. Open a pull request describing the change and its verification.

## License

This project is licensed under the [Apache License 2.0](LICENSE).

## Collaborators

Thanks to the people who have contributed to my-toolbox:

<table>
  <tr>
    <td align="center">
      <a href="https://github.com/MatheusFS-dev" title="Matheus Ferreira Silva">
        <img src="https://avatars.githubusercontent.com/u/99222557?v=4" width="100px" alt="Matheus Ferreira Silva's GitHub avatar" /><br />
        <sub><b>Matheus Ferreira Silva</b></sub>
      </a>
    </td>
  </tr>
</table>
