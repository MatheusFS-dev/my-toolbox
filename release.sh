#!/bin/sh

# Interactively run or observe the release controller from the repository root.
set -eu

# shellcheck disable=SC1007
repository_root=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
# shellcheck source=assets/payload/packages/scripts/lib/toolbox_output.sh
# shellcheck disable=SC1091
. "$repository_root/assets/payload/packages/scripts/lib/toolbox_output.sh"

if [ "$#" -ne 0 ]; then
    tb_output_init release
    tb_error 'release.sh is interactive. Run it without arguments.'
    exit 2
fi

tb_output_init release
while :; do
    tb_menu 'Choose release action' \
        '1. Release current changes' \
        '2. Watch an existing release'
    if ! IFS= read -r release_action; then
        tb_error 'Release action input closed.'
        exit 1
    fi
    case "$release_action" in
        1)
            if ! tb_ask_required 'Release message (required)'; then
                tb_error 'Release message input closed.'
                exit 1
            fi
            exec sh "$repository_root/tooling/release/release.sh" --message "$TB_ANSWER"
            ;;
        2)
            if ! tb_ask_required 'Commit SHA to watch (required)'; then
                tb_error 'Commit SHA input closed.'
                exit 1
            fi
            exec sh "$repository_root/tooling/release/release.sh" --watch "$TB_ANSWER"
            ;;
        *) tb_warn 'Choose 1 to release current changes or 2 to watch an existing release.' ;;
    esac
done
