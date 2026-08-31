#!/bin/sh
set -eu

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

printf '%s.%s.%s\n' "$major" "$minor" "$((patch + 1))"
