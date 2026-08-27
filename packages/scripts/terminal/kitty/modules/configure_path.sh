#!/usr/bin/env bash
set -e
MODULE_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$MODULE_DIR/shared.sh"

configure_path() {
    if [[ ":$PATH:" != *":$HOME/.cargo/bin:"* ]]; then
        echo "Adding ~/.cargo/bin to PATH permanently"
        echo 'export PATH="$HOME/.cargo/bin:$PATH"' >> "$HOME/.bashrc"
        echo 'export PATH="$HOME/.cargo/bin:$PATH"' >> "$HOME/.zshrc" 2>/dev/null || true
    fi
}

run_step "PATH configuration" "configure_path"
