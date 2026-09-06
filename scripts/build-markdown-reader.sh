#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
    printf 'Usage: scripts/build-markdown-reader.sh RUST_TARGET OUTPUT_DIRECTORY\n' >&2
    exit 1
fi
rust_target=$1
output_directory=$2
case "$(uname -s):$(uname -m):$rust_target" in
    Linux:x86_64:x86_64-unknown-linux-musl|Linux:aarch64:aarch64-unknown-linux-musl) ;;
    *) printf 'Reader builds require the matching native Linux host and a supported MUSL target.\n' >&2; exit 1 ;;
esac
if [ -z "$output_directory" ] || { [ -e "$output_directory" ] && [ ! -d "$output_directory" ]; }; then
    printf 'Output directory must be a nonempty directory path.\n' >&2
    exit 1
fi
for command in cargo readelf; do
    if ! command -v "$command" >/dev/null 2>&1; then
        printf 'Reader build requires %s.\n' "$command" >&2
        exit 1
    fi
done
repository_root=$(unset CDPATH; cd -- "$(dirname "$0")/.." && pwd)
target_directory=${CARGO_TARGET_DIR:-"$repository_root/packages/search/fork-markdown-reader/target"}
case "$target_directory" in /*) ;; *) target_directory="$PWD/$target_directory" ;; esac
cargo build --locked --release --target "$rust_target" \
    --manifest-path "$repository_root/packages/search/fork-markdown-reader/Cargo.toml" \
    --target-dir "$target_directory"
reader="$target_directory/$rust_target/release/tb-markdown-reader"
if [ ! -f "$reader" ] || [ ! -s "$reader" ] || [ ! -x "$reader" ] || [ -L "$reader" ]; then
    printf 'Cargo did not produce a regular executable reader: %s\n' "$reader" >&2
    exit 1
fi

# Capture inspection output separately so an inspector failure cannot be hidden
# by a successful grep. A MUSL release must not need a loader or shared library.
program_headers=$(readelf -lW "$reader")
dynamic_entries=$(readelf -dW "$reader")
if printf '%s\n' "$program_headers" | grep -E '^[[:space:]]*INTERP([[:space:]]|$)' >/dev/null; then
    printf 'Reader contains an ELF INTERP program header.\n' >&2
    exit 1
fi
if printf '%s\n' "$dynamic_entries" | grep -F '(NEEDED)' >/dev/null; then
    printf 'Reader contains an ELF dynamic NEEDED dependency.\n' >&2
    exit 1
fi
"$reader" --tb-self-check
mkdir -p "$output_directory/libexec"
cp "$reader" "$output_directory/libexec/tb-markdown-reader"
chmod 755 "$output_directory/libexec/tb-markdown-reader"
