#!/usr/bin/env bash
set -e
MODULE_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$MODULE_DIR/shared.sh"

AUTO_START_ZELLIJ="${1:-n}"

configure_kitty() {
    local kitty_config_dir="$HOME/.config/kitty"
    local kitty_config_file="$kitty_config_dir/kitty.conf"
    local shell_command="/bin/zsh"

    mkdir -p "$kitty_config_dir"

    if [[ -f "$kitty_config_file" ]]; then
        local backup_file="${kitty_config_file}.backup.$(date +%Y%m%d-%H%M%S)"
        cp "$kitty_config_file" "$backup_file"
        echo "Existing Kitty configuration backed up to $backup_file"
    fi

    if [[ "$AUTO_START_ZELLIJ" =~ ^[Yy]$ ]]; then
        echo "Kitty will auto-start Zellij."
        echo "Warning: run Codex directly in Kitty, outside Zellij, when terminal pets are required."
        shell_command="$HOME/.cargo/bin/zellij -l welcome"
    else
        echo "Kitty will open Zsh directly."
    fi

    cat > "$kitty_config_file" <<EOF_CONFIG
# Kitty configuration translated from the supplied Alacritty setup.
# It reproduces the Alacritty defaults used by that setup as closely as possible.

# Shell
shell $shell_command

# Initial terminal size, equivalent to Alacritty's 90 columns x 26 lines.
remember_window_size no
initial_window_width 90c
initial_window_height 26c

# Font
font_family FiraCode Nerd Font
bold_font auto
italic_font auto
bold_italic_font auto
font_size 11.0

# Window appearance
background_opacity 1.0
window_padding_width 0
window_margin_width 0
hide_window_decorations no
placement_strategy center

# Cursor. Disable Kitty's shell-driven beam cursor to retain Alacritty's block cursor.
shell_integration no-cursor
cursor_shape block
cursor_blink_interval 0
cursor none

# Selection and clipboard behavior
copy_on_select no
selection_foreground none
selection_background none
map ctrl+shift+c copy_to_clipboard
map ctrl+shift+v paste_from_clipboard
map shift+enter send_text all \x0a

# Alacritty default primary colors
foreground #d8d8d8
background #181818

# Alacritty default normal colors
color0 #181818
color1 #ac4242
color2 #90a959
color3 #f4bf75
color4 #6a9fb5
color5 #aa759f
color6 #75b5aa
color7 #d8d8d8

# Alacritty default bright colors
color8 #6b6b6b
color9 #c55555
color10 #aac474
color11 #feca88
color12 #82b8c8
color13 #c28cb8
color14 #93d3c3
color15 #f8f8f8

# Keep URLs and common terminal features enabled.
detect_urls yes
allow_hyperlinks yes
EOF_CONFIG

    echo "Kitty configuration written to $kitty_config_file"
}

run_step "Kitty configuration" "configure_kitty"
