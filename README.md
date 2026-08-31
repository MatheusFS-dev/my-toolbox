![my-toolbox header](https://capsule-render.vercel.app/api?height=190&type=blur&color=7f5af0&section=header&text=my-toolbox&fontColor=fffffe&fontSize=42)

<div align="center">

```text
 __  __ __   __      _____ ___   ___  _     ____   _____  __
|  \/  |\ \ / /     |_   _/ _ \ / _ \| |   | __ ) / _ \ \ \/ /
| |\/| | \ V /        | || | | | | | | |   |  _ \| | | | \  /
| |  | |  | |         | || |_| | |_| | |___| |_) | |_| | /  \
|_|  |_|  |_|         |_| \___/ \___/|_____|____/ \___/ /_/\_\
```

</div>

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

Bash and Zsh must both be installed so the installer can validate their profile changes before publication. Released `tb` binaries are statically compiled, so Go is not required to install, run, or update my-toolbox.

### Windows PowerShell

```powershell
irm https://matheusfs-dev.github.io/my-toolbox/install.ps1 | iex
```

or , via cutt.ly:

```powershell
irm https://cutt.ly/tbwin | iex
```

On Windows, the installer adds `%LOCALAPPDATA%\my-toolbox\bin` to the user `PATH` and activates it in the current PowerShell session, making `tb` available immediately. Running the installer again repairs the managed `PATH` entry without duplicating equivalent entries.

The installers automatically register top-level `tb` completion for Bash, Zsh, Windows PowerShell 5.1, and PowerShell 7. They add marked source blocks to `$HOME/.bashrc`, `${ZDOTDIR:-$HOME}/.zshrc`, and both PowerShell `CurrentUserAllHosts` profiles. Open a new shell session after installation to activate completion.

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
- `install-claude`: Install Claude Code for the current user on Linux or Windows. Skips installation when `claude` is already available.
- `install-antigravity`: Install Antigravity for the current user on Linux or Windows. Skips installation when `agy` is already available.

### Base Tools

- `install-uv`: Install uv for the current user on Linux or Windows without changing shell `PATH` configuration. Skips installation when `uv` is already available.
- `install-gh`: Download, verify, and install the latest GitHub CLI for the current user. Shows `PATH` guidance when needed.

### Agent Plugins

- `install-superpowers-codex`: Add the Superpowers plugin to Codex. Requires Codex plugin management, skips an existing installation, and leaves other plugins unchanged.
- `install-superpowers-claude`: Add the Superpowers plugin to Claude Code for the current user. Requires plugin management, skips an existing installation, and leaves other plugins unchanged.
- `install-superpowers-antigravity`: Add the Superpowers plugin to Antigravity from its GitHub repository. Requires plugin management, skips an existing installation, and leaves other plugins unchanged.

### Agent Workspace

- `setup-agents-codex`: Set up global Codex instructions, configuration, optional profiles, and packaged skills. Shows every conflict before asking whether to replace or back it up.
- `setup-agents-claude`: Set up global Claude Code instructions, settings, and packaged skills. Shows every conflict before asking whether to replace or back it up.
- `setup-agents-antigravity`: Set up global Antigravity instructions, settings, and packaged skills. Shows every conflict before asking whether to replace or back it up.
- `setup-agents-project` (direct only): Add instruction files for selected agents to an existing project. Can update `.gitignore` and back up conflicting managed instruction files.

### Terminal

- `setup-alacritty` (native Linux): Build an Alacritty-based terminal setup on Debian or Ubuntu. Choose shell tools, fonts, desktop integrations, and default-terminal options; the existing Alacritty configuration is replaced without a backup.
- `setup-kitty` (native Linux): Build a Kitty-based terminal setup on Debian or Ubuntu. Choose shell tools, fonts, desktop integrations, and default-terminal options; the existing Kitty configuration is backed up before replacement.
- `setup-windows` (Windows): Set up Windows Terminal, PowerShell 7, selected fonts, and terminal tools with WinGet. Backs up managed configuration when possible and reports each result.
- `setup-wsl` (WSL): Set up selected shell and terminal tools on Ubuntu 22.04 or 24.04 under WSL. Uses `sudo` for system dependencies, backs up managed configuration when possible, and continues past optional feature failures.
- `set-vscode-wsl-cwd` (Windows): Open a chosen WSL directory in VS Code and use it as the working directory of a managed terminal profile. Preserves JSONC comments, backs up changed settings, and supports `-Undo`.
- `set-default-cwd` (WSL): Make Bash and Zsh start in a chosen WSL directory when opened from home. Preserves unrelated shell configuration and backs up changed files.

### System Utilities

- `change-grub-order` (native Linux): Choose the default GRUB boot entry from an interactive list. Backs up the current GRUB settings before applying the change.
- `setup-venv` (Linux or WSL): Add or remove a `venv` shell command that activates the nearest `.venv`. Keeps unrelated Bash and Zsh configuration but does not create backups.
- `toggle-nopasswd-sudo` (Linux or WSL): Enable or disable passwordless `sudo` for one Linux or WSL user. Validates enabling changes and manages only the toolbox-owned sudoers file.

### Project Utilities

- `create-env-alias` (Linux or WSL): Create a Bash or Zsh alias that activates a chosen `.venv`. Previews changes, confirms replacements separately, and can back up conflicts.
- `bootstrap-python-from-venv` (Linux or WSL): Generate requirements, `pyproject.toml`, and `.python-version` from imports found in Python files and optional notebooks. Preserves unrelated TOML, stops on ambiguous input, and can run `uv lock`.
- `create-project-template`: Merge the packaged project template into an existing directory without deleting destination-only files. Checks every conflict before asking to overwrite and does not create backups.

Copied Bash and PowerShell tools receive direct arguments unchanged. The three Project Utilities are interactive and reject command-line arguments. Vendored Alacritty, Kitty, and WSL setup scripts target their documented Debian or Ubuntu environments; optional desktop integrations also require the corresponding upstream GNOME or Nautilus tools. The toolbox preserves those scripts and their exit behavior byte-for-byte.

## Uninstallation

Run `tb uninstall` and confirm removal. The command removes the toolbox wrapper and all installed toolbox versions. On Windows, it also removes each exact managed wrapper-directory entry from the user `PATH` while preserving unrelated entries.

Uninstallation does not remove tools, plugins, agent configurations, or generated workspaces.

## Development

Development and release builds require Go 1.25.8, as declared in `go.mod`, and Python 3. Go is a build dependency only; users of released `tb` binaries do not need it. Run the core test suites and installer test with:

```sh
go test ./...
python3 -m unittest discover -s packages/agent-workspace-template/source/tests -v
python3 -m unittest discover -s packages/others/tests -v
sh scripts/install_test.sh
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
