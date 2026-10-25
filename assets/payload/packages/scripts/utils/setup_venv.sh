#!/usr/bin/env bash
set -e
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/toolbox_output.sh"
tb_output_init setup-venv

ASSUME_YES=false
while [[ "$#" -gt 0 ]]; do
    case "$1" in
        -y|--yes)
            ASSUME_YES=true
            shift
            ;;
        *)
            tb_error "Unknown argument: $1. Usage: $0 [-y|--yes]. Nothing changed." >&2
            exit 1
            ;;
    esac
done

FILES=(
    "$HOME/.bashrc"
)

if command -v zsh >/dev/null 2>&1; then
    FILES+=("$HOME/.zshrc")
else
    tb_warn 'Skipping ~/.zshrc because zsh was not found.'
fi

# Detect if venv function is currently installed in any of the files
IS_INSTALLED=0
for FILE in "${FILES[@]}"; do
    if [ -f "$FILE" ] && grep -qE "^[[:space:]]*venv\(\)[[:space:]]*\{" "$FILE"; then
        IS_INSTALLED=1
        break
    fi
done

if [ "$IS_INSTALLED" -eq 1 ]; then
    tb_info 'Python venv auto-activator is currently installed.'
    if [[ "$ASSUME_YES" == "true" ]]; then
        tb_warn 'Skipping uninstall because the default answer is N.'
        exit 0
    fi
    while true; do
        tb_prompt 'Do you want to uninstall it? [y/N] (default: n):'
        read -r answer || { tb_info 'Input cancelled; nothing changed.'; exit 1; }
        case "${answer,,}" in
            y|yes) break ;;
            ''|n|no)
            tb_info 'Uninstall cancelled; nothing changed.'
            exit 0
            ;;
            *) tb_warn 'Enter yes or no, or press Enter for no.' >&2 ;;
        esac
    done

    tb_info 'Removing the Python venv auto-activator...'

    UNINSTALLED_FILES=()
    for FILE in "${FILES[@]}"; do
        if [ ! -e "$FILE" ]; then
            continue
        fi

        TEMPORARY_FILE="$(mktemp)"

        awk '
            /^# ---- Auto-activate nearest Python .venv ----$/ || /^# Auto-activate nearest Python .venv$/ {
                in_venv_block = 1
                next
            }

            /^# ---- End Auto-activate nearest Python .venv ----$/ {
                in_venv_block = 0
                next
            }

            in_venv_block && /^}$/ {
                in_venv_block = 0
                next
            }

            !in_venv_block {
                print
            }
        ' "$FILE" > "$TEMPORARY_FILE"

        cat "$TEMPORARY_FILE" > "$FILE"
        rm -f "$TEMPORARY_FILE"
        UNINSTALLED_FILES+=("$FILE")
        tb_info "Removed Python venv auto-activator from $FILE"
    done

    if [ "${#UNINSTALLED_FILES[@]}" -gt 0 ]; then
        tb_info 'Restart your terminal, or run the matching command:'
        for FILE in "${UNINSTALLED_FILES[@]}"; do
            tb_info "source $FILE"
        done
    fi
    tb_success 'Python venv auto-activator removed successfully.'
else
    tb_info 'Python venv auto-activator is not installed.'
    if [[ "$ASSUME_YES" == "true" ]]; then
        tb_info 'Using install default: yes.'
        answer="y"
    else
        while true; do
            tb_prompt 'Do you want to install it? [y/N] (default: n):'
            read -r answer || { tb_info 'Input cancelled; nothing changed.'; exit 1; }
            case "${answer,,}" in
                y|yes) break ;;
                ''|n|no) tb_info 'Installation cancelled; nothing changed.'; exit 0 ;;
                *) tb_warn 'Please enter yes, y, no, n, or press Enter for no.' >&2 ;;
            esac
        done
    fi

    tb_info 'Writing the Python venv auto-activator...'

    INSTALLED_FILES=()
    for FILE in "${FILES[@]}"; do
        if [ ! -e "$FILE" ]; then
            tb_warn "Skipping $FILE because it does not exist."
            continue
        fi

        TEMPORARY_FILE="$(mktemp)"

        # Remove the managed block first (precautionary)
        awk '
            /^# ---- Auto-activate nearest Python .venv ----$/ || /^# Auto-activate nearest Python .venv$/ {
                in_venv_block = 1
                next
            }

            /^# ---- End Auto-activate nearest Python .venv ----$/ {
                in_venv_block = 0
                next
            }

            in_venv_block && /^}$/ {
                in_venv_block = 0
                next
            }

            !in_venv_block {
                print
            }
        ' "$FILE" > "$TEMPORARY_FILE"

        {
            cat "$TEMPORARY_FILE"
            echo ""
            cat <<'VENV_BLOCK'
# ---- Auto-activate nearest Python .venv ----
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
# ---- End Auto-activate nearest Python .venv ----
VENV_BLOCK
        } > "$FILE"

        rm -f "$TEMPORARY_FILE"
        INSTALLED_FILES+=("$FILE")
        tb_info "Installed Python venv auto-activator in $FILE"
    done

    if [ "${#INSTALLED_FILES[@]}" -gt 0 ]; then
        tb_info 'Restart your terminal or run:'
        for FILE in "${INSTALLED_FILES[@]}"; do
            tb_info "source $FILE"
        done
    fi
    tb_success 'Python venv auto-activator installed successfully.'
fi
