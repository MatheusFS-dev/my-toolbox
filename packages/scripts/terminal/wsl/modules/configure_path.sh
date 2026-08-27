#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/shared.sh"

block='export PATH="$HOME/.local/bin:$HOME/.cargo/bin:$PATH"'
write_managed_block "$HOME/.bashrc" path "$block"
write_managed_block "$HOME/.zshrc" path "$block"

