#!/usr/bin/env bash
set -e
MODULE_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$MODULE_DIR/shared.sh"

install_starship() {
    echo "Installing Starship..."
    curl -sS https://starship.rs/install.sh | sh -s -- -y

    add_starship_to_shell() {
        local config_file="$1"
        local init_line="$2"

        if [[ -f "$config_file" ]]; then
            if ! grep -qF "starship init" "$config_file"; then
                echo "Adding Starship to $config_file"
                echo "" >> "$config_file"
                echo "# Starship prompt" >> "$config_file"
                echo "$init_line" >> "$config_file"
            else
                echo "Starship already present in $config_file, skipping."
            fi
        else
            echo "$config_file not found, skipping."
        fi
    }

    add_starship_to_shell "$HOME/.bashrc" 'eval "$(starship init bash)"'
    add_starship_to_shell "$HOME/.zshrc" 'eval "$(starship init zsh)"'
}

run_step "Starship installation" "install_starship"
