#!/usr/bin/env bash
set -e
MODULE_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$MODULE_DIR/shared.sh"

install_fzf_tab() {
    # 1. Install fzf if not present
    if ! command -v fzf &> /dev/null; then
        echo "Installing fzf..."
        apt-get update
        apt-get install -y fzf
    else
        echo "fzf is already installed."
    fi

    # 2. Clone fzf-tab plugin
    local oh_my_zsh_dir="$HOME/.oh-my-zsh"
    local plugin_dir="$oh_my_zsh_dir/custom/plugins/fzf-tab"

    if [[ ! -d "$oh_my_zsh_dir" ]]; then
        echo "Oh My Zsh not detected. Please install Oh My Zsh first."
        return 1
    fi

    run_as_user mkdir -p "$oh_my_zsh_dir/custom/plugins"
    if [[ ! -d "$plugin_dir" ]]; then
        echo "Cloning fzf-tab repository..."
        run_as_user git clone https://github.com/Aloxaf/fzf-tab "$plugin_dir"
    else
        echo "fzf-tab plugin repository already exists, skipping clone."
    fi

    # 3. Configure ~/.zshrc
    local zshrc="$HOME/.zshrc"
    if [[ -f "$zshrc" ]]; then
        echo "Configuring fzf-tab in $zshrc..."
        run_as_user python3 -c '
import sys
import re

zshrc_path = sys.argv[1]

with open(zshrc_path, "r") as f:
    content = f.read()

# 1. Update plugins list
plugins_match = re.search(r"^plugins=\s*\((.*?)\)", content, re.MULTILINE | re.DOTALL)
if plugins_match:
    plugins_content = plugins_match.group(1)
    plugins_list = [p.strip() for p in plugins_content.split("\n") if p.strip() and not p.strip().startswith("#")]
    
    if "fzf-tab" not in plugins_list:
        if "zsh-syntax-highlighting" in plugins_content:
            new_plugins_content = plugins_content.replace(
                "zsh-syntax-highlighting", 
                "fzf-tab\n  zsh-syntax-highlighting"
            )
        else:
            new_plugins_content = plugins_content + "\n  fzf-tab"
        
        start, end = plugins_match.span()
        content = content[:start] + f"plugins=({new_plugins_content})" + content[end:]

# 2. Add zstyle configurations for fzf-tab if not present
if "# fzf-tab configuration" not in content:
    config_block = """
# fzf-tab configuration
# Disable sort for specific commands (e.g. git checkout)
zstyle \x27:completion:*:git-checkout:*\x27 sort false
# Set descriptions format to enable group support
zstyle \x27:completion:*:descriptions\x27 format \x27[%d]\x27
# Enable colored output for file completions
zstyle \x27:completion:*\x27 list-colors ${(s.:.)LS_COLORS}
# Preview files/directories when using \x27cd\x27 (uses eza if available, fallback to ls)
if command -v eza &> /dev/null; then
    zstyle \x27:fzf-tab:complete:cd:*\x27 fzf-preview \x27eza --color=always $realpath\x27
else
    zstyle \x27:fzf-tab:complete:cd:*\x27 fzf-preview \x27ls --color=always $realpath\x27
fi
# Switch groups using < and >
zstyle \x27:fzf-tab:*\x27 switch-group \x27<\x27 \x27>\x27
"""
    if "source $ZSH/oh-my-zsh.sh" in content:
        content = content.replace("source $ZSH/oh-my-zsh.sh", "source $ZSH/oh-my-zsh.sh\n" + config_block)
    else:
        content += "\n" + config_block

# Clean up multiple empty lines
while "\n\n\n" in content:
    content = content.replace("\n\n\n", "\n\n")

with open(zshrc_path, "w") as f:
    f.write(content)
' "$zshrc"
        echo "fzf-tab configured in $zshrc successfully."
    else
        echo "Warning: $zshrc not found, skipping plugin configuration."
    fi

    # 4. Configure Amazon Q autocomplete to ignore Tab key to resolve conflicts
    echo "Checking if Amazon Q is installed..."
    if command -v q &> /dev/null; then
        echo "Amazon Q is installed. Configuring autocomplete to ignore the Tab key..."
        q settings autocomplete.keybindings.tab ignore || true
    else
        echo "Amazon Q is not installed, skipping configuration."
    fi
}

run_step "fzf-tab installation" "install_fzf_tab"
