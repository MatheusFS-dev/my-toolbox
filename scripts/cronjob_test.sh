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
printf '%s\n' '#!/bin/sh' 'printf "claude %s\n" "$*" >> "$CRON_TEST_ROOT/calls"' 'echo Claude hello' 'exit "${CLAUDE_TEST_EXIT:-0}"' > "$test_root/bin/claude"
printf '%s\n' '#!/bin/sh' 'printf "agy %s\n" "$*" >> "$CRON_TEST_ROOT/calls"' 'echo Antigravity hello' 'exit "${AGY_TEST_EXIT:-0}"' > "$test_root/bin/agy"
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

# Each client owns a separate data directory, state directory, wrapper, and cron block.
claude_script="$repository_root/packages/macros/cronjob/claude_hi.sh"
agy_script="$repository_root/packages/macros/cronjob/agy_hi.sh"
jq -e '.arguments == [{"id":"time","prompt":"Daily local time (HH:MM)","type":"time_24h","default":"07:00"}]' "$repository_root/packages/macros/cronjob/claude_hi.json" >/dev/null
jq -e '.arguments == [{"id":"time","prompt":"Daily local time (HH:MM)","type":"time_24h","default":"07:00"}]' "$repository_root/packages/macros/cronjob/agy_hi.json" >/dev/null
rm -f "$test_root/calls"
sh "$script" 06:45
sh "$claude_script" 07:00
sh "$agy_script" 08:30
[ ! -e "$test_root/calls" ]
[ -x "$HOME/.local/bin/claude-hi" ]
[ -x "$HOME/.local/bin/agy-hi" ]
[ -x "$HOME/.local/bin/codex-hi" ]
[ -f "$XDG_DATA_HOME/codex-hi/config" ]
[ -f "$XDG_DATA_HOME/claude-hi/config" ]
[ -f "$XDG_DATA_HOME/agy-hi/config" ]
[ -f "$XDG_STATE_HOME/my-toolbox/claude-hi/events.log" ]
[ -f "$XDG_STATE_HOME/my-toolbox/agy-hi/events.log" ]
grep '^0 7 \* \* \* ' "$test_root/crontab"
grep '^30 8 \* \* \* ' "$test_root/crontab"
grep '^45 6 \* \* \* ' "$test_root/crontab"
grep -Fx '0 12 * * * echo unrelated' "$test_root/crontab"

# Reconfiguration changes only the named client's cron entry.
rm -f "$test_root/calls"
sh "$claude_script" 09:15
[ ! -e "$test_root/calls" ]
[ "$(grep -c '^# >>> my-toolbox claude-hi >>>$' "$test_root/crontab")" -eq 1 ]
grep '^15 9 \* \* \* ' "$test_root/crontab"
grep '^30 8 \* \* \* ' "$test_root/crontab"
if sh "$agy_script" 24:00; then exit 1; fi

# A shared lifecycle lock serializes every client's crontab transaction.
cp "$test_root/crontab" "$test_root/before-lifecycle-lock"
(
    flock 7
    for locked_client in codex claude agy; do
        case "$locked_client" in
            codex) locked_script=$script; locked_time=06:50;;
            claude) locked_script=$claude_script; locked_time=09:20;;
            agy) locked_script=$agy_script; locked_time=08:35;;
        esac
        if sh "$locked_script" "$locked_time"; then exit 1; fi
        cmp "$test_root/before-lifecycle-lock" "$test_root/crontab"
    done
) 7> "$XDG_STATE_HOME/my-toolbox/cronjob.lifecycle.lock"

# Runtime uses the exact client commands and records success, error, and waiting events.
"$HOME/.local/bin/claude-hi" --run
grep -Fx 'claude --print --no-session-persistence --permission-mode plan Hi' "$test_root/calls"
grep 'SUCCESS: Claude exited successfully.' "$XDG_STATE_HOME/my-toolbox/claude-hi/events.log"
grep 'WAITING: next run ' "$XDG_STATE_HOME/my-toolbox/claude-hi/events.log"
if CLAUDE_TEST_EXIT=7 "$HOME/.local/bin/claude-hi" --run; then exit 1; fi
grep 'ERROR: Claude exit 7' "$XDG_STATE_HOME/my-toolbox/claude-hi/events.log"
"$HOME/.local/bin/agy-hi" --run
grep -Fx 'agy --print --mode plan Hi' "$test_root/calls"
grep 'SUCCESS: Antigravity exited successfully.' "$XDG_STATE_HOME/my-toolbox/agy-hi/events.log"
grep 'WAITING: next run ' "$XDG_STATE_HOME/my-toolbox/agy-hi/events.log"
if AGY_TEST_EXIT=9 "$HOME/.local/bin/agy-hi" --run; then exit 1; fi
grep 'ERROR: Antigravity exit 9' "$XDG_STATE_HOME/my-toolbox/agy-hi/events.log"
timeout 2 "$HOME/.local/bin/claude-hi" > "$test_root/claude-view" || [ "$?" -eq 124 ]
grep 'SCHEDULE:' "$test_root/claude-view"
grep 'WAITING:' "$test_root/claude-view"
if grep -v '^\[[0-9][0-9][0-9][0-9]-' "$test_root/claude-view"; then exit 1; fi

# Uninstall uses the same shared lifecycle lock before replacing the crontab.
cp "$test_root/crontab" "$test_root/before-uninstall-lifecycle-lock"
(
    flock 7
    for locked_wrapper in codex-hi claude-hi agy-hi; do
        if "$HOME/.local/bin/$locked_wrapper" --uninstall; then exit 1; fi
        cmp "$test_root/before-uninstall-lifecycle-lock" "$test_root/crontab"
    done
) 7> "$XDG_STATE_HOME/my-toolbox/cronjob.lifecycle.lock"

# Uninstalling either client leaves the other client and unrelated cron entries intact.
"$HOME/.local/bin/claude-hi" --uninstall
[ ! -e "$HOME/.local/bin/claude-hi" ]
[ ! -e "$XDG_DATA_HOME/claude-hi" ]
[ ! -e "$XDG_STATE_HOME/my-toolbox/claude-hi" ]
[ -x "$HOME/.local/bin/agy-hi" ]
[ -x "$HOME/.local/bin/codex-hi" ]
grep '^30 8 \* \* \* ' "$test_root/crontab"
grep '^45 6 \* \* \* ' "$test_root/crontab"
sh "$claude_script" 09:15
"$HOME/.local/bin/agy-hi" --uninstall
[ ! -e "$HOME/.local/bin/agy-hi" ]
[ ! -e "$XDG_DATA_HOME/agy-hi" ]
[ ! -e "$XDG_STATE_HOME/my-toolbox/agy-hi" ]
[ -x "$HOME/.local/bin/claude-hi" ]
[ -x "$HOME/.local/bin/codex-hi" ]
grep '^15 9 \* \* \* ' "$test_root/crontab"
grep '^45 6 \* \* \* ' "$test_root/crontab"
sh "$agy_script" 08:30
"$HOME/.local/bin/codex-hi" --uninstall
[ ! -e "$HOME/.local/bin/codex-hi" ]
[ ! -e "$XDG_DATA_HOME/codex-hi" ]
[ ! -e "$XDG_STATE_HOME/my-toolbox/codex-hi" ]
[ -x "$HOME/.local/bin/claude-hi" ]
[ -x "$HOME/.local/bin/agy-hi" ]
grep '^15 9 \* \* \* ' "$test_root/crontab"
grep '^30 8 \* \* \* ' "$test_root/crontab"
"$HOME/.local/bin/claude-hi" --uninstall
"$HOME/.local/bin/agy-hi" --uninstall
[ "$(cat "$test_root/crontab")" = "$(printf '%s\n' '# unrelated job' '0 12 * * * echo unrelated')" ]
printf 'Cronjob integration checks passed.\n'
