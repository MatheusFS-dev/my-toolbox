#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=/dev/null
. "$SCRIPT_DIR/../lib/toolbox_output.sh"
tb_output_init set-english-us-locale

TARGET_LOCALE=en_US.UTF-8
TARGET_LANGUAGE=en_US:en
ROOT=
USER_HOME=$HOME
if [[ ${LOCALE_TEST_MODE:-0} == 1 ]]; then
    ROOT=${LOCALE_TEST_ROOT:?}
    USER_HOME=${LOCALE_TEST_HOME:?}
fi
SYSTEM_LOCALE="$ROOT/etc/default/locale"
USER_LOCALE="$USER_HOME/.config/environment.d/90-locale.conf"

if [[ ${1:-} == --help ]]; then
    tb_info 'Usage: set-english-us-locale [status]'
    exit 0
fi
if [[ $# -gt 1 || ( $# -eq 1 && $1 != status ) ]]; then
    tb_error 'Usage: set-english-us-locale [status]' >&2
    exit 64
fi
if [[ ${LOCALE_TEST_MODE:-0} != 1 ]]; then
    # shellcheck disable=SC1091
    . /etc/os-release
    if [[ $ID != ubuntu ]]; then tb_error 'Ubuntu is required; run this tool on Ubuntu.' >&2; exit 1; fi
fi

report=()
if [[ ! -f $SYSTEM_LOCALE ]] || ! grep -qx 'LANG=en_US.UTF-8' "$SYSTEM_LOCALE" || ! grep -qx 'LANGUAGE=en_US:en' "$SYSTEM_LOCALE"; then
    report+=('System locale differs')
fi
if [[ ! -f $USER_LOCALE ]] || ! grep -qx 'LANG=en_US.UTF-8' "$USER_LOCALE" || ! grep -qx 'LANGUAGE=en_US:en' "$USER_LOCALE"; then
    report+=('User session locale differs')
fi
for file in "$USER_HOME/.pam_environment" "$USER_HOME/.profile"; do
    if [[ -f $file ]] && grep -Eq '^[[:space:]]*(export[[:space:]]+)?(LANG|LANGUAGE|LC_[A-Z_]+)[[:space:]]*=' "$file"; then
        report+=("Conflicting override: $file")
    fi
done
if [[ ${LOCALE_TEST_MODE:-0} == 1 ]]; then
    installed=${LOCALE_TEST_PACKAGES:-0}
    generated=${LOCALE_TEST_GENERATED:-0}
    region=${LOCALE_TEST_REGION:-}
else
    installed=1
    for package in locales language-pack-en language-pack-gnome-en; do
        dpkg-query -W -f='${db:Status-Abbrev}' "$package" 2>/dev/null | grep -q '^ii' || installed=0
    done
    generated=0
    locale -a | grep -qi '^en_US\.utf8$' && generated=1
    region=''
    if command -v gsettings >/dev/null && gsettings list-schemas | grep -qx 'org.gnome.system.locale' && gsettings list-keys org.gnome.system.locale | grep -qx region; then
        region=$(gsettings get org.gnome.system.locale region)
    fi
fi
[[ $installed == 1 ]] || report+=('English language packs differ')
[[ $generated == 1 ]] || report+=('Generated en_US.UTF-8 locale differs')
if [[ -n $region && $region != "'en_US.UTF-8'" && $region != en_US.UTF-8 ]]; then report+=('GNOME region differs'); fi

if (( ${#report[@]} )); then
    for message in "${report[@]}"; do tb_info "$message"; done
    action=setup
else
    tb_info 'English US locale is already configured.'
    action=reinstall
fi
tb_info 'A complete logout and new login are needed for session changes.'
[[ ${1:-} == status ]] && exit 0
tb_ask_yes_no "Confirm $action of English US locale?" no || { tb_info 'Cancelled; nothing changed.'; exit 1; }
if [[ $TB_ANSWER != yes ]]; then tb_info 'Cancelled; nothing changed.'; exit 0; fi

tb_info 'Installing locale packages and writing locale configuration...'
if [[ ${LOCALE_TEST_MODE:-0} != 1 ]]; then
    sudo apt-get update
    sudo apt-get install -y locales language-pack-en language-pack-gnome-en
    sudo locale-gen "$TARGET_LOCALE"
    sudo update-locale --reset LANG="$TARGET_LOCALE" LANGUAGE="$TARGET_LANGUAGE"
    if command -v localectl >/dev/null; then sudo localectl set-locale LANG="$TARGET_LOCALE" LANGUAGE="$TARGET_LANGUAGE"; fi
else
    mkdir -p "$(dirname "$SYSTEM_LOCALE")"
    printf 'LANG=%s\nLANGUAGE=%s\n' "$TARGET_LOCALE" "$TARGET_LANGUAGE" > "$SYSTEM_LOCALE"
fi
for file in "$USER_HOME/.pam_environment" "$USER_HOME/.profile"; do
    if [[ -f $file ]] && grep -Eq '^[[:space:]]*(export[[:space:]]+)?(LANG|LANGUAGE|LC_[A-Z_]+)[[:space:]]*=' "$file"; then
        cp -p -- "$file" "${file}.locale-backup"
        sed -i -E '/^[[:space:]]*(export[[:space:]]+)?(LANG|LANGUAGE|LC_[A-Z_]+)[[:space:]]*=/d' "$file"
    fi
done
mkdir -p "$(dirname "$USER_LOCALE")"
if [[ -f $USER_LOCALE ]]; then cp -p -- "$USER_LOCALE" "${USER_LOCALE}.locale-backup"; fi
printf 'LANG=%s\nLANGUAGE=%s\n' "$TARGET_LOCALE" "$TARGET_LANGUAGE" > "$USER_LOCALE"
if [[ ${LOCALE_TEST_MODE:-0} != 1 && -n $region ]]; then gsettings set org.gnome.system.locale region "$TARGET_LOCALE"; fi
tb_success 'English US locale configured successfully. Log out completely and log back in.'
