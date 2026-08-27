#!/bin/sh
set -eu

resolver=scripts/next-release-version.sh

actual=$(printf '%s' '' | sh "$resolver")
[ "$actual" = '0.1.1' ] || {
    printf 'No releases resolved to %s, expected 0.1.1.\n' "$actual" >&2
    exit 1
}

actual=$(printf '%s\n' 'v0.2.33068735793' 'v2.0.0' 'draft' | sh "$resolver")
[ "$actual" = '0.1.1' ] || {
    printf 'Non-pattern releases resolved to %s, expected 0.1.1.\n' "$actual" >&2
    exit 1
}

actual=$(printf '%s\n' 'v0.1.1' 'v0.1.9' 'v0.1.4' 'v0.1.01' | sh "$resolver")
[ "$actual" = '0.1.10' ] || {
    printf 'Existing releases resolved to %s, expected 0.1.10.\n' "$actual" >&2
    exit 1
}
