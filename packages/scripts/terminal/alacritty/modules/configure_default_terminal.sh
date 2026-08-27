#!/usr/bin/env bash
set -e
MODULE_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$MODULE_DIR/shared.sh"

configure_default_terminal() {
    echo "Configuring Alacritty as the system default terminal."
    echo "Select the number corresponding to Alacritty."

    update-alternatives --config x-terminal-emulator
}

run_step "System default terminal configuration" "configure_default_terminal"
