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
    # Variables expand when the generated stub runs.
    # shellcheck disable=SC2016
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

assert_wsl_shift_enter_selection() {
    local output normalized_setup
    normalized_setup="$test_root/setup_wsl.sh"
    tr -d '\r' < "$repository_root/packages/scripts/terminal/wsl/setup_wsl.sh" > "$normalized_setup"
    output="$({
        # shellcheck source=/dev/null
        source "$normalized_setup"
        reset_options
        parse_args --yes --skip-shift-enter
        select_features
        [[ "${SELECTED[shift_enter]}" == false ]]

        reset_options
        parse_args --yes
        select_features
        [[ "${SELECTED[shift_enter]}" == true ]]

        run_as_target() {
            # shellcheck disable=SC2317
            printf '%s\n' "$*"
        }
        run_feature shift_enter
    } 2>&1)" || {
        printf '%s\n' 'WSL Shift+Enter selection checks failed.' >&2
        printf '%s\n' "$output" >&2
        exit 1
    }
    if ! grep -F 'configure_shift_enter.sh' <<< "$output" >/dev/null; then
        printf '%s\n' 'WSL setup did not dispatch the Shift+Enter configurator.' >&2
        printf '%s\n' "$output" >&2
        exit 1
    fi
}

assert_wsl_shift_enter_selection

assert_wsl_yes_no_retries() {
    local normalized_setup output status
    normalized_setup="$test_root/setup_wsl-prompts.sh"
    tr -d '\r' < "$repository_root/packages/scripts/terminal/wsl/setup_wsl.sh" > "$normalized_setup"
    set +e
    output="$({
        # shellcheck source=/dev/null
        source "$normalized_setup"
        prompt_yes_no 'Enable test feature?'
    } <<< $'maybe\nno' 2>&1)"
    status=$?
    set -e
    if [[ $status -eq 0 ]] || ! grep -F 'Please enter yes, y, no, n' <<< "$output" >/dev/null; then
        printf '%s\n' 'WSL yes/no prompt did not warn and retry invalid input.' >&2
        printf '%s\n' "$output" >&2
        exit 1
    fi
}

assert_setup_venv_retries() {
    local fixture_root output
    fixture_root="$test_root/setup-venv"
    mkdir -p "$fixture_root"
    : > "$fixture_root/.bashrc"
    output="$(HOME="$fixture_root" bash "$repository_root/packages/scripts/utils/setup_venv.sh" <<< $'maybe\nno' 2>&1)"
    if ! grep -F 'Please enter yes, y, no, n' <<< "$output" >/dev/null; then
        printf '%s\n' 'setup-venv did not warn for an invalid confirmation.' >&2
        exit 1
    fi
    if grep -Fq 'venv()' "$fixture_root/.bashrc"; then
        printf '%s\n' 'setup-venv wrote configuration before accepting input.' >&2
        exit 1
    fi
}

assert_default_cwd_retries() {
    local fixture_root target output
    fixture_root="$test_root/default-cwd"
    target="$fixture_root/project"
    mkdir -p "$fixture_root/home" "$target"
    : > "$fixture_root/home/.bashrc"
    : > "$fixture_root/home/.zshrc"
    output="$(HOME="$fixture_root/home" bash "$repository_root/packages/scripts/terminal/wsl/set_default_cwd.sh" <<< $'relative\n'"$target" 2>&1)"
    if ! grep -F 'must be an absolute path' <<< "$output" >/dev/null; then
        printf '%s\n' 'set-default-cwd did not warn for an invalid path.' >&2
        exit 1
    fi
    if ! grep -Fq "$target" "$fixture_root/home/.bashrc"; then
        printf '%s\n' 'set-default-cwd did not accept the valid retried path.' >&2
        exit 1
    fi
}

assert_desktop_terminal_prompts_retry() {
    local terminal_name output prompt_library
    for terminal_name in alacritty kitty; do
        prompt_library="$test_root/${terminal_name}-prompt.sh"
        sed -n '/^prompt_yes_no()/,/^}/p' \
            "$repository_root/packages/scripts/terminal/$terminal_name/setup_${terminal_name}.sh" \
            > "$prompt_library"
        output="$({
            # The dynamically sourced prompt function reads this variable.
            # shellcheck disable=SC2034
            ASSUME_YES=false
            # shellcheck source=/dev/null
            source "$prompt_library"
            prompt_yes_no 'Install test feature?' y
        } <<< $'maybe\nno' 2>&1)"
        if ! grep -F 'Please enter yes, y, no, n' <<< "$output" >/dev/null; then
            printf '%s prompt did not warn and retry invalid yes/no input.\n' "$terminal_name" >&2
            exit 1
        fi
    done
}

assert_nopasswd_prompts_retry() {
    local output
    output="$(env -u SUDO_USER bash "$repository_root/packages/scripts/utils/toggle_nopasswd_sudo.sh" <<< $'missing-toolbox-user\nroot\nmaybe\nno' 2>&1)"
    if ! grep -F "does not exist" <<< "$output" >/dev/null ||
       ! grep -F 'Enter yes, y, no, n' <<< "$output" >/dev/null; then
        printf '%s\n' 'toggle-nopasswd-sudo did not retry invalid user and confirmation input.' >&2
        exit 1
    fi
}

assert_wsl_yes_no_retries
assert_setup_venv_retries
assert_default_cwd_retries
assert_desktop_terminal_prompts_retry
if [[ $(id -u) -eq 0 ]]; then
    assert_nopasswd_prompts_retry
fi

printf '%s\n' 'Terminal setup checks passed.'
