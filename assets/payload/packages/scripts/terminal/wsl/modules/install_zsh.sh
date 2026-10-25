#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/shared.sh"

oh_my_zsh="$HOME/.oh-my-zsh"
plugin_dir="$oh_my_zsh/custom/plugins/zsh-syntax-highlighting"

if [[ ! -d "$oh_my_zsh" ]]; then
    installer="$(mktemp)"
    trap 'rm -f "$installer"' EXIT
    curl -fsSL https://raw.githubusercontent.com/ohmyzsh/ohmyzsh/master/tools/install.sh -o "$installer"
    env RUNZSH=no CHSH=no KEEP_ZSHRC=yes sh "$installer" --unattended
fi

mkdir -p "$oh_my_zsh/custom/plugins"
if [[ ! -d "$plugin_dir" ]]; then
    git clone --depth 1 https://github.com/zsh-users/zsh-syntax-highlighting.git "$plugin_dir"
fi

read -r -d '' block <<'EOF' || true
if [[ -r "$HOME/.oh-my-zsh/custom/plugins/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh" ]]; then
    source "$HOME/.oh-my-zsh/custom/plugins/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh"
fi
ZSH_HIGHLIGHT_STYLES[command]='fg=green'
ZSH_HIGHLIGHT_STYLES[unknown-token]='fg=red,bold'
EOF
write_managed_block "$HOME/.zshrc" zsh-syntax-highlighting "$block"
chmod -R go-w "$oh_my_zsh"

