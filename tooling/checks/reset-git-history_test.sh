#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
script="$root/assets/payload/packages/scripts/utils/reset_git_history.sh"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
git init -q --bare "$tmp/remote.git"
git init -q "$tmp/repo"
git -C "$tmp/repo" config user.name Test
git -C "$tmp/repo" config user.email test@example.com
printf 'first\n' > "$tmp/repo/file"
git -C "$tmp/repo" add file
git -C "$tmp/repo" commit -qm first
git -C "$tmp/repo" remote add origin "$tmp/remote.git"
git -C "$tmp/repo" push -q -u origin HEAD
old=$(git -C "$tmp/repo" rev-parse HEAD)
printf '\n%s\nno\n' "$tmp/repo" | NO_COLOR=1 bash "$script" > "$tmp/out" || true
grep -q 'A value is required' "$tmp/out"
grep -q 'Cancelled' "$tmp/out"
[ "$(git -C "$tmp/repo" rev-parse HEAD)" = "$old" ]
printf 'changed\n' > "$tmp/repo/file"
printf '%s\nRESET\n' "$tmp/repo" | NO_COLOR=1 bash "$script" > "$tmp/out"
grep -q 'reset-git-history SUCCESS' "$tmp/out"
[ "$(git -C "$tmp/repo" rev-list --count HEAD)" = 1 ]
[ "$(git -C "$tmp/repo" rev-parse HEAD)" = "$(git --git-dir="$tmp/remote.git" rev-parse "$(git -C "$tmp/repo" symbolic-ref HEAD)")" ]
