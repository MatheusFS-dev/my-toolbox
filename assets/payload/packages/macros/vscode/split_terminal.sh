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
cat > "$work/extension/package.json" <<'TB_PACKAGE'
{
  "name": "split-terminal",
  "displayName": "Toolbox Split Terminal",
  "description": "Open the default X11 terminal at the workspace root and tile it beside VS Code.",
  "version": "1.0.0",
  "publisher": "my-toolbox",
  "engines": { "vscode": "^1.74.0" },
  "main": "./extension.js",
  "extensionKind": ["ui"],
  "capabilities": { "untrustedWorkspaces": { "supported": true }, "virtualWorkspaces": false },
  "contributes": {
    "commands": [{ "command": "myToolbox.openSplitTerminal", "title": "Toolbox: Open Split Terminal" }],
    "keybindings": [{ "command": "myToolbox.openSplitTerminal", "key": "ctrl+alt+s", "when": "isLinux" }]
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

function splitArea(monitor, workarea) {
  const x = Math.max(monitor.x, workarea.x)
  const y = Math.max(monitor.y, workarea.y)
  const width = Math.min(monitor.x + monitor.width, workarea.x + workarea.width) - x
  const height = Math.min(monitor.y + monitor.height, workarea.y + workarea.height) - y
  if (width < 3 || height < 1) throw new Error('The monitor has no usable desktop area.')
  const left = Math.floor(width * 2 / 3)
  return { code: { x, y, width: left, height }, terminal: { x: x + left, y, width: width - left, height } }
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
      let layout = splitArea(monitor, { x: area[0], y: area[1], width: area[2], height: area[3] })
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
          layout = splitArea(monitor, box)
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
  context.subscriptions.push(vscode.commands.registerCommand('myToolbox.openSplitTerminal', createCommand(vscode)))
}
module.exports = { activate, workspaceRoot, splitArea, createCommand }
TB_EXTENSION
cat > "$work/extension.vsixmanifest" <<'TB_MANIFEST'
<?xml version="1.0" encoding="utf-8"?>
<PackageManifest Version="2.0.0" xmlns="http://schemas.microsoft.com/developer/vsx-schema/2011">
  <Metadata>
    <Identity Language="en-US" Id="split-terminal" Version="1.0.0" Publisher="my-toolbox" />
    <DisplayName>Toolbox Split Terminal</DisplayName>
    <Description xml:space="preserve">Open a workspace terminal and tile it beside VS Code on Linux X11.</Description>
    <Tags>terminal,linux</Tags><Categories>Other</Categories><GalleryFlags>Public</GalleryFlags>
    <Properties><Property Id="Microsoft.VisualStudio.Code.Engine" Value="^1.74.0" /><Property Id="Microsoft.VisualStudio.Code.ExtensionKind" Value="ui" /></Properties>
  </Metadata>
  <Installation><InstallationTarget Id="Microsoft.VisualStudio.Code" /></Installation>
  <Dependencies />
  <Assets><Asset Type="Microsoft.VisualStudio.Code.Manifest" Path="extension/package.json" Addressable="true" /></Assets>
</PackageManifest>
TB_MANIFEST
cat > "$work/[Content_Types].xml" <<'TB_TYPES'
<?xml version="1.0" encoding="utf-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="json" ContentType="application/json"/><Default Extension="js" ContentType="application/javascript"/><Default Extension="vsixmanifest" ContentType="text/xml"/><Default Extension="xml" ContentType="text/xml"/><Default Extension="" ContentType="text/plain"/></Types>
TB_TYPES
# Keep the package local and self-contained, without npm or marketplace access.
(cd "$work" && zip -q -r split-terminal.vsix extension extension.vsixmanifest '[Content_Types].xml') || fail 'Could not create the VSIX package.'

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
  return JSON.parse(text).some(binding => String(binding.key || '').toLowerCase().split('+').sort().join('+') === 'alt+ctrl+s' && binding.command !== 'myToolbox.openSplitTerminal')
}
try {
  const files = [path.join(base, 'keybindings.json')]
  const profiles = path.join(base, 'profiles')
  if (fs.existsSync(profiles)) for (const entry of fs.readdirSync(profiles)) files.push(path.join(profiles, entry, 'keybindings.json'))
  process.exit(files.some(check) ? 2 : 0)
} catch { process.exit(3) }
TB_BINDINGS
then
    tb_warn 'An existing Ctrl+Alt+S binding was found, or user bindings could not be parsed. Review Keyboard Shortcuts if the shortcut does not run Toolbox: Open Split Terminal. User bindings were preserved.'
fi
tb_info 'Installing my-toolbox.split-terminal through the VS Code CLI.'
if ! code --install-extension "$work/split-terminal.vsix" --force > "$work/install.log" 2>&1; then
    tb_error 'VS Code extension installation failed. CLI output follows.' >&2
    cat "$work/install.log" >&2
    exit 1
fi
tb_success 'Installed VS Code split terminal. Reload existing VS Code windows, then press Ctrl+Alt+S in a local workspace. Remove with: code --uninstall-extension my-toolbox.split-terminal'
