#!/bin/sh

# Commit the prepared tracked changes, then observe the CI-gated release chain.
# --watch is deliberately read-only so automation can observe submodule updates.

set -u

# shellcheck disable=SC1007
repository_root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
# shellcheck source=../../assets/payload/packages/scripts/lib/toolbox_output.sh
# shellcheck disable=SC1091
. "$repository_root/assets/payload/packages/scripts/lib/toolbox_output.sh"

git_bin=${GIT_BIN:-git}
gh_bin=${GH_BIN:-gh}
timeout_seconds=${RELEASE_TIMEOUT_SECONDS:-2700}
sleep_seconds=${RELEASE_SLEEP_SECONDS:-10}
message=
watch_sha=
json=false
report_result=
report_sha=

usage() {
    printf 'Usage: tooling/release/release.sh [--json] (--message TEXT | --watch SHA)\n' >&2
}

json_escape() {
    printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g; s/\r/\\r/g; s/\n/\\n/g'
}

write_report() {
    [ "$json" = true ] || return 0
    printf '{"result":"%s","sha":"%s"}\n' \
        "$(json_escape "$report_result")" "$(json_escape "$report_sha")" >&3
}

finish() {
    exit_code=$1
    report_result=$2
    report_sha=${3:-$report_sha}
    write_report
    exit "$exit_code"
}

fail() {
    tb_error "$1"
    finish 1 failure
}

timed_out() {
    tb_warn "$1"
    finish 2 incomplete
}

valid_sha() {
    case "$1" in
        ????????????????????????????????????????)
            case "$1" in *[!0123456789abcdef]*) return 1 ;; esac
            return 0
            ;;
    esac
    return 1
}

remote_repository() {
    remote_url=$($git_bin config --get remote.origin.url 2>/dev/null) || return 1
    case "$remote_url" in
        https://github.com/*/*.git) remote_url=${remote_url#https://github.com/}; remote_url=${remote_url%.git} ;;
        https://github.com/*/*) remote_url=${remote_url#https://github.com/} ;;
        git@github.com:*/*.git) remote_url=${remote_url#git@github.com:}; remote_url=${remote_url%.git} ;;
        git@github.com:*/*) remote_url=${remote_url#git@github.com:} ;;
        *) return 1 ;;
    esac
    printf '%s\n' "$remote_url"
}

is_released() {
    released_shas=$($gh_bin api --paginate "repos/$repository/releases?per_page=100" \
        --jq '.[].target_commitish' 2>/dev/null) || return 2
    printf '%s\n' "$released_shas" | grep -Fx "$1" >/dev/null
}

run_for() {
    $gh_bin run list --repo "$repository" --workflow "$1" --commit "$report_sha" --limit 20 \
        --json databaseId,status,conclusion --jq '.[0] | [.databaseId, .status, .conclusion] | @tsv' 2>/dev/null
}

inspect_run() {
    run_id=$1
    run_state=$($gh_bin run view "$run_id" --repo "$repository" --json status,conclusion,jobs \
        --jq '[.status, .conclusion, ([.jobs[] | select(.status == "completed" and .conclusion != "success") | .name] | join(","))] | @tsv' 2>/dev/null) || return 2
    IFS='	' read -r state_status state_conclusion failed_jobs <<EOF
$run_state
EOF
    [ "$state_status" = completed ] || return 3
    case "$state_conclusion" in
        failure|cancelled|timed_out|action_required|startup_failure|skipped)
            tb_error "$2 run $run_id finished with $state_conclusion${failed_jobs:+ (failed jobs: $failed_jobs)}."
            return 1
            ;;
    esac
    [ -z "$failed_jobs" ] || {
        tb_error "$2 run $run_id has failed jobs: $failed_jobs."
        return 1
    }
    [ "$state_status" = completed ] && [ "$state_conclusion" = success ]
}

wait_for_workflow() {
    workflow=$1
    started_at=$(date +%s)
    tb_info "Waiting for $workflow for $report_sha."
    while :; do
        run=$(run_for "$workflow")
        query_status=$?
        [ "$query_status" -eq 0 ] || fail "Could not query $workflow runs."
        if [ -n "$run" ] && [ "$run" != null ]; then
            run_id=${run%%	*}
            case "$run_id" in
                ''|null|*[!0-9]*) ;;
                *)
                    inspect_run "$run_id" "$workflow"
                    inspect_status=$?
                    if [ "$inspect_status" -eq 0 ]; then
                        tb_info "$workflow run $run_id succeeded."
                        return 0
                    fi
                    [ "$inspect_status" -eq 1 ] && finish 1 failure
                    [ "$inspect_status" -eq 3 ] || fail "Could not inspect $workflow run $run_id."
                    ;;
            esac
        fi
        now=$(date +%s)
        elapsed=$((now - started_at))
        if [ "$elapsed" -ge "$timeout_seconds" ]; then
            timed_out "Timed out after ${timeout_seconds}s waiting for $workflow for $report_sha."
        fi
        sleep "$sleep_seconds"
    done
}

watch_release() {
    valid_sha "$report_sha" || fail "Watch SHA must be a lowercase 40-character commit SHA."
    repository=$(remote_repository) || fail "origin must be a GitHub repository URL."
    is_released "$report_sha"
    release_check_status=$?
    if [ "$release_check_status" -eq 0 ]; then
        tb_success "Commit $report_sha already has a release."
        finish 0 already_released
    fi
    [ "$release_check_status" -eq 1 ] || fail "Could not query GitHub releases."
    wait_for_workflow CI
    wait_for_workflow 'Dispatch release'
    wait_for_workflow release.yml
    tb_success "Released $report_sha."
    finish 0 released
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --message)
            [ "$#" -ge 2 ] || { usage; exit 1; }
            message=$2
            shift 2
            ;;
        --watch)
            [ "$#" -ge 2 ] || { usage; exit 1; }
            watch_sha=$2
            shift 2
            ;;
        --json) json=true; shift ;;
        --help|-h) usage; exit 0 ;;
        *) usage; exit 1 ;;
    esac
done

if [ "$json" = true ]; then
    exec 3>&1
    exec 1>&2
fi
tb_output_init release

if [ -n "$message" ] && [ -n "$watch_sha" ]; then
    fail 'Use either --message or --watch, not both.'
fi
if [ -z "$message" ] && [ -z "$watch_sha" ]; then
    fail 'One of --message or --watch is required.'
fi

$gh_bin auth status >/dev/null 2>&1 || fail 'GitHub CLI authentication is required.'

if [ -n "$watch_sha" ]; then
    report_sha=$watch_sha
    watch_release
fi

[ -n "$message" ] || fail 'Release message cannot be empty.'
branch=$($git_bin branch --show-current 2>/dev/null) || fail 'Could not determine the current branch.'
[ "$branch" = main ] || fail 'Release controller only runs on main.'
head_sha=$($git_bin rev-parse HEAD 2>/dev/null) || fail 'Could not resolve HEAD.'
origin_sha=$($git_bin rev-parse origin/main 2>/dev/null) || fail 'Could not resolve origin/main.'
[ "$head_sha" = "$origin_sha" ] || fail 'Local main must exactly match origin/main before release.'

tb_info 'Staging tracked changes, committing, and pushing main.'
$git_bin add -u || fail 'Could not stage tracked changes.'
no_changes=false
if $git_bin diff --cached --quiet; then
    no_changes=true
fi
tb_info 'Publishing recorded submodule commits before the parent release.'
submodule_paths=$($git_bin config -f "$repository_root/.gitmodules" --get-regexp '^submodule\..*\.path$') || fail 'Could not read submodule paths.'
while IFS=' ' read -r submodule_key submodule_path; do
    [ -n "$submodule_path" ] || continue
    submodule_name=${submodule_key%.path}
    submodule_url=$($git_bin config -f "$repository_root/.gitmodules" --get "$submodule_name.url") || fail "Could not resolve the URL for $submodule_path."
    submodule_sha=$($git_bin rev-parse ":$submodule_path") || fail "Could not resolve the staged commit for $submodule_path."
    $git_bin -C "$repository_root/$submodule_path" fetch "$submodule_url" refs/heads/main >/dev/null || fail "Could not fetch main for $submodule_path."
    if ! $git_bin -C "$repository_root/$submodule_path" merge-base --is-ancestor "$submodule_sha" FETCH_HEAD; then
        $git_bin -C "$repository_root/$submodule_path" push "$submodule_url" "$submodule_sha:refs/heads/main" >/dev/null || fail "Could not publish $submodule_path."
    fi
done <<EOF
$submodule_paths
EOF
if [ "$no_changes" = true ]; then
    tb_warn 'No tracked changes are staged for release.'
    finish 3 no_changes "$head_sha"
fi
$git_bin commit -m "$message" >/dev/null || fail 'Could not create the release commit.'
$git_bin push origin main >/dev/null || fail 'Could not push main.'
report_sha=$($git_bin rev-parse HEAD 2>/dev/null) || fail 'Could not resolve the pushed commit.'
watch_release
