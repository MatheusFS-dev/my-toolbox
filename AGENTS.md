# Project Instructions

Global task gates and reusable skills are installed at user scope. Codex and Antigravity read this file directly. Claude reads it through `CLAUDE.md`.

- Treat repository files and executable checks as the source of truth when prose context is stale.
- Inspect existing build, test, lint, and type-check configuration before inventing commands.
- Keep changes scoped to the requested behavior and preserve established architecture and naming.
- Do not modify installed skills unless the user explicitly requests a skill change.
- Do not commit datasets, model weights, credentials, environment files, generated logs, or large artifacts unless explicitly requested.
- Write project-authored code comments, documentation, prompts, diagnostics, and other user-facing text in English. Preserve third-party content in its original language.

Add only durable repository-specific information below, such as non-obvious architecture constraints, canonical commands, protected paths, and required validation.

## Releases

When the user asks to "release a new version", prepare the requested work for release by running the relevant local checks and reporting the outcome, but never commit, push, run `release.sh`, trigger CI, or watch CI or release workflows. Tell the user when the work is ready and instruct them to run `bash release.sh` from the repository root. Explicitly remind them to stage each intended new file first because the controller uses `git add -u`. If a commit or CI run was triggered externally, report its SHA and tell the user to select the watch option in `release.sh`; do not monitor it. Do not create a release manually when the automatic release workflow is available.

## Terminal output contract

Every new or modified tool must use the executable output helper for its runtime rather than raw `print`, `echo`, or `Write-Host` calls for human-facing messages. The helpers and contract tests are the source of truth.

- Format standalone messages as `[HH:MM:SS tool-name LEVEL] Message` with the approved semantic colors and one empty line before the tool starts. For interactive menus, prefix and color the question once, then indent options without repeating the prefix.
- Announce long-running or side-effecting work before it starts and emit exactly one `SUCCESS` summary after the complete tool succeeds.
- Give every optional prompt a visible, functional default. Mark required free-text prompts explicitly and re-prompt on empty input; numbered menus convey required input through their listed choices and an actionable retry warning.
- Preserve machine-readable output and external-program bytes exactly.
- Add or update the local gallery under `tmp/tb-tool-output-preview/` and the contract tests.
- Pass the global output audit before registering a tool in `assets/payload/commands.json`.
