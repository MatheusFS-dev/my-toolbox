#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/shared.sh"

target_user="${1:-}"
if [[ -z "$target_user" || "$target_user" == root ]]; then
    tb_error 'A non-root target user is required; no shell was changed.' >&2
    exit 1
fi
zsh_path="$(command -v zsh)"
chsh -s "$zsh_path" "$target_user"

