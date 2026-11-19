# VS Code Split Terminal Side Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prompt for Left or Right when Ctrl+Alt+S runs, then put a fresh terminal in the chosen third of the monitor on Linux X11 and Windows.

**Architecture:** Each installer embeds its platform's extension.js. Both commands use a native VS Code Quick Pick before side effects. Linux passes the side to `splitArea`; Windows passes it to `split.ps1` in Place mode, which mirrors the existing coordinates.

**Tech Stack:** VS Code extension API, Node.js, Bash, PowerShell, X11 utilities, Win32 window placement.

**Spec:** `docs/designs/2026-11-19-vscode-split-terminal-side-design.md`

## Global Constraints

- Keep Ctrl+Alt+S and the existing one-third terminal, two-thirds VS Code split.
- Preserve existing local-workspace, focus, launch, and placement safeguards.
- Write project-authored text in English.
- Do not commit, push, install, or release.
- Put any new temporary test script under `tests/agents/vscode-split-terminal/` and remove it after validation.

## Review Focus

- Cancelled side picker: no process launch, placement call, or error notification.
- Multi-root workspace: selected folder still determines the terminal directory.
- Left choice on monitors with negative or positive origins: rectangles remain adjacent within the work area.
- Right choice: coordinates and focus match current behavior.
- Concurrent command invocation while the picker is open: only one invocation proceeds.

---

### Task 1: Linux side choice and layout

**Files:**
- Modify: `assets/payload/packages/macros/vscode/split_terminal.sh`
- Modify: `tooling/checks/vscode_split_terminal_test.cjs`

**Interfaces:**
- `splitArea(monitor, workarea, side)` returns `{ code, terminal }` rectangles; `side` is `'left'` or `'right'`.
- The command obtains `side` from `vscode.window.showQuickPick` before workspace selection or any external command.

- [ ] Add failing checks for Left and Right rectangles, cancellation, and one prompt per invocation. Extend the existing test file; do not create a new script.
- [ ] Run `node --test tooling/checks/vscode_split_terminal_test.cjs` and confirm the new checks fail.
- [ ] Add the Quick Pick and pass its result through both initial and recalculated Linux layouts. Leave the current Right coordinates intact.
- [ ] Rerun the Linux Node checks and confirm they pass.

### Task 2: Windows side choice and placement

**Files:**
- Modify: `assets/payload/packages/macros/vscode/split_terminal.ps1`
- Modify: `tooling/checks/vscode_split_terminal_windows_test.cjs`

**Interfaces:**
- The extension sends `-Side left|right` only for helper Place mode. Probe mode remains unchanged.
- `split.ps1` accepts `Side` as a validated choice and computes terminal and VS Code rectangles from the selected side.

- [ ] Add failing checks for picker cancellation, the side passed to Place, and mirrored Windows placement coordinates in the embedded helper source.
- [ ] Run `node --test tooling/checks/vscode_split_terminal_windows_test.cjs` and confirm the new checks fail.
- [ ] Add the Quick Pick, validated helper parameter, and side-dependent placement. Move the terminal to the chosen monitor before final placement as the helper currently does.
- [ ] Rerun the Windows Node checks and confirm they pass.

### Task 3: Documentation and validation

**Files:**
- Modify: `docs/vscode-split-terminal.md`
- Modify if its description needs updating: `assets/payload/packages/macros/vscode/split_terminal.json`

- [ ] Document the prompt, both layouts, and cancellation in the Linux and Windows instructions and desktop smoke check.
- [ ] Run `node --test tooling/checks/vscode_split_terminal_test.cjs tooling/checks/vscode_split_terminal_windows_test.cjs` and confirm all tests pass.
- [ ] Run `go test ./internal/features/macros ./internal/ui`, `sh -n assets/payload/packages/macros/vscode/split_terminal.sh`, `shellcheck assets/payload/packages/macros/vscode/split_terminal.sh`, `sh tooling/checks/output_contract_test.sh`, and `python3 tooling/checks/output_contract_audit.py`; report unavailable tools separately.
- [ ] Inspect `git diff` for unrelated changes and ensure generated test artifacts are removed.
