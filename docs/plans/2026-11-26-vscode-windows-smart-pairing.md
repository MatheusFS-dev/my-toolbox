# VS Code split terminal smart pairing on Windows Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Windows Terminal Beside extension link the terminal and VS Code so focusing either raises both and resizing one resizes the other, matching the Linux `terminalBeside.smart` behavior.

**Architecture:** A new `pair.ps1` embedded in the Windows installer compiles a small C# type (`TbGeometry` for pure geometry, `TbPair` for the watcher) and watches both windows with `SetWinEventHook`. After `split.ps1` places the windows, `extension.js` spawns `pair.ps1` detached and waits for a `READY` line. The helper ends itself when the link ends.

**Tech Stack:** Windows PowerShell 5.1 with `Add-Type` (C# 5), Node `node:test`, VS Code extension JavaScript, Go macro tests.

**Spec:** `docs/designs/2026-11-26-vscode-windows-smart-pairing-design.md`

## Global Constraints

- Windows PowerShell 5.1 only, no new runtime dependency, no Node native modules.
- C# inside `Add-Type` must compile as C# 5: no string interpolation, no `?.`, no expression-bodied members.
- Rectangles are `int[4]` as `{x, y, width, height}` in visible DWM frame coordinates.
- Constants: minimum other-window width 100 px, adjacency tolerance 2 px, frame comparison tolerance 1 px, message wait timeout 200 ms.
- Setting `terminalBeside.smart` defaults to `true`. Extension version becomes `1.1.0` in `package.json` and in `extension.vsixmanifest`.
- Linux files are untouched. Existing `Place` arguments end with `-Side`, `<side>`, `-Percent`, `<percent>` and must stay that way.
- Project text is English. Do not commit, push, run `release.sh`, or create tags. Tell the user to run `bash release.sh` when done.
- Test scripts the agent writes live under `tests/agents/vscode-windows-pairing/` as `<kind>_<subject>.ps1`, and are deleted at the end. Permanent regression tests go in `tooling/checks/`.
- Scripts print progress. `pair.ps1` prints only protocol lines (`READY`, `ENDED <reason>`) like `split.ps1`, so it is not a tool under the terminal output contract.

## Review Focus

- A window is minimized then restored: location events carry off-screen coordinates and must neither unlink nor move the other window.
- VS Code or the terminal closes while the helper runs: the helper exits within about one second and leaves no orphan.
- Ctrl+Alt+S twice on the same VS Code window: the older helper stops and the two never fight over the layout.
- The helper fails to start or compile: the user sees an error naming the cause and the windows stay placed.
- Mixed DPI or rounding leaves a 1 to 2 px difference after a resize: adjacency must still hold and must not unlink the pair.

---

## File Structure

- Modify `assets/payload/packages/macros/vscode/split_terminal.ps1`: embed `pair.ps1`, update `package.json`, `extension.js`, `split.ps1` (print terminal handle), README text, archive entry list, vsix manifest version.
- Modify `tooling/checks/vscode_split_terminal_windows_test.cjs`: fixture and new tests.
- Modify `internal/features/macros/macro_vscode_windows_test.go`: expect `extension/pair.ps1` in the vsix.
- Modify `assets/payload/packages/macros/vscode/README.md` and `docs/vscode-split-terminal.md`: Windows smart pairing text.
- Temporary: `tests/agents/vscode-windows-pairing/` (the spike files already there become the smoke test, then are deleted).

### Task 1: Geometry type and `pair.ps1` plumbing

**Files:**
- Modify: `assets/payload/packages/macros/vscode/split_terminal.ps1` (new `Save-File ... 'pair.ps1'` block, archive entry list near line 440)
- Modify: `internal/features/macros/macro_vscode_windows_test.go:126`
- Test: `tooling/checks/vscode_split_terminal_windows_test.cjs`

**Interfaces:**
- Produces: embedded file `pair.ps1` readable by the existing `embedded('pair.ps1')` test helper. Its first `Add-Type -TypeDefinition @"` block defines `public static class TbGeometry` with `public static int[] FollowResize(int[] changed, int[] other, int minWidth)` returning the new frame of `other` or `null`, `public static bool IsAdjacent(int[] a, int[] b, int tolerance)`, and `public static bool SameFrame(int[] a, int[] b, int tolerance)`.
- Semantics, ported from `pairGeometry.js` in `split_terminal.sh`: `FollowResize` keeps the other window's outer edge and moves the shared edge to follow `changed`. If `changed.x <= other.x` the other starts at `changed.x + changed.width` and ends at its old right edge, otherwise it starts at its old `x` and ends at `changed.x`. The result takes `changed`'s `y` and `height`. Width below `minWidth` returns `null`. `IsAdjacent` is true when the horizontal gap is at most `tolerance` and the vertical overlap is positive.

- [ ] **Step 1: Write the failing tests**

Add to `vscode_split_terminal_windows_test.cjs` a helper `compileGeometry()` that extracts the text between `Add-Type -TypeDefinition @"` and `"@` from `embedded('pair.ps1')`, and a test `'geometry follows resize and adjacency like Linux'` (skipped when `process.platform !== 'win32'`) that runs one `powershell.exe -NoProfile` call, loads the C# with `Add-Type`, and prints JSON for these cases, with `rect(x,y,w,h)` as in the Linux test:

```js
// base = { code: rect(0,0,1280,1080), terminal: rect(1280,0,640,1080) }
followResize(terminal rect(1000,0,920,1080), other code)  -> rect(0,0,1000,1080)
followResize(code rect(0,0,1500,1080), other terminal)    -> rect(1500,0,420,1080)
followResize(code rect(100,0,1180,1080), other terminal)  -> rect(1280,0,640,1080)
followResize(code rect(0,0,1280,900), other terminal)     -> rect(1280,0,640,900)
followResize(code rect(0,0,1850,1080), other terminal, 100) -> null
isAdjacent(rect(0,0,1280,1080), rect(1281,0,639,1080), 2) -> true
isAdjacent(rect(0,0,1280,1080), rect(1290,0,630,1080), 2) -> false
isAdjacent(rect(0,0,1280,500), rect(1280,600,640,400), 2) -> false
```

Also add `'installer embeds pair.ps1 in the archive'` asserting `source` contains `'extension/pair.ps1'` and that `embedded('pair.ps1')` matches `/TbGeometry/`.

- [ ] **Step 2: Run to verify failure**

Run: `node --test tooling/checks/vscode_split_terminal_windows_test.cjs`
Expected: the two new tests FAIL with "pair.ps1 missing from the installer".

- [ ] **Step 3: Add the `pair.ps1` block and archive entry**

In `split_terminal.ps1`, after the `split.ps1` block add `Save-File (Join-Path $Extension 'pair.ps1') @' ... '@` containing a `param()` block (declared in Task 2) and the `Add-Type -TypeDefinition @" ... "@` with `TbGeometry` only for now. Append `'extension/pair.ps1'` to the archive entry list. In `macro_vscode_windows_test.go` add `"extension/pair.ps1"` to the expected names.

- [ ] **Step 4: Run to verify pass**

Run: `node --test tooling/checks/vscode_split_terminal_windows_test.cjs` and `go test ./internal/features/macros`
Expected: PASS for both.

### Task 2: The `TbPair` watcher and its desktop smoke test

**Files:**
- Modify: `assets/payload/packages/macros/vscode/split_terminal.ps1` (`pair.ps1` block)
- Create: `tests/agents/vscode-windows-pairing/smoke_pair-helper.ps1` (reworked from `debug_pair-driver.ps1`)
- Reference only: `tests/agents/vscode-windows-pairing/debug_pair-helper-spike.ps1` holds the proven algorithm. Port it, do not copy its debug logging.

**Interfaces:**
- Consumes: `TbGeometry` from Task 1.
- Produces: `pair.ps1` parameters `-CodeHandle <string>`, `-TerminalHandle <string>`. It prints `READY` once all hooks are installed, then `ENDED <reason>` on exit, where reason is one of `closed`, `maximized`, `fullscreen`, `dragged-away`, `too-narrow`, `replaced`. Public C# entry: `public static string TbPair.Run(long code, long terminal, string stopName, Action ready)` returning the reason.

- [ ] **Step 1: Write the failing smoke test**

Rework the driver into `smoke_pair-helper.ps1`. It extracts `pair.ps1` from the installer's here-string into the scratchpad, opens three WinForms windows (A code, B terminal, C blocker), starts the helper, and checks, using adjacency of the two visible frames instead of absolute pixels (tolerance 2 px):
resize A wider and B follows, resize B and A follows, focus A and focus B each raise both above C, minimize and restore B and nothing unlinks or moves, a second helper for the same `-CodeHandle` makes the first exit with `replaced`, closing B ends the helper with `closed` within 1 second, drag A away ends it with `dragged-away`, and maximizing A ends it with `maximized`. Use `AttachThreadInput` plus the Alt tap for focus as in the spike, and rename variables so none differ only by case. The script prints each stage and `PASS`/`FAIL` lines.

- [ ] **Step 2: Run to verify failure**

Run: `powershell.exe -NoProfile -ExecutionPolicy Bypass -File tests/agents/vscode-windows-pairing/smoke_pair-helper.ps1`
Expected: FAIL at helper start because `pair.ps1` has no `TbPair`.

- [ ] **Step 3: Implement `TbPair` and the `pair.ps1` body**

Add to the C# block `public static class TbPair` per the spike: three `SetWinEventHook` ranges (foreground `0x0003`, location change `0x800B`, destroy `0x8001`) with flags 0, the callback in a static field, DPI awareness context `-4`, a loop of `MsgWaitForMultipleObjects` with 200 ms timeout and `PeekMessage` draining, and exit when either window is gone or a stop is requested. Behavior beyond the spike:
- Ignore location events while `IsIconic` for either window, and ignore the pair's own echo by comparing each new visible frame with the last known frame using `SameFrame(.., 1)`.
- A window that `IsZoomed` ends the link with `maximized`. A visible frame equal to its monitor rectangle ends it with `fullscreen`.
- A pure move (same size, new position) that fails `IsAdjacent(.., 2)` ends it with `dragged-away`. A size change applies `FollowResize(.., 100)` and ends with `too-narrow` on `null`.
- Raise order: other window first, focused window second, each by topmost then non-topmost with `SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE`, skipping a minimized window.
- Replacement: a named `EventWaitHandle` `Local\TerminalBeside.Pair.<codeHandle>`. A new helper opens an existing one and sets it, waits up to 3 s for the old owner's named `Mutex` of the same suffix to be released, then creates its own. The old helper exits with `replaced`.
- `pair.ps1` writes `READY` to stdout after the hooks exist, `ENDED <reason>` at exit, and on any exception writes the message to stderr and exits 1.

- [ ] **Step 4: Run to verify pass**

Run the smoke script again.
Expected: every check prints `PASS` and the summary shows `fail=0`.

### Task 3: Extension integration, manifest, and `split.ps1` handle output

**Files:**
- Modify: `assets/payload/packages/macros/vscode/split_terminal.ps1` (`package.json`, `extension.js`, `split.ps1`, vsix manifest version)
- Test: `tooling/checks/vscode_split_terminal_windows_test.cjs`
- Modify: `internal/features/macros/macro_vscode_windows_test.go` (manifest version if asserted)

**Interfaces:**
- Consumes: `pair.ps1` protocol from Task 2.
- Produces: in `extension.js`, `createCommand(vscode, options)` accepts `options.pairHelper` (default `join(__dirname, 'pair.ps1')`). It reads `terminalBeside.smart` (default `true`). After a successful `Place`, when smart, it calls `startPair(code, terminal)` which spawns `powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File <pairHelper> -CodeHandle <code> -TerminalHandle <terminal>` with `{ detached: true, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'] }`, resolves on a `READY` line, rejects with the stderr text on exit before `READY`, on spawn error, or after 10 s without `READY`, then destroys the pipes and `unref`s. `split.ps1` `Place` mode prints `OK` then `TERMINAL=<handle>` as separate lines.

- [ ] **Step 1: Write the failing tests**

Update the `runtime()` fixture: `getConfiguration: () => ({ get: (key, fallback) => key === 'smart' ? (config.smart ?? true) : (config.percent || 100 / 3) })`, the Place stdout to `'OK\r\nTERMINAL=5151\r\n'`, and a `spawn` mock that returns an event emitter with `stdout`/`stderr` emitters for `powershell.exe` and emits `READY` (or exits or errors per `config.pair`). Add tests:
- `'manifest declares smart setting and version 1.1.0'`: `terminalBeside.smart` is a boolean with default `true`, version is `1.1.0`, `embedded('package.json')` and the vsix manifest agree.
- `'starts the pair helper after placement'`: one `wt.exe` launch and one `powershell.exe` spawn whose args end with `['-CodeHandle', '4242', '-TerminalHandle', '5151']`, `detached` true, `windowsHide` true, no errors.
- `'smart false places once and starts no helper'`.
- `'reports a helper that fails or never reports READY'` for exit-with-stderr, spawn error, and a 10 s timeout using an injected `sleep`/timer option, each yielding exactly one error message that contains the cause.
- `'helper path with spaces stays one argument'` using `pairHelper: 'C:\\Users\\Jane Doe\\ext\\pair.ps1'`.
- `'split.ps1 prints the terminal handle'` asserting `embedded('split.ps1')` contains `'TERMINAL=' + $Terminal.ToInt64()`.
Keep the existing assertion that Place args end with `-Side`, `Left`, `-Percent`, `40`.

- [ ] **Step 2: Run to verify failure**

Run: `node --test tooling/checks/vscode_split_terminal_windows_test.cjs`
Expected: the new tests FAIL, existing tests still PASS.

- [ ] **Step 3: Implement**

In the `package.json` block add the `terminalBeside.smart` property (description as in Linux without the GNOME sentence, saying it links the terminal and VS Code) and set `"version": "1.1.0"`. Set the vsix manifest `Version="1.1.0"`. Edit `split.ps1` to print `TERMINAL=` after `OK`. In `extension.js` parse `^TERMINAL=(\d+)$` from the Place output and implement `startPair` as specified. Keep the `finally { busy = false }` flow so a pair failure still releases the command.

- [ ] **Step 4: Run to verify pass**

Run: `node --test tooling/checks/vscode_split_terminal_windows_test.cjs` and `go test ./internal/features/macros`
Expected: PASS for both.

### Task 4: Documentation

**Files:**
- Modify: `assets/payload/packages/macros/vscode/README.md`, the embedded `README.md` block in `split_terminal.ps1`, `docs/vscode-split-terminal.md` (Windows section)

**Interfaces:** none.

- [ ] **Step 1: Write the text**

State that on Windows `terminalBeside.smart` is on by default and links the terminal and VS Code: focusing or Alt-Tabbing to either raises both and resizing one resizes the other, and the link ends when either window closes, is maximized or made fullscreen, or is dragged away. State that no extra install is needed, a reload of VS Code is enough, that a background `powershell.exe` runs while a pair is linked, that setting it to `false` places the windows once, and that a helper start failure shows an error. Replace the README sentence that says smart pairing needs the GNOME extension so it distinguishes Linux and Windows. Add the new checks to the Windows checks list.

- [ ] **Step 2: Verify**

Run: `node --test tooling/checks/vscode_split_terminal_windows_test.cjs` (the embedded README test still matches `^# Terminal Beside`).
Expected: PASS.

### Task 5: Full verification and cleanup

**Files:**
- Delete: everything under `tests/agents/vscode-windows-pairing/` (and the empty folder), plus scratchpad logs.

**Interfaces:** none.

- [ ] **Step 1: Run every check**

Run, one per step:
`node --test tooling/checks/vscode_split_terminal_windows_test.cjs`
`node --test tooling/checks/vscode_split_terminal_test.cjs`
`go test ./internal/features/macros ./internal/ui`
`python tooling/checks/output_contract_audit.py`
`powershell.exe -NoProfile -Command "[void][scriptblock]::Create((Get-Content -Raw assets/payload/packages/macros/vscode/split_terminal.ps1))"` (parse check)
Expected: all pass or print no error. The Linux test file must still pass unchanged.

- [ ] **Step 2: Build and install the vsix locally**

Run the installer once with `powershell -ExecutionPolicy Bypass -File assets/payload/packages/macros/vscode/split_terminal.ps1` and confirm it prints the `SUCCESS` line. Ask the user to reload VS Code and do the manual check: Ctrl+Alt+S with a real Windows Terminal, resize each window by dragging, Alt-Tab in and out, minimize and restore, close each window.

- [ ] **Step 3: Remove temporary artifacts**

Delete `tests/agents/vscode-windows-pairing/` and the scratchpad files. Run `git status` and confirm only the intended files changed.

- [ ] **Step 4: Report**

Tell the user the work is ready and to run `bash release.sh` from the repository root. Do not commit or run it.
