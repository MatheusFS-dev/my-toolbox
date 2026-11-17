const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const vm = require('node:vm')
const path = require('node:path')
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
  const sandbox = { module: { exports: {} }, require, process, setTimeout, Buffer, __dirname: '/ext' }
  vm.runInNewContext(embedded('extension.js'), sandbox)
  return sandbox.module.exports
}

test('manifest binds Ctrl+Alt+S on Windows only', () => {
  const manifest = JSON.parse(embedded('package.json'))
  assert.deepEqual(manifest.contributes.keybindings, [{ command: 'myToolbox.openSplitTerminal', key: 'ctrl+alt+s', when: 'isWindows' }])
  assert.equal(manifest.name, 'split-terminal')
  assert.equal(manifest.publisher, 'my-toolbox')
})

test('chooses workspace roots and rejects remote folders', async () => {
  const { workspaceRoot } = load()
  const messages = []
  const folder = name => ({ name, uri: { scheme: 'file', fsPath: 'C:\\projects\\' + name } })
  const api = { env: {}, workspace: { workspaceFolders: [folder('one')] }, window: {
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
  const root = config.root || 'C:\\work\\my project'
  const api = { env: {}, workspace: { workspaceFolders: [{ uri: { scheme: 'file', fsPath: root } }] }, window: { showErrorMessage: message => errors.push(message) } }
  const execFile = async (command, args) => {
    calls.push([command, args])
    assert.equal(command, 'powershell.exe')
    const mode = args[args.indexOf('-Mode') + 1]
    if (mode === 'Probe') return { stdout: config.notCode ? 'NOTCODE\r\n' : 'CODE=4242\r\nTERMINALS=11,22\r\n' }
    if (config.placeFails) throw Object.assign(new Error('Command failed'), { stderr: 'Terminal startup timed out. VS Code was left in place.\r\n' })
    return { stdout: 'OK\r\n' }
  }
  const spawn = (command, args, options) => {
    launches.push({ command, args, options })
    return { on(event, listener) { if (config.launchError && event === 'error') listener(new Error('wt missing')) }, unref() {} }
  }
  const command = createCommand(api, { execFile, spawn, env: {}, platform: config.platform || 'win32', helper: 'C:\\ext\\split.ps1' })
  return { command, calls, errors, launches }
}

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
  assert.equal(JSON.stringify(place.slice(place.indexOf('-Mode'))), JSON.stringify(['-Mode', 'Place', '-CodeHandle', '4242', '-Known', '11,22', '-TimeoutMs', '8000']))
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
