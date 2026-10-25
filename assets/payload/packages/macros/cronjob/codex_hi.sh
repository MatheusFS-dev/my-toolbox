#!/bin/sh
# my-toolbox codex-hi managed runtime v1
set -eu
umask 077

stamp() { date '+%Y-%m-%dT%H:%M:%S%:z'; }
say() { printf '[%s] %s\n' "$(stamp)" "$*"; }
fail() { say "ERROR: $*" >&2; exit 1; }
quote() { printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"; }
valid_time() {
    case "$1" in [01][0-9]:[0-5][0-9]|2[0-3]:[0-5][0-9]) return 0;; *) return 1;; esac
}

if [ "${1:-}" = --check ]; then
    if [ "$#" -ne 2 ] || ! valid_time "$2"; then fail 'Expected HH:MM (00:00–23:59).'; fi
    exit 0
fi

data_root="${XDG_DATA_HOME:-$HOME/.local/share}/codex-hi"
state_base="${XDG_STATE_HOME:-$HOME/.local/state}/my-toolbox"
state_root="$state_base/codex-hi"
lifecycle_lock="$state_base/cronjob.lifecycle.lock"
wrapper="$HOME/.local/bin/codex-hi"
start_marker='# >>> my-toolbox codex-hi >>>'
end_marker='# <<< my-toolbox codex-hi <<<'
mode=${1:-}

# The installed wrapper supplies a fixed configuration path across cron's sparse environment.
if [ "$mode" = --runtime ]; then
    [ "$#" -ge 2 ] || fail 'Missing configuration.'
    config=$2
    shift 2
    if [ ! -f "$config" ] || [ -L "$config" ]; then fail 'Configuration missing.'; fi
    # shellcheck source=/dev/null
    . "$config"
    state_base=${state_root%/*}
    lifecycle_lock="$state_base/cronjob.lifecycle.lock"
    export PATH HOME
    if [ -n "$saved_codex_home" ]; then CODEX_HOME=$saved_codex_home; export CODEX_HOME; fi
    mode=${1:---watch}
    [ "$#" -le 1 ] || fail 'Usage: codex-hi [--uninstall]'
    case "$mode" in --watch|--run|--uninstall) ;; *) fail 'Usage: codex-hi [--uninstall]';; esac
fi

next_run() {
    next=$(date -d "today $schedule" '+%s')
    if [ "$next" -le "$(date +%s)" ]; then next=$(date -d "tomorrow $schedule" '+%s'); fi
    date -d "@$next" '+%Y-%m-%dT%H:%M:%S%:z'
}
event() { say "$*" >> "$state_root/events.log"; }

# Refuse malformed or duplicated ownership markers before changing a crontab.
read_crontab() {
    if ! LC_ALL=C crontab -l > "$transaction/original" 2> "$transaction/cron-error"; then
        if ! grep -q '^no crontab for ' "$transaction/cron-error"; then
            fail "Cannot read crontab: $(cat "$transaction/cron-error")"
        fi
        : > "$transaction/original"
    fi
    awk -v begin="$start_marker" -v end="$end_marker" '
      $0 == begin { if (inside || seen++) exit 1; inside=1; next }
      $0 == end { if (!inside) exit 1; inside=0; next }
      index($0, begin) || index($0, end) { exit 1 }
      !inside { print }
      END { if (inside) exit 1 }
    ' "$transaction/original" > "$transaction/clean" || fail 'Malformed codex-hi crontab markers.'
}

case "$mode" in
    --run)
        exec 9> "$state_root/run.lock"
        if ! flock -n 9; then event 'OVERLAP: previous run still active; skipped.'; exit 0; fi
        [ ! -f "$state_root/running" ] || event 'ERROR: previous run ended without recording a result.'
        printf '%s\n' "$$" > "$state_root/running"
        # Invoked by the EXIT trap.
        # shellcheck disable=SC2317
        finish() {
            result=$?
            trap - 0
            if [ "$result" -eq 0 ]; then event 'SUCCESS: Codex exited successfully.'
            else event "ERROR: Codex exit $result; log: $state_root/latest-run.log"; fi
            rm -f "$state_root/running"
            event "WAITING: next run $(next_run)."
            exit "$result"
        }
        trap finish 0
        trap 'exit 130' INT
        trap 'exit 143' TERM HUP
        event 'SENT: submitting Hi to Codex.'
        cd "$data_root"
        result=0
        "$codex_path" exec --ephemeral --skip-git-repo-check --sandbox read-only --color never 'Just reply with "Hi" and do nothing else' </dev/null > "$state_root/latest-run.log" 2>&1 || result=$?
        while IFS= read -r line || [ -n "$line" ]; do event "CODEX: $line"; done < "$state_root/latest-run.log"
        exit "$result"
        ;;
    --watch)
        say "SCHEDULE: daily $schedule local time; next run $(next_run)."
        offset=$(wc -l < "$state_root/events.log")
        first=$((offset - 49))
        if [ "$first" -lt 1 ]; then first=1; fi
        if [ "$offset" -gt 0 ]; then sed -n "$first,${offset}p" "$state_root/events.log"; fi
        previous_health=
        trap 'say "Status viewer stopped."; exit 0' INT TERM HUP
        ticks=0
        while [ -f "$data_root/config" ]; do
            if [ "$ticks" -eq 0 ]; then
                old_schedule=$schedule
                # shellcheck source=/dev/null
                . "$data_root/config"
                if [ "$old_schedule" != "$schedule" ]; then say "SCHEDULE: daily $schedule; next run $(next_run)."; fi
                health='OK'
                systemctl is-active --quiet "$cron_unit" || health='cron service inactive'
                systemctl is-enabled --quiet "$cron_unit" || health="$health; cron service disabled"
                if ! crontab -l 2>/dev/null | grep -Fx "$cron_line" >/dev/null; then health="$health; schedule missing or changed"; fi
                if [ "$health" != "$previous_health" ]; then say "CRON: $health"; previous_health=$health; fi
            fi
            count=$(wc -l < "$state_root/events.log")
            if [ "$count" -gt "$offset" ]; then
                sed -n "$((offset + 1)),${count}p" "$state_root/events.log"
                offset=$count
            fi
            ticks=$(((ticks + 1) % 30))
            sleep 1 2>/dev/null
        done
        say 'Installation removed; viewer stopped.'
        exit 0
        ;;
    --uninstall)
        transaction=$(mktemp -d)
        trap 'rm -rf "$transaction"' 0
        exec 8> "$state_root/install.lock"
        flock -n 8 || fail 'Another installation is running.'
        exec 9> "$state_root/run.lock"
        flock -n 9 || fail 'A Codex run is active; retry uninstall when it finishes.'
        exec 7> "$lifecycle_lock"
        flock -n 7 || fail 'Another cronjob lifecycle operation is running.'
        read_crontab
        crontab "$transaction/clean" || fail 'Cannot remove scheduled job.'
        # Delete only exact files belonging to this installation, never a parent tree.
        grep -q '^# my-toolbox codex-hi wrapper v1$' "$wrapper" && rm -f "$wrapper"
        rm -f "$data_root/config" "$data_root/runtime.sh"
        rm -f "$state_root/events.log" "$state_root/latest-run.log" "$state_root/running" "$state_root/run.lock" "$state_root/install.lock"
        rmdir "$data_root" "$state_root" 2>/dev/null || :
        say 'Uninstalled codex-hi and removed its logs; shared cron service left running.'
        exit 0
        ;;
    --*) fail 'Usage: codex-hi [--uninstall]';;
esac

[ "$#" -le 1 ] || fail 'Usage: sh codex_hi.sh [HH:MM]'
schedule=${1:-}
if [ -z "$schedule" ]; then
    printf '[%s] Daily local time HH:MM [07:00]: ' "$(stamp)"
    IFS= read -r schedule || fail 'No time supplied.'
    schedule=${schedule:-07:00}
fi
valid_time "$schedule" || fail 'Expected HH:MM (00:00–23:59).'
[ "$(uname -s)" = Linux ] || fail 'Native systemd Linux is required.'
case "$(uname -r)" in *[Mm]icrosoft*|*WSL*) fail 'WSL is not supported.';; esac
for dependency in systemctl crontab flock date awk sed grep mktemp cp mv chmod tail wc; do
    command -v "$dependency" >/dev/null || fail "Missing dependency: $dependency"
done
codex_path=$(command -v codex) || fail 'Install Codex and log in before configuring this job.'
case "$codex_path" in /*) ;; *) fail 'Codex must resolve to an absolute executable path.';; esac
cron_unit=
for unit in cron.service crond.service; do
    if [ "$(systemctl show -p LoadState --value "$unit" 2>/dev/null)" = loaded ]; then cron_unit=$unit; break; fi
done
[ -n "$cron_unit" ] || fail 'Install a cron service first; no cron.service or crond.service found.'
for path in "$data_root" "$state_root" "$wrapper" "$data_root/config" "$data_root/runtime.sh" "$state_root/events.log" "$state_root/latest-run.log" "$state_root/run.lock" "$state_root/install.lock" "$lifecycle_lock"; do
    [ ! -L "$path" ] || fail "Refusing symbolic link: $path"
done
if [ -e "$wrapper" ]; then
    if [ ! -f "$wrapper" ] || ! grep -q '^# my-toolbox codex-hi wrapper v1$' "$wrapper"; then fail "Unmanaged command: $wrapper"; fi
fi
if [ -e "$data_root" ]; then
    if [ ! -f "$data_root/runtime.sh" ] || ! grep -q '^# my-toolbox codex-hi managed runtime v1$' "$data_root/runtime.sh"; then
        fail "Unmanaged installation directory: $data_root"
    fi
fi
mkdir -p "$data_root" "$state_root" "$(dirname "$wrapper")"
exec 8> "$state_root/install.lock"
flock -n 8 || fail 'Another installation is running.'
exec 9> "$state_root/run.lock"
flock -n 9 || fail 'A Codex run is active; retry configuration later.'
exec 7> "$lifecycle_lock"
flock -n 7 || fail 'Another cronjob lifecycle operation is running.'
transaction=$(mktemp -d "$data_root/.install.XXXXXX")
published=0
committed=0
cleanup() {
    status=$?
    if [ "$published" -eq 1 ] && [ "$committed" -eq 0 ]; then
        for name in config runtime.sh; do
            if [ -f "$transaction/$name.old" ]; then cp "$transaction/$name.old" "$data_root/$name"; else rm -f "$data_root/$name"; fi
        done
        if [ -f "$transaction/wrapper.old" ]; then cp "$transaction/wrapper.old" "$wrapper"; else rm -f "$wrapper"; fi
    fi
    rm -rf "$transaction"
    if [ ! -f "$data_root/config" ]; then rmdir "$data_root" 2>/dev/null || :; fi
    exit "$status"
}
trap cleanup 0
trap 'exit 130' INT
trap 'exit 143' TERM HUP
read_crontab
hour=${schedule%:*}; hour=${hour#0}; minute=${schedule#*:}; minute=${minute#0}
cron_line="$minute $hour * * * $(quote "$wrapper") --run"
# Percent is special even inside shell quotes in a crontab command.
cron_line=$(printf '%s' "$cron_line" | sed 's/%/\\%/g')
case "$wrapper" in *'
'*) fail 'Newlines in installation paths are unsupported.';; esac
for name in config runtime.sh; do [ ! -f "$data_root/$name" ] || cp -p "$data_root/$name" "$transaction/$name.old"; done
[ ! -f "$wrapper" ] || cp -p "$wrapper" "$transaction/wrapper.old"
saved_codex_home=${CODEX_HOME:-}
{
    for name in data_root state_root wrapper schedule cron_unit cron_line codex_path saved_codex_home PATH HOME; do
        case "$name" in
            data_root) value=$data_root;; state_root) value=$state_root;; wrapper) value=$wrapper;;
            schedule) value=$schedule;; cron_unit) value=$cron_unit;; cron_line) value=$cron_line;;
            codex_path) value=$codex_path;; saved_codex_home) value=$saved_codex_home;; PATH) value=$PATH;; HOME) value=$HOME;;
        esac
        printf '%s=%s\n' "$name" "$(quote "$value")"
    done
} > "$transaction/config"
cp "$0" "$transaction/runtime.sh"
{
    printf '%s\n' '#!/bin/sh' '# my-toolbox codex-hi wrapper v1'
    printf 'exec /bin/sh %s --runtime %s "$@"\n' "$(quote "$data_root/runtime.sh")" "$(quote "$data_root/config")"
} > "$transaction/wrapper"
chmod 700 "$transaction/wrapper" "$transaction/runtime.sh"
if ! systemctl is-enabled --quiet "$cron_unit" || ! systemctl is-active --quiet "$cron_unit"; then
    say "Enabling and starting $cron_unit (sudo may request your password)."
    if [ "$(id -u)" -eq 0 ]; then systemctl enable --now "$cron_unit"; else sudo systemctl enable --now "$cron_unit"; fi
fi
if ! systemctl is-enabled --quiet "$cron_unit" || ! systemctl is-active --quiet "$cron_unit"; then fail 'Cron did not become enabled and active.'; fi
published=1
mv "$transaction/config" "$data_root/config"
mv "$transaction/runtime.sh" "$data_root/runtime.sh"
wrapper_stage=$(mktemp "$(dirname "$wrapper")/.codex-hi.XXXXXX")
cp "$transaction/wrapper" "$wrapper_stage"
chmod 700 "$wrapper_stage"
mv "$wrapper_stage" "$wrapper"
printf '%s\n%s\n%s\n' "$start_marker" "$cron_line" "$end_marker" >> "$transaction/clean"
crontab "$transaction/clean" || fail 'Cannot install crontab.'
committed=1
event "WAITING: daily $schedule; next run $(next_run)."
say "Installed daily Codex Hi at $schedule. Run codex-hi to view status."
case ":$PATH:" in *":$HOME/.local/bin:"*) ;; *) say "Add $HOME/.local/bin to PATH to run codex-hi.";; esac
