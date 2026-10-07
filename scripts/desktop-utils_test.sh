#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/home/.config/environment.d" "$tmp/root/etc/default" "$tmp/rules"
locale_script="$root/packages/scripts/utils/set_english_us_locale.sh"
polkit_script="$root/packages/scripts/utils/toggle_polkit_prompts.sh"
export LOCALE_TEST_MODE=1 LOCALE_TEST_ROOT="$tmp/root" LOCALE_TEST_HOME="$tmp/home"
export LOCALE_TEST_PACKAGES=0 LOCALE_TEST_GENERATED=0 LOCALE_TEST_REGION="'de_DE.UTF-8'"
bash "$locale_script" status > "$tmp/out"
grep -q 'System locale differs' "$tmp/out"
grep -q 'English language packs differ' "$tmp/out"
printf '\n' | bash "$locale_script" > "$tmp/out"
test ! -e "$tmp/root/etc/default/locale"
printf 'export LANG=de_DE.UTF-8\nkeep=yes\n' > "$tmp/home/.profile"
printf 'y\n' | bash "$locale_script" > "$tmp/out"
grep -q 'keep=yes' "$tmp/home/.profile"
test -f "$tmp/home/.profile.locale-backup"
grep -q 'LANG=en_US.UTF-8' "$tmp/root/etc/default/locale"
export LOCALE_TEST_PACKAGES=1 LOCALE_TEST_GENERATED=1 LOCALE_TEST_REGION="'en_US.UTF-8'"
printf 'n\n' | bash "$locale_script" > "$tmp/out"
grep -q 'Confirm reinstall' "$tmp/out"
printf 'y\n' | bash "$locale_script" > "$tmp/out"
grep -q 'Done.' "$tmp/out"
export POLKIT_TOGGLE_TEST_MODE=1 POLKIT_TOGGLE_RULE_DIR="$tmp/rules" POLKIT_TOGGLE_TEST_USER=testuser
bash "$polkit_script" status > "$tmp/out"
grep -q 'ENABLED' "$tmp/out"
printf '\n' | bash "$polkit_script" > "$tmp/out"
test ! -e "$tmp/rules/00-current-user-no-auth.rules"
printf 'y\n' | bash "$polkit_script" > "$tmp/out"
test -f "$tmp/rules/00-current-user-no-auth.rules"
grep -q 'DISABLED' "$tmp/out"
printf 'y\n' | bash "$polkit_script" > "$tmp/out"
test ! -e "$tmp/rules/00-current-user-no-auth.rules"
echo unexpected > "$tmp/rules/00-current-user-no-auth.rules"
if printf 'y\n' | bash "$polkit_script" > "$tmp/out" 2>&1; then exit 1; fi
grep -q 'CONFLICT' "$tmp/out"
grep -q unexpected "$tmp/rules/00-current-user-no-auth.rules"
echo 'desktop utility tests passed'
