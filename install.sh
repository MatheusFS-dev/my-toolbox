#!/bin/sh
set -eu

repository="MatheusFS-dev/my-toolbox"
data_root="${XDG_DATA_HOME:-$HOME/.local/share}/my-toolbox"
versions_root="$data_root/versions"
current_file="$data_root/current.txt"
wrapper_root="$HOME/.local/bin"
wrapper_path="$wrapper_root/tb"
temporary_root=
temporary_current=
temporary_wrapper=
staging_payload=
current_stage=0
current_stage_name=
published_version=0
published_wrapper=0
activated=0
saved_wrapper=
saved_wrapper_candidate=

if [ -t 1 ]; then
    color_info='\033[36m'
    color_ok='\033[32m'
    color_fail='\033[31m'
    color_reset='\033[0m'
else
    color_info=
    color_ok=
    color_fail=
    color_reset=
fi

status_line() {
    kind=$1
    message=$2
    color=$color_info
    if [ "$kind" = "OK" ]; then
        color=$color_ok
    elif [ "$kind" = "FAIL" ]; then
        color=$color_fail
    fi
    printf '%b[%s]%b %s\n' "$color" "$kind" "$color_reset" "$message"
}

start_stage() {
    current_stage=$1
    current_stage_name=$2
    status_line INFO "Stage $current_stage/7: $current_stage_name"
}

complete_stage() {
    status_line OK "Stage $current_stage/7: $current_stage_name"
}

on_exit() {
    exit_status=$1
    trap - 0
    if [ "$exit_status" -ne 0 ]; then
        if [ "$activated" -eq 1 ]; then
            rm -f "$current_file"
        fi
        if [ "$published_wrapper" -eq 1 ]; then
            rm -f "$wrapper_path"
            if [ -n "$saved_wrapper" ] && [ -e "$saved_wrapper" ]; then
                mv "$saved_wrapper" "$wrapper_path"
            fi
        elif [ -n "$saved_wrapper" ] && [ -e "$saved_wrapper" ]; then
            mv "$saved_wrapper" "$wrapper_path"
        fi
        if [ "$published_version" -eq 1 ]; then
            rm -rf "$version_root"
        fi
    fi
    if [ -n "$temporary_current" ] && [ -e "$temporary_current" ]; then
        rm -f "$temporary_current"
    fi
    if [ -n "$temporary_wrapper" ] && [ -e "$temporary_wrapper" ]; then
        rm -f "$temporary_wrapper"
    fi
    if [ -n "$staging_payload" ] && [ -d "$staging_payload" ]; then
        rm -rf "$staging_payload"
    fi
    if [ "$exit_status" -eq 0 ] && [ -n "$saved_wrapper" ] && [ -e "$saved_wrapper" ]; then
        rm -f "$saved_wrapper"
    fi
    if [ -n "$temporary_root" ] && [ -d "$temporary_root" ]; then
        rm -rf "$temporary_root"
    fi
    if [ "$exit_status" -ne 0 ] && [ "$current_stage" -ne 0 ]; then
        status_line FAIL "Stage $current_stage/7: $current_stage_name" >&2
    fi
    exit "$exit_status"
}
trap 'on_exit "$?"' 0
trap 'exit 1' HUP INT TERM

cat <<'BANNER'
 __  __ __   __  _____ ___   ___  _     ____   _____  __
|  \/  |\ \ / / |_   _/ _ \ / _ \| |   | __ ) / _ \ \/ /
| |\/| | \ V /    | || | | | | | | |   |  _ \| | | \  /
| |  | |  | |     | || |_| | |_| | |___| |_) | |_| /  \
|_|  |_|  |_|     |_| \___/ \___/|_____|____/ \___/_/\_\
BANNER

start_stage 1 prerequisites
if [ -f "$current_file" ]; then
    current_version=$(sed -n '1p' "$current_file")
    status_line INFO "my-toolbox $current_version is already installed. Run tb update to upgrade."
    complete_stage
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
command -v tar >/dev/null 2>&1 || {
    printf 'tar is required to extract my-toolbox.\n' >&2
    exit 1
}
complete_stage

start_stage 2 "release lookup"
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
complete_stage

start_stage 3 download
temporary_root=$(mktemp -d)
base_url="https://github.com/$repository/releases/download/$tag"
curl -fsSL "$base_url/$archive" -o "$temporary_root/$archive"
curl -fsSL "$base_url/$archive.sha256" -o "$temporary_root/$archive.sha256"
complete_stage

start_stage 4 checksum
(
    cd "$temporary_root"
    sha256sum -c "$archive.sha256" >/dev/null
)
complete_stage

start_stage 5 extraction/validation
mkdir -p "$versions_root"
staging_payload=$(mktemp -d "$versions_root/.install-$version.XXXXXX")
tar -xzf "$temporary_root/$archive" -C "$staging_payload"
for required in tb commands.json version.txt \
    packages/agent-workspace-template/source/scripts/linux/python3/install_codex.py \
    packages/agent-workspace-template/source/scripts/linux/python3/install_claude.py \
    packages/agent-workspace-template/source/scripts/linux/python3/install_antigravity.py \
    packages/agent-workspace-template/source/scripts/linux/python3/install_project.py \
    packages/agent-workspace-template/source/scripts/linux/python2/install_codex.py \
    packages/agent-workspace-template/source/scripts/linux/python2/install_claude.py \
    packages/agent-workspace-template/source/scripts/linux/python2/install_antigravity.py \
    packages/agent-workspace-template/source/scripts/linux/python2/install_project.py \
    packages/agent-workspace-template/source/scripts/linux/python2/requirements.txt \
    packages/scripts/terminal/alacritty/setup_alacritty.sh \
    packages/scripts/terminal/kitty/setup_kitty.sh \
    packages/scripts/terminal/wsl/setup_wsl.sh \
    packages/scripts/terminal/wsl/set_default_cwd.sh \
    packages/scripts/utils/change_grub_order.sh \
    packages/scripts/utils/setup_venv.sh \
    packages/scripts/utils/toggle_nopasswd_sudo.sh \
    packages/others/create_env_alias.py \
    packages/others/bootstrap_python_from_venv.py \
    packages/others/create_project_template.py; do
    [ -f "$staging_payload/$required" ] || {
        printf 'Downloaded payload is missing %s.\n' "$required" >&2
        exit 1
    }
done
[ "$(sed -n '1p' "$staging_payload/version.txt")" = "$version" ] || {
    printf 'Downloaded payload version does not match release %s.\n' "$version" >&2
    exit 1
}
complete_stage

start_stage 6 installation
mkdir -p "$versions_root" "$wrapper_root"
temporary_wrapper=$(mktemp "$wrapper_root/.tb.XXXXXX")
{
    printf '%s\n' '#!/bin/sh'
    printf '%s\n' 'set -eu'
    # Wrapper variables must remain literal until the installed wrapper runs.
    # shellcheck disable=SC2016
    printf '%s\n' 'data_root="${XDG_DATA_HOME:-$HOME/.local/share}/my-toolbox"'
    # shellcheck disable=SC2016
    printf '%s\n' 'current=$(sed -n "1p" "$data_root/current.txt")'
    # shellcheck disable=SC2016
    printf '%s\n' 'exec "$data_root/versions/$current/tb" "$@"'
} > "$temporary_wrapper"
chmod 755 "$temporary_wrapper"
if [ -d "$wrapper_path" ]; then
    printf 'Wrapper path is an existing directory: %s.\n' "$wrapper_path" >&2
    exit 1
fi
if [ -e "$wrapper_path" ] || [ -L "$wrapper_path" ]; then
    saved_wrapper_candidate=$(mktemp "$wrapper_root/.tb.previous.XXXXXX")
    rm -f "$saved_wrapper_candidate"
    mv "$wrapper_path" "$saved_wrapper_candidate"
    saved_wrapper=$saved_wrapper_candidate
fi
published_version=1
mv "$staging_payload" "$version_root"
staging_payload=
published_wrapper=1
mv "$temporary_wrapper" "$wrapper_path"
temporary_wrapper=
complete_stage

start_stage 7 activation
temporary_current="$data_root/current.txt.new"
printf '%s\n' "$version" > "$temporary_current"
activated=1
mv "$temporary_current" "$current_file"
temporary_current=
status_line OK "Installed my-toolbox $version."
case ":$PATH:" in
    *":$wrapper_root:"*) ;;
    *) status_line INFO "Add $wrapper_root to PATH to run tb." ;;
esac
complete_stage
