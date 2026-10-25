#!/bin/sh
set -eu

minimum_version=${TOOLBOX_RELEASE_MINIMUM_VERSION:-}
if [ -n "$minimum_version" ] && ! printf '%s\n' "$minimum_version" | awk '
    NR == 1 && $0 ~ /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/ { valid = 1; next }
    { invalid = 1 }
    END { exit !(valid && !invalid) }
'; then
    printf 'Invalid minimum release version: %s\n' "$minimum_version" >&2
    exit 1
fi

# Select the highest published canonical semantic version. Numeric component
# sorting prevents a large pre-1.0 patch from outranking a stable release.
latest_version=$(
    awk -F. '$0 ~ /^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/ { sub(/^v/, "", $1); print $1 "." $2 "." $3 }' |
        sort -t. -k1,1n -k2,2n -k3,3n |
        tail -n 1
)
latest_version=${latest_version:-0.1.0}
IFS=. read -r major minor patch <<EOF
$latest_version
EOF

if [ -n "$minimum_version" ]; then
    highest_version=$(printf '%s\n%s\n' "$latest_version" "$minimum_version" |
        sort -t. -k1,1n -k2,2n -k3,3n |
        tail -n 1)
    if [ "$highest_version" = "$minimum_version" ] && [ "$latest_version" != "$minimum_version" ]; then
        printf '%s\n' "$minimum_version"
        exit 0
    fi
fi

printf '%s.%s.%s\n' "$major" "$minor" "$((patch + 1))"
