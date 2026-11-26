# VS Code split terminal smart pairing on Windows

## Intent

On Linux, `terminalBeside.smart` links the terminal and VS Code as a pair. Focusing or Alt-Tabbing to either window raises both, and resizing one window resizes the other. The link ends when either window closes, is maximized or made fullscreen, or is dragged away from the other. The same behavior is wanted on Windows, where the extension today only places the two windows once.

## Behavior

- Smart pairing is on by default through `terminalBeside.smart`. Turning it off places the windows once, as today.
- Focusing either window raises both without moving focus away from the focused one.
- Resizing one window moves the shared edge, so the other window keeps its outer edge and follows. If the other window would become narrower than 100 px, the link ends.
- A pure move that leaves the windows non-adjacent (gap above 2 px or no vertical overlap) ends the link. Closing, maximizing, or fullscreening either window also ends it.
- Pairing a window that already has a pair replaces the older link.
- Geometry is identical to the Linux `pairGeometry.js`: `splitPair`, `followResize`, and `isAdjacent`, using the visible DWM frame so invisible borders do not create gaps.
- If the helper cannot start, Ctrl+Alt+S shows an error, as Linux does when the GNOME extension is missing.

## Approaches

1. **WinEvent hooks in a background PowerShell helper (chosen).** `SetWinEventHook` reports foreground, location-change, and destroy events. Event-driven, no new dependency, ships inside the `.vsix`.
2. Polling positions every ~50 ms. Simpler, but visible latency and constant CPU.
3. AutoHotkey or another external tool. Adds a dependency the project avoids.

A throwaway spike confirmed approach 1 on this machine with real windows: resize followed in both directions, foreground raised both windows above a third window, a drag away ended the link, and echoes from our own placement did not loop.

## Design

- A new `pair.ps1` inside the extension compiles a small C# type with `Add-Type` (C# 5, Windows PowerShell 5.1). It hooks three event ranges, keeps the callback delegate in a static field, and pumps messages with `MsgWaitForMultipleObjects` plus `PeekMessage`, with a 200 ms timeout so it also notices closed windows.
- Raising uses topmost then non-topmost with no activation, applied to the other window first and the focused window second.
- Own placements are not echoed. The helper compares each new frame with the last known frame and applies a target only when it differs by more than 1 px.
- After a successful `Place`, `extension.js` spawns `pair.ps1` detached with both handles, the side, and the width percent. A named event per VS Code handle lets a new helper stop an older one. The helper exits when the link ends or either window disappears.
- `package.json` adds the `terminalBeside.smart` setting, and the version moves from 1.0.0 to 1.1.0, matching the Linux smart release. The vsix manifest and the file list in the archive include `pair.ps1`.
- Docs update: the README embedded in the installer, `assets/payload/packages/macros/vscode/README.md`, and the Windows section of `docs/vscode-split-terminal.md`.

## Non-goals and constraints

- No release, commit, or installation. The user runs `bash release.sh` later.
- Linux code is untouched. Wayland, remote, and virtual workspaces stay unsupported.
- No new runtime dependency. Windows PowerShell 5.1 only.

## Left to the builder

- Fullscreen detection for Electron (window rectangle equals the monitor rectangle) and maximize detection through `IsZoomed`. Both are untested in the spike.
- Whether to precompile the C# type at install time to cut startup time. Not required.
- Tolerances for DPI rounding. The spike saw a 2 px difference on a right-edge resize, so tests should compare adjacency of the two frames rather than absolute numbers.

## Verification

- Node tests in `tooling/checks/vscode_split_terminal_windows_test.cjs` for the manifest setting, spawning the helper after placement, skipping it when smart is off, and error handling.
- Pure geometry cases ported from the Linux tests.
- A desktop smoke test with throwaway windows for resize in both directions, raise, drag away, close, and replacement, saved under `tests/agents/vscode-windows-pairing/` and removed afterwards.
- `go test ./internal/features/macros`, the output contract audit, and a PowerShell parse check of `split_terminal.ps1`.
