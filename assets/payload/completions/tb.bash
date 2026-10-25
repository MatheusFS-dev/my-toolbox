# shellcheck shell=bash

# Completes only the first argument after tb from the live command catalog.
_tb_completion() {
    COMPREPLY=()
    if [ "$COMP_CWORD" -ne 1 ]; then
        return 0
    fi

    local candidate
    local candidates
    candidates=$(tb __complete) || return 1
    while IFS= read -r candidate; do
        case "$candidate" in
            "${COMP_WORDS[COMP_CWORD]}"*) COMPREPLY+=("$candidate") ;;
        esac
    done <<< "$candidates"
}

complete -F _tb_completion tb
