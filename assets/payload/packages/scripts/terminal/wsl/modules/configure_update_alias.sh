#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/shared.sh"

block="alias update='sudo apt update && sudo apt upgrade'"
write_managed_block "$HOME/.bashrc" update-alias "$block"
write_managed_block "$HOME/.zshrc" update-alias "$block"

