#!/usr/bin/env bash
set -e

if [ "$EUID" -ne 0 ]; then
    echo "Error: This script must be run with sudo." >&2
    echo "Please run: sudo $0" >&2
    exit 1
fi

if [[ -n "$SUDO_USER" ]]; then
    export USER="$SUDO_USER"
    export HOME
    HOME=$(getent passwd "$SUDO_USER" | cut -d: -f6)
fi

run_as_user() {
    if [[ -n "$SUDO_USER" ]]; then
        sudo -u "$SUDO_USER" env HOME="$HOME" USER="$USER" "$@"
    else
        "$@"
    fi
}

main() {
    if ! command -v nautilus >/dev/null 2>&1; then
        echo "Error: Nautilus is not installed; install Nautilus before adding this integration." >&2
        return 1
    fi

    if ! run_as_user sh -c 'command -v code >/dev/null 2>&1'; then
        echo "Error: VS Code command 'code' is not available for $USER." >&2
        return 1
    fi

    local script_directory extension_source extension_target user_id runtime_dir session_bus
    script_directory=$(dirname "${BASH_SOURCE[0]}")
    extension_source="$script_directory/vscode_nautilus.py"
    extension_target="$HOME/.local/share/nautilus-python/extensions/vscode_nautilus.py"
    user_id=$(id -u "$USER")
    runtime_dir="/run/user/$user_id"
    session_bus="$runtime_dir/bus"

    if [[ ! -f "$extension_source" ]]; then
        echo "Error: VS Code Nautilus extension not found at $extension_source." >&2
        return 1
    fi
    if [[ ! -S "$session_bus" ]]; then
        echo "Error: User D-Bus session socket not found at $session_bus." >&2
        return 1
    fi

    apt-get install -y python3-nautilus
    run_as_user install -D -m 0644 "$extension_source" "$extension_target"

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

main "$@"
