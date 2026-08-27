#!/bin/sh
set -eu

repository_root=$(unset CDPATH; cd -- "$(dirname "$0")/.." && pwd)
temporary_root=$(mktemp -d)
trap 'rm -rf "$temporary_root"' EXIT HUP INT TERM

# The release layout is independent of Go compilation, so a fixed compiler
# fixture keeps this regression test focused on the archives users download.
mkdir -p "$temporary_root/bin" "$temporary_root/dist"
ln -s "$repository_root/scripts/testdata/go" "$temporary_root/bin/go"
PATH="$temporary_root/bin:$PATH" sh "$repository_root/scripts/build-release.sh" \
    0.1.4 "$temporary_root/dist"

for platform in linux-amd64 linux-arm64; do
    archive="$temporary_root/dist/toolbox-$platform.tar.gz"
    entries=$(tar -tzf "$archive")

    # The updater rejects a root directory header because it resolves to the
    # extraction destination itself. Release archives must omit that header.
    if printf '%s\n' "$entries" | grep -Fx './' >/dev/null; then
        printf '%s contains the unsafe root entry ./\n' "$archive" >&2
        exit 1
    fi

    for required_entry in tb commands.json packages/ version.txt; do
        if ! printf '%s\n' "$entries" | grep -Fx "$required_entry" >/dev/null; then
            printf '%s is missing %s.\n' "$archive" "$required_entry" >&2
            exit 1
        fi
    done
done
