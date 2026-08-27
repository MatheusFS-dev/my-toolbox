#!/usr/bin/env bash

# Must run with sudo/root
if [ "$EUID" -ne 0 ]; then
    echo "Error: This script must be run with sudo." >&2
    echo "Please run: sudo $0" >&2
    exit 1
fi

# Resolve the real invoking user's home directory if run via sudo
if [[ -n "$SUDO_USER" ]]; then
    export USER="$SUDO_USER"
    export HOME=$(getent passwd "$SUDO_USER" | cut -d: -f6)
fi

# Run a command as the invoking user
run_as_user() {
    # sudo preserves the caller's umask. Enforce safe modes for shell plugins
    # and completion files regardless of the user's configured umask.
    (
        umask 022
        if [[ -n "$SUDO_USER" ]]; then
            sudo -u "$SUDO_USER" env HOME="$HOME" USER="$USER" "$@"
        else
            "$@"
        fi
    )
}

# Repair files created in the user's home by an earlier sudo-based run.
repair_user_ownership() {
    local target="$1"

    if [[ -n "$SUDO_USER" && -e "$target" ]]; then
        chown -R "$SUDO_USER:$(id -gn "$SUDO_USER")" "$target"
    fi
}

# Helper function to run a step and catch any errors, preventing it from blocking subsequent steps
run_step() {
    local step_name="$1"
    local step_cmd="$2"
    echo "========================================="
    echo "Starting: $step_name"
    echo "========================================="
    # Temporarily disable set -e for the step execution to catch failures
    set +e
    eval "$step_cmd"
    local status=$?
    set -e
    if [[ $status -eq 0 ]]; then
        echo "Successfully completed: $step_name"
    else
        echo "Warning: '$step_name' failed with status $status, but continuing..." >&2
    fi
    echo ""
}
