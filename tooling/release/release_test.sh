#!/bin/sh
set -eu

# shellcheck disable=SC1007
repository_root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
controller="$repository_root/tooling/release/release.sh"
test_root=$(mktemp -d)
trap 'rm -rf "$test_root"' EXIT HUP INT TERM

mkdir -p "$test_root/bin"
command_log="$test_root/commands"

cat > "$test_root/bin/git" <<'SH'
#!/bin/sh
set -eu
printf 'git %s\n' "$*" >> "$TB_RELEASE_TEST_LOG"
case "$1 ${2:-}" in
    'branch --show-current')
        if [ "${TB_RELEASE_TEST_SCENARIO:-success}" = non-main ]; then printf 'feature\n'; else printf 'main\n'; fi
        ;;
    'rev-parse HEAD') printf '%s\n' 0123456789012345678901234567890123456789 ;;
    'rev-parse origin/main')
        if [ "${TB_RELEASE_TEST_SCENARIO:-success}" = stale ]; then
            printf '%s\n' abcdefabcdefabcdefabcdefabcdefabcdefabcd
        else
            printf '%s\n' 0123456789012345678901234567890123456789
        fi
        ;;
    'config --get') printf '%s\n' https://github.com/example/toolbox.git ;;
    'diff --cached')
        [ "${TB_RELEASE_TEST_SCENARIO:-success}" != no-change ] || exit 0
        exit 1
        ;;
    *) : ;;
esac
SH

cat > "$test_root/bin/gh" <<'SH'
#!/bin/sh
set -eu
printf 'gh %s\n' "$*" >> "$TB_RELEASE_TEST_LOG"
scenario=${TB_RELEASE_TEST_SCENARIO:-success}
case "$1 ${2:-}" in
    'auth status') [ "$scenario" != unauthenticated ] || exit 1 ;;
    'api --paginate') [ "$scenario" != already-released ] || printf '%s\n' 0123456789012345678901234567890123456789 ;;
    'run list')
        case "$*" in
            *'workflow CI'*)
                [ "$scenario" != failed-ci ] || { printf '1\tcompleted\tfailure\n'; exit 0; }
                [ "$scenario" != timeout ] || exit 0
                printf '1\tcompleted\tsuccess\n'
                ;;
            *'workflow Dispatch release'*)
                case "$scenario" in missing-dispatch|timeout) exit 0 ;; esac
                printf '2\tcompleted\tsuccess\n'
                ;;
            *'workflow release.yml'*)
                case "$scenario" in missing-release|timeout) exit 0 ;; esac
                printf '3\tcompleted\tsuccess\n'
                ;;
        esac
        ;;
    'run view')
        if [ "$scenario" = failed-ci ] && [ "$3" = 1 ]; then
            printf 'completed\tfailure\tunit\n'
        else
            printf 'completed\tsuccess\t\n'
        fi
        ;;
esac
SH
chmod +x "$test_root/bin/git" "$test_root/bin/gh"

run_controller() {
    scenario=$1
    shift
    TB_RELEASE_TEST_LOG="$command_log" \
        TB_RELEASE_TEST_SCENARIO="$scenario" \
        PATH="$test_root/bin:$PATH" \
        RELEASE_TIMEOUT_SECONDS=0 \
        RELEASE_SLEEP_SECONDS=0 \
        TB_OUTPUT_COLOR=never \
        sh "$controller" "$@" > "$test_root/stdout" 2> "$test_root/stderr"
    return $?
}

expect_status() {
    expected=$1
    shift
    set +e
    run_controller "$@"
    actual=$?
    set -e
    [ "$actual" -eq "$expected" ] || {
        printf 'Expected status %s, got %s for %s.\n' "$expected" "$actual" "$*" >&2
        cat "$test_root/stderr" >&2
        exit 1
    }
}

: > "$command_log"
expect_status 0 success --message 'release relocation'
grep -Fx 'git add -u' "$command_log" >/dev/null
grep -F 'git commit -m release relocation' "$command_log" >/dev/null
grep -F 'git push origin main' "$command_log" >/dev/null
grep -F 'SUCCESS] Released 0123456789012345678901234567890123456789.' "$test_root/stdout" >/dev/null

expect_status 3 no-change --message 'nothing to release'
expect_status 1 non-main --message 'wrong branch'
expect_status 1 stale --message 'stale branch'
expect_status 1 unauthenticated --message 'no auth'
expect_status 1 unauthenticated --watch 0123456789012345678901234567890123456789
expect_status 1 failed-ci --message 'bad CI'
expect_status 2 missing-dispatch --message 'missing chained workflow'
expect_status 2 timeout --message 'timed out'
expect_status 0 already-released --watch 0123456789012345678901234567890123456789
grep -F 'already has a release' "$test_root/stdout" >/dev/null

: > "$command_log"
expect_status 0 success --watch 0123456789012345678901234567890123456789
if grep -E 'git (add|commit|push)' "$command_log" >/dev/null; then
    printf 'Watch-only mode changed repository state.\n' >&2
    exit 1
fi

expect_status 0 success --json --watch 0123456789012345678901234567890123456789
python3 - "$test_root/stdout" <<'PY'
import json
import sys

report = json.load(open(sys.argv[1], encoding="utf-8"))
assert report["result"] == "released"
assert report["sha"] == "0123456789012345678901234567890123456789"
PY
[ ! -s "$test_root/stderr" ] || grep -F 'release INFO]' "$test_root/stderr" >/dev/null

printf 'release controller tests passed\n'
