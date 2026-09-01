#!/usr/bin/env bash
set -e
MODULE_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$MODULE_DIR/shared.sh"

nautilus_integration() {
    if ! command -v nautilus >/dev/null 2>&1; then
        echo "Warning: Nautilus is not installed; skipping integration." >&2
        return 0
    fi

    echo "Configuring Nautilus integration with Alacritty."

    apt install -y python3-nautilus gir1.2-gtk-4.0 python3-pip
    run_as_user python3 -m pip install --user --break-system-packages nautilus-open-any-terminal

    run_as_user glib-compile-schemas "$HOME/.local/share/glib-2.0/schemas/" 2>/dev/null || true

    run_as_user gsettings set com.github.stunkymonkey.nautilus-open-any-terminal terminal alacritty

    run_as_user nautilus -q || true
}

run_step "Nautilus integration" "nautilus_integration"
