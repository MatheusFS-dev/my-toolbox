#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/shared.sh"

stamp="${BACKUP_STAMP:-$(date +%Y%m%d-%H%M%S)}"
backup_dir="$HOME/.local/state/project-template/wsl-backups/${stamp}-$$"
mkdir -p "$backup_dir/zellij"

[[ ! -f "$HOME/.bashrc" ]] || cp -p "$HOME/.bashrc" "$backup_dir/.bashrc"
[[ ! -f "$HOME/.zshrc" ]] || cp -p "$HOME/.zshrc" "$backup_dir/.zshrc"
[[ ! -f "$HOME/.config/zellij/config.kdl" ]] || \
    cp -p "$HOME/.config/zellij/config.kdl" "$backup_dir/zellij/config.kdl"

tb_info "Backups stored in $backup_dir"

