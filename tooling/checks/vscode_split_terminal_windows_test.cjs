const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const vm = require('node:vm')
const path = require('node:path')
const os = require('node:os')
const { EventEmitter } = require('node:events')
const childProcess = require('node:child_process')
const installer = path.join(__dirname, '../../assets/payload/packages/macros/vscode/split_terminal.ps1')
const source = fs.readFileSync(installer, 'utf8')

function embedded(name) {
  const marker = `Save-File (Join-Path $Extension '${name}') @'\n`
  const start = source.indexOf(marker)
  assert.ok(start >= 0, `${name} missing from the installer`)
  const body = source.slice(start + marker.length)
  return body.slice(0, body.indexOf("\n'@"))
}
function load() {
  const sandbox = { module: { exports: {} }, require, process, setTimeout, clearTimeout, Buffer, __dirname: '/ext' }
  vm.runInNewContext(embedded('extension.js'), sandbox)
  return sandbox.module.exports
}

test('manifest binds Ctrl+Alt+S on Windows only', () => {
  const manifest = JSON.parse(embedded('package.json'))
  assert.deepEqual(manifest.contributes.keybindings, [{ command: 'terminalBeside.openTerminal', key: 'ctrl+alt+s', when: 'isWindows' }])
  assert.equal(manifest.name, 'terminal-beside')
  assert.equal(manifest.publisher, 'MatheusFS-dev')
  assert.equal(manifest.author, 'MatheusFS-dev')
  assert.equal(manifest.displayName, 'Terminal Beside')
  assert.equal(manifest.icon, 'icon.png')
  const encoded = source.split("    Save-File (Join-Path $Extension 'icon.base64') @'\n")[1]?.split("\n'@")[0]
  assert.ok(encoded)
  assert.deepEqual(Buffer.from(encoded, 'base64'), fs.readFileSync(path.join(path.dirname(installer), 'icon.png')))
  assert.match(embedded('README.md'), /^# Terminal Beside/m)
})

test('chooses workspace roots and rejects remote folders', async () => {
  const { workspaceRoot } = load()
  const messages = []
  const folder = name => ({ name, uri: { scheme: 'file', fsPath: 'C:\\projects\\' + name } })
  const api = { env: {}, workspace: { getConfiguration: () => ({ get: () => config.percent || 100 / 3 }), workspaceFolders: [folder('one')] }, window: {
    showWarningMessage: m => messages.push(m), showWorkspaceFolderPick: async () => folder('two')
  } }
  assert.equal(await workspaceRoot(api), 'C:\\projects\\one')
  api.workspace.workspaceFolders.push(folder('two'))
  assert.equal(await workspaceRoot(api), 'C:\\projects\\two')
  api.workspace.workspaceFolders = []
  assert.equal(await workspaceRoot(api), undefined)
  api.workspace.workspaceFolders = [folder('one')]
  api.env.remoteName = 'wsl'
  assert.equal(await workspaceRoot(api), undefined)
  assert.equal(messages.length, 2)
})

function runtime(config = {}) {
  const { createCommand } = load()
  const calls = []
  const errors = []
  const launches = []
  const pairs = []
  const root = config.root || 'C:\\work\\my project'
  const picks = []
  const settings = { smart: config.smart ?? true, terminalWidthPercent: config.percent || 100 / 3 }
  const api = { env: {}, workspace: { getConfiguration: () => ({ get: (key, fallback) => key in settings ? settings[key] : fallback }), workspaceFolders: [{ uri: { scheme: 'file', fsPath: root } }] }, window: { showErrorMessage: message => errors.push(message), showQuickPick: async choices => { picks.push(choices); return config.side === 'cancel' ? undefined : config.side || 'Right' } } }
  const execFile = async (command, args) => {
    calls.push([command, args])
    assert.equal(command, 'powershell.exe')
    const mode = args[args.indexOf('-Mode') + 1]
    if (mode === 'Probe') return { stdout: config.notCode ? 'NOTCODE\r\n' : 'CODE=4242\r\nTERMINALS=11,22\r\n' }
    if (config.placeFails) throw Object.assign(new Error('Command failed'), { stderr: 'Terminal startup timed out. VS Code was left in place.\r\n' })
    return { stdout: 'OK\r\nTERMINAL=5151\r\n' }
  }
  const pairProcess = (command, args, options) => {
    const child = new EventEmitter()
    child.stdout = Object.assign(new EventEmitter(), { destroy() {} })
    child.stderr = Object.assign(new EventEmitter(), { destroy() {} })
    child.unref = () => { child.unrefCalled = true }
    pairs.push({ command, args, options, child })
    setImmediate(() => {
      if (config.pair === 'error') child.emit('error', new Error('spawn powershell.exe ENOENT'))
      else if (config.pair === 'exit') { child.stderr.emit('data', Buffer.from('Add-Type failed: bad C#\r\n')); child.emit('exit', 1); child.emit('close', 1) }
      // Node may report exit before the stdio streams are drained, so stderr and READY can arrive after it.
      else if (config.pair === 'late-stderr') { child.emit('exit', 1); child.stderr.emit('data', Buffer.from('Add-Type failed: bad C#\r\n')); child.emit('close', 1) }
      else if (config.pair === 'late-ready') { child.emit('exit', 0); child.stdout.emit('data', Buffer.from('READY\r\n')); child.emit('close', 0) }
      else if (config.pair !== 'silent') { child.stdout.emit('data', Buffer.from('REA')); child.stdout.emit('data', Buffer.from('DY\r\n')) }
    })
    return child
  }
  const spawn = (command, args, options) => {
    if (command === 'powershell.exe') return config.realPair ? childProcess.spawn(command, args, options) : pairProcess(command, args, options)
    launches.push({ command, args, options })
    return { on(event, listener) { if (config.launchError && event === 'error') listener(new Error('wt missing')) }, unref() {} }
  }
  const command = createCommand(api, { execFile, spawn, env: {}, platform: config.platform || 'win32', helper: 'C:\\ext\\split.ps1', pairHelper: config.pairHelper || 'C:\\ext\\pair.ps1', pairTimeoutMs: config.pairTimeoutMs || 30 })
  return { command, calls, errors, launches, picks, pairs }
}

test('cancelling side choice launches nothing', async () => {
  const fixture = runtime({ side: 'cancel' })
  await fixture.command()
  assert.equal(fixture.picks.length, 1)
  assert.deepEqual(fixture.launches, [])
  assert.deepEqual(fixture.calls, [])
})

test('passes left choice to placement helper', async () => {
  const fixture = runtime({ side: 'Left', percent: 40 })
  await fixture.command()
  assert.equal(JSON.stringify(fixture.calls[1][1].slice(-4)), JSON.stringify(['-Side', 'Left', '-Percent', '40']))
  const helper = embedded('split.ps1')
  assert.match(helper, /\$TerminalX = if \(\$Side -eq 'Left'\) \{ \$X \}/)
  assert.match(helper, /\$CodeX = if \(\$Side -eq 'Left'\) \{ \$X \+ \$TerminalWidth \}/)
})

test('probes focus, launches Windows Terminal safely and places both windows', async () => {
  const fixture = runtime()
  await fixture.command()
  assert.deepEqual(fixture.errors, [])
  assert.equal(fixture.launches.length, 1)
  const launch = fixture.launches[0]
  assert.equal(launch.command, 'wt.exe')
  assert.equal(JSON.stringify(launch.args), JSON.stringify(['-w', 'new', '--title', 'my project', '--suppressApplicationTitle', '-d', 'C:\\work\\my project']))
  assert.equal(launch.options.cwd, 'C:\\work\\my project')
  assert.equal(launch.options.shell, undefined)
  const place = fixture.calls[1][1]
  assert.equal(JSON.stringify(place.slice(place.indexOf('-Mode'))), JSON.stringify(['-Mode', 'Place', '-CodeHandle', '4242', '-Known', '11,22', '-TimeoutMs', '8000', '-Side', 'Right', '-Percent', String(100 / 3)]))
  assert.ok(fixture.calls[0][1].includes('-NonInteractive') && fixture.calls[0][1].includes('C:\\ext\\split.ps1'))
})

test('escapes semicolons in the starting directory', async () => {
  const fixture = runtime({ root: 'C:\\work\\a;b' })
  await fixture.command()
  assert.equal(fixture.launches[0].args.at(-1), 'C:\\work\\a\\;b')
})

test('refuses non-Windows, unfocused VS Code, placement and launch failures', async () => {
  for (const config of [{ platform: 'linux' }, { notCode: true }, { placeFails: true }, { launchError: true, placeFails: true }]) {
    const fixture = runtime(config)
    await fixture.command()
    assert.equal(fixture.errors.length, 1, JSON.stringify(config))
    if (config.platform || config.notCode) assert.equal(fixture.launches.length, 0, JSON.stringify(config))
  }
  const failed = runtime({ placeFails: true })
  await failed.command()
  assert.match(failed.errors[0], /timed out/)
  const launchFailed = runtime({ launchError: true, placeFails: true })
  await launchFailed.command()
  assert.match(launchFailed.errors[0], /wt missing/)
})

test('ignores concurrent invocation and permits a fresh terminal after completion', async () => {
  const fixture = runtime()
  await Promise.all([fixture.command(), fixture.command()])
  assert.equal(fixture.launches.length, 1)
  await fixture.command()
  assert.equal(fixture.launches.length, 2)
})

test('installer embeds pair.ps1 in the archive', () => {
  assert.ok(source.includes("'extension/pair.ps1'"))
  assert.match(embedded('pair.ps1'), /TbGeometry/)
})

function csharpOf(scriptText) {
  const match = /Add-Type -TypeDefinition @"\n([\s\S]*?)\n"@/.exec(scriptText)
  assert.ok(match, 'pair.ps1 has no Add-Type block')
  return match[1]
}

// Compiles the embedded C# once and prints one line per call: "x,y,w,h", "null", "True" or "False".
function evaluate(calls) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'tb-geometry-'))
  try {
    const file = path.join(dir, 'geometry.cs')
    fs.writeFileSync(file, csharpOf(embedded('pair.ps1')))
    const frame = rect => '[int[]](' + rect.join(',') + ')'
    const lines = calls.map(call => {
      const args = call.args.map(arg => Array.isArray(arg) ? frame(arg) : String(arg)).join(', ')
      return '$r = [TbGeometry]::' + call.fn + '(' + args + '); if ($null -eq $r) { "null" } elseif ($r -is [bool]) { [string]$r } else { $r -join "," }'
    })
    const script = ["Add-Type -Path '" + file + "'", ...lines].join('\n')
    const out = childProcess.execFileSync('powershell.exe', ['-NoProfile', '-NonInteractive', '-Command', script], { encoding: 'utf8' })
    return out.trim().split(/\r?\n/)
  } finally {
    fs.rmSync(dir, { recursive: true, force: true })
  }
}

test('geometry follows resize and adjacency like Linux', { skip: process.platform !== 'win32' }, () => {
  const codeRect = [0, 0, 1280, 1080]
  const terminalRect = [1280, 0, 640, 1080]
  const lines = evaluate([
    { fn: 'FollowResize', args: [[1000, 0, 920, 1080], codeRect, 100] },
    { fn: 'FollowResize', args: [[0, 0, 1500, 1080], terminalRect, 100] },
    { fn: 'FollowResize', args: [[100, 0, 1180, 1080], terminalRect, 100] },
    { fn: 'FollowResize', args: [[0, 0, 1280, 900], terminalRect, 100] },
    { fn: 'FollowResize', args: [[0, 0, 1850, 1080], terminalRect, 100] },
    { fn: 'IsAdjacent', args: [codeRect, [1281, 0, 639, 1080], 2] },
    { fn: 'IsAdjacent', args: [codeRect, [1290, 0, 630, 1080], 2] },
    { fn: 'IsAdjacent', args: [[0, 0, 1280, 500], [1280, 600, 640, 400], 2] },
    { fn: 'SameFrame', args: [[0, 0, 10, 10], [1, 1, 11, 9], 1] },
    { fn: 'SameFrame', args: [[0, 0, 10, 10], [2, 0, 10, 10], 1] }
  ])
  assert.deepEqual(lines, ['0,0,1000,1080', '1500,0,420,1080', '1280,0,640,1080', '1280,0,640,900', 'null', 'True', 'False', 'False', 'True', 'False'])
})

test('manifest declares the smart setting and version 1.1.0', () => {
  const manifest = JSON.parse(embedded('package.json'))
  const smart = manifest.contributes.configuration.properties['terminalBeside.smart']
  assert.equal(smart.type, 'boolean')
  assert.equal(smart.default, true)
  assert.equal(manifest.version, '1.1.0')
  assert.match(source, /Id="terminal-beside" Version="1\.1\.0"/)
})

test('split.ps1 prints the terminal handle after OK', () => {
  assert.ok(embedded('split.ps1').includes("'TERMINAL=' + $Terminal.ToInt64()"))
})

test('starts the pair helper after placement', async () => {
  const fixture = runtime()
  await fixture.command()
  assert.deepEqual(fixture.errors, [])
  assert.equal(fixture.calls.length, 2)
  assert.equal(fixture.pairs.length, 1)
  const pair = fixture.pairs[0]
  assert.equal(JSON.stringify(pair.args), JSON.stringify(['-NoLogo', '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-File', 'C:\\ext\\pair.ps1', '-CodeHandle', '4242', '-TerminalHandle', '5151']))
  // On Windows, detached makes PowerShell exit silently with status 0 when its stdio is piped.
  assert.notEqual(pair.options.detached, true)
  assert.equal(pair.options.windowsHide, true)
  assert.equal(JSON.stringify(pair.options.stdio), JSON.stringify(['ignore', 'pipe', 'pipe']))
  assert.equal(pair.child.unrefCalled, true)
})

test('smart false places once and starts no helper', async () => {
  const fixture = runtime({ smart: false })
  await fixture.command()
  assert.deepEqual(fixture.errors, [])
  assert.equal(fixture.calls.length, 2)
  assert.equal(fixture.launches.length, 1)
  assert.equal(fixture.pairs.length, 0)
})

test('does not start the helper when placement fails', async () => {
  const fixture = runtime({ placeFails: true })
  await fixture.command()
  assert.equal(fixture.errors.length, 1)
  assert.equal(fixture.pairs.length, 0)
})

test('reports a helper that fails, is missing or never reports READY', async () => {
  for (const [pair, pattern] of [['exit', /bad C#/], ['late-stderr', /bad C#/], ['error', /ENOENT/], ['silent', /did not report READY/]]) {
    const fixture = runtime({ pair })
    await fixture.command()
    assert.equal(fixture.errors.length, 1, pair)
    assert.match(fixture.errors[0], /^Split terminal: Smart pairing could not start: /, pair)
    assert.match(fixture.errors[0], pattern, pair)
    assert.equal(fixture.calls.length, 2, 'the windows stay placed: ' + pair)
  }
})

test('a helper path with spaces stays one argument', async () => {
  const helper = 'C:\\Users\\Jane Doe\\.vscode\\extensions\\terminal-beside\\pair.ps1'
  const fixture = runtime({ pairHelper: helper })
  await fixture.command()
  const args = fixture.pairs[0].args
  assert.equal(args[args.indexOf('-File') + 1], helper)
})

test('accepts READY that arrives after the exit event', async () => {
  const fixture = runtime({ pair: 'late-ready' })
  await fixture.command()
  assert.deepEqual(fixture.errors, [])
})

test('split.ps1 stops an older pair helper before moving any window', () => {
  const helper = embedded('split.ps1')
  const stop = helper.indexOf('Local\\TerminalBeside.Pair.')
  assert.ok(stop > 0, 'split.ps1 does not signal the older helper')
  assert.ok(stop < helper.indexOf('[TbWin]::Restore($Terminal)'), 'the older helper must stop before the first move')
})

test('a real PowerShell helper reports READY with the spawn options the extension uses', { skip: process.platform !== 'win32' }, async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'tb-pair-ready-'))
  try {
    const helper = path.join(dir, 'pair.ps1')
    fs.writeFileSync(helper, "[Console]::Out.WriteLine('READY')\nStart-Sleep -Milliseconds 300\n")
    const fixture = runtime({ realPair: true, pairHelper: helper, pairTimeoutMs: 15000 })
    await fixture.command()
    assert.deepEqual(fixture.errors, [])
    await new Promise(resolve => setTimeout(resolve, 800))
  } finally {
    fs.rmSync(dir, { recursive: true, force: true })
  }
})
