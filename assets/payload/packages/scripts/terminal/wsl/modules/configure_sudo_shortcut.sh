#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/shared.sh"

read -r -d '' block <<'EOF' || true
sudo-command-line() {
    [[ -z $BUFFER ]] && zle up-history
    if [[ $BUFFER != sudo\ * ]]; then
        BUFFER="sudo $BUFFER"
        CURSOR=$((CURSOR + 5))
    fi
}
zle -N sudo-command-line
bindkey "\e\e" sudo-command-line
EOF
write_managed_block "$HOME/.zshrc" sudo-shortcut "$block"

