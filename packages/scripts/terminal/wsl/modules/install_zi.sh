#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/shared.sh"

curl -fsSL https://raw.githubusercontent.com/z-shell/src/main/public/sh/install.sh | \
    sh -s -- -i skip -b main
write_managed_block "$HOME/.zshrc" zi $'if [[ -r "$HOME/.local/share/zi/bin/zi.zsh" ]]; then\n    source "$HOME/.local/share/zi/bin/zi.zsh"\n    autoload -Uz _zi\n    (( ${+_comps} )) && _comps[zi]=_zi\n    zicompinit\nfi'
