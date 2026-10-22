# Git Remote Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a cross-platform `tb git-sync` interactive command that copies one remote branch's tracked snapshot onto another remote branch while preserving destination history and never force-pushing.

**Architecture:** Add a direct-only built-in workflow coordinated by `App`, with Git operations behind an injectable runner and typed repository, selection, credential, comparison, and execution units. The workflow uses a temporary detached Git worktree so the user's current checkout is never switched, creates a commit on the fetched destination tip, and pushes the exact expected update with normal fast-forward semantics.

**Tech Stack:** Go 1.25.8, Git CLI, Charm Huh, Go standard library tests with temporary repositories and bare remotes.

**Spec:** `docs/superpowers/specs/2026-10-22-git-remote-sync-design.md`

## Global Constraints

- Support Linux AMD64, Linux ARM64, WSL, and Windows AMD64.
- Accept invocation from any subdirectory of a non-bare Git worktree.
- Support any pair of configured Git remotes and independently selected remote branches.
- Require a fully clean working tree, including untracked files.
- Preserve destination history and never invoke `--force`, `--force-with-lease`, destructive reset, automatic stash, or automatic discard.
- Do not read, log, store, or pass credentials in process arguments.
- Use argument arrays for Git execution, never shell command strings.
- Do not add dependencies unless existing Charm Huh and standard-library facilities are insufficient.
- Every new function and method receives the repository's complete Google-style doc comment convention.
- Keep product changes scoped to this feature and never commit credentials or generated logs.

## Review Focus

- Remote names, branch names, paths, and commit messages containing spaces or shell metacharacters remain literal arguments and never reach a shell.
- Symbolic remote HEAD may be absent or stale, in which case branch selection remains usable without inventing a branch.
- Empty repositories, deleted remote branches, detached caller state, and Git worktrees return specific safe errors.
- Cleanup failure is joined with the primary failure and never falsely reports restoration success.
- Credential configuration changes only the selected URL scope and never overwrites unrelated helpers.

---

### Task 1: Register the direct built-in command

**Files:**
- Modify: `commands.json`
- Modify: `src/catalog_test.go`
- Modify: `src/app_test.go`
- Modify: `README.md`
- Modify: `src/testdata/help-linux-native.txt`
- Modify: `src/testdata/help-linux-wsl.txt`
- Modify: `src/testdata/help-windows.txt`

**Interfaces:**
- Consumes: Existing catalog loading, direct command execution, help rendering, and `__complete`.
- Produces: A direct-only `git-sync` built-in catalog command on every supported environment with the derived `git` capability.

- [ ] **Step 1: Write failing catalog and application tests**

Add assertions that `git-sync` exists exactly once, is `direct`, is available in all three environments, uses the `builtin` protocol, appears in help and completion output, and rejects positional arguments.

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./src -run 'TestRepositoryCatalog|TestRepositoryCompletion|TestGitSync' -v`

Expected: FAIL because `git-sync` and the `git` capability do not exist.

- [ ] **Step 3: Add catalog metadata and capability**

Add capability `git` with label `Git`, environment support for Linux, WSL, and Windows, and remediation to install Git on PATH. Add `git-sync` under `Project Utilities`, package `builtin`, direct visibility, builtin protocol, explicit Git requirements, and all published entrypoints. Update catalog digest and environment completion expectations.

- [ ] **Step 4: Update user-visible documentation fixtures**

Document the command, clean-tree requirement, tracked-snapshot semantics, normal fast-forward push, and the no-force guarantee. Regenerate or manually update exact help fixtures using the repository's existing fixture pattern.

- [ ] **Step 5: Run focused tests**

Run: `go test ./src -run 'TestRepositoryCatalog|TestRepositoryCompletion|TestGitSync|TestHelp' -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add commands.json README.md src/capabilities.go src/catalog_test.go src/app_test.go src/testdata
git commit -m "feat: register git remote sync command"
```

### Task 2: Implement repository and remote discovery

**Files:**
- Create: `src/git_sync.go`
- Create: `src/git_sync_test.go`

**Interfaces:**
- Consumes: `context.Context`, an initial working directory, and an injectable `GitRunner`.
- Produces:
  - `type GitRunner interface { Run(ctx context.Context, directory string, environment []string, arguments ...string) (string, error) }`
  - `type GitRepository struct { Root string; OriginalRef string; OriginalCommit string; Remotes []GitRemote }`
  - `type GitRemote struct { Name string; FetchURL string; PushURL string }`
  - `type GitBranch struct { Name string; Commit string; Default bool }`
  - `func DiscoverGitRepository(ctx context.Context, runner GitRunner, directory string) (GitRepository, error)`
  - `func FetchRemoteBranches(ctx context.Context, runner GitRunner, repository GitRepository, remote GitRemote) ([]GitBranch, error)`

- [ ] **Step 1: Write failing discovery tests**

Create temporary repositories that assert discovery from a nested directory, clean-tree enforcement for staged, unstaged, and untracked changes, detached caller state capture, fewer-than-two-remotes rejection, URL preservation, branch enumeration, default-branch marking, missing symbolic HEAD fallback, and metacharacter-safe runner arguments.

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./src -run 'TestDiscoverGitRepository|TestFetchRemoteBranches' -v`

Expected: FAIL because discovery types and functions do not exist.

- [ ] **Step 3: Implement the Git runner and discovery**

Use `git rev-parse --show-toplevel`, `git status --porcelain=v1 --untracked-files=normal`, `git remote`, and `git remote get-url [--push]`. Fetch a selected remote with an explicit refspec into remote-tracking refs, enumerate with `for-each-ref`, and resolve the optional symbolic `refs/remotes/<remote>/HEAD`. Wrap every failure with its stage and safe remediation.

- [ ] **Step 4: Run focused tests**

Run: `go test ./src -run 'TestDiscoverGitRepository|TestFetchRemoteBranches' -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/git_sync.go src/git_sync_test.go
git commit -m "feat: discover git remotes and branches"
```

### Task 3: Implement credential policy without handling secrets

**Files:**
- Create: `src/git_sync_credentials.go`
- Create: `src/git_sync_credentials_test.go`
- Create: `src/git_sync_credentials_unix.go`
- Create: `src/git_sync_credentials_windows.go`

**Interfaces:**
- Consumes: Selected HTTPS remote URL, platform, repository-scoped Git runner, and a user-selected mode.
- Produces:
  - `type CredentialMode string` with `CredentialSecure`, `CredentialStore`, and `CredentialEphemeral`.
  - `type CredentialSupport struct { HTTPS bool; ConfiguredHelper string; SecureHelpers []string }`
  - `func InspectCredentialSupport(ctx context.Context, runner GitRunner, repository GitRepository, remote GitRemote, platform string) (CredentialSupport, error)`
  - `func ConfigureCredentialMode(ctx context.Context, runner GitRunner, repository GitRepository, remote GitRemote, mode CredentialMode, helper string) error`

- [ ] **Step 1: Write failing credential tests**

Assert SSH/file remotes bypass credential questions, configured secure helpers are preserved, secure mode scopes the selected helper to the selected HTTPS URL, store mode writes an explicit URL-scoped helper only after selection, ephemeral mode disables persistence for the operation environment, and no test input contains or records a token.

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./src -run 'TestInspectCredential|TestConfigureCredential' -v`

Expected: FAIL because credential policy does not exist.

- [ ] **Step 3: Implement helper detection and scoped configuration**

Inspect `credential.helper` using `git config --show-origin --get-all` and recognize Git Credential Manager on Windows plus configured secure helpers on Unix. Configure only `credential.<normalized-url>.helper`. For plaintext storage, require the already confirmed mode and configure `store` with an explicit warning retained in the presentation layer. For ephemeral mode, pass `-c credential.helper=` only to authentication-requiring Git operations rather than changing persistent config. Never prompt for or consume a secret directly.

- [ ] **Step 4: Add platform guidance**

Unix guidance names an existing secure helper such as libsecret when executable and configured. Windows guidance recognizes `manager` and `manager-core`. If none exists, return actionable installation/configuration guidance without silently selecting plaintext storage.

- [ ] **Step 5: Run focused tests**

Run: `go test ./src -run 'TestInspectCredential|TestConfigureCredential' -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add src/git_sync_credentials*.go
git commit -m "feat: add git credential policy"
```

### Task 4: Plan, compare, and execute synchronization transactionally

**Files:**
- Create: `src/git_sync_execute.go`
- Create: `src/git_sync_execute_test.go`

**Interfaces:**
- Consumes: Discovered repository, selected source and destination branch tips, commit message, credential operation environment, and `GitRunner`.
- Produces:
  - `type GitSyncSelection struct { SourceRemote GitRemote; SourceBranch GitBranch; DestinationRemote GitRemote; DestinationBranch GitBranch; CommitMessage string }`
  - `type GitSyncChanges struct { Added int; Modified int; Deleted int }`
  - `type GitSyncPlan struct { Repository GitRepository; Selection GitSyncSelection; Changes GitSyncChanges }`
  - `type GitSyncResult struct { Commit string; Changes GitSyncChanges; NoChanges bool }`
  - `func PlanGitSync(ctx context.Context, runner GitRunner, repository GitRepository, selection GitSyncSelection) (GitSyncPlan, error)`
  - `func ExecuteGitSync(ctx context.Context, runner GitRunner, plan GitSyncPlan) (GitSyncResult, error)`

- [ ] **Step 1: Write failing integration tests with bare remotes**

Test different branch names, more than two remotes, added/modified/deleted counts, identical snapshots, destination history preservation, source deletion propagation, Git identity failure, destination branch deletion, non-fast-forward race rejection, push rejection, cleanup after commit/push failure, and literal handling of special characters.

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./src -run 'TestPlanGitSync|TestExecuteGitSync' -v`

Expected: FAIL because planning and execution functions do not exist.

- [ ] **Step 3: Implement snapshot comparison**

Compare fetched source and destination commits with `git diff --name-status -z`, parse NUL-delimited status records including rename/copy records, and aggregate exact added, modified, and deleted counts. Reject identical source/destination remote-branch selections.

- [ ] **Step 4: Implement temporary-worktree execution**

Create a temporary directory, add a detached worktree at the recorded destination commit, restore the complete source tree into index and worktree, stage deletions, verify the staged diff, create the commit, and push `HEAD:refs/heads/<destination>` normally. Remove the worktree and temporary directory in deferred cleanup. Join cleanup errors with any primary error.

- [ ] **Step 5: Protect against destination races**

Push with a normal ref update and no force options. A non-fast-forward rejection must report that the destination changed and that no force push was attempted.

- [ ] **Step 6: Run focused tests**

Run: `go test ./src -run 'TestPlanGitSync|TestExecuteGitSync' -v`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add src/git_sync_execute.go src/git_sync_execute_test.go
git commit -m "feat: execute history-preserving remote sync"
```

### Task 5: Add the interactive terminal workflow

**Files:**
- Modify: `src/app.go`
- Modify: `src/app_test.go`
- Modify: `src/main.go`
- Modify: `src/tui.go`
- Modify: `src/tui_test.go`
- Create: `src/git_sync_workflow.go`
- Create: `src/git_sync_workflow_test.go`

**Interfaces:**
- Consumes: Discovery, credential, planning, and execution interfaces from Tasks 2 through 4.
- Produces:
  - `type GitSyncUI interface` with typed remote, branch, credential, commit-message, and final-confirmation methods.
  - `type GitSyncWorkflow struct { Runner GitRunner; UI GitSyncUI; Platform string; Directory string; Output io.Writer }`
  - `func (workflow GitSyncWorkflow) Run(ctx context.Context) error`
  - App dispatch for `tb git-sync` that rejects extra arguments and invokes the workflow directly.

- [ ] **Step 1: Write failing orchestration tests**

Using fake UI and runner boundaries, assert question order, source removal from destination options, detected-default branch selection, HTTPS credential choices, plaintext warning, SSH bypass, editable default commit message, No-default final confirmation, cancellation before mutation, no-change exit, success summary, and exact error-stage reporting.

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./src -run 'TestGitSyncWorkflow|TestGitSyncCommand' -v`

Expected: FAIL because workflow and dispatch do not exist.

- [ ] **Step 3: Extend UI with focused Git-sync forms**

Use Charm Huh single-select, input, and confirm components. Keep the presentation width and styling consistent with existing toolbox forms. Never render credential values. Final confirmation displays repository root, source and destination URLs/branches, change counts, commit message, history-preservation behavior, and the no-force guarantee.

- [ ] **Step 4: Wire production dependencies**

Construct the OS Git runner, current working directory, platform, terminal streams, and Huh implementation in `main.go`. Dispatch `git-sync` before generic catalog execution while retaining catalog-based availability, help, and completion.

- [ ] **Step 5: Run focused tests**

Run: `go test ./src -run 'TestGitSyncWorkflow|TestGitSyncCommand|TestHuh' -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add src/app.go src/app_test.go src/main.go src/tui.go src/tui_test.go src/git_sync_workflow.go src/git_sync_workflow_test.go
git commit -m "feat: add interactive git sync workflow"
```

### Task 6: Verify cross-platform behavior and documentation

**Files:**
- Modify: `README.md`
- Modify: `docs/superpowers/specs/2026-10-22-git-remote-sync-design.md` only if implementation reveals a factual mismatch.
- Modify: Test fixtures identified by verification failures.

**Interfaces:**
- Consumes: Complete feature.
- Produces: Verified Linux and Windows builds, passing repository checks, and final user documentation.

- [ ] **Step 1: Run formatting and static analysis**

Run: `gofmt -w src/git_sync*.go src/app.go src/main.go src/tui.go`

Run: `go vet ./...`

Expected: no diagnostics.

- [ ] **Step 2: Run all Go tests**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 3: Run repository shell checks**

Run: `for test in scripts/*_test.sh; do sh "$test"; done`

Expected: every applicable shell test passes. Record environment-specific skips exactly as emitted.

- [ ] **Step 4: Run PowerShell checks when PowerShell is available**

Run: `pwsh -NoProfile -File scripts/completion_test.ps1` and the repository's other `*_test.ps1` scripts.

Expected: PASS, or document that PowerShell is unavailable in the verification environment.

- [ ] **Step 5: Cross-compile published targets**

Run:
```bash
GOOS=linux GOARCH=amd64 go build ./src
GOOS=linux GOARCH=arm64 go build ./src
GOOS=windows GOARCH=amd64 go build ./src
```

Expected: all targets build without errors. Remove local build outputs before commit.

- [ ] **Step 6: Re-read README and help output**

Verify the documentation matches actual behavior, especially credential scope, clean-tree rejection, destination history preservation, and the guarantee that no force push occurs.

- [ ] **Step 7: Commit final documentation or fixture corrections**

```bash
git add README.md src/testdata
git commit -m "docs: document git remote sync"
```
