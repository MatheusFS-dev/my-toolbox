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
if [ -e "$test_symlink" ] || [ -L "$test_symlink" ]; then
    printf 'Release symlink fixture path already exists: %s\n' "$test_symlink" >&2
    exit 1
fi
ln -s README.md "$test_symlink"

# The release layout is independent of Go compilation, so a fixed compiler
# fixture keeps this regression test focused on the archives users download.
mkdir -p "$temporary_root/bin" "$temporary_root/dist"
ln -s "$repository_root/scripts/testdata/go" "$temporary_root/bin/go"
mkdir -p "$temporary_root/stale"
printf 'stale\n' > "$temporary_root/stale/stale-entry.txt"
(
    cd "$temporary_root/stale"
    zip -q "$temporary_root/dist/toolbox-windows-amd64.zip" stale-entry.txt
)
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
    if printf '%s\n' "$entries" | grep -E '(^|/)__pycache__/|\.py[co]$' >/dev/null; then
        printf '%s contains generated Python bytecode.\n' "$archive" >&2
        exit 1
    fi
    if printf '%s\n' "$entries" | grep -E '(^|/)\.git(/|$)' >/dev/null; then
        printf '%s contains checkout metadata.\n' "$archive" >&2
        exit 1
    fi

    for required_entry in tb commands.json completions/ completions/_tb completions/tb.bash completions/tb.ps1 packages/ version.txt; do
        if ! printf '%s\n' "$entries" | grep -Fx "$required_entry" >/dev/null; then
            printf '%s is missing %s.\n' "$archive" "$required_entry" >&2
            exit 1
        fi
    done

    find "$repository_root/packages" \
        \( -type d \( -name .git -o -name __pycache__ \) -prune \) -o \
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
for required_entry in tb.exe commands.json completions/ completions/_tb completions/tb.bash completions/tb.ps1 packages/ version.txt; do
    if ! printf '%s\n' "$windows_entries" | grep -Fx "$required_entry" >/dev/null; then
        printf '%s is missing %s.\n' "$windows_archive" "$required_entry" >&2
        exit 1
    fi
done
if ! zipinfo -l "$windows_archive" packages/others/template/.release-symlink-test | grep '^l' >/dev/null; then
    printf 'Windows release did not preserve the template symlink.\n' >&2
    exit 1
fi
find "$repository_root/packages" \
    \( -type d \( -name .git -o -name __pycache__ \) -prune \) -o \
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
