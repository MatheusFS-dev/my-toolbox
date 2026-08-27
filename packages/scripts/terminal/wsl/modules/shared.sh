#!/usr/bin/env bash

write_managed_block() {
    local file="$1"
    local feature="$2"
    local content="$3"
    local comment_prefix="${4:-#}"
    local start_marker="$comment_prefix >>> project-template:wsl:$feature >>>"
    local end_marker="$comment_prefix <<< project-template:wsl:$feature <<<"
    local directory temp_file

    directory="$(dirname "$file")"
    mkdir -p "$directory"
    touch "$file"
    temp_file="$(mktemp "$directory/.project-template.XXXXXX")"

    awk -v start="$start_marker" -v end="$end_marker" '
        $0 == start { inside = 1; next }
        $0 == end { inside = 0; next }
        !inside { print }
    ' "$file" > "$temp_file"

    while [[ -s "$temp_file" && "$(tail -c 1 "$temp_file" | wc -l)" -eq 0 ]]; do
        printf '\n' >> "$temp_file"
    done
    if [[ -s "$temp_file" ]]; then
        printf '\n' >> "$temp_file"
    fi
    printf '%s\n%s\n%s\n' "$start_marker" "$content" "$end_marker" >> "$temp_file"
    chmod 0644 "$temp_file"
    mv "$temp_file" "$file"
}

