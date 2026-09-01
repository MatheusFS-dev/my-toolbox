#!/usr/bin/env bash
set -euo pipefail

repository_root=$(cd -- "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
test_root=$(mktemp -d)
trap 'rm -rf "$test_root"' EXIT HUP INT TERM

make_fixture() {
    local terminal_name=$1
    local fixture_root="$test_root/$terminal_name"

    mkdir -p "$fixture_root/modules" "$fixture_root/bin" "$fixture_root/home"
    cp "$repository_root/packages/scripts/terminal/$terminal_name/modules/nautilus_integration.sh" \
        "$fixture_root/modules/nautilus_integration.sh"
    if [[ -f "$repository_root/packages/scripts/terminal/$terminal_name/modules/kitty_nautilus.py" ]]; then
        cp "$repository_root/packages/scripts/terminal/$terminal_name/modules/kitty_nautilus.py" \
            "$fixture_root/modules/kitty_nautilus.py"
    fi

    cat > "$fixture_root/modules/shared.sh" <<'SH'
run_as_user() {
    "$@"
}

run_step() {
    local step_name=$1
    local step_cmd=$2
    local status

    set +e
    eval "$step_cmd"
    status=$?
    set -e
    if [[ $status -ne 0 ]]; then
        printf "Warning: '%s' failed with status %s, but continuing...\n" \
            "$step_name" "$status" >&2
    fi
}
SH

    ln -s "$(command -v bash)" "$fixture_root/bin/bash"
    ln -s "$(command -v dirname)" "$fixture_root/bin/dirname"
    # shellcheck disable=SC2016 -- variables expand when the generated stub runs.
    printf '%s\n' '#!/bin/sh' 'printf "unexpected command: %s\n" "$0" >> "$COMMAND_LOG"' 'exit 91' \
        > "$fixture_root/bin/apt"
    cp "$fixture_root/bin/apt" "$fixture_root/bin/apt-get"
    cp "$fixture_root/bin/apt" "$fixture_root/bin/python3"
    cp "$fixture_root/bin/apt" "$fixture_root/bin/gsettings"
    cp "$fixture_root/bin/apt" "$fixture_root/bin/glib-compile-schemas"
    chmod 755 "$fixture_root/bin/apt" "$fixture_root/bin/apt-get" \
        "$fixture_root/bin/python3" "$fixture_root/bin/gsettings" \
        "$fixture_root/bin/glib-compile-schemas"
}

assert_missing_nautilus_is_skipped() {
    local terminal_name=$1
    local fixture_root="$test_root/$terminal_name"
    local output_file="$fixture_root/output"
    local command_log="$fixture_root/commands.log"

    if ! HOME="$fixture_root/home" USER="${USER:-test-user}" COMMAND_LOG="$command_log" \
        PATH="$fixture_root/bin" \
        "$fixture_root/bin/bash" "$fixture_root/modules/nautilus_integration.sh" \
        > "$output_file" 2>&1; then
        printf '%s Nautilus integration aborted when Nautilus was unavailable.\n' \
            "$terminal_name" >&2
        cat "$output_file" >&2
        exit 1
    fi

    if ! grep -F 'Nautilus is not installed; skipping integration.' "$output_file" >/dev/null; then
        printf '%s Nautilus integration did not report the skipped step.\n' \
            "$terminal_name" >&2
        cat "$output_file" >&2
        exit 1
    fi
    if [[ -e "$command_log" ]]; then
        printf '%s Nautilus integration ran configuration commands while Nautilus was unavailable.\n' \
            "$terminal_name" >&2
        cat "$command_log" >&2
        exit 1
    fi
}

for terminal_name in alacritty kitty; do
    make_fixture "$terminal_name"
    assert_missing_nautilus_is_skipped "$terminal_name"
done

printf '%s\n' 'Terminal setup Nautilus checks passed.'
