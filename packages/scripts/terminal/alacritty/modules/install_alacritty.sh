#!/usr/bin/env bash
set -e
MODULE_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$MODULE_DIR/shared.sh"

install_alacritty() {
    echo "Installing Alacritty. This requires sudo."
    apt install -y alacritty
}

run_step "Alacritty installation" "install_alacritty"
