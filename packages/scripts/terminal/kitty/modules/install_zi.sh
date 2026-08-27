#!/usr/bin/env bash
set -e
MODULE_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$MODULE_DIR/shared.sh"

install_zi() {
    echo "Installing Zi for Zsh..."
    apt update
    apt install -y zsh git curl ca-certificates

    run_as_user sh -c "$(curl -fsSL https://raw.githubusercontent.com/z-shell/src/main/public/sh/install.sh)" -- -b main
}

run_step "Zi installation" "install_zi"
