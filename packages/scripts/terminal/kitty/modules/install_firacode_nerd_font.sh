#!/usr/bin/env bash
set -e
MODULE_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$MODULE_DIR/shared.sh"

install_firacode_nerd_font() {
    echo "Installing font prerequisites (wget, unzip, fontconfig)..."
    apt-get update
    apt-get install -y wget unzip fontconfig

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

    local kitty_config="$HOME/.config/kitty/kitty.conf"
    if [[ -f "$kitty_config" ]]; then
        if ! grep -qF "font_family FiraCode Nerd Font" "$kitty_config"; then
            echo "Configuring Kitty to use FiraCode Nerd Font..."
            cat >> "$kitty_config" <<'EOF_CONFIG'

# FiraCode Nerd Font
font_family FiraCode Nerd Font
bold_font auto
italic_font auto
bold_italic_font auto
font_size 11.0
EOF_CONFIG
        else
            echo "Kitty is already configured to use FiraCode Nerd Font."
        fi
    fi
}

run_step "FiraCode Nerd Font installation" "install_firacode_nerd_font"
