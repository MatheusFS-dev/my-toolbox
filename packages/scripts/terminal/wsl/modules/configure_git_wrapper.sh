#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/shared.sh"

read -r -d '' block <<'EOF' || true
_git_clone_cd_target() {
    local repo='' target='' arg=''
    while (( $# > 0 )); do
        arg="$1"
        case "$arg" in
            --) shift; repo="${1:-}"; target="${2:-}"; break ;;
            -b|-o|-u|-c|-j|--branch|--origin|--upload-pack|--template|--reference|--reference-if-able|--separate-git-dir|--jobs|--depth|--shallow-since|--shallow-exclude|--config|--filter|--server-option)
                shift; (( $# == 0 )) || shift ;;
            --*=*|--*|-*) shift ;;
            *) if [[ -z "$repo" ]]; then repo="$arg"; else target="$arg"; fi; shift ;;
        esac
    done
    if [[ -n "$target" ]]; then printf '%s\n' "$target"; return; fi
    repo="${repo%/}"; repo="${repo##*/}"; repo="${repo##*:}"; repo="${repo%.git}"
    printf '%s\n' "$repo"
}
git() {
    if [[ "${1:-}" == clone ]]; then
        local target status
        target="$(_git_clone_cd_target "${@:2}")"
        command git "$@"; status=$?
        if (( status == 0 )) && [[ -n "$target" && -d "$target" ]]; then
            cd -- "$target" || return "$status"
            printf 'git-clone-cd: changed directory to %s\n' "$target"
        fi
        return "$status"
    fi
    command git "$@"
}
EOF
write_managed_block "$HOME/.bashrc" git-wrapper "$block"
write_managed_block "$HOME/.zshrc" git-wrapper "$block"

