# Resetting Git History to a Single Initial Commit

This guide shows how to rewrite a repository so `main` contains only one clean root commit.

## Step 1: Create a new orphan branch

An orphan branch starts with no parent commit, which lets you create a brand new root commit.

```bash
git switch --orphan reset-main
```

If your repository contains only the files you want to keep, you can stage them directly after switching. If the orphan checkout leaves the tree empty, restore the files from the old branch or from the remote branch first.

## Step 2: Restore the project files if needed

If the working tree is empty after creating the orphan branch, bring the current project snapshot back from the existing branch:

```bash
git restore -s origin/main -- .
```

If you are rebuilding from a different branch name, replace `origin/main` with the correct source branch.

## Step 3: Stage and commit everything

Add all files and create the first commit for the new history:

```bash
git add -A
git commit --no-verify -m "Initial release"
```

The `--no-verify` flag skips local hooks if a broken hook blocks the commit.

## Step 4: Rename the branch back to `main`

```bash
git branch -M main
```

This makes the rewritten branch replace the main branch name locally.

## Step 5: Force-push the new history

```bash
git push --force origin main
```

This replaces the remote `main` history with the new single-commit history.

## Step 6: Verify the result

Confirm that the branch now contains only one commit:

```bash
git rev-list --count HEAD
git log --oneline --decorate -3
```

A clean reset should show:

- commit count: `1`
- the only commit message: `Initial release`


