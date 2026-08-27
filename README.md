# my-toolbox

`tb` is a portable terminal toolbox for Linux x64, Linux ARM64, and Windows x64. It installs supported command-line agents, base tools, Superpowers plugins, and agent workspace templates without `sudo`.

## Usage

```text
tb list
tb <tool> [arguments...]
tb update
tb version
tb help
```

`tb list` opens the Arrow/Space checkbox selector. It collects every required answer before running any selected tool, executes tools in catalog order, and stops at the first failure. Running `tb` without arguments is invalid and directs the user to `tb list`.

The command catalog is defined in `commands.json`. Initial tools include Codex, Claude, Antigravity, uv, gh, the corresponding Superpowers plugins, and global or project-scoped agent workspace setup.

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

## Development

The repository requires Go and Python 3. Run:

```sh
go test ./...
python3 -m unittest discover -s packages/agent-workspace-template/source/tests -v
```

Create release archives with:

```sh
scripts/build-release.sh 0.1.123 dist
```

The build produces Linux x64, Linux ARM64, and Windows x64 archives, corresponding SHA-256 files, and `version.txt`. Release, Pages, and submodule automation is defined under `.github/workflows`; repository secrets and Pages settings must be configured separately.
