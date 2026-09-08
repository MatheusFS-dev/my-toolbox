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
- [Macros](#macros)
- [Tool Catalog](#tool-catalog)
- [Monitor](#monitor)
- [Uninstallation](#uninstallation)
- [Development](#development)
- [Contributing](#contributing)
- [License](#license)
- [Collaborators](#collaborators)
- [References](#references)

## Overview

| Capability | Details |
| --- | --- |
| Supported systems | Linux x64, Linux ARM64, and Windows x64 |
| Interactive workflow | Categorized, multi-select terminal interface through `tb list` |
| Guide library | Searchable bundled Markdown articles through `tb search` |
| Macro library | Searchable AutoHotkey (Windows) and ydotool (Linux) scripts through `tb macros` |
| Platform awareness | Native Linux, WSL, and Windows filtering before commands are shown or run |
| Catalog source | Tool names, categories, descriptions, and platform rules in `commands.json` |
| Maintenance | Built-in macro browser, search, update, version, help, and uninstall commands |

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

Every release includes `tb-markdown-reader` for its platform. Rust and a separate upstream reader installation are not required to install, run, or update the toolbox. Both installers validate the bundled reader with its self-check before activating the release.

Bootstrap installation does not replace an existing toolbox. When a newer release is available, `tb update` stages it alongside the active version, verifies the archive and bundled reader, then switches `current.txt` to activate it. The previous version is retained. If staging, validation, or activation fails, the active version, wrapper, and current-version pointer are preserved.

## Usage

```text
tb list
tb macros
tb search
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

Run `tb search` to browse the bundled Markdown guides. Type to filter by title, headings, or body text; use Up and Down to choose a result and Enter to open it. Search pauses while the bundled reader occupies the terminal. Escape or `q` closes the reader, including any open reader panel, and resumes the same query, selection, and results position.

The reader renders headings, emphasis, lists, task lists, tables, links, and YAML/TOML frontmatter in GitHub Dark colors. Code cards use syntax highlighting and wrap long lines to the terminal width. Mermaid diagrams render as terminal text, math uses Unicode approximations, and ordinary Markdown images display their alternative text.

| Reader control | Action |
| --- | --- |
| Mouse wheel, Up/Down, or `k`/`j` | Navigate through the document |
| Page Up/Page Down | Move one document page |
| `u`/`d` | Move half a document page |
| `gg`/`G` | Jump to the document beginning/end |
| Home/End | Move to the current line's beginning/end |
| `f` / `o` | Open or dismiss the internal-link picker / heading outline |
| Enter | Open or close the selected table or Mermaid panel; follow a picker selection |
| Click `[ Copy ]` / press `c` | Copy that code card / the first code card intersecting the viewport |
| Escape or `q` | Return to search |

Copying includes the whole code block, preserving its whitespace and trailing newline when present, without Markdown fences, color escapes, or display wrapping. The reader tries the native clipboard first, then emits OSC 52 for terminals that support clipboard writes. Mermaid diagrams and frontmatter are not code-copy cards.

If the bundled reader cannot be located, prepared, or started, or exits with an error, `tb search` closes its interface, writes a warning to stderr, and prints the selected article's raw Markdown to stdout. It appends a newline only when the source lacks one and exits successfully if both outputs can be written.

`tb list` excludes direct-only commands, while `tb help` includes them. Running `tb` without arguments is invalid and directs you to `tb list`. Help output uses ANSI styling only when standard output is a terminal; redirected output remains plain text with the same hierarchy.

Tab completion suggests environment-supported built-in, listed, and direct-only command names for the first argument after `tb`. The toolbox does not add flag, path, or later-argument suggestions for commands delegated to selected tools.

## Macros

Run `tb macros` to browse the bundled macro library. Type to search each macro's title, description, relative path, and script content; use Up and Down to move, Enter to open the action menu, and Escape or Ctrl+C to cancel. Supported OSs appear in red below each description. Both subpackages remain visible, but **Run** is disabled on unsupported OSs; **Download** stays available.

Both subpackages include **Press key after X ms**, with defaults of `Enter` and `12000000` milliseconds. **Run** asks for the values and schedules the macro in the background, prints its PID, and returns immediately.

- **autohotkey — Windows only:** requires AutoHotkey v2. If no working runtime is found, the toolbox offers the official Windows installer and validates the result. Run `tb macros` again after installation.
- **ydotool — Linux:** uses Linux input events for Wayland or X11. If ydotool is missing or incompatible, the toolbox offers to build checksum-verified v1.0.4 into `~/.local/bin`, using CMake, Make, and a C compiler, then continues running the selected macro. This also supports Linux ARM64. Older 0.1.x packages are rejected.

`tb macros` automatically reuses a working `ydotoold` or starts it in the background. When `/dev/uinput` requires elevated access, `sudo` may ask for your password. The toolbox creates a socket owned by your user with mode `0600`, waits for startup, and passes the socket configuration to the macro automatically. An existing `YDOTOOL_SOCKET` setting is respected; otherwise the toolbox uses `~/.ydotool_socket` when starting its daemon. Later runs reuse the daemon, and it starts again on demand after a reboot.

The toolbox validates macro arguments before daemon startup and prints log paths for daemon startup and background macro errors. If authorization or startup fails, the macro is not scheduled.

The ydotool timer accepts `Enter`, `Space`, `Tab`, `Escape`, arrow/navigation keys, or a numeric Linux keycode (1–767). Numeric codes refer to physical keys, not layout-independent characters. A downloaded timer can be run as `sh press_key_after_x_ms.sh Enter 1000` with ydotool on `PATH` and the daemon configured.

**Download** copies only the selected `.ahk` or `.sh` file into an existing directory, accepts absolute paths, paths relative to the current directory, and `~` paths, and asks before overwriting. The downloaded script retains its direct-execution defaults.

Macro packages live below `packages/macros`. The `autohotkey` driver discovers regular `.ahk` files recursively; `ydotool` discovers `.sh` files. Ydotool scripts must support `--check` followed by the macro arguments, validating them without sleeping or sending input. A script can have an optional same-basename `.json` sidecar containing its title, description, and ordered argument definitions; scripts without sidecars derive their title from the filename and receive no toolbox-supplied arguments. Adding a script to either subpackage is automatic; another subpackage requires a driver implementation.

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
- `setup-wsl` (WSL): Set up selected shell and terminal tools on Ubuntu 22.04 or 24.04 under WSL. It can also configure Shift+Enter to insert line breaks in Windows Terminal and VS Code terminals. Uses `sudo` for system dependencies, backs up managed configuration when possible, and continues past optional feature failures.
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

## Monitor

Install Monitor from `tb list` or directly:

```sh
tb install-monitor
```

Monitor supervises one or more non-interactive Python scripts on Linux or WSL. It validates the targets, lets you review the queue and Python 3 interpreter, assigns a shared or per-script run title, and then executes the scripts sequentially:

```sh
monitor script.py
monitor first.py second.py
```

During a run, the terminal dashboard shows queue progress, elapsed time, recent output, resource usage, restart activity, and email-delivery results. Monitor samples CPU and process-tree memory, detects NVIDIA GPU usage when available, writes logs and optional charts below each script's `runs/monitor_logs/` directory, and stops the active target and remaining queue when you press Ctrl+C.

Runtime safeguards include configurable crash retries with backoff, rapid-crash detection for possible code errors, memory-leak detection, and either memory-aware or time-scheduled restarts. Email notifications can report heartbeats, recovery, scheduled restarts, final failures, completion, possible leaks, and possible code errors. SMTP credentials and defaults are configured interactively and stored with current-user-only permissions under `~/.monitor`:

```sh
monitor config
```

Before launch, choose **Edit this run** to override the interpreter, sampling, restart, notification, report, and viewer settings without changing the saved defaults. Run `monitor --help` for the command summary and `monitor --version` for the installed version.

## Uninstallation

Run `tb uninstall` and confirm removal. The command removes the toolbox wrapper and all installed toolbox versions. On Windows, it also removes each exact managed wrapper-directory entry from the user `PATH` while preserving unrelated entries.

Uninstallation does not remove tools, plugins, agent configurations, or generated workspaces.

## Development

Development and release builds require Go 1.25.8, as declared in `go.mod`, and Python 3.9 or newer. Building or testing the bundled reader also requires Rust and Cargo; its build workflow uses the stable Rust toolchain. These build dependencies are not required for bootstrap installation. Run the core test suites and installer tests with:

```sh
go test ./...
cargo test --locked --manifest-path packages/search/fork-markdown-reader/Cargo.toml
python3 -m pip install -r packages/monitor_runtime/requirements.txt
PYTHONPATH=packages/monitor_runtime python3 -m unittest discover -s packages/monitor_runtime/tests -v
python3 -m unittest discover -s packages/agent-workspace-template/source/tests -v
python3 -m unittest discover -s packages/others/tests -v
sh scripts/install_test.sh
sh scripts/build-release_test.sh
sh scripts/build-markdown-reader_test.sh
bash scripts/terminal-setup_test.sh
```

CI additionally validates release/version scripts, shell completion, PowerShell 5.1 and 7 installers and reader-build checks, the Windows terminal hotkey, WSL Shift+Enter bindings, shell syntax, ShellCheck, race detection, and cross-platform builds. The [reader build workflow](.github/workflows/markdown-reader.yml) builds and self-checks each reader on its native platform: Linux MUSL binaries must have no ELF interpreter or dynamic dependencies, and Windows MSVC binaries must have no dynamic VC/UCRT imports.

Create release archives by passing a canonical three-part version, an output directory, and a directory containing all three verified native readers:

```sh
scripts/build-release.sh 0.1.123 dist readers
```

The reader directory must contain these files, with executable permissions on Linux readers:

```text
readers/
  linux-amd64/libexec/tb-markdown-reader
  linux-arm64/libexec/tb-markdown-reader
  windows-amd64/libexec/tb-markdown-reader.exe
```

The release workflow assembles that directory from the native build artifacts. For local native builds, use [build-markdown-reader.sh](scripts/build-markdown-reader.sh) with the matching MUSL target and an output directory, or [build-markdown-reader.ps1](scripts/build-markdown-reader.ps1) with an output directory on Windows x64; the workflow records the required compiler and binary-inspection setup.

The build produces Linux x64, Linux ARM64, and Windows x64 archives, corresponding SHA-256 files, and `version.txt`. Each archive includes its compiled reader under `libexec`; the reader's Rust sources and Cargo build artifacts are excluded. Release, Pages, and submodule automation is defined under `.github/workflows`; repository secrets and settings are configured separately.

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

## References

- The bundled reader is derived from [leboiko/markdown-reader](https://github.com/leboiko/markdown-reader) 1.35.1 at [commit `186698c`](https://github.com/leboiko/markdown-reader/tree/186698caba1f6c4f9932296da03c5599b35408d0). Credit goes to its original author and contributors. The local fork remains available under its [MIT License](packages/search/fork-markdown-reader/LICENSE).
