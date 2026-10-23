#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname "$0")" && pwd)
# shellcheck disable=SC1091
. "$repo_root/packages/scripts/lib/toolbox_output.sh"
tb_output_init install-local

fail() {
    tb_error "$1"
    exit 1
}

show_path_guide() {
    tb_info 'If tb command is not recognized, do:'
    tb_info 'For this terminal:'
    tb_info "export PATH=\"\$HOME/.local/bin:\$PATH\""
    tb_info 'Persist for Bash:'
    tb_info "printf '%s\\n' 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> \"\$HOME/.bashrc\""
    tb_info 'Persist for Zsh:'
    tb_info "printf '%s\\n' 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> \"\$HOME/.zshrc\""
    tb_info 'Persist for SSH login shells that read ~/.profile:'
    tb_info "printf '%s\\n' 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> \"\$HOME/.profile\""
}

case "$(uname -s):$(uname -m)" in
    Linux:x86_64) rust_target=x86_64-unknown-linux-musl ;;
    Linux:aarch64) rust_target=aarch64-unknown-linux-musl ;;
    *) fail 'Local installation supports Linux x64 and ARM64, including WSL.' ;;
esac

command -v go >/dev/null 2>&1 || fail 'Go 1.25.8 is required. Install it and retry.'
data_root=${XDG_DATA_HOME:-"$HOME/.local/share"}/my-toolbox
versions_root=$data_root/versions
current_file=$data_root/current.txt
wrapper_root=$HOME/.local/bin
wrapper=$wrapper_root/tb
current=
if [ -f "$current_file" ]; then
    IFS= read -r current < "$current_file"
fi
base_version=$(printf '%s\n' "$current" | sed -n 's/^\([0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*\).*$/\1/p')
base_version=${base_version:-0.0.0}
version="$base_version-local.$(date -u +%Y%m%d%H%M%S)"
version_root=$versions_root/$version
[ ! -e "$version_root" ] || fail "Local version $version already exists. Retry after one second."
mkdir -p "$versions_root" "$wrapper_root"
stage=$(mktemp -d "$versions_root/.install-$version.XXXXXX")
activated=0
cleanup() {
    if [ "$activated" -eq 0 ] && [ -d "$stage" ]; then
        rm -rf -- "$stage"
    fi
}
trap cleanup EXIT HUP INT TERM

tb_info 'Checking the local source and bundled Markdown reader...'
mkdir -p "$stage/libexec"
reader=${TOOLBOX_READER:-}
if [ -z "$reader" ] && [ -n "$current" ] && [ -x "$versions_root/$current/libexec/tb-markdown-reader" ]; then
    reader=$versions_root/$current/libexec/tb-markdown-reader
fi
if [ -n "$reader" ]; then
    cp "$reader" "$stage/libexec/tb-markdown-reader"
else
    command -v cargo >/dev/null 2>&1 || fail 'Cargo is required to build the bundled Markdown reader.'
    tb_info 'Building the bundled Markdown reader...'
    "$repo_root/scripts/build-markdown-reader.sh" "$rust_target" "$stage" || fail 'Markdown reader build failed; the active version was not changed.'
fi
"$stage/libexec/tb-markdown-reader" --tb-self-check >/dev/null || fail 'Markdown reader self-check failed; the active version was not changed.'
cp "$repo_root/packages/search/fork-markdown-reader/LICENSE" "$stage/libexec/tb-markdown-reader-LICENSE"

tb_info 'Building and staging the local toolbox...'
cp "$repo_root/commands.json" "$stage/commands.json"
cp -R "$repo_root/completions" "$stage/completions"
tar -C "$repo_root" --exclude='packages/search/fork-markdown-reader' --exclude='*/.git' --exclude='*/__pycache__' -cf - packages | tar -C "$stage" -xf -
printf '%s\n' "$version" > "$stage/version.txt"
(cd "$repo_root" && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$stage/tb" ./src) || fail 'Go build failed; the active version was not changed.'
"$stage/tb" help >/dev/null || fail 'Local toolbox smoke test failed; the active version was not changed.'

tb_info 'Activating the local toolbox version...'
mv "$stage" "$version_root"
activated=1
if [ ! -e "$wrapper" ]; then
    cat > "$wrapper" <<'SH'
#!/bin/sh
set -eu
data_root="${XDG_DATA_HOME:-$HOME/.local/share}/my-toolbox"
IFS= read -r current < "$data_root/current.txt"
exec "$data_root/versions/$current/tb" "$@"
SH
    chmod 755 "$wrapper"
fi
candidate=$(mktemp "$data_root/.current.XXXXXX")
printf '%s\n' "$version" > "$candidate"
mv "$candidate" "$current_file"
show_path_guide
tb_success "Installed local toolbox $version. Run $wrapper version to verify it."
