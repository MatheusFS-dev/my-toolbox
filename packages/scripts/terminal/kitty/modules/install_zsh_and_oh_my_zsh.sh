#!/usr/bin/env bash
set -e
MODULE_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$MODULE_DIR/shared.sh"

install_zsh_and_oh_my_zsh() {
    # Installs Zsh prerequisites, Oh My Zsh, and the requested plugin.
    local oh_my_zsh_dir="$HOME/.oh-my-zsh"
    local plugin_dir="$oh_my_zsh_dir/custom/plugins/zsh-syntax-highlighting"

    echo "Installing Zsh prerequisites. This requires sudo."
    apt update
    apt install -y git-core zsh curl

    if [[ ! -d "$oh_my_zsh_dir" ]]; then
        echo "Installing Oh My Zsh..."
        run_as_user env RUNZSH=no CHSH=no sh -c "$(curl -fsSL https://raw.github.com/robbyrussell/oh-my-zsh/master/tools/install.sh)"
    else
        echo "Oh My Zsh already installed, skipping."
    fi

    run_as_user mkdir -p "$oh_my_zsh_dir/custom/plugins"
    if [[ ! -d "$plugin_dir" ]]; then
        run_as_user git clone https://github.com/zsh-users/zsh-syntax-highlighting "$plugin_dir"
    else
        echo "zsh-syntax-highlighting already exists, skipping clone"
    fi

    local zshrc="$HOME/.zshrc"
    if [[ -f "$zshrc" ]]; then
        local configured_zshrc="$zshrc.tmp"
        run_as_user awk '
            BEGIN { in_plugins = 0; found_plugins = 0 }
            /^[[:space:]]*plugins[[:space:]]*=/ {
                in_plugins = 1
                found_plugins = 1
            }
            in_plugins {
                gsub(/zsh-syntax-highlighting[[:space:]]*/, "")
                if ($0 ~ /\)/) {
                    sub(/\)/, " zsh-syntax-highlighting)")
                    in_plugins = 0
                }
            }
            !found_plugins && /^source \$ZSH\/oh-my-zsh\.sh$/ {
                print "plugins=(zsh-syntax-highlighting)"
                found_plugins = 1
            }
            { print }
        ' "$zshrc" | run_as_user tee "$configured_zshrc" >/dev/null
        run_as_user mv "$configured_zshrc" "$zshrc"

        if ! grep -Fq '# zsh-syntax-highlighting colors' "$zshrc"; then
            run_as_user tee -a "$zshrc" >/dev/null <<'EOF'

# zsh-syntax-highlighting colors
ZSH_HIGHLIGHT_STYLES[command]='fg=green'
ZSH_HIGHLIGHT_STYLES[unknown-token]='fg=red,bold'
EOF
        fi
    else
        echo "Warning: $zshrc not found, skipping plugin configuration."
    fi

    # compinit refuses completion trees containing group/other-writable paths.
    # Repair existing installations too, including plugins from earlier runs.
    run_as_user chmod -R go-w "$oh_my_zsh_dir"
}

run_step "Zsh and Oh My Zsh installation" "install_zsh_and_oh_my_zsh"
