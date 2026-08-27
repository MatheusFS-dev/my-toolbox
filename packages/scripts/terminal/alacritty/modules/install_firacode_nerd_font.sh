#!/usr/bin/env bash
set -e
MODULE_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$MODULE_DIR/shared.sh"

install_firacode_nerd_font() {
    # 1. Install prerequisites
    echo "Installing font prerequisites (wget, unzip, fontconfig)..."
    apt-get update
    apt-get install -y wget unzip fontconfig

    # 2. Download and install FiraCode Nerd Font
    local font_dir="$HOME/.local/share/fonts/FiraCode"
    if [[ ! -d "$font_dir" ]]; then
        echo "Downloading and installing FiraCode Nerd Font..."
        run_as_user mkdir -p "$font_dir"
        local temp_zip="/tmp/FiraCode.zip"
        run_as_user wget -O "$temp_zip" https://github.com/ryanoasis/nerd-fonts/releases/latest/download/FiraCode.zip
        run_as_user unzip -o "$temp_zip" -d "$font_dir"
        rm -f "$temp_zip"
        echo "Updating font cache..."
        run_as_user fc-cache -fv
    else
        echo "FiraCode Nerd Font is already downloaded at $font_dir."
    fi

    # 3. Configure Alacritty to use FiraCode Nerd Font
    local alacritty_config="$HOME/.config/alacritty/alacritty.toml"
    if [[ -f "$alacritty_config" ]]; then
        if ! grep -qF "FiraCode Nerd Font" "$alacritty_config"; then
            echo "Configuring Alacritty to use FiraCode Nerd Font..."
            cat >> "$alacritty_config" << 'EOF'

[font]
size = 11.0

[font.normal]
family = "FiraCode Nerd Font"
style = "Regular"

[font.bold]
family = "FiraCode Nerd Font"
style = "Bold"

[font.italic]
family = "FiraCode Nerd Font"
style = "Italic"

[font.bold_italic]
family = "FiraCode Nerd Font"
style = "Bold Italic"
EOF
        else
            echo "Alacritty is already configured to use FiraCode Nerd Font."
        fi
    fi
}

run_step "FiraCode Nerd Font installation" "install_firacode_nerd_font"
