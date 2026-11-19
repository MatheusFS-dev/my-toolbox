#!/bin/sh
# Self-contained installer. The embedded helper is copied from scripts/lib/toolbox_output.sh.
set -eu
#!/bin/sh

# Shared terminal output and prompt helpers for toolbox shell tools.

tb_output_init() {
    TB_OUTPUT_TOOL=$1
    TB_OUTPUT_STARTED=0
    if [ -n "${NO_COLOR+x}" ]; then
        TB_OUTPUT_MODE=never
    elif [ -n "${TB_OUTPUT_COLOR:-}" ]; then
        TB_OUTPUT_MODE=$TB_OUTPUT_COLOR
    elif [ -t 1 ] && [ "${TERM:-}" != dumb ]; then
        case "${TERM:-}" in
            *256color*) TB_OUTPUT_MODE=256 ;;
            *) TB_OUTPUT_MODE=basic ;;
        esac
    else
        TB_OUTPUT_MODE=never
    fi
}

tb_output_time() {
    if [ -n "${TB_OUTPUT_CLOCK:-}" ]; then
        printf '%s' "$TB_OUTPUT_CLOCK"
    else
        date '+%H:%M:%S'
    fi
}

tb_output_line() {
    tb_level=$1
    tb_message=$2
    tb_color=$3
    if [ "$TB_OUTPUT_STARTED" -eq 0 ]; then
        printf '\n'
        TB_OUTPUT_STARTED=1
    fi
    tb_prefix="[$(tb_output_time) $TB_OUTPUT_TOOL $tb_level] $tb_message"
    if [ "$TB_OUTPUT_MODE" != never ] && [ -n "$tb_color" ]; then
        printf '\033[%sm%s\033[0m\n' "$tb_color" "$tb_prefix"
    else
        printf '%s\n' "$tb_prefix"
    fi
}

tb_info() {
    tb_output_line INFO "$1" ''
}

tb_prompt() {
    tb_output_line PROMPT "$1" 36
}

tb_menu() {
    tb_prompt "$1"
    shift
    for tb_menu_option in "$@"; do
        if [ "$TB_OUTPUT_MODE" != never ]; then
            printf '\033[36m  %s\033[0m\n' "$tb_menu_option"
        else
            printf '  %s\n' "$tb_menu_option"
        fi
    done
}

tb_warn() {
    tb_warning_color=33
    if [ "$TB_OUTPUT_MODE" = 256 ]; then
        tb_warning_color='38;5;208'
    fi
    tb_output_line WARN "$1" "$tb_warning_color"
}

tb_error() {
    tb_output_line ERROR "$1" 31
}

tb_success() {
    tb_output_line SUCCESS "$1" 32
}

tb_ask_yes_no() {
    tb_question=$1
    tb_default=$2
    while true; do
        if [ "$tb_default" = yes ] || [ "$tb_default" = y ]; then
            tb_marker='Y/n'
            tb_default_name=Y
        else
            tb_marker='y/N'
            tb_default_name=n
        fi
        tb_prompt "$tb_question [$tb_marker] (default: $tb_default_name):"
        if ! IFS= read -r TB_ANSWER; then
            return 1
        fi
        case "$TB_ANSWER" in
            '') TB_ANSWER=$tb_default; return 0 ;;
            y|Y|yes|YES|Yes) TB_ANSWER=yes; return 0 ;;
            n|N|no|NO|No) TB_ANSWER=no; return 0 ;;
            *) tb_warn "Enter y or n, or press Enter for $tb_default_name." ;;
        esac
    done
}

tb_ask_required() {
    tb_question=$1
    while true; do
        tb_prompt "$tb_question: "
        if ! IFS= read -r TB_ANSWER; then
            return 1
        fi
        if [ -n "$TB_ANSWER" ]; then
            return 0
        fi
        tb_warn "A value is required; enter a value to continue."
    done
}

tb_output_init vscode-split-terminal
fail() { tb_error "$1" >&2; exit 1; }
tb_info 'Checking Linux X11 and installer dependencies.'
[ "$(uname -s)" = Linux ] || fail 'This macro supports Linux only.'
if [ "${XDG_SESSION_TYPE:-}" = wayland ] || [ -n "${WAYLAND_DISPLAY:-}" ]; then
    fail 'Wayland is unsupported. Log into an X11 session and retry.'
fi
[ -n "${DISPLAY:-}" ] || fail 'An X11 display is required.'
missing=''
for dependency in code node zip xdotool wmctrl xrandr xprop xwininfo x-terminal-emulator; do
    if ! command -v "$dependency" >/dev/null 2>&1; then missing="$missing $dependency"; fi
done
[ -z "$missing" ] || fail "Missing:$missing. Install VS Code and your default terminal, then install the helpers (Debian/Ubuntu: sudo apt install nodejs zip xdotool wmctrl x11-xserver-utils x11-utils). Retry this installer."
work=$(mktemp -d)
trap 'rm -rf "$work"' 0
trap 'exit 130' INT
trap 'exit 143' TERM HUP
mkdir "$work/extension"
tb_info 'Generating the VS Code extension package.'
base64 -d > "$work/extension/icon.png" <<'TB_ICON'
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
TB_ICON
cat > "$work/extension/package.json" <<'TB_PACKAGE'
{
  "name": "terminal-beside",
  "displayName": "Terminal Beside",
  "description": "Open the default X11 terminal at the workspace root and tile it beside VS Code.",
  "version": "1.0.0",
  "publisher": "MatheusFS-dev",
  "engines": { "vscode": "^1.74.0" },
  "main": "./extension.js",
  "icon": "icon.png",
  "author": "MatheusFS-dev",
  "extensionKind": ["ui"],
  "capabilities": { "untrustedWorkspaces": { "supported": true }, "virtualWorkspaces": false },
  "contributes": {
    "commands": [{ "command": "terminalBeside.openTerminal", "title": "Terminal Beside: Open Terminal" }],
    "keybindings": [{ "command": "terminalBeside.openTerminal", "key": "ctrl+alt+s", "when": "isLinux" }],
    "configuration": { "title": "Terminal Beside", "properties": { "terminalBeside.terminalWidthPercent": { "type": "number", "default": 33.333333333333336, "minimum": 1, "maximum": 99, "description": "Width of the terminal as a percentage of the monitor work area." } } }
  }
}
TB_PACKAGE
cat > "$work/extension/extension.js" <<'TB_EXTENSION'
'use strict'
const childProcess = require('node:child_process')
const { promisify } = require('node:util')
const { randomUUID } = require('node:crypto')
const { basename } = require('node:path')

async function workspaceRoot(vscode) {
  const folders = vscode.workspace.workspaceFolders || []
  if (vscode.env.remoteName || folders.some(folder => folder.uri.scheme !== 'file')) {
    vscode.window.showWarningMessage('Split terminal supports local workspaces only. Open a local folder first.')
    return
  }
  if (!folders.length) {
    vscode.window.showWarningMessage('Open a workspace folder before opening a split terminal.')
    return
  }
  const folder = folders.length === 1 ? folders[0] : await vscode.window.showWorkspaceFolderPick({ placeHolder: 'Choose the terminal workspace root' })
  return folder?.uri.fsPath
}

function splitArea(monitor, workarea, side = 'Right', percent = 100 / 3) {
  const x = Math.max(monitor.x, workarea.x)
  const y = Math.max(monitor.y, workarea.y)
  const width = Math.min(monitor.x + monitor.width, workarea.x + workarea.width) - x
  const height = Math.min(monitor.y + monitor.height, workarea.y + workarea.height) - y
  if (width < 3 || height < 1) throw new Error('The monitor has no usable desktop area.')
  const terminalWidth = Math.max(1, Math.min(width - 1, Math.round(width * percent / 100)))
  const codeWidth = width - terminalWidth
  if (side === 'Left') return { terminal: { x, y, width: terminalWidth, height }, code: { x: x + terminalWidth, y, width: codeWidth, height } }
  return { code: { x, y, width: codeWidth, height }, terminal: { x: x + codeWidth, y, width: terminalWidth, height } }
}

function numbers(text) {
  const value = text.split('=')[1]
  if (!value || !/^\s*-?\d+(?:\s*,\s*-?\d+)*\s*$/.test(value)) throw new Error('Unable to read X11 desktop properties.')
  return value.match(/-?\d+/g).map(Number)
}

function createCommand(vscode, options = {}) {
  const env = options.env || process.env
  const platform = options.platform || process.platform
  const execFile = options.execFile || promisify(childProcess.execFile)
  const spawn = options.spawn || childProcess.spawn
  const sleep = options.sleep || (ms => new Promise(resolve => setTimeout(resolve, ms)))
  const now = options.now || Date.now
  const uuid = options.uuid || randomUUID
  const run = async (command, args) => {
    try {
      return String((await execFile(command, args, { env: { ...env, LC_ALL: 'C' }, timeout: 3000, maxBuffer: 1024 * 1024 })).stdout).trim()
    } catch (error) {
      throw new Error(`${command} failed: ${error.code === 'ENOENT' ? 'dependency missing. Re-run the macro installer.' : error.message}`)
    }
  }
  let busy = false
  return async () => {
    if (busy) return
    busy = true
    try {
      const side = await vscode.window.showQuickPick(['Left', 'Right'], { placeHolder: 'Place terminal on which side?' })
      if (!side) return
      const percent = vscode.workspace.getConfiguration('terminalBeside').get('terminalWidthPercent', 100 / 3)
      if (!Number.isFinite(percent) || percent < 1 || percent > 99) throw new Error('Terminal width must be between 1 and 99 percent.')
      const root = await workspaceRoot(vscode)
      if (!root) return
      if (platform !== 'linux' || env.XDG_SESSION_TYPE === 'wayland' || env.WAYLAND_DISPLAY || !env.DISPLAY) {
        throw new Error('Split terminal requires Linux X11. Wayland sessions are unsupported.')
      }
      // Validate every tool before launching, so missing dependencies cannot disturb the layout.
      for (const [name, args] of [
        ['xdotool', ['--version']], ['wmctrl', ['-m']], ['xrandr', ['--version']],
        ['xprop', ['-root', '_NET_CURRENT_DESKTOP']], ['xwininfo', ['-version']]
      ]) await run(name, args)
      const codeWindow = await run('xdotool', ['getactivewindow'])
      if (!/^\d+$/.test(codeWindow)) throw new Error('Cannot identify the focused VS Code window.')
      const wmClass = await run('xprop', ['-id', codeWindow, 'WM_CLASS'])
      if (!/"(?:(?:code|code-oss|vscodium)(?:-[^"]*)?|com\.microsoft\.vscode)"/i.test(wmClass)) throw new Error('Focus VS Code before opening a split terminal.')
      const geometry = await run('xdotool', ['getwindowgeometry', '--shell', codeWindow])
      const field = name => {
        const match = geometry.match(new RegExp(`^${name}=(-?\\d+)$`, 'm'))
        if (!match) throw new Error('Cannot read the VS Code window geometry.')
        return Number(match[1])
      }
      const center = { x: field('X') + field('WIDTH') / 2, y: field('Y') + field('HEIGHT') / 2 }
      const monitors = [...(await run('xrandr', ['--listactivemonitors'])).matchAll(/^\s*\d+:\s*\+(\*?)\S*\s+(\d+)\/\d+x(\d+)\/\d+([+-]\d+)([+-]\d+)/gm)]
        .map(match => ({ primary: match[1] === '*', width: Number(match[2]), height: Number(match[3]), x: Number(match[4]), y: Number(match[5]) }))
      const monitor = monitors.find(m => center.x >= m.x && center.x < m.x + m.width && center.y >= m.y && center.y < m.y + m.height)
      if (!monitor) throw new Error('Cannot identify the VS Code monitor.')
      const desktop = numbers(await run('xprop', ['-root', '_NET_CURRENT_DESKTOP']))[0]
      const areas = numbers(await run('xprop', ['-root', '_NET_WORKAREA']))
      const area = areas.slice(desktop * 4, desktop * 4 + 4)
      if (area.length !== 4) throw new Error('Cannot read the current desktop work area.')
      let layout = splitArea(monitor, { x: area[0], y: area[1], width: area[2], height: area[3] }, side, percent)
      const title = `tb-split-${uuid()}`
      let launchError
      const terminal = spawn('x-terminal-emulator', ['-T', title], { cwd: root, env, detached: true, stdio: 'ignore' })
      terminal.on('error', error => { launchError = error })
      terminal.on('exit', code => { if (code) launchError = new Error(`Terminal exited with status ${code}`) })
      terminal.unref()
      const deadline = now() + 5000
      let terminalWindow
      while (now() < deadline) {
        if (launchError) throw launchError
        const clients = await run('wmctrl', ['-l'])
        const candidates = clients.split('\n').filter(line => line.trim().endsWith(` ${title}`))
        for (const candidate of candidates) {
          const id = candidate.split(/\s+/)[0]
          if (!/^0x[\da-f]+$/i.test(id) || BigInt(id) === BigInt(codeWindow)) continue
          if (await run('xdotool', ['getwindowname', id]) === title) {
            // Require a mapped client with real geometry before touching VS Code.
            await run('xdotool', ['getwindowgeometry', '--shell', id])
            terminalWindow = id
            break
          }
        }
        if (terminalWindow) break
        await sleep(100)
      }
      if (!terminalWindow) throw new Error('Terminal startup timed out. VS Code was left in place. Check that the default terminal supports -T and creates a fresh window.')
      await run('wmctrl', ['-ir', terminalWindow, '-T', basename(root) || root])
      // The window manager or the application can move or resize a window after our request, so the
      // visible rectangle is measured and the layout re-applied until it matches.
      const measure = async id => {
        const info = await run('xwininfo', ['-id', id])
        const read = label => {
          const match = info.match(new RegExp(`${label}:\\s*(-?\\d+)`))
          if (!match) throw new Error('Cannot measure the window geometry.')
          return Number(match[1])
        }
        const box = { x: read('Absolute upper-left X'), y: read('Absolute upper-left Y'), width: read('Width'), height: read('Height') }
        const frame = await run('xprop', ['-id', id, '_NET_FRAME_EXTENTS'])
        if (/:.*not found\./.test(frame)) {
          const gtk = await run('xprop', ['-id', id, '_GTK_FRAME_EXTENTS'])
          if (!/:.*not found\./.test(gtk)) {
            const e = numbers(gtk)
            return { x: box.x + e[0], y: box.y + e[2], width: box.width - e[0] - e[1], height: box.height - e[2] - e[3] }
          }
        }
        // Server decorations lie outside the client area that xwininfo reports.
        const e = numbers(frame)
        return { x: box.x - e[0], y: box.y - e[2], width: box.width + e[0] + e[1], height: box.height + e[2] + e[3] }
      }
      const fits = (got, rect) => ['x', 'y', 'width', 'height'].every(key => Math.abs(got[key] - rect[key]) <= 2)
      const place = async (id, rect, recheck = false) => {
        let last = 'unknown'
        // Window managers differ on which corner wmctrl positions, so each miss corrects the next request.
        const adjust = { x: 0, y: 0, width: 0, height: 0 }
        for (let attempt = 0; attempt < 10; attempt++) {
          if (recheck && attempt === 0 && fits(await measure(id), rect)) return
          await run('wmctrl', ['-ir', id, '-b', 'remove,fullscreen'])
          await run('wmctrl', ['-ir', id, '-b', 'remove,maximized_vert,maximized_horz'])
          const deadline = now() + 2000
          while (/_(?:MAXIMIZED_VERT|MAXIMIZED_HORZ|FULLSCREEN)\b/.test(await run('xprop', ['-id', id, '_NET_WM_STATE']))) {
            if (now() >= deadline) throw new Error('The window manager did not restore the window.')
            await sleep(50)
          }
          const frame = await run('xprop', ['-id', id, '_NET_FRAME_EXTENTS'])
          const clientDecorations = /:.*not found\./.test(frame)
          const extents = numbers(clientDecorations ? await run('xprop', ['-id', id, '_GTK_FRAME_EXTENTS']) : frame)
          if (extents.length !== 4 || extents.some(value => value < 0)) throw new Error('Cannot read window decorations.')
          // GTK shadows are inside the client window. Server decorations are outside it.
          const x = rect.x - (clientDecorations ? extents[0] : 0) + adjust.x
          const y = rect.y - (clientDecorations ? extents[2] : 0) + adjust.y
          const sign = clientDecorations ? 1 : -1
          const baseWidth = rect.width + sign * (extents[0] + extents[1])
          const baseHeight = rect.height + sign * (extents[2] + extents[3])
          if (baseWidth < 1 || baseHeight < 1) throw new Error('Window decorations exceed the available area.')
          const width = Math.max(1, baseWidth + adjust.width)
          const height = Math.max(1, baseHeight + adjust.height)
          await run('wmctrl', ['-ir', id, '-e', `0,${x},${y},${width},${height}`])
          // wmctrl uses -1 as a sentinel. xdotool can place windows at that coordinate.
          if (x === -1 || y === -1) await run('xdotool', ['windowmove', '--', id, String(x), String(y)])
          await sleep(250)
          const got = await measure(id)
          last = `${got.width}x${got.height} at ${got.x},${got.y}`
          if (fits(got, rect)) return
          for (const key of Object.keys(adjust)) adjust[key] += rect[key] - got[key]
        }
        throw new Error(`The window manager did not accept the window layout (wanted ${rect.width}x${rect.height} at ${rect.x},${rect.y}, got ${last}).`)
      }
      // Panels and docks can differ per monitor, and X11 only reports one combined work area. Maximizing
      // the new terminal on VS Code's monitor shows the area the window manager really enforces there.
      const probeWorkarea = async () => {
        const state = async () => /_MAXIMIZED_VERT\b/.test(await run('xprop', ['-id', terminalWindow, '_NET_WM_STATE']))
        const settle = async wanted => {
          const limit = now() + 1500
          while ((await state()) !== wanted) {
            if (now() >= limit) throw new Error('probe timeout')
            await sleep(50)
          }
          await sleep(150)
        }
        try {
          await run('wmctrl', ['-ir', terminalWindow, '-b', 'remove,fullscreen'])
          await run('wmctrl', ['-ir', terminalWindow, '-b', 'remove,maximized_vert,maximized_horz'])
          await run('wmctrl', ['-ir', terminalWindow, '-e', `0,${monitor.x + 60},${monitor.y + 60},400,300`])
          await sleep(150)
          await run('wmctrl', ['-ir', terminalWindow, '-b', 'add,maximized_vert,maximized_horz'])
          await settle(true)
          const box = await measure(terminalWindow)
          await run('wmctrl', ['-ir', terminalWindow, '-b', 'remove,maximized_vert,maximized_horz'])
          await settle(false)
          // A maximized window that left the target monitor means the probe is not trustworthy.
          const cx = box.x + box.width / 2
          const cy = box.y + box.height / 2
          if (cx < monitor.x || cx >= monitor.x + monitor.width || cy < monitor.y || cy >= monitor.y + monitor.height) return
          layout = splitArea(monitor, box, side, percent)
        } catch {
          // Keep the combined work area from the window manager.
          try { await run('wmctrl', ['-ir', terminalWindow, '-b', 'remove,maximized_vert,maximized_horz']) } catch {}
        }
      }
      await probeWorkarea()
      await place(terminalWindow, layout.terminal)
      await place(codeWindow, layout.code)
      // Let late Electron resizes land, then confirm VS Code kept its slot.
      await sleep(400)
      await place(codeWindow, layout.code, true)
      await place(terminalWindow, layout.terminal, true)
      await run('xdotool', ['windowactivate', '--sync', terminalWindow])
    } catch (error) {
      vscode.window.showErrorMessage(`Split terminal: ${error.message}`)
    } finally {
      busy = false
    }
  }
}

function activate(context) {
  const vscode = require('vscode')
  context.subscriptions.push(vscode.commands.registerCommand('terminalBeside.openTerminal', createCommand(vscode)))
}
module.exports = { activate, workspaceRoot, splitArea, createCommand }
TB_EXTENSION
cat > "$work/extension/README.md" <<'TB_README'
# Terminal Beside

Terminal Beside opens a separate terminal window beside VS Code at the root of a local workspace. Choose whether the terminal appears on the left or right of the current monitor.

**Author:** MatheusFS-dev

## Supported systems

- Linux with an X11 session and a default `x-terminal-emulator` that accepts `-T TITLE`
- Windows with Windows Terminal (`wt.exe`)

Remote workspaces, virtual workspaces, and Linux Wayland sessions are not supported.

## Install

The standalone installers in this directory build and install the extension through the VS Code CLI. On Linux, run `sh split_terminal.sh`. On Windows, run `powershell -ExecutionPolicy Bypass -File split_terminal.ps1`. Reload existing VS Code windows after installation.

If you previously installed `my-toolbox.split-terminal`, uninstall it before using Terminal Beside to avoid two extensions claiming the same shortcut.

## Use

Open a local workspace in VS Code, then press **Ctrl+Alt+S** or run **Terminal Beside: Open Terminal** from the Command Palette. Choose **Left** or **Right**. Cancelling the choice opens nothing.

The terminal starts at the selected workspace root and receives keyboard focus. In a workspace with several folders, choose the folder before the terminal opens.

## Configure width

In VS Code Settings, search for **Terminal Beside: Terminal Width Percent**. Enter a value from 1 to 99. The default is one third of the monitor work area. The setting also works in `settings.json`:

```json
"terminalBeside.terminalWidthPercent": 40
```

This example gives the terminal 40% of the available width and VS Code the remaining 60%.

## Uninstall

Run `code --uninstall-extension MatheusFS-dev.terminal-beside`.
TB_README
cat > "$work/extension.vsixmanifest" <<'TB_MANIFEST'
<?xml version="1.0" encoding="utf-8"?>
<PackageManifest Version="2.0.0" xmlns="http://schemas.microsoft.com/developer/vsx-schema/2011">
  <Metadata>
    <Identity Language="en-US" Id="terminal-beside" Version="1.0.0" Publisher="MatheusFS-dev" />
    <DisplayName>Terminal Beside</DisplayName>
    <Description xml:space="preserve">Open a workspace terminal and tile it beside VS Code on Linux X11.</Description>
    <Tags>terminal,linux</Tags><Categories>Other</Categories><GalleryFlags>Public</GalleryFlags>
    <Properties><Property Id="Microsoft.VisualStudio.Code.Engine" Value="^1.74.0" /><Property Id="Microsoft.VisualStudio.Code.ExtensionKind" Value="ui" /></Properties>
  </Metadata>
  <Installation><InstallationTarget Id="Microsoft.VisualStudio.Code" /></Installation>
  <Dependencies />
  <Assets><Asset Type="Microsoft.VisualStudio.Code.Manifest" Path="extension/package.json" Addressable="true" /><Asset Type="Microsoft.VisualStudio.Code.Readme" Path="extension/README.md" Addressable="true" /><Asset Type="Microsoft.VisualStudio.Code.Icons.Default" Path="extension/icon.png" Addressable="true" /></Assets>
</PackageManifest>
TB_MANIFEST
cat > "$work/[Content_Types].xml" <<'TB_TYPES'
<?xml version="1.0" encoding="utf-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="png" ContentType="image/png"/><Default Extension="json" ContentType="application/json"/><Default Extension="js" ContentType="application/javascript"/><Default Extension="vsixmanifest" ContentType="text/xml"/><Default Extension="xml" ContentType="text/xml"/><Default Extension="" ContentType="text/plain"/></Types>
TB_TYPES
# Keep the package local and self-contained, without npm or marketplace access.
(cd "$work" && zip -q -r terminal-beside.vsix extension extension.vsixmanifest '[Content_Types].xml') || fail 'Could not create the VSIX package.'

# Check default and named profiles. JSONC comments and trailing commas are allowed.
tb_info 'Checking existing Ctrl+Alt+S user bindings.'
if ! node - "$work" <<'TB_BINDINGS'
const fs = require('node:fs')
const path = require('node:path')
const base = path.join(process.env.XDG_CONFIG_HOME || path.join(process.env.HOME, '.config'), 'Code', 'User')
function check(file) {
  if (!fs.existsSync(file)) return false
  let text = fs.readFileSync(file, 'utf8')
  // Preserve quoted strings while removing JSONC comments and trailing commas.
  text = text.replace(/"(?:\\.|[^"\\])*"|\/\/[^\n]*|\/\*[\s\S]*?\*\//g, token => token.startsWith('"') ? token : '')
  text = text.replace(/"(?:\\.|[^"\\])*"|,\s*(?=[}\]])/g, token => token.startsWith('"') ? token : '')
  return JSON.parse(text).some(binding => String(binding.key || '').toLowerCase().split('+').sort().join('+') === 'alt+ctrl+s' && binding.command !== 'terminalBeside.openTerminal')
}
try {
  const files = [path.join(base, 'keybindings.json')]
  const profiles = path.join(base, 'profiles')
  if (fs.existsSync(profiles)) for (const entry of fs.readdirSync(profiles)) files.push(path.join(profiles, entry, 'keybindings.json'))
  process.exit(files.some(check) ? 2 : 0)
} catch { process.exit(3) }
TB_BINDINGS
then
    tb_warn 'An existing Ctrl+Alt+S binding was found, or user bindings could not be parsed. Review Keyboard Shortcuts if the shortcut does not run Terminal Beside: Open Terminal. User bindings were preserved.'
fi
tb_info 'Installing MatheusFS-dev.terminal-beside through the VS Code CLI.'
if ! code --install-extension "$work/terminal-beside.vsix" --force > "$work/install.log" 2>&1; then
    tb_error 'VS Code extension installation failed. CLI output follows.' >&2
    cat "$work/install.log" >&2
    exit 1
fi
tb_success 'Installed VS Code split terminal. Reload existing VS Code windows, then press Ctrl+Alt+S in a local workspace and choose Left or Right. Remove with: code --uninstall-extension MatheusFS-dev.terminal-beside'
