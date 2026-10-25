#!/usr/bin/env bash
# Toggles passwordless sudo (NOPASSWD) for a user through /etc/sudoers.d.
# The script detects the user's current state, reports it, and offers the
# opposite action, so the same script enables and disables the permission.
# The rule is managed in /etc/sudoers.d/zz-tb-uid-<uid>-nopasswd and always
# validated with `visudo -cf` before installation. If later sudoers rules
# override it, a toolbox-owned @include is added at the end of /etc/sudoers.
#
# Usage: sudo bash toggle_nopasswd_sudo.sh [user] [-y] [-h]
#   [user]      target user to change (default: sudo invoker, $SUDO_USER)
#   -y, --yes   apply the change without asking for confirmation
#   -h, --help  show this help

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/../lib/toolbox_output.sh"
tb_output_init toggle-nopasswd-sudo

log_info()  { tb_info "$*"; }
log_ok()    { tb_success "$*"; }
log_warn()  { tb_warn "$*"; }
log_error() { tb_error "$*" >&2; }

usage() {
    cat <<EOF
Toggles passwordless sudo (NOPASSWD) for a user. Detects the current state and
offers the opposite action, so the same script enables and disables the
permission.

Usage: sudo bash $0 [user] [-y] [-h]
  [user]      target user to change (default: sudo invoker, \$SUDO_USER)
  -y, --yes   apply the change without asking for confirmation
  -h, --help  show this help
EOF
}

# ---------------------------------------------------------------------------
# Arguments
# ---------------------------------------------------------------------------
ASSUME_YES=0
TARGET_USER=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        -y|--yes)  ASSUME_YES=1 ;;
        -h|--help) usage; exit 0 ;;
        -*)        log_error "Unknown option: $1"; usage; exit 1 ;;
        *)         TARGET_USER="$1" ;;
    esac
    shift
done

# ---------------------------------------------------------------------------
# Preconditions: root + valid target user
# ---------------------------------------------------------------------------
if [[ "$(id -u)" -ne 0 ]]; then
    log_error "This script must be run as root."
    log_info  "Try: sudo bash $0"
    exit 1
fi

# Without an argument, the target is the sudo invoker (SUDO_USER). Only when the
# script runs directly as root without sudo do we ask for the target user,
# because there is no non-root invoker to assume.
if [[ -z "$TARGET_USER" ]]; then
    if [[ -n "${SUDO_USER:-}" && "$SUDO_USER" != "root" ]]; then
        TARGET_USER="$SUDO_USER"
    else
        while true; do
            tb_prompt 'Target user'
            read -r TARGET_USER || { log_info 'Input cancelled; nothing changed.'; exit 1; }
            if [[ -z "$TARGET_USER" ]]; then
                log_warn "Target user cannot be empty."
            elif ! id "$TARGET_USER" >/dev/null 2>&1; then
                log_warn "User '${TARGET_USER}' does not exist."
            else
                break
            fi
        done
    fi
fi

if [[ -z "$TARGET_USER" ]]; then
    log_error "Target user cannot be empty."
    exit 1
fi
if ! id "$TARGET_USER" >/dev/null 2>&1; then
    log_error "User '${TARGET_USER}' does not exist."
    exit 1
fi

TARGET_UID="$(id -u "$TARGET_USER")"
SUDOERS_FILE="/etc/sudoers.d/zz-tb-uid-${TARGET_UID}-nopasswd"
PREVIOUS_SUDOERS_FILE="/etc/sudoers.d/99-tb-uid-${TARGET_UID}-nopasswd"
LEGACY_SUDOERS_FILE="/etc/sudoers.d/99-${TARGET_USER}-nopasswd"
SUDOERS_MAIN="/etc/sudoers"
MANAGED_INCLUDE="@include ${SUDOERS_FILE} # tb-nopasswd-uid-${TARGET_UID}"

# ---------------------------------------------------------------------------
# State detection: does the user currently have passwordless sudo?
# Run a command as the target user without a password, since a NOPASSWD entry
# in `sudo -l` may be overridden by a later matching PASSWD rule.
# ---------------------------------------------------------------------------
user_has_nopasswd() {
    sudo -u "$TARGET_USER" -- sudo -n id -u >/dev/null 2>&1
}

# Enable: install the NOPASSWD rule in the managed file after validation.
enable_nopasswd() {
    local tmp
    tmp="$(mktemp)"
    echo "${TARGET_USER} ALL=(ALL) NOPASSWD:ALL" > "$tmp"
    chmod 440 "$tmp"

    if ! visudo -cf "$tmp" >/dev/null; then
        rm -f "$tmp"
        log_error "visudo validation failed; nothing was changed."
        return 1
    fi

    install -m 0440 -o root -g root "$tmp" "$SUDOERS_FILE" || { rm -f "$tmp"; return 1; }
    rm -f "$tmp" || return 1
    rm -f "$PREVIOUS_SUDOERS_FILE" "$LEGACY_SUDOERS_FILE" || return 1
    log_info "Rule installed at: ${SUDOERS_FILE}"
}

# A vendor sudoers file may place password-requiring rules after @includedir.
# In that case include our validated rule once more at the end of the file.
update_main_include() {
    local operation="$1" tmp
    tmp="$(mktemp /etc/sudoers.tb.XXXXXXXX)" || return 1
    cp "$SUDOERS_MAIN" "$tmp" || { rm -f "$tmp"; return 1; }
    if [[ "$operation" == add ]]; then
        if ! grep -Fxq "$MANAGED_INCLUDE" "$tmp"; then
            printf '\n%s\n' "$MANAGED_INCLUDE" >> "$tmp"
        fi
    else
        sed -i "\|^${MANAGED_INCLUDE}$|d" "$tmp" || { rm -f "$tmp"; return 1; }
    fi
    chmod 0440 "$tmp" || { rm -f "$tmp"; return 1; }
    if ! visudo -cf "$tmp" >/dev/null; then
        rm -f "$tmp"
        log_error "The updated sudoers file failed validation; /etc/sudoers was not changed."
        return 1
    fi
    mv -f "$tmp" "$SUDOERS_MAIN"
}

# Disable: remove only our managed file.
disable_nopasswd() {
    if grep -Fxq "$MANAGED_INCLUDE" "$SUDOERS_MAIN"; then
        update_main_include remove || return 1
        log_info "Removed the toolbox include from ${SUDOERS_MAIN}."
    fi
    rm -f "$PREVIOUS_SUDOERS_FILE" "$LEGACY_SUDOERS_FILE" || return 1
    if [[ -f "$SUDOERS_FILE" ]]; then
        rm -f "$SUDOERS_FILE" || return 1
        log_info "File removed: ${SUDOERS_FILE}"
    else
        log_warn "Our managed file does not exist (${SUDOERS_FILE});"
        log_warn "NOPASSWD comes from another sudoers rule."
    fi
}

# List where else NOPASSWD may be defined for diagnostics.
report_external_nopasswd() {
    local hits
    hits="$(grep -rIlE "NOPASSWD" /etc/sudoers /etc/sudoers.d 2>/dev/null \
            | grep -v "^${SUDOERS_FILE}$" || true)"
    if [[ -n "$hits" ]]; then
        log_warn "NOPASSWD rules also appear in:"
        while IFS= read -r f; do tb_warn "$f"; done <<<"$hits"
    fi
    log_warn "It may also be a group rule, for example %sudo or %admin."
}

# ---------------------------------------------------------------------------
# Current state -> opposite action
# ---------------------------------------------------------------------------
if user_has_nopasswd; then
    CURRENT="passwordless"
    ACTION="disable"
    ACTION_DESC="require the password again"
else
    CURRENT="password required"
    ACTION="enable"
    ACTION_DESC="enable passwordless sudo"
fi

log_info "Target user ......: ${TARGET_USER}"
log_info "Current state ....: sudo ${CURRENT}"
log_info "Offered action ...: ${ACTION_DESC}"

# ---------------------------------------------------------------------------
# Confirmation
# ---------------------------------------------------------------------------
if [[ "$ASSUME_YES" -ne 1 ]]; then
    while true; do
        tb_prompt "Do you want to ${ACTION_DESC} for '${TARGET_USER}'? [y/N] (default: n):"
        read -r confirm || { log_info 'Input cancelled; nothing changed.'; exit 1; }
        case "${confirm,,}" in
            y|yes) break ;;
            ''|n|no) log_info "Operation canceled. No changes made."; exit 0 ;;
            *) log_warn "Enter yes, y, no, n, or press Enter for no." ;;
        esac
    done
fi

# ---------------------------------------------------------------------------
# Application + result verification
# ---------------------------------------------------------------------------
if [[ "$ACTION" == "enable" ]]; then
    tb_info "Writing the managed sudoers rule for ${TARGET_USER}..."
    enable_nopasswd || { log_error "Failed to enable NOPASSWD."; exit 1; }
    if user_has_nopasswd; then
        log_ok "Done: '${TARGET_USER}' now uses passwordless sudo."
    else
        tb_info "The managed rule did not take effect; adding a final toolbox include..."
        had_main_include=0
        if grep -Fxq "$MANAGED_INCLUDE" "$SUDOERS_MAIN"; then
            had_main_include=1
        fi
        if ! update_main_include add; then
            log_error "Could not add the final toolbox include to /etc/sudoers."
            exit 1
        fi
        if ! user_has_nopasswd; then
            if [[ "$had_main_include" -eq 0 ]]; then
                update_main_include remove || log_error "Could not roll back the final toolbox include."
            fi
            log_error "Sudo still requires a password after adding the final include."
            log_error "Check for conflicts in /etc/sudoers and /etc/sudoers.d."
            exit 1
        fi
        log_ok "Done: '${TARGET_USER}' now uses passwordless sudo."
    fi
else
    tb_info "Removing the managed sudoers rule for ${TARGET_USER}..."
    disable_nopasswd
    if ! user_has_nopasswd; then
        log_ok "Done: '${TARGET_USER}' now needs a password for sudo."
    else
        log_error "Passwordless sudo remains after removing our file;"
        log_error "another NOPASSWD rule exists outside this script's control."
        report_external_nopasswd
        exit 1
    fi
fi
