#!/usr/bin/env bash
set -e
MODULE_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "$MODULE_DIR/shared.sh"

install_kitty() {
    local kitty_app="$HOME/.local/kitty.app"
    local local_bin="$HOME/.local/bin"
    local applications_dir="$HOME/.local/share/applications"
    local desktop_file="$applications_dir/kitty.desktop"

    # Older versions of this installer ran Kitty as root with the user's HOME,
    # leaving a root-owned cache that makes Kitty exit during startup.
    repair_user_ownership "$HOME/.cache/kitty"

    echo "Installing the latest Kitty release..."
    run_as_user sh -c 'curl -L https://sw.kovidgoyal.net/kitty/installer.sh | sh /dev/stdin'
    run_as_user mkdir -p "$local_bin" "$applications_dir"
    run_as_user ln -sf "$kitty_app/bin/kitty" "$kitty_app/bin/kitten" "$local_bin/"
    run_as_user cp "$kitty_app/share/applications/kitty.desktop" "$desktop_file"
    run_as_user sed -i "s|Icon=kitty|Icon=$kitty_app/share/icons/hicolor/256x256/apps/kitty.png|g" "$desktop_file"
    run_as_user sed -i "s|Exec=kitty|Exec=$kitty_app/bin/kitty|g" "$desktop_file"

    repair_user_ownership "$HOME/.cache/kitty"
}

run_step "Kitty installation" "install_kitty"
