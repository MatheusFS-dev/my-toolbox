#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/shared.sh"

view="${1:-list}"
case "$(uname -m)" in
    x86_64) target='x86_64-unknown-linux-gnu' ;;
    aarch64|arm64) target='aarch64-unknown-linux-gnu' ;;
    *) printf 'Unsupported architecture for eza: %s\n' "$(uname -m)" >&2; exit 1 ;;
esac

mkdir -p "$HOME/.local/bin"
if [[ ! -x "$HOME/.local/bin/eza" ]]; then
    temp_dir="$(mktemp -d)"
    trap 'rm -rf "$temp_dir"' EXIT
    archive="$temp_dir/eza.tar.gz"
    curl -fsSL "https://github.com/eza-community/eza/releases/latest/download/eza_${target}.tar.gz" -o "$archive"
    tar -xzf "$archive" -C "$temp_dir"
    install -m 0755 "$temp_dir/eza" "$HOME/.local/bin/eza"
fi

if [[ "$view" == list ]]; then
    block=$'eza_extra_params="-1"\nzi light z-shell/zsh-eza'
else
    block='zi light z-shell/zsh-eza'
fi
write_managed_block "$HOME/.zshrc" eza "$block"

