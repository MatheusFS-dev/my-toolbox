#!/usr/bin/env bash
MODULE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$MODULE_DIR/../../../lib/toolbox_output.sh"
tb_output_init "${TB_OUTPUT_TOOL:-setup-kitty}"

# Must run with sudo/root
if [ "$EUID" -ne 0 ]; then
    tb_error "Root access is required; rerun with sudo $0." >&2
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
    tb_info "Configuring $step_name..."
    # Temporarily disable set -e for the step execution to catch failures
    set +e
    eval "$step_cmd"
    local status=$?
    set -e
    if [[ $status -eq 0 ]]; then
        tb_info "Completed step: $step_name"
    else
        tb_warn "$step_name failed with status $status; skipping this optional step." >&2
    fi
}
