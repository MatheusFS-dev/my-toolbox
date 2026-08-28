# my-toolbox

`tb` is a portable terminal toolbox for Linux x64, Linux ARM64, and Windows x64. It installs supported command-line agents, base tools, Superpowers plugins, and agent workspace templates without `sudo`.

## Usage

```text
tb list
tb <tool> [arguments...]
tb update
tb uninstall
tb version
tb help
```

`tb list` opens a categorized selector. Up and Down move between tools, Space toggles the focused tool, Enter continues, and Escape or Ctrl+C cancels. The title and controls remain visible while the tool rows scroll. Selected markers and names are green; descriptions remain gray.

```text
SELECT TOOLS

  Agents
› ◯ install-codex
    Download and execute OpenAI’s official Codex installer for the current
    Linux or Windows platform when `codex` is not already available from
    PATH or its supported user installation path. Run with closed input;
    the toolbox performs no separate post-install executable or
    configuration check.
  ◉ install-claude
    Download and execute Anthropic’s official Claude Code installer for
    the current Linux or Windows platform when `claude` is not already
    available from PATH or its supported user installation path. Run with
    closed input; the toolbox performs no separate post-install executable
    or configuration check.

↑/↓ move • space select • enter run • esc cancel
1 tool selected
```

The example is shortened to show the row layout. The live selector wraps at the current terminal width, capped at 72 columns.

`tb list` collects every required answer before running any selected tool, executes tools in catalog order, and stops at the first failure. `tb list` excludes direct-only commands; `tb help` includes them. Both commands filter tools for native Linux, WSL, or Windows before building categories, so unsupported categories are omitted when empty. Direct use of an unsupported tool returns an explicit error. Running `tb` without arguments is invalid and directs the user to `tb list`.

`tb help` uses the same category order and description wrapping. ANSI styling is enabled only when standard output is a terminal; redirected help is plain text with the same hierarchy. Wide output is capped at 72 columns, while narrower terminals use their live width.

## Tool catalog

The command catalog is defined in `commands.json`. The following descriptions document the reviewed implementation behavior.

### Agents

- `install-codex`: Download and execute OpenAI’s official Codex installer for the current Linux or Windows platform when `codex` is not already available from PATH or its supported user installation path. Run with closed input; the toolbox performs no separate post-install executable or configuration check.
- `install-claude`: Download and execute Anthropic’s official Claude Code installer for the current Linux or Windows platform when `claude` is not already available from PATH or its supported user installation path. Run with closed input; the toolbox performs no separate post-install executable or configuration check.
- `install-antigravity`: Download and execute Google’s official Antigravity CLI installer for the current Linux or Windows platform when `agy` is not already available from PATH or its supported user installation path. Run with closed input; the toolbox performs no separate post-install executable or configuration check.

### Base Tools

- `install-uv`: Download and execute Astral’s official uv installer for the current Linux or Windows platform when `uv` is unavailable. Set `UV_NO_MODIFY_PATH=1` so the installer does not edit shell PATH configuration; the toolbox performs no separate post-install executable check.
- `install-gh`: Resolve the latest GitHub CLI release, download its platform archive and published SHA-256 checksums, verify the selected archive, and atomically install only the `gh` executable into `~/.local/bin` on Linux or `%LOCALAPPDATA%\my-toolbox\bin` on Windows. Print PATH guidance when that directory is not active.

### Agent Plugins

- `install-superpowers-codex`: Require an installed Codex CLI with plugin management, inspect `codex plugin list`, and add `superpowers@openai-curated` when absent. Skip an existing Superpowers installation and leave other Codex plugins unchanged.
- `install-superpowers-claude`: Require an installed Claude Code CLI with plugin management, inspect `claude plugin list`, and install `superpowers@claude-plugins-official` at user scope when absent. Skip an existing installation and leave other Claude plugins unchanged.
- `install-superpowers-antigravity`: Require an installed Antigravity CLI with plugin management, inspect `agy plugin list`, and install Superpowers from its GitHub repository when absent. Skip an existing installation and leave other Antigravity plugins unchanged.

### Agent Workspace

- `setup-agents-codex`: Validate the packaged global instructions, Codex configuration template, optional profiles, and skill packages; render valid TOML into `~/.codex/config.toml`; and install selected profiles and packaged skills under `~/.codex`. List all managed conflicts before writing, require replacement confirmation, and optionally create adjacent backups.
- `setup-agents-claude`: Validate the packaged instructions, Claude settings JSON, and skill packages, then install `CLAUDE.md`, `settings.json`, and each packaged skill under `~/.claude`. List all managed conflicts before writing, require replacement confirmation, and optionally create adjacent backups.
- `setup-agents-antigravity`: Validate the packaged instructions, Antigravity settings JSON, and skill packages, then install `GEMINI.md` under `~/.gemini` and settings and skills under `~/.gemini/antigravity-cli`. List all managed conflicts before writing, require replacement confirmation, and optionally create adjacent backups.
- `setup-agents-project` (direct only): Prompt for an existing project and selected agent formats, install `AGENTS.md` for Codex or Antigravity and `CLAUDE.md` for Claude, and optionally append managed instruction and Superpowers paths to `.gitignore`. Preserve unrelated `.gitignore` content and optionally back up conflicting managed instruction files.

### Terminal

- `setup-alacritty` (native Linux): On native Debian or Ubuntu Linux, run through sudo and prompt for Alacritty, Zsh, Rust, Zellij, Starship, fonts, shell helpers, Nautilus integration, and default-terminal configuration. Apply system packages and user configuration under the invoking user’s home; optional module failures do not stop later modules, and selected Alacritty configuration replaces `~/.config/alacritty/alacritty.toml` without a backup.
- `setup-kitty` (native Linux): On native Debian or Ubuntu Linux, run through sudo and prompt for Kitty, Zsh, Rust, Zellij, Starship, fonts, shell helpers, Nautilus integration, and default-terminal configuration. Install Kitty under the invoking user’s home, continue after optional module failures, and back up an existing `~/.config/kitty/kitty.conf` before replacing it.
- `setup-windows` (Windows): On native Windows 10 build 17763 or newer, or Windows 11, use WinGet to install or update Windows Terminal and PowerShell 7 plus selected fonts and terminal utilities. Write managed Terminal, PowerShell, Starship, Zellij, and VS Code configuration, attempt a timestamped backup under `%LOCALAPPDATA%\project-template\windows-backups`, and report each feature’s result.
- `setup-wsl` (WSL): On Ubuntu 22.04 or 24.04 under WSL, run through sudo and install selected Zsh, Rust, Zellij, Starship, eza, fzf, and shell-integration features for the invoking user. Install apt dependencies in one pass, attempt backups under `~/.local/state/project-template/wsl-backups`, continue after optional feature failures, and report that the terminal must be restarted.
- `set-vscode-wsl-cwd` (Windows): On Windows, validate an absolute directory in the default WSL distribution, add or update the managed `WSL (project-template)` terminal profile in VS Code’s user `settings.json`, and open that WSL directory in VS Code. Preserve JSONC comments and file encoding, create a backup when settings change, and support `-Undo` for removing the managed profile.
- `set-default-cwd` (WSL): On WSL, prompt for an explicit existing absolute directory and update managed blocks in `~/.bashrc` and `~/.zshrc` so shells starting in the home directory change to it. Preserve unrelated content, permissions, and line endings; back up changed existing files and reject symlinks or malformed markers.

### System Utilities

- `change-grub-order` (native Linux): On native Linux through sudo, parse `/boot/grub/grub.cfg`, display submenu-aware boot entries and the current default, and prompt for a replacement. Back up `/etc/default/grub`, update `GRUB_DEFAULT`, optionally disable `GRUB_SAVEDEFAULT`, run `update-grub`, and print the resulting configuration.
- `setup-venv` (Linux or WSL): On Linux or WSL, install or remove a managed `venv` shell function in `~/.bashrc` and, when Zsh is installed, `~/.zshrc`. The function searches the current directory and its parents for `.venv/bin/activate`; unrelated shell content is retained, but this script does not create backups.
- `toggle-nopasswd-sudo` (Linux or WSL): On Linux or WSL through sudo, detect whether the selected user currently has passwordless sudo and offer the opposite action. Manage only `/etc/sudoers.d/99-<user>-nopasswd`, validate an enabling rule with `visudo`, verify the effective sudo state afterward, and report external rules when disabling cannot remove NOPASSWD access.

### Project Utilities

- `create-env-alias` (Linux or WSL): Validate a selected virtual environment or project containing `.venv`, prompt for an alias and explicit Bash, Zsh, or both selection, and preview activation changes to `~/.bashrc` and/or `~/.zshrc`. Require separate replacement confirmation, optionally back up conflicts, preserve unrelated content and permissions, and replace each file atomically.
- `bootstrap-python-from-venv` (Linux or WSL): Scan project Python files and optional notebooks once, exclude standard-library and local modules, and map remaining imports through a selected virtual environment’s distribution metadata. Generate unpinned requirements, `pyproject.toml`, and `.python-version`, preserve unrelated TOML, stop on malformed or ambiguous input, optionally back up conflicts, and optionally validate and run `uv lock`.
- `create-project-template`: Dynamically discover and recursively merge the packaged template into an explicit existing destination, including dotfiles, empty directories, and symlinks. Preserve destination-only paths, reject overlap and entry-type mismatches before copying, list same-type conflicts once, and overwrite them only after explicit confirmation without creating template backups.

The copied Bash and PowerShell tools receive direct arguments unchanged. The three Project Utilities are interactive and reject command-line arguments. The vendored Alacritty, Kitty, and WSL setup scripts target the documented Debian or Ubuntu environments; their optional desktop integrations also require the upstream GNOME/Nautilus tools. The toolbox preserves those scripts and their exit behavior byte-for-byte.

## Installation

Linux:

```sh
curl -fsSL https://matheusfs-dev.github.io/my-toolbox/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://matheusfs-dev.github.io/my-toolbox/install.ps1 | iex
```

Bootstrap installation does not replace an existing toolbox. Use `tb update` to download and verify a newer release, validate its payload, install it into a versioned user directory, and atomically switch the stable wrapper.

The published URLs become available only after GitHub Pages is enabled for the repository and the included Pages workflow has deployed successfully.

## Uninstallation

Run `tb uninstall` and confirm removal. The command removes the toolbox wrapper and every installed toolbox version. It does not remove tools, plugins, agent configurations, or generated workspaces.

## Development

The repository requires Go and Python 3. Run:

```sh
go test ./...
python3 -m unittest discover -s packages/agent-workspace-template/source/tests -v
python3 -m unittest discover -s packages/others/tests -v
sh scripts/install_test.sh
```

Create release archives with:

```sh
scripts/build-release.sh 0.1.123 dist
```

The build produces Linux x64, Linux ARM64, and Windows x64 archives, corresponding SHA-256 files, and `version.txt`. Release, Pages, and submodule automation is defined under `.github/workflows`; repository secrets and Pages settings must be configured separately.
