#!/usr/bin/env bash
set -e
MODULE_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$MODULE_DIR/shared.sh"

add_sudo_shortcut() {
    local zshrc="$HOME/.zshrc"

    local sudo_code
    read -r -d '' sudo_code << 'EOF' || true

# Sudo shortcut (Press ESC twice)
sudo-command-line() {
    [[ -z $BUFFER ]] && zle up-history
    if [[ $BUFFER != sudo\ * ]]; then
        BUFFER="sudo $BUFFER"
        CURSOR=$(( CURSOR + 5 ))
    fi
}
zle -N sudo-command-line
bindkey "\e\e" sudo-command-line
EOF

    if [[ -f "$zshrc" ]]; then
        if ! grep -qF "sudo-command-line()" "$zshrc"; then
            echo "Adding sudo shortcut to $zshrc..."
            echo "$sudo_code" >> "$zshrc"
        else
            echo "sudo shortcut already present in $zshrc, skipping."
        fi
    fi
}

run_step "Sudo shortcut binding" "add_sudo_shortcut"
