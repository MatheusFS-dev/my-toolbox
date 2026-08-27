#!/usr/bin/env bash
set -e
MODULE_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$MODULE_DIR/shared.sh"

add_word_navigation() {
    local zshrc="$HOME/.zshrc"
    local bashrc="$HOME/.bashrc"

    if [[ -f "$zshrc" ]]; then
        if ! grep -qF "bindkey '^[[1;5C' forward-word" "$zshrc"; then
            echo "Adding word navigation bindings to $zshrc..."
            cat >> "$zshrc" << 'EOF'

# Word navigation with CTRL + Arrows
bindkey '^[[1;5C' forward-word
bindkey '^[[1;5D' backward-word
# Additional common sequences for different terminals
bindkey '^[[1;3C' forward-word
bindkey '^[[1;3D' backward-word
bindkey '^[[C' forward-word
bindkey '^[[D' backward-word
EOF
        fi
    fi

    if [[ -f "$bashrc" ]]; then
        if ! grep -qF "forward-word" "$bashrc"; then
            echo "Adding word navigation bindings to $bashrc..."
            cat >> "$bashrc" << 'EOF'

# Word navigation with CTRL + Arrows
bind '"\e[1;5C": forward-word'
bind '"\e[1;5D": backward-word'
# Additional common sequences for different terminals
bind '"\e[1;3C": forward-word'
bind '"\e[1;3D": backward-word'
bind '"\e[5C": forward-word'
bind '"\e[5D": backward-word'
EOF
        fi
    fi
}

run_step "Word navigation bindings" "add_word_navigation"
