#!/usr/bin/env bash
set -e
MODULE_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$MODULE_DIR/shared.sh"

EZA_LIST_VIEW="${1:-1}"

install_zsh_eza() {
    # 1. Install eza if not present
    if ! command -v eza &> /dev/null; then
        echo "Installing eza..."
        apt-get update
        apt-get install -y eza
    else
        echo "eza is already installed."
    fi

    # 2. Add zsh-eza to ~/.zshrc using zi
    local zshrc="$HOME/.zshrc"
    if [[ -f "$zshrc" ]]; then
        echo "Configuring zsh-eza in $zshrc..."
        run_as_user python3 -c '
import sys
path = sys.argv[1]
want_list = sys.argv[2] == "1"

with open(path, "r") as f:
    content = f.read()

# Prepare configurations
extra_params_line = "\n# Configure zsh-eza to display in a list, not side by side\neza_extra_params=\"-1\"\n" if want_list else ""
plugin_line = "\n# Load z-shell/zsh-eza plugin\nzi light z-shell/zsh-eza\n"

# Remove existing eza_extra_params configurations if present to avoid duplicates
lines = content.splitlines()
new_lines = []
for line in lines:
    if "Configure zsh-eza to display in a list" in line or "eza_extra_params=" in line:
        continue
    new_lines.append(line)
content = "\n".join(new_lines) + "\n"

# Now add plugin and optional list view configuration
if "z-shell/zsh-eza" not in content:
    addition = extra_params_line + plugin_line
    if "zicompinit" in content:
        content = content.replace("zicompinit", "zicompinit" + addition)
    else:
        content += addition
else:
    # Plugin is already there, just insert eza_extra_params before it if wanted
    if want_list:
        content = content.replace("zi light z-shell/zsh-eza", "eza_extra_params=\"-1\"\nzi light z-shell/zsh-eza")

# Clean up multiple empty lines
while "\n\n\n" in content:
    content = content.replace("\n\n\n", "\n\n")

with open(path, "w") as f:
    f.write(content)
' "$zshrc" "$EZA_LIST_VIEW"
    else
        echo "Warning: $zshrc not found, skipping plugin configuration."
    fi
}

run_step "zsh-eza installation" "install_zsh_eza"
