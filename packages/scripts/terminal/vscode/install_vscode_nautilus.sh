#!/usr/bin/env bash
set -e
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../../lib/toolbox_output.sh"
tb_output_init install-vscode-nautilus

if [ "$EUID" -ne 0 ]; then
    tb_error "Root access is required; rerun with sudo $0." >&2
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
    tb_info 'Checking Nautilus, VS Code, and the desktop session...'
    if ! command -v nautilus >/dev/null 2>&1; then
        tb_error 'Nautilus is not installed; install it before retrying. Nothing changed.' >&2
        return 1
    fi

    if ! run_as_user sh -c 'command -v code >/dev/null 2>&1'; then
        tb_error "VS Code command 'code' is unavailable for $USER; install its shell command and retry. Nothing changed." >&2
        return 1
    fi

    local script_directory extension_source extension_target user_id runtime_dir session_bus
    script_directory=$SCRIPT_DIR
    extension_source="$script_directory/vscode_nautilus.py"
    extension_target="$HOME/.local/share/nautilus-python/extensions/vscode_nautilus.py"
    user_id=$(id -u "$USER")
    runtime_dir="/run/user/$user_id"
    session_bus="$runtime_dir/bus"

    if [[ ! -f "$extension_source" ]]; then
        tb_error "VS Code Nautilus extension is missing at $extension_source; reinstall toolbox and retry. Nothing changed." >&2
        return 1
    fi
    if [[ ! -S "$session_bus" ]]; then
        tb_error "User D-Bus session socket is missing at $session_bus; sign in to the desktop session and retry. Nothing changed." >&2
        return 1
    fi

    tb_info 'Installing the Nautilus Python integration dependency...'
    apt-get install -y python3-nautilus
    tb_info 'Writing the VS Code Nautilus extension...'
    run_as_user install -D -m 0644 "$extension_source" "$extension_target"

    set +e
    run_as_user env \
        XDG_RUNTIME_DIR="$runtime_dir" \
        DBUS_SESSION_BUS_ADDRESS="unix:path=$session_bus" \
        nautilus -q
    local nautilus_status=$?
    set -e
    if [[ $nautilus_status -ne 0 && $nautilus_status -ne 255 ]]; then
        tb_error "Nautilus reload failed with status $nautilus_status; the extension remains installed, so restart Nautilus manually." >&2
        return "$nautilus_status"
    fi
    tb_success 'VS Code Nautilus integration installed successfully.'
}

main "$@"
