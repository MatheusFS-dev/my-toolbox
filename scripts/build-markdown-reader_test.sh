#!/bin/sh
set -eu

repository_root=$(unset CDPATH; cd -- "$(dirname "$0")/.." && pwd)
temporary_root=$(mktemp -d)
cleanup() {
    rm -rf "$temporary_root"
}
trap cleanup EXIT HUP INT TERM
case "$(uname -m)" in
    x86_64) target=x86_64-unknown-linux-musl; foreign=aarch64-unknown-linux-musl ;;
    aarch64) target=aarch64-unknown-linux-musl; foreign=x86_64-unknown-linux-musl ;;
    *) printf 'Reader build tests require an x64 or ARM64 Linux host.\n' >&2; exit 1 ;;
esac
mkdir -p "$temporary_root/bin"
for command in cargo readelf; do
    cp "$repository_root/scripts/testdata/reader-build/$command" "$temporary_root/bin/$command"
    chmod 755 "$temporary_root/bin/$command"
done
export PATH="$temporary_root/bin:$PATH"
export CARGO_TARGET_DIR="$temporary_root/target"
export TB_TEST_READER_FIXTURE="$repository_root/scripts/testdata/reader"
export TB_TEST_CARGO_LOG="$temporary_root/cargo-calls"
build_script="$repository_root/scripts/build-markdown-reader.sh"

# Omitting input validation must never let Cargo run with an invalid or foreign
# target, since every built reader must execute its self-check on this host.
expect_invalid() {
    if sh "$build_script" "$@" > "$temporary_root/error" 2>&1; then
        printf 'Reader build unexpectedly accepted invalid arguments: %s\n' "$*" >&2
        exit 1
    fi
    if [ -e "$TB_TEST_CARGO_LOG" ]; then
        printf 'Reader build invoked Cargo before validating input.\n' >&2
        exit 1
    fi
}
expect_invalid
expect_invalid "$target"
expect_invalid "$target" "$temporary_root/output" extra
expect_invalid x86_64-unknown-linux-gnu "$temporary_root/output"
expect_invalid "$foreign" "$temporary_root/output"
expect_invalid "$target" ''
expect_invalid "$target" "$TB_TEST_READER_FIXTURE"

sh "$build_script" "$target" "$temporary_root/reader output"
reader="$temporary_root/reader output/libexec/tb-markdown-reader"
test -x "$reader"
cmp "$TB_TEST_READER_FIXTURE" "$reader"
"$reader" --tb-self-check

# A dynamic dependency or an inspection error must fail without replacing a
# previously verified output binary.
for mode in interp needed failed; do
    if TB_TEST_ELF_MODE="$mode" sh "$build_script" "$target" "$temporary_root/reader output" > "$temporary_root/error" 2>&1; then
        printf 'Reader build accepted ELF inspection failure: %s\n' "$mode" >&2
        exit 1
    fi
    cmp "$TB_TEST_READER_FIXTURE" "$reader"
done
if TB_TEST_READER_FAIL=1 sh "$build_script" "$target" "$temporary_root/reader output" > "$temporary_root/error" 2>&1; then
    printf 'Reader build accepted a failing native self-check.\n' >&2
    exit 1
fi
cmp "$TB_TEST_READER_FIXTURE" "$reader"
