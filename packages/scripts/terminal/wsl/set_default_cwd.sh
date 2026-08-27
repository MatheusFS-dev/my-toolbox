#!/usr/bin/env bash
set -euo pipefail

START_MARKER='# >>> project-template:wsl:default-cwd >>>'
END_MARKER='# <<< project-template:wsl:default-cwd <<<'

fail() {
    printf 'Error: %s\n' "$1" >&2
    exit 1
}

prepare_update() {
    local file="$1"
    local block="$2"
    local output="$3"
    local newline=$'\n'

    if [[ -e "$file" && ! -f "$file" ]]; then
        fail "$file exists but is not a regular file."
    fi
    if [[ -f "$file" ]]; then
        if ! awk -v start="$START_MARKER" -v end="$END_MARKER" '
            { line = $0; sub(/\r$/, "", line) }
            line == start { if (inside) exit 2; inside = 1; next }
            line == end { if (!inside) exit 3; inside = 0; next }
            END { if (inside) exit 4 }
        ' "$file"; then
            fail "$file contains malformed managed default-cwd markers."
        fi
        if grep -q $'\r$' "$file"; then newline=$'\r\n'; fi
        awk -v start="$START_MARKER" -v end="$END_MARKER" '
            { line = $0; sub(/\r$/, "", line) }
            line == start { inside = 1; next }
            line == end { inside = 0; next }
            !inside { print $0 }
        ' "$file" > "$output"
        chmod --reference="$file" "$output"
    else
        : > "$output"
        chmod 0644 "$output"
    fi

    if [[ -s "$output" ]] && [[ "$(tail -c 1 "$output" | od -An -tu1 | tr -d ' \n')" != 10 ]]; then
        printf '%s' "$newline" >> "$output"
    fi
    while IFS= read -r line || [[ -n "$line" ]]; do
        printf '%s%s' "$line" "$newline"
    done <<< "$block" >> "$output"
}

make_backup() {
    local file="$1"
    local stamp backup
    stamp="$(date -u +%Y%m%d-%H%M%S-%N)"
    backup="${file}.project-template-${stamp}-$$-${RANDOM}.bak"
    cp -p -- "$file" "$backup"
    printf '%s' "$backup"
}

if (( $# != 0 )); then
    fail 'set_default_cwd.sh does not accept arguments; enter the directory at the prompt.'
fi

read -r -p 'Enter the default shell directory: ' target || fail 'A directory is required.'
[[ -n "$target" ]] || fail 'A directory is required.'
[[ "$target" == /* ]] || fail 'The directory must be an absolute path.'
[[ -d "$target" ]] || fail "The directory does not exist: $target"

escaped_target="${target//\'/\'\\\'\'}"
quoted_target="'$escaped_target'"
block="${START_MARKER}
if [ \"\$PWD\" = \"\$HOME\" ]; then
    cd -- ${quoted_target}
fi
${END_MARKER}"

files=("$HOME/.bashrc" "$HOME/.zshrc")
temps=()
changed=()
backups=()

for file in "${files[@]}"; do
    [[ ! -L "$file" ]] || fail "$file is a symbolic link; refusing to replace it."
done

cleanup() {
    local temp
    for temp in "${temps[@]:-}"; do
        [[ -n "$temp" ]] && rm -f -- "$temp"
    done
    return 0
}
trap cleanup EXIT

for file in "${files[@]}"; do
    temp="$(mktemp "${file}.project-template.XXXXXX")"
    temps+=("$temp")
    prepare_update "$file" "$block" "$temp"
    if [[ -f "$file" ]] && cmp -s -- "$file" "$temp"; then
        changed+=(false)
    else
        changed+=(true)
    fi
done

for index in "${!files[@]}"; do
    file="${files[$index]}"
    if [[ "${changed[$index]}" == true ]]; then
        backup=''
        if [[ -f "$file" ]]; then backup="$(make_backup "$file")"; fi
        mv -f -- "${temps[$index]}" "$file"
        temps[$index]=''
        backups+=("$backup")
    else
        backups+=('')
    fi
done

printf 'Configured default shell directory: %s\n' "$target"
for index in "${!files[@]}"; do
    if [[ -n "${backups[$index]}" ]]; then
        printf 'Backup for %s: %s\n' "${files[$index]}" "${backups[$index]}"
    elif [[ "${changed[$index]}" == true ]]; then
        printf 'Backup for %s: none (file was created)\n' "${files[$index]}"
    else
        printf 'Backup for %s: none (already current)\n' "${files[$index]}"
    fi
done
