#!/bin/sh
# Shell fixture expressions intentionally expand only when executed.
# shellcheck disable=SC2016
set -eu
repository_root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
test_root=$(mktemp -d)
trap 'rm -rf "$test_root"' 0
mkdir -p "$test_root/bin" "$test_root/user's home"
export HOME="$test_root/user's home" XDG_DATA_HOME="$test_root/data" XDG_STATE_HOME="$test_root/state"
export CRON_TEST_ROOT="$test_root"
export PATH="$test_root/bin:$PATH"
# Fixtures replace only external machine services and the paid Codex request.
printf '%s\n' '#!/bin/sh' 'case "$1" in -s) echo Linux;; -r) echo 6.8.0;; esac' > "$test_root/bin/uname"
printf '%s\n' '#!/bin/sh' 'case "$1" in show) echo loaded;; is-enabled|is-active) test -f "$CRON_TEST_ROOT/enabled";; enable) touch "$CRON_TEST_ROOT/enabled";; *) exit 1;; esac' > "$test_root/bin/systemctl"
printf '%s\n' '#!/bin/sh' 'exec "$@"' > "$test_root/bin/sudo"
printf '%s\n' '#!/bin/sh' 'if [ "$1" = -l ]; then cat "$CRON_TEST_ROOT/crontab"; else cp "$1" "$CRON_TEST_ROOT/crontab"; fi' > "$test_root/bin/crontab"
printf '%s\n' '#!/bin/sh' 'printf "%s\n" "$*" >> "$CRON_TEST_ROOT/calls"' 'echo Hello' 'exit "${CRON_TEST_EXIT:-0}"' > "$test_root/bin/codex"
chmod +x "$test_root/bin/"*
printf '%s\n' '# unrelated job' '0 12 * * * echo unrelated' > "$test_root/crontab"
script="$repository_root/packages/macros/cronjob/codex_hi.sh"
sh "$script" 07:00
[ ! -e "$test_root/calls" ]
[ -f "$test_root/enabled" ]
[ -x "$HOME/.local/bin/codex-hi" ]
cp "$test_root/crontab" "$test_root/before-unknown"
if "$HOME/.local/bin/codex-hi" 09:00; then exit 1; fi
cmp "$test_root/before-unknown" "$test_root/crontab"
grep '^0 7 \* \* \* ' "$test_root/crontab"
sh "$script" 08:30
[ "$(grep -c '^# >>> my-toolbox codex-hi >>>$' "$test_root/crontab")" -eq 1 ]
grep '^30 8 \* \* \* ' "$test_root/crontab"
grep -Fx '0 12 * * * echo unrelated' "$test_root/crontab"
# Invalid times and malformed ownership markers cannot modify the existing schedule.
cp "$test_root/crontab" "$test_root/saved"
if sh "$script" 24:00; then exit 1; fi
cmp "$test_root/saved" "$test_root/crontab"
printf '%s\n' '# >>> my-toolbox codex-hi >>>' >> "$test_root/crontab"
cp "$test_root/crontab" "$test_root/malformed"
if sh "$script" 09:00; then exit 1; fi
cmp "$test_root/malformed" "$test_root/crontab"
cp "$test_root/saved" "$test_root/crontab"
# Publication failure restores the working configuration.
cp "$test_root/bin/crontab" "$test_root/crontab-command"
printf '%s\n' '#!/bin/sh' 'if [ "$1" = -l ]; then cat "$CRON_TEST_ROOT/crontab"; else exit 1; fi' > "$test_root/bin/crontab"
cp "$XDG_DATA_HOME/codex-hi/config" "$test_root/config-saved"
if sh "$script" 09:00; then exit 1; fi
cmp "$test_root/config-saved" "$XDG_DATA_HOME/codex-hi/config"
cp "$test_root/crontab-command" "$test_root/bin/crontab"
# Holding the real lock prevents a second submission.
(
    flock 7
    "$HOME/.local/bin/codex-hi" --run
) 7> "$XDG_STATE_HOME/my-toolbox/codex-hi/run.lock"
[ ! -e "$test_root/calls" ]
grep 'OVERLAP:' "$XDG_STATE_HOME/my-toolbox/codex-hi/events.log"
"$HOME/.local/bin/codex-hi" --run
grep 'SUCCESS:' "$XDG_STATE_HOME/my-toolbox/codex-hi/events.log"
grep -Fx 'exec --ephemeral --skip-git-repo-check --sandbox read-only --color never Hi' "$test_root/calls"
if CRON_TEST_EXIT=7 "$HOME/.local/bin/codex-hi" --run; then exit 1; fi
grep 'ERROR: Codex exit 7' "$XDG_STATE_HOME/my-toolbox/codex-hi/events.log"
timeout 2 "$HOME/.local/bin/codex-hi" > "$test_root/view" || [ "$?" -eq 124 ]
grep 'SCHEDULE:' "$test_root/view"
grep 'WAITING:' "$test_root/view"
if grep -v '^\[[0-9][0-9][0-9][0-9]-' "$test_root/view"; then exit 1; fi
"$HOME/.local/bin/codex-hi" --uninstall
[ ! -e "$HOME/.local/bin/codex-hi" ]
[ ! -e "$XDG_DATA_HOME/codex-hi" ]
[ ! -e "$XDG_STATE_HOME/my-toolbox/codex-hi" ]
[ "$(cat "$test_root/crontab")" = "$(printf '%s\n' '# unrelated job' '0 12 * * * echo unrelated')" ]
printf 'Cronjob integration checks passed.\n'
