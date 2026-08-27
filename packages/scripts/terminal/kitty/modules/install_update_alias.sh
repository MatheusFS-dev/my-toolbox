#!/usr/bin/env bash
set -e
MODULE_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$MODULE_DIR/shared.sh"

install_update_alias() {
    local alias_name="update"
    local alias_command="sudo apt update && sudo apt upgrade"
    local alias_line="alias ${alias_name}='${alias_command}'"

    local targets=()
    if [[ -f "$HOME/.bashrc" ]]; then
        targets+=("$HOME/.bashrc")
    fi
    if [[ -f "$HOME/.zshrc" ]]; then
        targets+=("$HOME/.zshrc")
    fi

    for file in "${targets[@]}"; do
        if grep -qE "^[[:space:]]*alias[[:space:]]+${alias_name}=" "$file"; then
            run_as_user python3 -c '
import sys
file_path = sys.argv[1]
alias_name = sys.argv[2]
alias_line = sys.argv[3]

with open(file_path, "r") as f:
    lines = f.readlines()

replaced = False
for i, line in enumerate(lines):
    if line.strip().startswith(f"alias {alias_name}="):
        lines[i] = alias_line + "\n"
        replaced = True

if replaced:
    with open(file_path, "w") as f:
        f.writelines(lines)
' "$file" "$alias_name" "$alias_line"
            echo "Updated existing 'update' alias in $file"
        else
            {
                echo ""
                echo "# Custom update alias"
                echo "$alias_line"
            } >> "$file"
            echo "Added 'update' alias to $file"
        fi
    done
}

run_step "Update alias installation" "install_update_alias"
