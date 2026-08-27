#!/bin/sh
set -eu

# Only published tags in the requested release series contribute to the next
# patch. Canonical numeric components prevent aliases such as v0.1.01.
latest_patch=$(
    awk -F. '$0 ~ /^v0\.1\.(0|[1-9][0-9]*)$/ { print $3 }' |
        sort -n |
        tail -n 1
)

printf '0.1.%s\n' "$(( ${latest_patch:-0} + 1 ))"
