#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/shared.sh"

read -r -d '' block <<'EOF' || true
venv() {
    local dir="$PWD" activate_path=''
    while [[ "$dir" != / ]]; do
        if [[ -f "$dir/.venv/bin/activate" ]]; then
            activate_path="$dir/.venv/bin/activate"
            break
        fi
        dir="$(dirname "$dir")"
    done
    if [[ -z "$activate_path" ]]; then
        echo 'venv: no .venv/bin/activate found in this directory or its parents'
        return 1
    fi
    if [[ -n "${VIRTUAL_ENV:-}" && "$VIRTUAL_ENV" == "$(dirname "$(dirname "$activate_path")")" ]]; then
        echo "venv: already active: $VIRTUAL_ENV"
        return 0
    fi
    source "$activate_path"
    echo "venv: activated $VIRTUAL_ENV"
}
EOF
write_managed_block "$HOME/.bashrc" venv "$block"
write_managed_block "$HOME/.zshrc" venv "$block"

