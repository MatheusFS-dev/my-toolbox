#!/bin/sh
set -eu

if [ "$#" -ne 3 ]; then
    printf 'Usage: tooling/release/build-release.sh VERSION OUTPUT_DIRECTORY READER_DIRECTORY\n' >&2
    exit 1
fi

version=$1
output_directory=$2
reader_directory=$3
major=${version%%.*}
remainder=${version#*.}
minor=${remainder%%.*}
patch=${remainder#*.}
case "$version" in *.*.*) ;; *) printf 'Version must be a canonical three-part version.\n' >&2; exit 1 ;; esac
for component in "$major" "$minor" "$patch"; do
    case "$component" in ''|*[!0-9]*|0[0-9]*) printf 'Version must be a canonical three-part version.\n' >&2; exit 1 ;; esac
done

if [ -z "$output_directory" ] || { [ -e "$output_directory" ] && [ ! -d "$output_directory" ]; }; then
    printf 'Output directory must be a nonempty directory path.\n' >&2
    exit 1
fi
if [ -z "$reader_directory" ] || [ ! -d "$reader_directory" ]; then
    printf 'Reader directory must be an existing directory.\n' >&2
    exit 1
fi
# shellcheck disable=SC1007
reader_directory=$(CDPATH= cd -P -- "$reader_directory" && pwd)
# Validate the complete set before invoking Go or changing release output.
# Native build jobs already ran --tb-self-check; these binaries may be foreign
# to the packaging host and must not be executed here.
for platform in linux-amd64 linux-arm64 windows-amd64; do
    reader_name=tb-markdown-reader
    if [ "$platform" = windows-amd64 ]; then reader_name=tb-markdown-reader.exe; fi
    reader="$reader_directory/$platform/libexec/$reader_name"
    if [ ! -f "$reader" ] || [ -L "$reader" ] || [ ! -s "$reader" ] || [ ! -r "$reader" ]; then
        printf 'Reader must be a readable, nonempty regular file: %s\n' "$reader" >&2
        exit 1
    fi
    if [ "$platform" != windows-amd64 ] && [ ! -x "$reader" ]; then
        printf 'Linux reader must be executable: %s\n' "$reader" >&2
        exit 1
    fi
done

# The empty assignment prevents inherited CDPATH output from contaminating pwd.
# shellcheck disable=SC1007
repository_root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
mkdir -p "$output_directory"
# shellcheck disable=SC1007
output_directory=$(CDPATH= cd -P -- "$output_directory" && pwd)
temporary_root=$(mktemp -d)
cleanup() {
    rm -rf "$temporary_root"
}
trap cleanup EXIT HUP INT TERM

build_payload() {
    platform=$1
    goos=$2
    goarch=$3
    binary_name=$4
    payload="$temporary_root/$platform"
    mkdir -p "$payload"
    (
        cd "$repository_root"
        GOOS=$goos GOARCH=$goarch CGO_ENABLED=0 go build \
            -trimpath -ldflags "-s -w -X main.version=$version" \
            -o "$payload/$binary_name" ./cmd/tb
    )
    mkdir -p "$payload/libexec"
	if [ "$goos" = linux ]; then
		(
			cd "$repository_root/assets/payload/packages/opencode_local_llm"
			GOOS=$goos GOARCH=$goarch CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o "$payload/libexec/tb-local-llm" ./cmd/tb-local-llm
		)
		cp "$repository_root/assets/payload/packages/opencode_local_llm/src/models.json" "$payload/libexec/models.json"
	fi
    reader_name=tb-markdown-reader
    if [ "$goos" = windows ]; then reader_name=tb-markdown-reader.exe; fi
    cp -p "$reader_directory/$platform/libexec/$reader_name" "$payload/libexec/$reader_name"
    cp "$repository_root/assets/reader/fork-markdown-reader/LICENSE" \
        "$payload/libexec/tb-markdown-reader-LICENSE"
    cp "$repository_root/assets/payload/commands.json" "$payload/commands.json"
    cp -R "$repository_root/assets/payload/completions" "$payload/completions"
    tar -C "$repository_root/assets/payload" -cf - packages | tar -C "$payload" -xf -
    find "$payload/packages" -name .git -exec rm -rf {} +
    find "$payload/packages" -type d -name __pycache__ -prune -exec rm -rf {} +
    find "$payload/packages" -type f \( -name '*.pyc' -o -name '*.pyo' \) -delete
    printf '%s\n' "$version" > "$payload/version.txt"
}

build_payload linux-amd64 linux amd64 tb
build_payload linux-arm64 linux arm64 tb
build_payload windows-amd64 windows amd64 tb.exe

for platform in linux-amd64 linux-arm64; do
    archive="toolbox-$platform.tar.gz"
    tar -C "$temporary_root/$platform" -czf "$output_directory/$archive" \
        tb libexec commands.json completions packages version.txt
    (
        cd "$output_directory"
        sha256sum "$archive" > "$archive.sha256"
    )
done

archive=toolbox-windows-amd64.zip
(
    cd "$temporary_root/windows-amd64"
    zip -qry "$temporary_root/$archive" .
)
mv "$temporary_root/$archive" "$output_directory/$archive"
(
    cd "$output_directory"
    sha256sum "$archive" > "$archive.sha256"
)

template_root="$repository_root/assets/payload/packages/agent-workspace-template"
portable_root="$temporary_root/agent-configs-portable"
portable_home="$portable_root/home"
if [ ! -d "$template_root/skills" ] || [ ! -f "$template_root/instructions/global.md" ] || \
    [ ! -f "$template_root/configs/codex/config.toml.template" ] || \
    [ ! -f "$template_root/configs/claude/settings.json" ] || \
    [ ! -f "$template_root/configs/antigravity/settings.json" ] || \
    [ ! -f "$template_root/configs/opencode/opencode.json" ] || \
    [ ! -f "$template_root/configs/statusline/statusline.py" ]; then
    printf 'Agent workspace template is missing a required portable-package source.\n' >&2
    exit 1
fi

mkdir -p "$portable_home/.codex/skills" \
    "$portable_home/.claude/skills" \
    "$portable_home/.gemini/antigravity-cli/skills" \
    "$portable_home/.config/opencode/skills"
python3 - "$template_root" "$portable_home/.codex/config.toml" <<'PY'
from pathlib import Path
import sys
import tomllib

template_root = Path(sys.argv[1])
destination = Path(sys.argv[2])
marker = "{{GLOBAL_INSTRUCTIONS}}"
template = (template_root / "configs" / "codex" / "config.toml.template").read_text(
    encoding="utf-8"
)
instructions = (template_root / "instructions" / "global.md").read_text(encoding="utf-8")

if template.count(marker) != 1:
    raise RuntimeError("Codex configuration template must contain one global-instructions marker.")
if "'''" in instructions:
    raise RuntimeError("Global instructions cannot contain three consecutive single quotes.")

rendered = template.replace(marker, instructions)
tomllib.loads(rendered)
destination.write_text(rendered, encoding="utf-8")
PY
for profile in "$template_root"/configs/codex/*.config.toml; do
    [ -f "$profile" ] || continue
    if [ "$(basename "$profile")" = maestro.config.toml ]; then continue; fi
    cp "$profile" "$portable_home/.codex/$(basename "$profile")"
done
python3 - "$template_root" "$portable_home/.codex/maestro.config.toml" <<'PY'
from pathlib import Path
import sys
import tomllib

root = Path(sys.argv[1])
destination = Path(sys.argv[2])
marker = "{{MAESTRO_INSTRUCTIONS}}"
template = (root / "configs" / "codex" / "maestro.config.toml").read_text(encoding="utf-8")
instructions = (root / "instructions" / "global.md").read_text(encoding="utf-8") + "\n\n" + (root / "instructions" / "maestro.md").read_text(encoding="utf-8")
if template.count(marker) != 1 or "'''" in instructions:
    raise RuntimeError("Cannot render portable Maestro profile safely.")
rendered = template.replace(marker, instructions)
tomllib.loads(rendered)
destination.write_text(rendered, encoding="utf-8")
PY
cp -R "$template_root/skills/." "$portable_home/.codex/skills"

cp "$template_root/instructions/global.md" "$portable_home/.claude/CLAUDE.md"
cp "$template_root/configs/claude/settings.json" "$portable_home/.claude/settings.json"
cp "$template_root/configs/statusline/statusline.py" "$portable_home/.claude/statusline.py"
mkdir -p "$portable_home/.claude/agents"
cp "$template_root/configs/claude/agents/maestro.md" "$portable_home/.claude/agents/maestro.md"
cp -R "$template_root/skills/." "$portable_home/.claude/skills"

mkdir -p "$portable_home/.gemini/antigravity-cli"
cp "$template_root/instructions/global.md" "$portable_home/.gemini/GEMINI.md"
cp "$template_root/configs/antigravity/settings.json" \
    "$portable_home/.gemini/antigravity-cli/settings.json"
cp "$template_root/configs/statusline/statusline.py" \
    "$portable_home/.gemini/antigravity-cli/statusline.py"
mkdir -p "$portable_home/.gemini/config/agents/maestro"
cp "$template_root/configs/antigravity/agents/maestro/agent.md" \
    "$portable_home/.gemini/config/agents/maestro/agent.md"
cp -R "$template_root/skills/." "$portable_home/.gemini/antigravity-cli/skills"

cp "$template_root/instructions/global.md" "$portable_home/.config/opencode/AGENTS.md"
cp "$template_root/configs/opencode/opencode.json" \
    "$portable_home/.config/opencode/opencode.json"
cp -R "$template_root/skills/." "$portable_home/.config/opencode/skills"

cat > "$portable_root/README.md" <<'EOF'
# Portable Agent Configuration Package

This package configures Codex, Claude Code, Antigravity CLI, and OpenCode without
installing my-toolbox, packages, or system software.

Close those agents before copying files. The `home/` folder mirrors your user
home directory. Copy its contents into `%USERPROFILE%` on Windows or `$HOME`
on Linux. Back up existing `.codex`, `.claude`, `.gemini`, and `.config/opencode`
folders first,
then replace them with the folders in `home/`.

The package contains:

- `.codex/config.toml`, optional `research` and `maestro` profiles, and skills
- `.claude/CLAUDE.md`, settings, status line, skills, and optional `maestro` agent
- `.gemini/GEMINI.md`, Antigravity CLI settings, status line, skills, and optional `maestro` agent
- `.config/opencode/AGENTS.md`, permissions, and skills

Python 3 is required only for the Claude Code and Antigravity CLI status lines.
The remaining instructions, settings, and skills can still be copied when it
is unavailable.

No administrator account or `sudo` is required as long as you can write to
your own home directory.
EOF

archive=agent-configs-portable.zip
(
    cd "$temporary_root"
    zip -qry "$temporary_root/$archive" agent-configs-portable
)
mv "$temporary_root/$archive" "$output_directory/$archive"
printf '%s\n' "$version" > "$output_directory/version.txt"
