#!/usr/bin/env bash
set -e
MODULE_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$MODULE_DIR/shared.sh"

AUTO_START_ZELLIJ="${1:-n}"

configure_alacritty() {
    local alacritty_config_dir="$HOME/.config/alacritty"
    local alacritty_config_file="$alacritty_config_dir/alacritty.toml"

    mkdir -p "$alacritty_config_dir"

    if [[ "$AUTO_START_ZELLIJ" =~ ^[Yy]$ ]]; then
        echo "Alacritty will auto start Zellij."
        cat > "$alacritty_config_file" <<EOF
[shell]
program = "$HOME/.cargo/bin/zellij"
args = ["-l", "welcome"]

[window]
dimensions.columns = 90
dimensions.lines = 26

[selection]
save_to_clipboard = false

[[keyboard.bindings]]
key = "C"
mods = "Control|Shift"
action = "Copy"

[[keyboard.bindings]]
key = "V"
mods = "Control|Shift"
action = "Paste"

[[keyboard.bindings]]
key = "Enter"
mods = "Shift"
chars = "\n"
EOF
    else
        echo "Alacritty will open a shell without Zellij."
        cat > "$alacritty_config_file" <<EOF
[shell]
program = "/bin/zsh"

[window]
dimensions.columns = 90
dimensions.lines = 26

[selection]
save_to_clipboard = false

[[keyboard.bindings]]
key = "C"
mods = "Control|Shift"
action = "Copy"

[[keyboard.bindings]]
key = "V"
mods = "Control|Shift"
action = "Paste"

[[keyboard.bindings]]
key = "Enter"
mods = "Shift"
chars = "\n"
EOF
    fi

    echo "Alacritty configuration written to $alacritty_config_file"
    echo "Alacritty configured with selection.save_to_clipboard = false"
}

run_step "Alacritty configuration" "configure_alacritty"
