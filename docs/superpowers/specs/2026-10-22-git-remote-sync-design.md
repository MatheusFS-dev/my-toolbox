# Git Remote Sync Design

## Purpose

Add a cross-platform `tb git-sync` command that synchronizes the tracked-file snapshot from one Git remote branch onto another remote branch. The command provides force-like content replacement without rewriting destination history or issuing a force push.

## Supported environments

- Linux AMD64 and ARM64.
- WSL AMD64 and ARM64 where published.
- Windows AMD64.
- Invocation from the repository root or any subdirectory of a non-bare Git working tree.
- Any pair of configured Git remotes, regardless of hosting provider or transport.

Git must already be installed. At least two remotes must exist.

## User experience

Running `tb git-sync` opens an interactive terminal workflow consistent with the existing toolbox presentation.

The workflow:

1. Resolve the repository root with Git, starting at the current directory.
2. Require a clean working tree, including no staged, unstaged, or untracked changes.
3. Discover configured remotes and display each name and fetch URL.
4. Ask for the source remote.
5. Fetch the source remote and list its remote branches.
6. Ask for the source branch, defaulting to the detected remote default branch when available.
7. Ask for a different destination remote.
8. Fetch the destination remote and list its remote branches.
9. Ask for the destination branch, defaulting to the detected remote default branch when available.
10. Compare the tracked snapshots and show added, modified, and deleted counts.
11. Ask for a commit message, defaulting to `Sync <source>/<branch> to <destination>/<branch>`.
12. Show a final source, destination, branch, URL, change, and safety summary. Confirmation defaults to No.
13. Create and push the synchronization commit only after confirmation.

The command exits without committing or pushing when the snapshots match.

## Synchronization mechanism

The command must preserve destination history:

1. Fetch both selected remote branches.
2. Record the fetched destination tip.
3. Create a detached work state based on the destination tip.
4. Replace its tracked snapshot with the source remote branch using Git-native restore/index operations.
5. Commit only when the index differs.
6. Push the new commit to the selected destination branch using a normal non-force push.
7. Reject a non-fast-forward result if the remote destination changes after fetch.

No persistent synchronization branch is created. The original branch and working-tree state are restored before exit, including after failures. Temporary refs or worktrees, if used internally, must be removed.

The tool never uses `--force`, `--force-with-lease`, destructive reset, or automatic stash/discard operations.

## Authentication and credential storage

Authentication is delegated to Git so tokens and passwords use hidden Git prompts and are never placed in process arguments, logs, configuration owned by the toolbox, or terminal summaries.

When an HTTPS remote requires authentication, the interactive workflow asks how credentials should be handled:

- Secure credential manager: use an already configured OS-compatible Git credential helper, or offer guided installation/configuration when supported.
- Plaintext Git storage: configure Git's `store` helper only after explicitly warning that credentials are written unencrypted to the user's Git credentials file.
- Do not save: authenticate for the current operation without configuring persistent storage.

The choice applies through Git configuration. The tool must not read or persist the secret itself. SSH remotes continue through the user's existing SSH agent and key configuration without presenting the HTTPS storage question.

## Architecture

Implement `git-sync` as a toolbox builtin because repository discovery, Git command construction, validation, cleanup, and testability require structured control beyond a platform-specific shell script.

Focused units:

- Repository discovery and state validation.
- Remote and remote-branch discovery.
- Credential-helper inspection and configuration.
- Sync planning and snapshot comparison.
- Transactional execution with restoration and cleanup.
- Interactive presentation and confirmation.
- Catalog, help, completion, and README integration.

Git execution must use argument arrays, never shell command strings.

## Error handling

Stop before mutation when:

- The current location is outside a Git worktree.
- Git is unavailable.
- The working tree is not clean.
- Fewer than two remotes exist.
- A selected remote or branch disappears.
- Source and destination selections are identical.
- Fetch or authentication fails.
- Git identity is missing and a commit is required.

During mutation, always restore the original checkout state. A rejected push reports that no force push was attempted and recommends fetching and rerunning. Protected-branch rejection remains a remote policy error and is not bypassed.

Error messages include the failed stage and safe corrective action without exposing credentials.

## Testing

Use Go unit tests with a temporary local Git topology containing a working repository and multiple bare remotes. Tests cover:

- Repository discovery from a subdirectory.
- Dirty staged, unstaged, and untracked states.
- Remote and default-branch discovery.
- More than two remotes.
- Different branch names.
- No-difference exit.
- Added, modified, and deleted files.
- Destination history preservation.
- Destination race causing non-fast-forward rejection.
- Restoration after fetch, commit, and push failures.
- HTTPS credential-mode configuration without real secrets.
- SSH remotes bypassing credential storage.
- Catalog, help, completion, and documentation consistency.

Platform-independent Git behavior stays in shared Go files. OS-specific secure-helper discovery and installation guidance use build-tagged files and focused tests.

## Documentation

Update `README.md` and generated/help fixtures with:

- `tb git-sync` usage.
- The content-replacement and history-preservation model.
- The clean-worktree requirement.
- Credential-storage options and plaintext warning.
- The explicit guarantee that the command never force-pushes.
