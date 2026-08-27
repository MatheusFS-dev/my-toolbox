#!/bin/sh
set -eu

repository="MatheusFS-dev/my-toolbox"
data_root="${XDG_DATA_HOME:-$HOME/.local/share}/my-toolbox"
versions_root="$data_root/versions"
current_file="$data_root/current.txt"
wrapper_root="$HOME/.local/bin"
wrapper_path="$wrapper_root/tb"

if [ -f "$current_file" ]; then
    current_version=$(sed -n '1p' "$current_file")
    printf 'my-toolbox %s is already installed. Run tb update to upgrade.\n' "$current_version"
    exit 0
fi

case "$(uname -m)" in
    x86_64|amd64) archive="toolbox-linux-amd64.tar.gz" ;;
    aarch64|arm64) archive="toolbox-linux-arm64.tar.gz" ;;
    *) printf 'Unsupported Linux architecture: %s\n' "$(uname -m)" >&2; exit 1 ;;
esac

command -v curl >/dev/null 2>&1 || {
    printf 'curl is required to install my-toolbox.\n' >&2
    exit 1
}
command -v sha256sum >/dev/null 2>&1 || {
    printf 'sha256sum is required to verify my-toolbox.\n' >&2
    exit 1
}

release_json=$(curl -fsSL "https://api.github.com/repos/$repository/releases/latest")
tag=$(printf '%s' "$release_json" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
[ -n "$tag" ] || {
    printf 'Latest my-toolbox release did not contain a tag.\n' >&2
    exit 1
}
case "$tag" in v*) ;; *) printf 'Release tag is not a safe three-part version: %s\n' "$tag" >&2; exit 1 ;; esac
version=${tag#v}
major=${version%%.*}
remainder=${version#*.}
minor=${remainder%%.*}
patch=${remainder#*.}
case "$version" in *.*.*) ;; *) printf 'Release tag is not a safe three-part version: %s\n' "$tag" >&2; exit 1 ;; esac
for component in "$major" "$minor" "$patch"; do
    case "$component" in ''|*[!0-9]*|0[0-9]*) printf 'Release tag is not a safe three-part version: %s\n' "$tag" >&2; exit 1 ;; esac
done
version_root="$versions_root/$version"
[ ! -e "$version_root" ] || {
    printf 'Version directory already exists without an active installation: %s\n' "$version_root" >&2
    exit 1
}

temporary_root=$(mktemp -d)
cleanup() {
    rm -rf "$temporary_root"
}
trap cleanup EXIT HUP INT TERM
base_url="https://github.com/$repository/releases/download/$tag"
curl -fsSL "$base_url/$archive" -o "$temporary_root/$archive"
curl -fsSL "$base_url/$archive.sha256" -o "$temporary_root/$archive.sha256"
(
    cd "$temporary_root"
    sha256sum -c "$archive.sha256"
)
mkdir -p "$temporary_root/payload"
tar -xzf "$temporary_root/$archive" -C "$temporary_root/payload"

for required in tb commands.json version.txt \
    packages/agent-workspace-template/source/scripts/linux/python3/install_codex.py \
    packages/agent-workspace-template/source/scripts/linux/python3/install_claude.py \
    packages/agent-workspace-template/source/scripts/linux/python3/install_antigravity.py \
    packages/agent-workspace-template/source/scripts/linux/python3/install_project.py \
    packages/agent-workspace-template/source/scripts/linux/python2/install_codex.py \
    packages/agent-workspace-template/source/scripts/linux/python2/install_claude.py \
    packages/agent-workspace-template/source/scripts/linux/python2/install_antigravity.py \
    packages/agent-workspace-template/source/scripts/linux/python2/install_project.py \
    packages/agent-workspace-template/source/scripts/linux/python2/requirements.txt; do
    [ -f "$temporary_root/payload/$required" ] || {
        printf 'Downloaded payload is missing %s.\n' "$required" >&2
        exit 1
    }
done
[ "$(sed -n '1p' "$temporary_root/payload/version.txt")" = "$version" ] || {
    printf 'Downloaded payload version does not match release %s.\n' "$version" >&2
    exit 1
}
mkdir -p "$versions_root" "$wrapper_root"
temporary_wrapper="$wrapper_path.new"
{
    printf '%s\n' '#!/bin/sh'
    printf '%s\n' 'set -eu'
    printf '%s\n' 'data_root="${XDG_DATA_HOME:-$HOME/.local/share}/my-toolbox"'
    printf '%s\n' 'current=$(sed -n "1p" "$data_root/current.txt")'
    printf '%s\n' 'exec "$data_root/versions/$current/tb" "$@"'
} > "$temporary_wrapper"
chmod 755 "$temporary_wrapper"
mv "$temporary_wrapper" "$wrapper_path"
mv "$temporary_root/payload" "$version_root"

temporary_current="$data_root/current.txt.new"
printf '%s\n' "$version" > "$temporary_current"
mv "$temporary_current" "$current_file"

printf 'Installed my-toolbox %s.\n' "$version"
case ":$PATH:" in
    *":$wrapper_root:"*) ;;
    *) printf 'Add %s to PATH to run tb.\n' "$wrapper_root" ;;
esac
