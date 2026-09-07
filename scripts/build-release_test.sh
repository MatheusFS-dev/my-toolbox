#!/bin/sh
set -eu

repository_root=$(unset CDPATH; cd -- "$(dirname "$0")/.." && pwd)
temporary_root=$(mktemp -d)
test_symlink="$repository_root/packages/others/template/.release-symlink-test"
cleanup() {
    rm -f "$test_symlink"
    rm -rf "$temporary_root"
}
trap cleanup EXIT HUP INT TERM
tomli_assets='packages/others/_vendor/tomli/LICENSE
packages/others/_vendor/tomli/__init__.py
packages/others/_vendor/tomli/_parser.py
packages/others/_vendor/tomli/_re.py
packages/others/_vendor/tomli/_types.py
packages/others/_vendor/tomli/py.typed
packages/agent-workspace-template/source/scripts/linux/python3/_vendor/tomli/LICENSE
packages/agent-workspace-template/source/scripts/linux/python3/_vendor/tomli/__init__.py
packages/agent-workspace-template/source/scripts/linux/python3/_vendor/tomli/_parser.py
packages/agent-workspace-template/source/scripts/linux/python3/_vendor/tomli/_re.py
packages/agent-workspace-template/source/scripts/linux/python3/_vendor/tomli/_types.py
packages/agent-workspace-template/source/scripts/linux/python3/_vendor/tomli/py.typed
packages/agent-workspace-template/source/scripts/windows/_vendor/tomli/LICENSE
packages/agent-workspace-template/source/scripts/windows/_vendor/tomli/__init__.py
packages/agent-workspace-template/source/scripts/windows/_vendor/tomli/_parser.py
packages/agent-workspace-template/source/scripts/windows/_vendor/tomli/_re.py
packages/agent-workspace-template/source/scripts/windows/_vendor/tomli/_types.py
packages/agent-workspace-template/source/scripts/windows/_vendor/tomli/py.typed'
if [ -e "$test_symlink" ] || [ -L "$test_symlink" ]; then
    printf 'Release symlink fixture path already exists: %s\n' "$test_symlink" >&2
    exit 1
fi
ln -s README.md "$test_symlink"

# The release layout is independent of Go compilation, so a fixed compiler
# fixture keeps this regression test focused on the archives users download.
mkdir -p "$temporary_root/bin" "$temporary_root/dist"
ln -s "$repository_root/scripts/testdata/go" "$temporary_root/bin/go"
reader_directory="$temporary_root/readers"
for platform in linux-amd64 linux-arm64 windows-amd64; do
    reader_name=tb-markdown-reader
    if [ "$platform" = windows-amd64 ]; then reader_name=tb-markdown-reader.exe; fi
    mkdir -p "$reader_directory/$platform/libexec"
    cp "$repository_root/scripts/testdata/reader" "$reader_directory/$platform/libexec/$reader_name"
    printf '\n# %s\n' "$platform" >> "$reader_directory/$platform/libexec/$reader_name"
    chmod 755 "$reader_directory/$platform/libexec/$reader_name"
done
mkdir -p "$temporary_root/stale"
printf 'stale\n' > "$temporary_root/stale/stale-entry.txt"
(
    cd "$temporary_root/stale"
    zip -q "$temporary_root/dist/toolbox-windows-amd64.zip" stale-entry.txt
)
for platform in linux-amd64 linux-arm64; do
    tar -C "$temporary_root/stale" -czf "$temporary_root/dist/toolbox-$platform.tar.gz" stale-entry.txt
done

# Every invalid input must leave existing output intact and never invoke Go.
export TB_TEST_GO_CALL_LOG="$temporary_root/go-calls"
output_before=$(cksum "$temporary_root/dist/"*)
expect_invalid() {
    if PATH="$temporary_root/bin:$PATH" sh "$repository_root/scripts/build-release.sh" "$@" > "$temporary_root/error" 2>&1; then
        printf 'Release unexpectedly accepted invalid arguments: %s\n' "$*" >&2
        exit 1
    fi
    if [ -e "$TB_TEST_GO_CALL_LOG" ]; then
        printf 'Release invoked Go before completing input validation.\n' >&2
        exit 1
    fi
    if [ "$(cksum "$temporary_root/dist/"*)" != "$output_before" ] ||
        [ "$(find "$temporary_root/dist" -type f | wc -l)" -ne 3 ]; then
        printf 'Invalid release input changed existing output.\n' >&2
        exit 1
    fi
}
expect_invalid
expect_invalid 0.1.4 "$temporary_root/dist"
expect_invalid 0.1.4 "$temporary_root/dist" "$reader_directory" extra
for invalid_version in '' v0.1.4 01.1.4 0.01.4 0.1.04 0.1 0.1.4.2; do
    expect_invalid "$invalid_version" "$temporary_root/dist" "$reader_directory"
done
expect_invalid 0.1.4 '' "$reader_directory"
expect_invalid 0.1.4 "$temporary_root/dist/toolbox-windows-amd64.zip" "$reader_directory"
expect_invalid 0.1.4 "$temporary_root/dist" ''
expect_invalid 0.1.4 "$temporary_root/dist" "$temporary_root/missing-readers"
expect_invalid 0.1.4 "$temporary_root/dist" "$repository_root/scripts/testdata/reader"
for platform in linux-amd64 linux-arm64 windows-amd64; do
    reader_name=tb-markdown-reader
    if [ "$platform" = windows-amd64 ]; then reader_name=tb-markdown-reader.exe; fi
    reader="$reader_directory/$platform/libexec/$reader_name"
    mv "$reader" "$temporary_root/reader-saved"
    expect_invalid 0.1.4 "$temporary_root/dist" "$reader_directory"
    mkdir "$reader"
    expect_invalid 0.1.4 "$temporary_root/dist" "$reader_directory"
    rmdir "$reader"
    ln -s "$temporary_root/reader-saved" "$reader"
    expect_invalid 0.1.4 "$temporary_root/dist" "$reader_directory"
    rm "$reader"
    touch "$reader"
    chmod 755 "$reader"
    expect_invalid 0.1.4 "$temporary_root/dist" "$reader_directory"
    mv "$temporary_root/reader-saved" "$reader"
    if [ "$platform" != windows-amd64 ]; then
        chmod 644 "$reader"
        expect_invalid 0.1.4 "$temporary_root/dist" "$reader_directory"
        chmod 755 "$reader"
    fi
done

# Resolve relative paths with spaces and an inherited CDPATH. Reader-directory
# sources and build products must never leak into the release payload.
mkdir -p "$reader_directory/third_party/src" "$reader_directory/target"
touch "$reader_directory/Cargo.toml" "$reader_directory/Cargo.lock" "$reader_directory/target/debug-reader"
touch "$reader_directory/linux-amd64/libexec/unrelated-reader-asset"
ln -s "$reader_directory" "$temporary_root/reader link"
(
    cd "$temporary_root"
    TB_TEST_READER_FAIL=1 CDPATH="$temporary_root" PATH="$temporary_root/bin:$PATH" sh "$repository_root/scripts/build-release.sh" \
        0.1.4 dist './reader link'
)

for platform in linux-amd64 linux-arm64; do
    archive="$temporary_root/dist/toolbox-$platform.tar.gz"
    entries=$(tar -tzf "$archive")
    if printf '%s\n' "$entries" | grep -E '^stale-entry.txt$|^libexec/unrelated-reader-asset$' >/dev/null; then
        printf '%s contains a stale or unrelated entry.\n' "$archive" >&2
        exit 1
    fi

    # The updater rejects a root directory header because it resolves to the
    # extraction destination itself. Release archives must omit that header.
    if printf '%s\n' "$entries" | grep -Fx './' >/dev/null; then
        printf '%s contains the unsafe root entry ./\n' "$archive" >&2
        exit 1
    fi
    if printf '%s\n' "$entries" | grep -E '(^|/)__pycache__/|\.py[co]$' >/dev/null; then
        printf '%s contains generated Python bytecode.\n' "$archive" >&2
        exit 1
    fi
    if printf '%s\n' "$entries" | grep -E '(^|/)\.git(/|$)' >/dev/null; then
        printf '%s contains checkout metadata.\n' "$archive" >&2
        exit 1
    fi

    if printf '%s\n' "$entries" | grep -E '(^|/)(third_party|target)/|(^|/)Cargo\.(toml|lock)$|\.rs$' >/dev/null; then
        printf '%s contains reader sources or build products.\n' "$archive" >&2
        exit 1
    fi
    for required_entry in tb libexec/ libexec/tb-markdown-reader libexec/tb-markdown-reader-LICENSE commands.json completions/ completions/_tb completions/tb.bash completions/tb.ps1 packages/ packages/search/articles/ packages/macros/autohotkey/press_key_after_x_ms.ahk packages/macros/autohotkey/press_key_after_x_ms.json version.txt; do
        if ! printf '%s\n' "$entries" | grep -Fx "$required_entry" >/dev/null; then
            printf '%s is missing %s.\n' "$archive" "$required_entry" >&2
            exit 1
        fi
    done
    mkdir "$temporary_root/extracted-$platform"
    tar -xzf "$archive" -C "$temporary_root/extracted-$platform" libexec/tb-markdown-reader
    extracted_reader="$temporary_root/extracted-$platform/libexec/tb-markdown-reader"
    if [ ! -x "$extracted_reader" ] || ! cmp "$reader_directory/$platform/libexec/tb-markdown-reader" "$extracted_reader"; then
        printf '%s contains an invalid or non-executable reader.\n' "$archive" >&2
        exit 1
    fi
    tar -xOf "$archive" libexec/tb-markdown-reader-LICENSE |
        cmp "$repository_root/packages/search/fork-markdown-reader/LICENSE" -
    "$extracted_reader" --tb-self-check
    for tomli_asset in $tomli_assets; do
        if ! printf '%s\n' "$entries" | grep -Fx "$tomli_asset" >/dev/null; then
            printf '%s is missing required Tomli asset %s.\n' \
                "$archive" "$tomli_asset" >&2
            exit 1
        fi
    done

    find "$repository_root/packages" \
        \( -type d \( -path "$repository_root/packages/search/fork-markdown-reader" -o -name .git -o -name __pycache__ \) -prune \) -o \
        \( ! -name .git \( \( -type f ! -name '*.pyc' ! -name '*.pyo' \) -o -type l \) \) -print |
        while IFS= read -r source_path; do
        relative_path=${source_path#"$repository_root/"}
        if ! printf '%s\n' "$entries" | grep -Fx "$relative_path" >/dev/null; then
            printf '%s is missing packaged asset %s.\n' "$archive" "$relative_path" >&2
            exit 1
        fi
    done
done
if ! tar -tvzf "$temporary_root/dist/toolbox-linux-amd64.tar.gz" \
    packages/others/template/.release-symlink-test | grep '^l' >/dev/null; then
    printf 'Linux release did not preserve the template symlink.\n' >&2
    exit 1
fi

windows_archive="$temporary_root/dist/toolbox-windows-amd64.zip"
windows_entries=$(unzip -Z1 "$windows_archive")
if printf '%s\n' "$windows_entries" | grep -Fx 'stale-entry.txt' >/dev/null; then
    printf 'Windows release retained an entry from an older archive.\n' >&2
    exit 1
fi
if printf '%s\n' "$windows_entries" | grep -E '(^|/)(third_party|target)/|(^|/)Cargo\.(toml|lock)$|\.rs$' >/dev/null; then
    printf 'Windows release contains reader sources or build products.\n' >&2
    exit 1
fi
for required_entry in tb.exe libexec/ libexec/tb-markdown-reader.exe libexec/tb-markdown-reader-LICENSE commands.json completions/ completions/_tb completions/tb.bash completions/tb.ps1 packages/ packages/search/articles/ packages/macros/autohotkey/press_key_after_x_ms.ahk packages/macros/autohotkey/press_key_after_x_ms.json version.txt; do
    if ! printf '%s\n' "$windows_entries" | grep -Fx "$required_entry" >/dev/null; then
        printf '%s is missing %s.\n' "$windows_archive" "$required_entry" >&2
        exit 1
    fi
done
unzip -p "$windows_archive" libexec/tb-markdown-reader.exe > "$temporary_root/windows-reader"
cmp "$reader_directory/windows-amd64/libexec/tb-markdown-reader.exe" "$temporary_root/windows-reader"
unzip -p "$windows_archive" libexec/tb-markdown-reader-LICENSE |
    cmp "$repository_root/packages/search/fork-markdown-reader/LICENSE" -
for tomli_asset in $tomli_assets; do
    if ! printf '%s\n' "$windows_entries" | grep -Fx "$tomli_asset" >/dev/null; then
        printf '%s is missing required Tomli asset %s.\n' \
            "$windows_archive" "$tomli_asset" >&2
        exit 1
    fi
done
if ! zipinfo -l "$windows_archive" packages/others/template/.release-symlink-test | grep '^l' >/dev/null; then
    printf 'Windows release did not preserve the template symlink.\n' >&2
    exit 1
fi
find "$repository_root/packages" \
    \( -type d \( -path "$repository_root/packages/search/fork-markdown-reader" -o -name .git -o -name __pycache__ \) -prune \) -o \
    \( ! -name .git \( \( -type f ! -name '*.pyc' ! -name '*.pyo' \) -o -type l \) \) -print |
    while IFS= read -r source_path; do
    relative_path=${source_path#"$repository_root/"}
    if ! printf '%s\n' "$windows_entries" | grep -Fx "$relative_path" >/dev/null; then
        printf '%s is missing packaged asset %s.\n' "$windows_archive" "$relative_path" >&2
        exit 1
    fi
done
if printf '%s\n' "$windows_entries" | grep -E '(^|/)__pycache__/|\.py[co]$' >/dev/null; then
    printf 'Windows release contains generated Python bytecode.\n' >&2
    exit 1
fi
if printf '%s\n' "$windows_entries" | grep -E '(^|/)\.git(/|$)' >/dev/null; then
    printf 'Windows release contains checkout metadata.\n' >&2
    exit 1
fi
