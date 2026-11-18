const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const vm = require('node:vm')
const path = require('node:path')
const installer = path.join(__dirname, '../../assets/payload/packages/macros/vscode/split_terminal.sh')
function load() {
  const source = fs.existsSync(installer) ? fs.readFileSync(installer, 'utf8').split("<<'TB_EXTENSION'\n")[1]?.split('\nTB_EXTENSION')[0] : ''
  const sandbox = { module: { exports: {} }, require, process, setTimeout, Buffer }
  vm.runInNewContext(source || '', sandbox)
  return sandbox.module.exports
}
test('chooses workspace roots and rejects remote folders', async () => {
  const { workspaceRoot } = load()
  assert.equal(typeof workspaceRoot, 'function')
  const messages = []
  const folder = name => ({ name, uri: { scheme: 'file', fsPath: '/projects/' + name } })
  let picked = 0
  const api = { env: {}, workspace: { workspaceFolders: [folder('one')] }, window: {
    showWarningMessage: m => messages.push(m), showWorkspaceFolderPick: async () => { picked++; return folder('two') }
  } }
  assert.equal(await workspaceRoot(api), '/projects/one')
  api.workspace.workspaceFolders.push(folder('two'))
  assert.equal(await workspaceRoot(api), '/projects/two')
  assert.equal(picked, 1)
  api.window.showWorkspaceFolderPick = async () => undefined
  assert.equal(await workspaceRoot(api), undefined)
  api.workspace.workspaceFolders = []
  assert.equal(await workspaceRoot(api), undefined)
  api.workspace.workspaceFolders = [folder('one')]
  api.env.remoteName = 'ssh-remote'
  assert.equal(await workspaceRoot(api), undefined)
  delete api.env.remoteName
  api.workspace.workspaceFolders[0].uri.scheme = 'vscode-remote'
  assert.equal(await workspaceRoot(api), undefined)
  assert.equal(messages.length, 3)
})
test('splits monitor and workarea intersections including negative origins', () => {
  const { splitArea } = load()
  assert.equal(typeof splitArea, 'function')
  assert.deepEqual(JSON.parse(JSON.stringify(splitArea({ x: -1920, y: 0, width: 1920, height: 1080 }, { x: -1920, y: 32, width: 3840, height: 1048 }))), {
    code: { x: -1920, y: 32, width: 1280, height: 1048 }, terminal: { x: -640, y: 32, width: 640, height: 1048 }
  })
  assert.throws(() => splitArea({ x: 0, y: 0, width: 10, height: 10 }, { x: 20, y: 0, width: 10, height: 10 }))
})

function runtime(config = {}) {
  const { createCommand } = load()
  const calls = []
  const errors = []
  const launches = []
  let time = 0
  let lists = 0
  let stateReads = 0
  const boxes = {}
  const maxed = new Set()
  const ignored = {}
  const api = { env: {}, workspace: { workspaceFolders: [{ uri: { scheme: 'file', fsPath: "/tmp/project with ' quotes;$()" } }] }, window: { showErrorMessage: message => errors.push(message) } }
  const execFile = async (command, args, options) => {
    calls.push([command, [...args]])
    assert.equal(options.timeout, 3000)
    if (config.missing === command) throw Object.assign(new Error('missing'), { code: 'ENOENT' })
    let stdout = ''
    if (command === 'xdotool') {
      if (args[0] === 'getactivewindow') stdout = '100'
      if (args[0] === 'getwindowgeometry') {
        const id = args.at(-1)
        stdout = `WINDOW=${id}\nX=-1920\nY=0\nWIDTH=1920\nHEIGHT=1080\nSCREEN=0`
      }
      if (args[0] === 'getwindowname') stdout = config.wrongTitle ? 'other terminal' : 'tb-split-unique'
    }
    if (command === 'xprop') {
      if (args.includes('WM_CLASS')) stdout = config.wmClass || (config.wrongFocus ? 'WM_CLASS(STRING) = "kitty", "kitty"' : 'WM_CLASS(STRING) = "code", "Code"')
      if (args.includes('_NET_CURRENT_DESKTOP')) stdout = '_NET_CURRENT_DESKTOP(CARDINAL) = 1'
      if (args.includes('_NET_WORKAREA')) stdout = '_NET_WORKAREA(CARDINAL) = -1920, 0, 3840, 1080, -1920, 32, 3840, 1002'
      if (args.includes('_NET_FRAME_EXTENTS')) stdout = (config.noExtents || (config.gtkDecorations && args.includes('100'))) ? '_NET_FRAME_EXTENTS: not found.' : '_NET_FRAME_EXTENTS(CARDINAL) = 2, 2, 30, 2'
      if (args.includes('_GTK_FRAME_EXTENTS') && config.gtkDecorations) stdout = '_GTK_FRAME_EXTENTS(CARDINAL) = 16, 16, 10, 32'
      if (args.includes('_NET_WM_STATE')) {
        stdout = '_NET_WM_STATE(ATOM) = '
        if (args.includes('0x000000c8') && maxed.has('0x000000c8')) stdout += '_NET_WM_STATE_MAXIMIZED_VERT, _NET_WM_STATE_MAXIMIZED_HORZ'
        if (config.stuckMaximized || (config.maximized && stateReads++ === 0)) stdout += '_NET_WM_STATE_MAXIMIZED_VERT, _NET_WM_STATE_MAXIMIZED_HORZ'
      }
    }
    if (command === 'xwininfo') {
      const id = args[1]
      const box = boxes[id] || (id === '100' ? [-1920, 0, 1920, 1080] : [0, 0, 640, 480])
      // xwininfo reports the client area.
      stdout = `  Absolute upper-left X:  ${box[0]}\n  Absolute upper-left Y:  ${box[1]}\n  Width: ${box[2]}\n  Height: ${box[3]}`
    }
    if (command === 'xrandr' && args[0] === '--listactivemonitors') stdout = 'Monitors: 2\n 0: +DP1 1920/509x1080/286-1920+0 DP1\n 1: +*DP2 1920/509x1080/286+0+0 DP2'
    if (command === 'wmctrl' && args[0] === '-l') {
      lists++
      if (!config.timeout && lists > (config.delay || 0)) stdout = '0x000000c8 1 host tb-split-unique'
    }
    if (command === 'wmctrl' && args.at(-1) === 'add,maximized_vert,maximized_horz' && args[1] === '0x000000c8') {
      if (!config.maxBox) throw new Error('probe unavailable')
      maxed.add(args[1])
      const [x, y, w, h] = config.maxBox
      boxes[args[1]] = [x + 2, y + 30, w - 4, h - 32]
    }
    if (command === 'wmctrl' && args.at(-1) === 'remove,maximized_vert,maximized_horz') maxed.delete(args[1])
    if (command === 'wmctrl' && args.includes('-e')) {
      const [, , , w, h] = args.at(-1).split(',').map(Number)
      const [, x, y] = args.at(-1).split(',').map(Number)
      if ((ignored[args[1]] = (ignored[args[1]] || 0) + 1) > (config.ignoreFirst || 0)) boxes[args[1]] = (config.gtkDecorations && args[1] === '100') ? [x, y, w, h] : [x + 2, y + 30, w, h]
    }
    if (command === 'wmctrl' && args.includes('-b')) {
      assert.ok(args.at(-1).split(',').length <= 3, 'wmctrl supports at most two state properties per call')
    }
    return { stdout }
  }
  const spawn = (command, args, options) => {
    launches.push({ command, args, options })
    return { on(event, listener) { if (config.launchError && event === 'error') listener(new Error('launch denied')) }, unref() {} }
  }
  const command = createCommand(api, { execFile, spawn, env: { XDG_SESSION_TYPE: config.wayland ? 'wayland' : 'x11', DISPLAY: ':1', ...(config.gnome ? { XDG_CURRENT_DESKTOP: 'ubuntu:GNOME' } : {}) }, platform: config.platform || 'linux', now: () => time, sleep: async ms => { time += ms }, uuid: () => 'unique' })
  return { command, calls, errors, launches }
}

test('launches safely, waits for restoration, tiles usable space and focuses terminal', async () => {
  const fixture = runtime({ maximized: true, delay: 2 })
  await fixture.command()
  assert.deepEqual(fixture.errors, [])
  assert.equal(fixture.launches.length, 1)
  const launch = fixture.launches[0]
  assert.equal(launch.command, 'x-terminal-emulator')
  assert.equal(JSON.stringify(launch.args), '["-T","tb-split-unique"]')
  assert.equal(launch.options.cwd, "/tmp/project with ' quotes;$()")
  assert.equal(launch.options.shell, undefined)
  assert.deepEqual(fixture.calls.filter(([name, args]) => name === 'wmctrl' && args.includes('-e')), [
    ['wmctrl', ['-ir', '0x000000c8', '-e', '0,-1860,60,400,300']],
    ['wmctrl', ['-ir', '0x000000c8', '-e', '0,-640,32,636,970']],
    ['wmctrl', ['-ir', '100', '-e', '0,-1920,32,1276,970']]
  ])
  assert.deepEqual(fixture.calls.at(-1), ['xdotool', ['windowactivate', '--sync', '0x000000c8']])
})

test('re-applies the layout when the window manager reverts the first resize', async () => {
  const fixture = runtime({ ignoreFirst: 2 })
  await fixture.command()
  assert.deepEqual(fixture.errors, [])
  const moves = fixture.calls.filter(([name, args]) => name === 'wmctrl' && args.includes('-e') && args[1] === '100')
  assert.ok(moves.length >= 3)
})

test('reports an error when a window never accepts its size', async () => {
  const fixture = runtime({ ignoreFirst: 99 })
  await fixture.command()
  assert.match(fixture.errors[0], /did not accept the window layout/)
})

test('measures the real work area of the VS Code monitor by maximizing the terminal', async () => {
  const fixture = runtime({ maxBox: [-1920, 0, 1920, 1034] })
  await fixture.command()
  assert.deepEqual(fixture.errors, [])
  const moves = fixture.calls.filter(([name, args]) => name === 'wmctrl' && args.includes('-e') && args[1] === '100')
  assert.equal(moves.at(-1)[1].at(-1), '0,-1920,0,1276,1002')
})

test('startup timeout, wrong title, launch error and missing tools leave Code unchanged', async () => {
  for (const config of [{ timeout: true }, { wrongTitle: true }, { launchError: true }, { missing: 'wmctrl' }, { missing: 'xdotool' }, { missing: 'xrandr' }, { missing: 'xprop' }, { wrongFocus: true }, { wayland: true }, { platform: 'win32' }]) {
    const fixture = runtime(config)
    await fixture.command()
    assert.equal(fixture.errors.length, 1, JSON.stringify(config))
    assert.equal(fixture.calls.filter(([, args]) => args.includes('-ir')).length, 0)
    if (config.missing || config.wayland) assert.equal(fixture.launches.length, 0)
  }
})

test('ignores concurrent invocation and permits a fresh terminal after completion', async () => {
  const fixture = runtime()
  await Promise.all([fixture.command(), fixture.command()])
  assert.equal(fixture.launches.length, 1)
  assert.deepEqual(fixture.errors, [])
  await fixture.command()
  assert.equal(fixture.launches.length, 2)
})


test('terminal placement failures leave VS Code unchanged', async () => {
  for (const config of [{ noExtents: true }, { stuckMaximized: true }]) {
    const fixture = runtime(config)
    await fixture.command()
    assert.equal(fixture.errors.length, 1)
    assert.equal(fixture.calls.filter(([, args]) => args.includes('-ir') && args.includes('100')).length, 0)
    assert.equal(fixture.calls.filter(([, args]) => args[0] === 'windowactivate').length, 0)
  }
})

test('workarea intersection preserves positive monitor offsets and bottom panels', () => {
  const { splitArea } = load()
  assert.equal(JSON.stringify(splitArea({ x: 1920, y: 100, width: 1920, height: 1080 }, { x: 0, y: 32, width: 3840, height: 1100 })), JSON.stringify({
    code: { x: 1920, y: 100, width: 1280, height: 1032 }, terminal: { x: 3200, y: 100, width: 640, height: 1032 }
  }))
})


test('accepts the reverse-domain WM_CLASS reported by the installed VS Code', async () => {
  const fixture = runtime({ wmClass: 'WM_CLASS(STRING) = "com.microsoft.vscode", "com.microsoft.VSCode"' })
  await fixture.command()
  assert.deepEqual(fixture.errors, [])
  assert.equal(fixture.launches.length, 1)
  assert.deepEqual(fixture.calls.at(-1), ['xdotool', ['windowactivate', '--sync', '0x000000c8']])
})


test('accounts for GTK client-side shadows when Code has no server frame', async () => {
  const fixture = runtime({ wmClass: 'WM_CLASS(STRING) = "com.microsoft.vscode", "com.microsoft.VSCode"', gtkDecorations: true })
  await fixture.command()
  assert.deepEqual(fixture.errors, [])
  assert.deepEqual(fixture.calls.filter(([name, args]) => name === 'wmctrl' && args.includes('-e')), [
    ['wmctrl', ['-ir', '0x000000c8', '-e', '0,-1860,60,400,300']],
    ['wmctrl', ['-ir', '0x000000c8', '-e', '0,-640,32,636,970']],
    ['wmctrl', ['-ir', '100', '-e', '0,-1936,22,1312,1044']]
  ])
})


test('renames the identified terminal to the project directory name', async () => {
  const fixture = runtime()
  await fixture.command()
  assert.deepEqual(fixture.errors, [])
  const rename = fixture.calls.findIndex(([name, args]) => name === 'wmctrl' && args.includes('-T'))
  const identity = fixture.calls.findIndex(([name, args]) => name === 'xdotool' && args[0] === 'getwindowname')
  assert.ok(rename > identity && identity >= 0, 'rename must follow unique-window identification')
  assert.deepEqual(fixture.calls[rename], ['wmctrl', ['-ir', '0x000000c8', '-T', "project with ' quotes;$()"]])
})
