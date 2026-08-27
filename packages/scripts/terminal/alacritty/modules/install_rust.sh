#!/usr/bin/env bash
set -e
MODULE_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$MODULE_DIR/shared.sh"

install_rust() {
    if ! run_as_user command -v cargo &> /dev/null; then
        echo "Rust/cargo not found. Installing via rustup..."
        run_as_user sh -c "curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y"
        if [[ -f "$HOME/.cargo/env" ]]; then
            source "$HOME/.cargo/env"
        fi
    else
        echo "cargo already installed"
    fi
}

run_step "Rust/cargo installation" "install_rust"
