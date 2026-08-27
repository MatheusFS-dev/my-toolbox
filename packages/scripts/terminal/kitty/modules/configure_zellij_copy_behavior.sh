#!/usr/bin/env bash
set -e
MODULE_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$MODULE_DIR/shared.sh"

configure_zellij_copy_behavior() {
    local zellij_config_dir="$HOME/.config/zellij"
    local zellij_config_file="$zellij_config_dir/config.kdl"
    local zellij_bin="$HOME/.cargo/bin/zellij"

    mkdir -p "$zellij_config_dir"

    if [[ ! -x "$zellij_bin" ]]; then
        zellij_bin="$(command -v zellij || true)"
    fi

    if [[ ! -f "$zellij_config_file" ]]; then
        if [[ -n "$zellij_bin" && -x "$zellij_bin" ]]; then
            echo "Creating default Zellij config at $zellij_config_file"
            "$zellij_bin" setup --dump-config > "$zellij_config_file"
        else
            echo "Zellij binary not found. Creating minimal Zellij config."
            : > "$zellij_config_file"
        fi
    fi

    set_zellij_option() {
        local option_name="$1"
        local option_value="$2"

        if grep -Eq "^[[:space:]]*${option_name}[[:space:]]+" "$zellij_config_file"; then
            sed -i -E "s|^[[:space:]]*${option_name}[[:space:]]+.*|${option_name} ${option_value}|" "$zellij_config_file"
        else
            printf '\n%s %s\n' "$option_name" "$option_value" >> "$zellij_config_file"
        fi
    }

    set_zellij_option "mouse_mode" "true"
    set_zellij_option "copy_on_select" "true"
    set_zellij_option "copy_clipboard" '"system"'

    echo "Zellij system clipboard behavior configured in $zellij_config_file"
}

run_step "Zellij copy behavior configuration" "configure_zellij_copy_behavior"
