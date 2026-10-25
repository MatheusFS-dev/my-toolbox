#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/shared.sh"

mkdir -p "$HOME/.local/bin"
curl -fsSL https://starship.rs/install.sh | sh -s -- -y -b "$HOME/.local/bin"
write_managed_block "$HOME/.bashrc" starship 'if [[ -x "$HOME/.local/bin/starship" ]]; then
    eval "$("$HOME/.local/bin/starship" init bash)"
fi'
write_managed_block "$HOME/.zshrc" starship 'if [[ -x "$HOME/.local/bin/starship" ]]; then
    eval "$("$HOME/.local/bin/starship" init zsh)"
fi'
