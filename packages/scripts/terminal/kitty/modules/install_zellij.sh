#!/usr/bin/env bash
set -e
MODULE_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$MODULE_DIR/shared.sh"

install_zellij() {
    echo "Installing the native build toolchain required by Rust crates..."
    apt-get update
    apt-get install -y build-essential

    if [[ -f "$HOME/.cargo/env" ]]; then
        source "$HOME/.cargo/env"
    fi
    echo "Installing Zellij via cargo..."
    run_as_user env PATH="$HOME/.cargo/bin:$PATH" cargo install --locked zellij
}

run_step "Zellij installation" "install_zellij"
