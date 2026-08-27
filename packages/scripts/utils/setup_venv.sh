#!/usr/bin/env bash
set -e

ASSUME_YES=false
while [[ "$#" -gt 0 ]]; do
    case "$1" in
        -y|--yes)
            ASSUME_YES=true
            shift
            ;;
        *)
            echo "Unknown argument: $1" >&2
            echo "Usage: $0 [-y|--yes]" >&2
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
    echo "Skipping ~/.zshrc because zsh was not found."
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
    echo "Python venv auto-activator is currently installed."
    if [[ "$ASSUME_YES" == "true" ]]; then
        echo "Auto-skipping uninstall (default: No)."
        exit 0
    fi
    read -r -p "Do you want to uninstall it? [y/N]: " answer
    case "$answer" in
        y|Y|yes|YES) ;;
        *)
            echo "Cancelled."
            exit 0
            ;;
    esac

    echo ""
    echo "Uninstalling..."
    echo ""

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
        echo "Removed Python venv auto-activator from $FILE"
    done

    echo ""
    echo "Done."
    if [ "${#UNINSTALLED_FILES[@]}" -gt 0 ]; then
        echo "Restart your terminal, or run the matching command:"
        for FILE in "${UNINSTALLED_FILES[@]}"; do
            echo "  source $FILE"
        done
    fi
else
    echo "Python venv auto-activator is not installed."
    if [[ "$ASSUME_YES" == "true" ]]; then
        echo "Auto-confirming install (default: Yes)."
        answer="y"
    else
        read -r -p "Do you want to install it? [y/N]: " answer
    fi
    case "$answer" in
        y|Y|yes|YES) ;;
        *)
            echo "Cancelled."
            exit 0
            ;;
    esac

    echo ""
    echo "Installing..."
    echo ""

    INSTALLED_FILES=()
    for FILE in "${FILES[@]}"; do
        if [ ! -e "$FILE" ]; then
            echo "Skipping $FILE because it does not exist."
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
        echo "Installed Python venv auto-activator in $FILE"
    done

    echo ""
    echo "Done."
    if [ "${#INSTALLED_FILES[@]}" -gt 0 ]; then
        echo "Restart your terminal or run:"
        for FILE in "${INSTALLED_FILES[@]}"; do
            echo "  source $FILE"
        done
    fi
fi
