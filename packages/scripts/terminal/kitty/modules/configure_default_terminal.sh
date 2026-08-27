#!/usr/bin/env bash
set -e
MODULE_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$MODULE_DIR/shared.sh"

configure_default_terminal() {
    local kitty_bin="$HOME/.local/kitty.app/bin/kitty"

    if [[ ! -x "$kitty_bin" ]]; then
        echo "Error: Kitty executable not found at $kitty_bin" >&2
        return 1
    fi

    echo "Configuring Kitty as the system default terminal."
    update-alternatives --install /usr/bin/x-terminal-emulator x-terminal-emulator "$kitty_bin" 50
    update-alternatives --set x-terminal-emulator "$kitty_bin"
}

run_step "System default terminal configuration" "configure_default_terminal"
