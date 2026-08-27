#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/shared.sh"

read -r -d '' block <<'EOF' || true
extract() {
    if [[ ! -f "$1" ]]; then
        printf "'%s' is not a valid file\n" "$1"
        return 1
    fi
    case "$1" in
        *.tar.bz2|*.tbz2) tar xjf "$1" ;;
        *.tar.gz|*.tgz) tar xzf "$1" ;;
        *.tar) tar xf "$1" ;;
        *.bz2) bunzip2 "$1" ;;
        *.gz) gunzip "$1" ;;
        *.zip) unzip "$1" ;;
        *.7z) 7z x "$1" ;;
        *.rar) unrar x "$1" ;;
        *) printf "'%s' cannot be extracted via extract()\n" "$1"; return 1 ;;
    esac
}
EOF
write_managed_block "$HOME/.bashrc" extract "$block"
write_managed_block "$HOME/.zshrc" extract "$block"

