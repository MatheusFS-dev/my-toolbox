#!/usr/bin/env bash
set -e
MODULE_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$MODULE_DIR/shared.sh"

nautilus_integration() {
    echo "Configuring Nautilus integration with Kitty."

    local kitty_executable="$HOME/.local/kitty.app/bin/kitty"
    local extension_source="$MODULE_DIR/kitty_nautilus.py"
    local extension_target="$HOME/.local/share/nautilus-python/extensions/kitty_nautilus.py"
    local user_id
    user_id=$(id -u "$USER")
    local runtime_dir="/run/user/$user_id"
    local session_bus="$runtime_dir/bus"

    if [[ ! -x "$kitty_executable" ]]; then
        echo "Error: Kitty executable not found at $kitty_executable." >&2
        return 1
    fi

    if [[ ! -f "$extension_source" ]]; then
        echo "Error: Kitty Nautilus extension not found at $extension_source." >&2
        return 1
    fi

    if [[ ! -S "$session_bus" ]]; then
        echo "Error: User D-Bus session socket not found at $session_bus." >&2
        return 1
    fi

    apt-get install -y python3-nautilus
    run_as_user install -D -m 0644 "$extension_source" "$extension_target"

    # Nautilus returns 255 after a successful quit on Ubuntu 24.04, so handle
    # that observed behavior explicitly while preserving other errors.
    set +e
    run_as_user env \
        XDG_RUNTIME_DIR="$runtime_dir" \
        DBUS_SESSION_BUS_ADDRESS="unix:path=$session_bus" \
        nautilus -q
    local nautilus_status=$?
    set -e
    if [[ $nautilus_status -ne 0 && $nautilus_status -ne 255 ]]; then
        echo "Error: Nautilus reload failed with status $nautilus_status." >&2
        return "$nautilus_status"
    fi
}

nautilus_integration
