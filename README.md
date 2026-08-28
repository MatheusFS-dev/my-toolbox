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
    Install Codex for the current user on Linux or Windows. Skips
    installation when `codex` is already available.
  ◉ install-claude
    Install Claude Code for the current user on Linux or Windows. Skips
    installation when `claude` is already available.

↑/↓ move • space select • enter run • esc cancel
1 tool selected
```

The example is shortened to show the row layout. The live selector wraps at the current terminal width, capped at 72 columns.

`tb list` collects every required answer before running any selected tool, executes tools in catalog order, and stops at the first failure. `tb list` excludes direct-only commands; `tb help` includes them. Both commands filter tools for native Linux, WSL, or Windows before building categories, so unsupported categories are omitted when empty. Direct use of an unsupported tool returns an explicit error. Running `tb` without arguments is invalid and directs the user to `tb list`.

`tb help` shows the current toolbox version and uses the same category order and description wrapping. ANSI styling is enabled only when standard output is a terminal; redirected help is plain text with the same hierarchy. Wide output is capped at 72 columns, while narrower terminals use their live width.

## Tool catalog

The command catalog is defined in `commands.json`. These descriptions focus on each tool’s purpose and the safeguards that matter during use.

### Agents

- `install-codex`: Install Codex for the current user on Linux or Windows. Skips installation when `codex` is already available.
- `install-claude`: Install Claude Code for the current user on Linux or Windows. Skips installation when `claude` is already available.
- `install-antigravity`: Install Antigravity for the current user on Linux or Windows. Skips installation when `agy` is already available.

### Base Tools

- `install-uv`: Install uv for the current user on Linux or Windows without changing shell PATH configuration. Skips installation when `uv` is already available.
- `install-gh`: Download, verify, and install the latest GitHub CLI for the current user. Shows PATH guidance when needed.

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
- `setup-wsl` (WSL): Set up selected shell and terminal tools on Ubuntu 22.04 or 24.04 under WSL. Uses sudo for system dependencies, backs up managed configuration when possible, and continues past optional feature failures.
- `set-vscode-wsl-cwd` (Windows): Open a chosen WSL directory in VS Code and use it as the working directory of a managed terminal profile. Preserves JSONC comments, backs up changed settings, and supports `-Undo`.
- `set-default-cwd` (WSL): Make Bash and Zsh start in a chosen WSL directory when opened from home. Preserves unrelated shell configuration and backs up changed files.

### System Utilities

- `change-grub-order` (native Linux): Choose the default GRUB boot entry from an interactive list. Backs up the current GRUB settings before applying the change.
- `setup-venv` (Linux or WSL): Add or remove a `venv` shell command that activates the nearest `.venv`. Keeps unrelated Bash and Zsh configuration but does not create backups.
- `toggle-nopasswd-sudo` (Linux or WSL): Enable or disable passwordless sudo for one Linux or WSL user. Validates enabling changes and only manages the toolbox-owned sudoers file.

### Project Utilities

- `create-env-alias` (Linux or WSL): Create a Bash or Zsh alias that activates a chosen `.venv`. Previews changes, confirms replacements separately, and can back up conflicts.
- `bootstrap-python-from-venv` (Linux or WSL): Generate requirements, `pyproject.toml`, and `.python-version` from imports found in Python files and optional notebooks. Preserves unrelated TOML, stops on ambiguous input, and can run `uv lock`.
- `create-project-template`: Merge the packaged project template into an existing directory without deleting destination-only files. Checks every conflict before asking to overwrite and does not create backups.

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

Bootstrap installation does not replace an existing toolbox. When a newer release is available, `tb update` downloads the bootstrap installer, removes the managed toolbox installation, and runs the installer again. If installation fails after removal, run the installation command above to restore `tb`.

Versions up to `0.1.6` use the former archive updater and cannot receive the reinstall-based fix. Run the installation command once to move from `0.1.6` to a newer release.

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
