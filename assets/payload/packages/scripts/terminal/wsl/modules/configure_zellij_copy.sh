#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/shared.sh"

config="$HOME/.config/zellij/config.kdl"
block=$'mouse_mode true\ncopy_on_select true\ncopy_clipboard "system"'
write_managed_block "$config" zellij-copy "$block" '//'

