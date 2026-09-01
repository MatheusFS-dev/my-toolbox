#!/bin/sh
set -eu

resolver=scripts/next-release-version.sh

actual=$(printf '%s' '' | sh "$resolver")
[ "$actual" = '0.1.1' ] || {
    printf 'No releases resolved to %s, expected 0.1.1.\n' "$actual" >&2
    exit 1
}

actual=$(printf '%s\n' 'v1.0.01' 'v1.0' 'draft' | sh "$resolver")
[ "$actual" = '0.1.1' ] || {
    printf 'Non-pattern releases resolved to %s, expected 0.1.1.\n' "$actual" >&2
    exit 1
}

actual=$(printf '%s\n' 'v0.1.1' 'v0.1.9' 'v0.1.4' 'v0.1.01' | sh "$resolver")
[ "$actual" = '0.1.10' ] || {
    printf 'Existing releases resolved to %s, expected 0.1.10.\n' "$actual" >&2
    exit 1
}

actual=$(printf '%s\n' 'v0.99.99' 'v1.0.0' 'v0.1.33068735793' | sh "$resolver")
[ "$actual" = '1.0.1' ] || {
    printf 'Stable release resolved to %s, expected 1.0.1.\n' "$actual" >&2
    exit 1
}

actual=$(printf '%s\n' 'v1.0.0' | TOOLBOX_RELEASE_MINIMUM_VERSION=1.1.0 sh "$resolver")
[ "$actual" = '1.1.0' ] || {
    printf 'Minimum release resolved to %s, expected 1.1.0.\n' "$actual" >&2
    exit 1
}

actual=$(printf '%s\n' 'v1.1.0' | TOOLBOX_RELEASE_MINIMUM_VERSION=1.1.0 sh "$resolver")
[ "$actual" = '1.1.1' ] || {
    printf 'Minimum release patch advancement resolved to %s, expected 1.1.1.\n' "$actual" >&2
    exit 1
}

for minimum_version in 1.1 1.01.0 v1.1.0 1.1.0.0; do
    if printf '%s\n' 'v1.0.0' | TOOLBOX_RELEASE_MINIMUM_VERSION="$minimum_version" sh "$resolver" >/dev/null 2>&1; then
        printf 'Invalid minimum release version %s was accepted.\n' "$minimum_version" >&2
        exit 1
    fi
done
