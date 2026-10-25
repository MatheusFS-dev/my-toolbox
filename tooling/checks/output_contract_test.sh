#!/bin/sh
set -eu
# shellcheck disable=SC1007,SC2016

root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
helper="$root/assets/payload/packages/scripts/lib/toolbox_output.sh"

test_root=$(mktemp -d)
trap 'rm -rf "$test_root"' 0

# shellcheck disable=SC2016
env -u NO_COLOR TB_OUTPUT_CLOCK=09:32:23 TB_OUTPUT_COLOR=256 sh -c '
    . "$1"
    tb_output_init setup-wsl
    tb_info "Checking..."
    tb_prompt "Continue? [Y/n] (default: Y):"
    tb_warn "Optional step skipped."
    tb_error "Unable to write; nothing changed."
    tb_success "Completed successfully."
' sh "$helper" > "$test_root/color"

expected=$(printf '\n[09:32:23 setup-wsl INFO] Checking...\n\033[36m[09:32:23 setup-wsl PROMPT] Continue? [Y/n] (default: Y):\033[0m\n\033[38;5;208m[09:32:23 setup-wsl WARN] Optional step skipped.\033[0m\n\033[31m[09:32:23 setup-wsl ERROR] Unable to write; nothing changed.\033[0m\n\033[32m[09:32:23 setup-wsl SUCCESS] Completed successfully.\033[0m')
[ "$(cat "$test_root/color")" = "$expected" ]

NO_COLOR=1 TB_OUTPUT_CLOCK=09:32:23 TB_OUTPUT_COLOR=256 sh -c '
    . "$1"
    tb_output_init tool
    tb_warn "Plain warning."
' sh "$helper" > "$test_root/plain"
if grep "$(printf '\033')" "$test_root/plain"; then
    exit 1
fi

printf '\n\nvalue\n' | TB_OUTPUT_CLOCK=09:32:23 TB_OUTPUT_COLOR=never sh -c '
    . "$1"
    tb_output_init tool
    tb_ask_required "Required value"
    [ "$TB_ANSWER" = value ]
' sh "$helper" > "$test_root/required"
grep -F '[09:32:23 tool WARN] A value is required; enter a value to continue.' "$test_root/required" >/dev/null

printf '\n' | TB_OUTPUT_CLOCK=09:32:23 TB_OUTPUT_COLOR=never sh -c '
    . "$1"
    tb_output_init tool
    tb_ask_yes_no "Continue?" yes
    [ "$TB_ANSWER" = yes ]
' sh "$helper" > "$test_root/default"
grep -F '[09:32:23 tool PROMPT] Continue? [Y/n] (default: Y):' "$test_root/default" >/dev/null

TB_OUTPUT_CLOCK=09:32:23 TB_OUTPUT_COLOR=never sh -c '
    . "$1"
    tb_output_init tool
    tb_menu "Choose an option" "1. First" "2. Second"
' sh "$helper" > "$test_root/menu"
printf '\n[09:32:23 tool PROMPT] Choose an option\n  1. First\n  2. Second\n' > "$test_root/menu.expected"
cmp "$test_root/menu.expected" "$test_root/menu"

echo 'output contract shell tests passed'
