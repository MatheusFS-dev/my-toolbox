#!/usr/bin/env bash
set -e
MODULE_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$MODULE_DIR/shared.sh"

FUNCTION_BLOCK='
venv() {
    local dir="$PWD"
    local activate_path=""

    while [ "$dir" != "/" ]; do
        if [ -f "$dir/.venv/bin/activate" ]; then
            activate_path="$dir/.venv/bin/activate"
            break
        fi

        dir="$(dirname "$dir")"
    done

    if [ -z "$activate_path" ]; then
        echo "venv: no .venv/bin/activate found in this directory or its parents"
        return 1
    fi

    if [ -n "$VIRTUAL_ENV" ] && [ "$VIRTUAL_ENV" = "$(dirname "$(dirname "$activate_path")")" ]; then
        echo "venv: already active: $VIRTUAL_ENV"
        return 0
    fi

    source "$activate_path"
    echo "venv: activated $VIRTUAL_ENV"
}
'

install_to_file() {
    local file="$1"
    touch "$file"

    if grep -qE "^[[:space:]]*venv\(\)[[:space:]]*\{" "$file"; then
        echo "Already installed in $file"
        return 0
    fi

    {
        echo
        echo "# ---- Auto-activate nearest Python .venv ----"
        printf "%s\n" "$FUNCTION_BLOCK"
        echo "# ---- End Auto-activate nearest Python .venv ----"
    } >> "$file"

    echo "Installed in $file"
}

install_venv() {
    install_to_file "$HOME/.bashrc"
    if command -v zsh >/dev/null 2>&1 && [ -f "$HOME/.zshrc" ]; then
        install_to_file "$HOME/.zshrc"
    fi
}

run_step "Python venv auto-activator utility installation" "install_venv"
