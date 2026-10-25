#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/shared.sh"

write_managed_block "$HOME/.bashrc" word-navigation $'bind \'"\\e[1;5C": forward-word\'\nbind \'"\\e[1;5D": backward-word\''
write_managed_block "$HOME/.zshrc" word-navigation $'bindkey \'^[[1;5C\' forward-word\nbindkey \'^[[1;5D\' backward-word'

