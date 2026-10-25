# Paste Screenshot Path

Paste Screenshot Path is a Windows-only AutoHotkey v2 macro. It conditionally turns Ctrl+Shift+V into the absolute path of the current screenshot file. It never writes to, clears, or restores the clipboard.

## Setup

1. Install AutoHotkey v2 when prompted by <code>tb macros</code>.
2. Confirm that <code>%USERPROFILE%\Pictures\Screenshots</code> exists and that Windows saves screenshots there.
3. Run <code>tb macros</code>, select **Paste Screenshot Path**, then select **Run**.
4. On the first normal launch, choose whether to enable it at sign-in.

The startup launcher is stored at <code>%LOCALAPPDATA%\my-toolbox\macros\screenshot-path-paste\launcher.ahk</code>. A bundled launcher resolves <code>current.txt</code> so it starts the active toolbox version after updates. A downloaded script instead creates a launcher pinned to that downloaded script.

## Use

Take a screenshot normally. When the matching original file is saved, press Ctrl+Shift+V in a text field to insert its unquoted absolute path. Ctrl+V remains native image paste. Copying any other content, deleting or replacing the file, a missing file, an ambiguous match, or expiration returns Ctrl+Shift+V to the focused application.

The tray tooltip reports INACTIVE, PENDING, ACTIVE, or INVALIDATED. The tray menu provides **Status**, **Enable diagnostic logging**, **Enable at sign-in**, **Exit**, and **Uninstall**.

Diagnostics are off by default. When enabled, the log records only timestamped state and event names in <code>%LOCALAPPDATA%\my-toolbox\macros\screenshot-path-paste\diagnostics.log</code>. It does not contain paths, pixels, clipboard text, or hashes.

## Uninstall

Choose **Uninstall** from the tray menu. It removes only this macro's persistent directory, its exact Startup shortcut, and diagnostic log, then exits. The Windows toolbox uninstall performs the same scoped cleanup.

## Limitations

The macro fail-closes when it cannot prove one matching original file. It does not change Snipping Tool settings or support prior clipboard history. Simulated input can be blocked by secure-desktop prompts, applications that reject injected input, or elevated target windows.

## Manual acceptance procedure

Run each check on Windows 11 with AutoHotkey v2 and the macro running.

| ID | Procedure | Expected result |
|---|---|---|
| CA-01 | Capture with Win+Shift+S and save it. Focus a text field and press Ctrl+Shift+V. | Exact original path appears. |
| CA-02 | Use Print Screen only when it produces both a saved file and clipboard image. | Same as CA-01, with no changed capture shortcut. |
| CA-03 | With CA-01 active, press Ctrl+V. | Native image paste remains. |
| CA-04 | Press Ctrl+Shift+V repeatedly while active. | Same path each time, clipboard unchanged. |
| CA-05 | After a matched capture, copy text and test Ctrl+Shift+V in Windows Terminal. | Native terminal behavior, no path. |
| CA-06 | Copy a file, browser image, and text without a new screenshot. | No screenshot path. |
| CA-07 | Copy other content before a capture finishes saving. | Delayed file events do not reactivate the old path. |
| CA-08 | Create two matching candidates during the correlation window. | No path is inserted. |
| CA-09 | Put an image in the clipboard without a saved screenshot. | Native shortcut behavior. |
| CA-10 | Delete, move, or replace the matched file before pasting. | Native shortcut behavior. |
| CA-11 | Enable sign-in, restart, and sign in. | One tray instance starts and waits for a new capture. |
| CA-12 | Change clipboard content between capture detection and pasting. | No stale path. |
| CA-13 | Choose Exit, then choose Uninstall from the tray menu. | All shortcuts behave natively. |
| CA-14 | Try a target that rejects simulated text and inspect the clipboard. | Controlled failure, no clipboard modification. |
