#!/usr/bin/env bash
# Replace the current branch's local and origin history with one commit.
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
source "$SCRIPT_DIR/../lib/toolbox_output.sh"
tb_output_init reset-git-history

if [ "$#" -ne 0 ]; then
    tb_error 'Run this tool without arguments.' >&2
    exit 1
fi
tb_ask_required 'Path to repository root (required)' || exit 1
repo=$TB_ANSWER
root=$(git -C "$repo" rev-parse --show-toplevel)
cd "$root"
branch=$(git symbolic-ref -q HEAD) || {
    tb_error 'Check out a branch before running this tool.' >&2
    exit 1
}
name=${branch#refs/heads/}
if [ "$(git worktree list --porcelain | grep -c '^worktree ')" -ne 1 ]; then
    tb_error 'Remove linked worktrees before rewriting history.' >&2
    exit 1
fi
git var GIT_AUTHOR_IDENT >/dev/null
git var GIT_COMMITTER_IDENT >/dev/null
remote_url=$(git remote get-url --push origin)
tb_info 'Checking the origin branch before rewriting history...'
remote_line=$(git ls-remote --heads origin "$branch")
remote_oid=${remote_line%%[[:space:]]*}
tb_warn 'DANGER: This permanently rewrites Git history and cannot be undone without an independent backup.'
tb_warn "Repository: $root"
tb_warn "Branch: $name"
tb_warn "Push destination: $remote_url"
tb_warn 'The current working tree will become one commit and be force-pushed. Other local branches, tags, refs and reflogs will be deleted.'
tb_warn 'Other remote branches, tags and GitHub cached data are not removed.'
tb_prompt 'Type RESET to permanently rewrite this history (default: cancel):'
read -r confirmation || exit 1
if [ "$confirmation" != RESET ]; then
    tb_info 'Cancelled. No changes made.'
    exit 1
fi
tb_info 'Rewriting local history and pushing the new commit...'
git add -A
tree=$(git write-tree)
commit=$(printf 'First commit\n' | git commit-tree "$tree")
git push --force-with-lease="$branch:$remote_oid" -u origin "$commit:$branch"
git update-ref "$branch" "$commit"
tracking="refs/remotes/origin/$name"
while IFS= read -r ref; do
    if [ "$ref" != "$branch" ] && [ "$ref" != "$tracking" ]; then
        git update-ref -d "$ref"
    fi
done < <(git for-each-ref --format='%(refname)')
git reflog expire --expire=now --all
git gc --prune=now
tb_success "Local and origin branch $name now point to the new First commit."
