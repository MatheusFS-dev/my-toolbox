#!/usr/bin/env bash
set -euo pipefail

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
    echo 'Usage: set-english-us-locale [status]'
    exit 0
fi
if [[ $# -gt 1 || ( $# -eq 1 && $1 != status ) ]]; then
    echo 'Usage: set-english-us-locale [status]' >&2
    exit 64
fi
if [[ ${LOCALE_TEST_MODE:-0} != 1 ]]; then
    . /etc/os-release
    if [[ $ID != ubuntu ]]; then echo 'Ubuntu is required.' >&2; exit 1; fi
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
    printf '%s\n' "${report[@]}"
    action=setup
else
    echo 'English US locale is already configured.'
    action=reinstall
fi
echo 'A complete logout and new login are needed for session changes.'
[[ ${1:-} == status ]] && exit 0
printf 'Confirm %s of English US locale? [y/N]: ' "$action"
read -r answer || exit 1
case "$answer" in y|Y|yes|YES) ;; *) echo 'Canceled; no changes made.'; exit 0 ;; esac

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
echo 'Done. Log out completely and log back in.'
