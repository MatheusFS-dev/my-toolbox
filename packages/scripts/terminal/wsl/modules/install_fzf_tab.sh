#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/shared.sh"

plugin_dir="$HOME/.oh-my-zsh/custom/plugins/fzf-tab"
if [[ ! -d "$HOME/.oh-my-zsh" ]]; then
    printf 'Oh My Zsh is required for fzf-tab.\n' >&2
    exit 1
fi
mkdir -p "$(dirname "$plugin_dir")"
if [[ ! -d "$plugin_dir" ]]; then
    git clone --depth 1 https://github.com/Aloxaf/fzf-tab.git "$plugin_dir"
fi

read -r -d '' block <<'EOF' || true
source "$HOME/.oh-my-zsh/custom/plugins/fzf-tab/fzf-tab.plugin.zsh"
zstyle ':completion:*:descriptions' format '[%d]'
zstyle ':completion:*' list-colors ${(s.:.)LS_COLORS}
zstyle ':fzf-tab:*' switch-group '<' '>'
if command -v eza >/dev/null 2>&1; then
    zstyle ':fzf-tab:complete:cd:*' fzf-preview 'eza --color=always $realpath'
else
    zstyle ':fzf-tab:complete:cd:*' fzf-preview 'ls --color=always $realpath'
fi
EOF
write_managed_block "$HOME/.zshrc" fzf-tab "$block"

