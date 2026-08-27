#!/usr/bin/env bash
set -euo pipefail

stamp="${BACKUP_STAMP:-$(date +%Y%m%d-%H%M%S)}"
backup_dir="$HOME/.local/state/project-template/wsl-backups/${stamp}-$$"
mkdir -p "$backup_dir/zellij"

[[ ! -f "$HOME/.bashrc" ]] || cp -p "$HOME/.bashrc" "$backup_dir/.bashrc"
[[ ! -f "$HOME/.zshrc" ]] || cp -p "$HOME/.zshrc" "$backup_dir/.zshrc"
[[ ! -f "$HOME/.config/zellij/config.kdl" ]] || \
    cp -p "$HOME/.config/zellij/config.kdl" "$backup_dir/zellij/config.kdl"

printf 'Backups stored in %s\n' "$backup_dir"

