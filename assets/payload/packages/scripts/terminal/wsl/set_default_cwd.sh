#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=/dev/null
. "$SCRIPT_DIR/../../lib/toolbox_output.sh"
tb_output_init set-default-cwd

START_MARKER='# >>> project-template:wsl:default-cwd >>>'
END_MARKER='# <<< project-template:wsl:default-cwd <<<'

fail() {
    tb_error "$1" >&2
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

while true; do
    tb_prompt 'Enter the default shell directory: '
    read -r target || fail 'Input canceled.'
    if [[ -z "$target" ]]; then
        tb_warn 'A value is required; enter a value to continue.' >&2
    elif [[ "$target" != /* ]]; then
        tb_warn 'The directory must be an absolute path.' >&2
    elif [[ ! -d "$target" ]]; then
        tb_warn "The directory does not exist: $target" >&2
    else
        break
    fi
done

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

tb_info 'Writing shell configuration...'
for index in "${!files[@]}"; do
    file="${files[$index]}"
    if [[ "${changed[$index]}" == true ]]; then
        backup=''
        if [[ -f "$file" ]]; then backup="$(make_backup "$file")"; fi
        mv -f -- "${temps[$index]}" "$file"
        temps[index]=''
        backups+=("$backup")
    else
        backups+=('')
    fi
done

tb_info "Configured default shell directory: $target"
for index in "${!files[@]}"; do
    if [[ -n "${backups[$index]}" ]]; then
        tb_info "Backup for ${files[$index]}: ${backups[$index]}"
    elif [[ "${changed[$index]}" == true ]]; then
        tb_info "Backup for ${files[$index]}: none (file was created)"
    else
        tb_info "Backup for ${files[$index]}: none (already current)"
    fi
done
tb_success 'Default shell directory configured successfully.'
