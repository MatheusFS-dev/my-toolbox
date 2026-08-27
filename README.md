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

`tb list` opens the Arrow/Space checkbox selector. It collects every required answer before running any selected tool, executes tools in catalog order, and stops at the first failure. `tb list` and `tb help` show only tools supported by the current environment: native Linux, WSL, or Windows. Direct use of an unsupported tool returns an explicit error. Running `tb` without arguments is invalid and directs the user to `tb list`.

The command catalog is defined in `commands.json`. It includes Codex, Claude, Antigravity, uv, gh, the corresponding Superpowers plugins, global or project-scoped agent workspace setup, and these independent packages:

| Package | Commands |
|---|---|
| `scripts` | `setup-alacritty`, `setup-kitty`, `setup-windows`, `setup-wsl`, `set-vscode-wsl-cwd`, `set-default-cwd`, `change-grub-order`, `setup-venv`, `toggle-nopasswd-sudo` |
| `others` | `create-env-alias`, `bootstrap-python-from-venv`, `create-project-template` |

The copied Bash and PowerShell tools receive direct arguments unchanged. The three `others` tools are interactive and reject command-line arguments. `create-env-alias` targets Bash and Zsh virtual environments on Linux or WSL. `bootstrap-python-from-venv` infers unpinned dependencies from project imports and a selected Linux/WSL venv. `create-project-template` recursively merges the packaged `packages/others/template/` tree into an explicit existing destination.

The vendored Alacritty, Kitty, and WSL setup scripts target apt-based Debian/Ubuntu environments; their optional desktop integrations also require the upstream GNOME/Nautilus tools. The toolbox preserves those scripts and their exit behavior byte-for-byte.

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
