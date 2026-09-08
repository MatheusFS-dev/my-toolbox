#!/bin/sh
# Run with sh. --check validates arguments without sleeping or sending input.
set -eu

check_only=false
if [ "${1:-}" = --check ]; then
    check_only=true
    shift
fi
if [ "$#" -gt 2 ]; then
    printf 'Usage: sh %s [key] [delay_ms]\n' "$0" >&2
    exit 1
fi
key=${1:-Enter}
delay_ms=${2:-12000000}
case "$key" in
    Enter|enter|Return|return) code=28 ;;
    Space|space) code=57 ;;
    Tab|tab) code=15 ;;
    Escape|escape|Esc|esc) code=1 ;;
    Backspace|backspace) code=14 ;;
    Delete|delete|Del|del) code=111 ;;
    Up|up) code=103 ;;
    Down|down) code=108 ;;
    Left|left) code=105 ;;
    Right|right) code=106 ;;
    Home|home) code=102 ;;
    End|end) code=107 ;;
    PageUp|pageup) code=104 ;;
    PageDown|pagedown) code=109 ;;
    ''|*[!0-9]*) printf 'Unsupported key: %s. Use Enter, Space, Tab, Escape, arrow/navigation keys, or a Linux keycode (1-767).\n' "$key" >&2; exit 1 ;;
    *)
        code=$(printf '%s' "$key" | sed 's/^0*//')
        if [ -z "$code" ] || [ "${#code}" -gt 3 ] || [ "$code" -gt 767 ]; then
            printf 'Linux keycode must be between 1 and 767.\n' >&2
            exit 1
        fi
        ;;
esac
case "$delay_ms" in
    ''|*[!0-9]*) printf 'Delay must be a non-negative integer in milliseconds.\n' >&2; exit 1 ;;
esac
# Bound arithmetic to the signed 64-bit range, matching tb's argument validator.
delay_ms=$(printf '%s' "$delay_ms" | sed 's/^0*//')
delay_ms=${delay_ms:-0}
if [ "${#delay_ms}" -gt 19 ] || ! [ "$delay_ms" -le 9223372036854775807 ] 2>/dev/null; then
    printf 'Delay is too large (maximum 9223372036854775807 ms).\n' >&2
    exit 1
fi
if "$check_only"; then
    exit 0
fi
seconds=$((delay_ms / 1000))
milliseconds=$((delay_ms % 1000))
sleep "$(printf '%d.%03d' "$seconds" "$milliseconds")"
exec ydotool key "$code:1" "$code:0"
