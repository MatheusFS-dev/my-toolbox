#!/usr/bin/env bash
# Toggles passwordless sudo (NOPASSWD) for a user through /etc/sudoers.d.
# The script detects the user's current state, reports it, and offers the
# opposite action, so the same script enables and disables the permission.
# The rule is managed in /etc/sudoers.d/99-<user>-nopasswd and always
# validated with `visudo -cf` before installation.
#
# Usage: sudo bash toggle_nopasswd_sudo.sh [user] [-y] [-h]
#   [user]      target user to change (default: sudo invoker, $SUDO_USER)
#   -y, --yes   apply the change without asking for confirmation
#   -h, --help  show this help

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
log_ok()    { echo -e "${GREEN}[ OK ]${NC} $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }

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
        read -rp "Target user: " TARGET_USER
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

SUDOERS_FILE="/etc/sudoers.d/99-${TARGET_USER}-nopasswd"

# ---------------------------------------------------------------------------
# State detection: does the user currently have passwordless sudo?
# The source of truth is what sudo actually grants, covering our file, group
# rules, and any other rule, not just whether our file exists.
# ---------------------------------------------------------------------------
user_has_nopasswd() {
    sudo -l -U "$TARGET_USER" 2>/dev/null | grep -q 'NOPASSWD'
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

    install -m 0440 -o root -g root "$tmp" "$SUDOERS_FILE"
    rm -f "$tmp"
    log_info "Rule installed at: ${SUDOERS_FILE}"
}

# Disable: remove only our managed file.
disable_nopasswd() {
    if [[ -f "$SUDOERS_FILE" ]]; then
        rm -f "$SUDOERS_FILE"
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
        while IFS= read -r f; do echo "         $f"; done <<<"$hits"
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

echo
log_info "Target user ......: ${TARGET_USER}"
log_info "Current state ....: sudo ${CURRENT}"
log_info "Offered action ...: ${ACTION_DESC}"
echo

# ---------------------------------------------------------------------------
# Confirmation
# ---------------------------------------------------------------------------
if [[ "$ASSUME_YES" -ne 1 ]]; then
    read -rp "Do you want to ${ACTION_DESC} for '${TARGET_USER}'? [y/N]: " confirm
    case "${confirm,,}" in
        y|yes) : ;;
        *) log_info "Operation canceled. No changes made."; exit 0 ;;
    esac
fi

# ---------------------------------------------------------------------------
# Application + result verification
# ---------------------------------------------------------------------------
if [[ "$ACTION" == "enable" ]]; then
    enable_nopasswd || { log_error "Failed to enable NOPASSWD."; exit 1; }
    if user_has_nopasswd; then
        log_ok "Done: '${TARGET_USER}' now uses passwordless sudo."
    else
        log_error "The rule was installed, but sudo still requires a password."
        log_error "Check for conflicts in /etc/sudoers and /etc/sudoers.d."
        exit 1
    fi
else
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
