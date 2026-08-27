#!/usr/bin/env bash
set -euo pipefail

target_user="${1:-}"
if [[ -z "$target_user" || "$target_user" == root ]]; then
    printf 'A non-root target user is required.\n' >&2
    exit 1
fi
zsh_path="$(command -v zsh)"
chsh -s "$zsh_path" "$target_user"

