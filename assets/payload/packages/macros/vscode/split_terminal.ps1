# Self-contained installer. The embedded helper is copied from scripts/terminal/windows/modules/output.ps1.
[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'

Set-StrictMode -Version Latest

$script:ToolboxOutputStarted = $false
$script:ToolboxOutputName = ''
$script:ToolboxClock = { [DateTime]::Now.ToString('HH:mm:ss') }

function Initialize-ToolboxOutput {
    param([Parameter(Mandatory = $true)][string]$ToolName)

    $script:ToolboxOutputName = $ToolName
    $script:ToolboxOutputStarted = $false
}

function Test-ToolboxColor {
    if (Test-Path Env:NO_COLOR) { return $false }
    if ([Console]::IsOutputRedirected) { return $false }
    return $Host.Name -eq 'ConsoleHost'
}

function Test-Toolbox256Color {
    if (-not (Test-ToolboxColor)) { return $false }
    return $env:TERM -like '*256color*' -or
        -not [string]::IsNullOrWhiteSpace($env:COLORTERM) -or
        -not [string]::IsNullOrWhiteSpace($env:WT_SESSION)
}

function Write-ToolboxLine {
    param(
        [Parameter(Mandatory = $true)][ValidateSet('INFO', 'PROMPT', 'WARN', 'ERROR', 'SUCCESS')][string]$Level,
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Message
    )

    if (-not $script:ToolboxOutputStarted) {
        [Console]::Out.WriteLine('')
        $script:ToolboxOutputStarted = $true
    }
    foreach ($MessageLine in ($Message -split "`r?`n")) {
        $Rendered = '[{0} {1} {2}] {3}' -f (& $script:ToolboxClock), $script:ToolboxOutputName, $Level, $MessageLine
        if (-not (Test-ToolboxColor) -or $Level -eq 'INFO') {
            [Console]::Out.WriteLine($Rendered)
            continue
        }
        if ($Level -eq 'WARN' -and (Test-Toolbox256Color)) {
            $Escape = [char]27
            [Console]::Out.WriteLine("${Escape}[38;5;208m${Rendered}${Escape}[0m")
            continue
        }
        $Color = switch ($Level) {
            'PROMPT' { 'Cyan' }
            'WARN' { 'Yellow' }
            'ERROR' { 'Red' }
            'SUCCESS' { 'Green' }
        }
        Write-Host $Rendered -ForegroundColor $Color
    }
}

function Write-ToolboxInfo { param([string]$Message) Write-ToolboxLine INFO $Message }
function Write-ToolboxPrompt { param([string]$Message) Write-ToolboxLine PROMPT $Message }
function Write-ToolboxMenu {
    param([string]$Question, [string[]]$Options)

    Write-ToolboxPrompt $Question
    foreach ($Option in $Options) {
        $Rendered = "  $Option"
        if (Test-ToolboxColor) { Write-Host $Rendered -ForegroundColor Cyan }
        else { [Console]::Out.WriteLine($Rendered) }
    }
}
function Write-ToolboxWarn { param([string]$Message) Write-ToolboxLine WARN $Message }
function Write-ToolboxError { param([string]$Message) Write-ToolboxLine ERROR $Message }
function Write-ToolboxSuccess { param([string]$Message) Write-ToolboxLine SUCCESS $Message }

function Read-ToolboxYesNo {
    param(
        [Parameter(Mandatory = $true)][string]$Question,
        [Parameter(Mandatory = $true)][bool]$Default
    )

    $DefaultName = if ($Default) { 'Y' } else { 'n' }
    $Marker = if ($Default) { 'Y/n' } else { 'y/N' }
    while ($true) {
        Write-ToolboxPrompt "$Question [$Marker] (default: $DefaultName):"
        $Answer = Read-Host
        if ([string]::IsNullOrWhiteSpace($Answer)) { return $Default }
        switch ($Answer.Trim().ToLowerInvariant()) {
            { $_ -in @('y', 'yes') } { return $true }
            { $_ -in @('n', 'no') } { return $false }
            default { Write-ToolboxWarn "Enter y or n, or press Enter for $DefaultName." }
        }
    }
}

function Read-ToolboxRequired {
    param([Parameter(Mandatory = $true)][string]$Question)

    while ($true) {
        Write-ToolboxPrompt "${Question}: "
        $Answer = Read-Host
        if (-not [string]::IsNullOrWhiteSpace($Answer)) { return $Answer.Trim() }
        Write-ToolboxWarn 'A value is required; enter a value to continue.'
    }
}

Initialize-ToolboxOutput -ToolName 'vscode-split-terminal'
$Work = $null
try {
    Write-ToolboxInfo 'Checking Windows and installer dependencies.'
    if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) { throw 'This macro supports Windows only.' }
    $Missing = @()
    foreach ($Dependency in @('code', 'wt', 'powershell')) {
        if (-not (Get-Command $Dependency -ErrorAction SilentlyContinue)) { $Missing += $Dependency }
    }
    if ($Missing.Count -gt 0) {
        throw ('Missing: {0}. Install VS Code with its command line launcher on PATH, Windows Terminal (winget install Microsoft.WindowsTerminal) and Windows PowerShell, then retry this installer.' -f ($Missing -join ' '))
    }
    $Work = Join-Path ([IO.Path]::GetTempPath()) ('tb-split-terminal-' + [Guid]::NewGuid().ToString('N'))
    $Extension = Join-Path $Work 'extension'
    New-Item -ItemType Directory -Path $Extension | Out-Null
    $Utf8 = New-Object System.Text.UTF8Encoding($false)
    function Save-File([string]$Path, [string]$Text) {
        [IO.File]::WriteAllText($Path, ($Text -replace "`r`n", "`n"), $Utf8)
    }

    Write-ToolboxInfo 'Generating the VS Code extension package.'
    Save-File (Join-Path $Extension 'icon.base64') @'
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
'@
    [IO.File]::WriteAllBytes((Join-Path $Extension 'icon.png'), [Convert]::FromBase64String((Get-Content -Raw (Join-Path $Extension 'icon.base64'))))
    Remove-Item (Join-Path $Extension 'icon.base64')
    Save-File (Join-Path $Extension 'package.json') @'
{
  "name": "terminal-beside",
  "displayName": "Terminal Beside",
  "description": "Open Windows Terminal at the workspace root and tile it beside VS Code.",
  "version": "1.1.0",
  "publisher": "MatheusFS-dev",
  "engines": { "vscode": "^1.74.0" },
  "main": "./extension.js",
  "icon": "icon.png",
  "author": "MatheusFS-dev",
  "extensionKind": ["ui"],
  "capabilities": { "untrustedWorkspaces": { "supported": true }, "virtualWorkspaces": false },
  "contributes": {
    "commands": [{ "command": "terminalBeside.openTerminal", "title": "Terminal Beside: Open Terminal" }],
    "keybindings": [{ "command": "terminalBeside.openTerminal", "key": "ctrl+alt+s", "when": "isWindows" }],
    "configuration": { "title": "Terminal Beside", "properties": { "terminalBeside.smart": { "type": "boolean", "default": true, "description": "Link the terminal and VS Code so focusing either raises both and resizing one resizes the other. Turn off to only place the windows once." }, "terminalBeside.terminalWidthPercent": { "type": "number", "default": 33.333333333333336, "minimum": 1, "maximum": 99, "description": "Width of the terminal as a percentage of the monitor work area." } } }
  }
}
'@
    Save-File (Join-Path $Extension 'extension.js') @'
'use strict'
const childProcess = require('node:child_process')
const { promisify } = require('node:util')
const { win32, join } = require('node:path')

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

function createCommand(vscode, options = {}) {
  const env = options.env || process.env
  const platform = options.platform || process.platform
  const execFile = options.execFile || promisify(childProcess.execFile)
  const spawn = options.spawn || childProcess.spawn
  const helper = options.helper || join(__dirname, 'split.ps1')
  const pairHelper = options.pairHelper || join(__dirname, 'pair.ps1')
  const pairTimeout = options.pairTimeoutMs || 10000
  const powershell = ['-NoLogo', '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-File', helper]
  const run = async args => {
    try {
      return String((await execFile('powershell.exe', [...powershell, ...args], { env, timeout: 20000, maxBuffer: 1024 * 1024, windowsHide: true })).stdout).trim()
    } catch (error) {
      const detail = String(error.stderr || '').trim() || error.message
      throw new Error(error.code === 'ENOENT' ? 'powershell.exe is missing. Re-run the macro installer.' : detail)
    }
  }
  // The pair helper keeps running after this call and ends itself when the link ends.
  // It is not spawned detached: on Windows a detached PowerShell with piped stdio exits at once with status 0.
  // It is ready once it prints READY, so an early exit or a missing READY is a start failure.
  const startPair = (code, terminal) => new Promise((resolve, reject) => {
    const child = spawn('powershell.exe', ['-NoLogo', '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-File', pairHelper, '-CodeHandle', code, '-TerminalHandle', terminal], { env, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'] })
    let out = ''
    let err = ''
    let settled = false
    const finish = failure => {
      if (settled) return
      settled = true
      clearTimeout(timer)
      if (failure) {
        try { child.kill() } catch { /* the helper is already gone */ }
        reject(failure)
        return
      }
      child.stdout.destroy()
      child.stderr.destroy()
      child.unref()
      resolve()
    }
    const timer = setTimeout(() => finish(new Error('the helper did not report READY in time')), pairTimeout)
    child.on('error', error => finish(error))
    child.on('close', status => finish(new Error(err.trim() || `the helper exited with status ${status}`)))
    child.stdout.on('data', chunk => { out += chunk; if (/^READY\s*$/m.test(out)) finish() })
    child.stderr.on('data', chunk => { err += chunk })
  })
  let busy = false
  return async () => {
    if (busy) return
    busy = true
    try {
      const side = await vscode.window.showQuickPick(['Left', 'Right'], { placeHolder: 'Place terminal on which side?' })
      if (!side) return
      const percent = vscode.workspace.getConfiguration('terminalBeside').get('terminalWidthPercent', 100 / 3)
      if (!Number.isFinite(percent) || percent < 1 || percent > 99) throw new Error('Terminal width must be between 1 and 99 percent.')
      const smart = vscode.workspace.getConfiguration('terminalBeside').get('smart', true) !== false
      const root = await workspaceRoot(vscode)
      if (!root) return
      if (platform !== 'win32') throw new Error('Split terminal requires Windows.')
      const probe = await run(['-Mode', 'Probe'])
      const code = /^CODE=(\d+)$/m.exec(probe)
      if (!code) throw new Error('Focus VS Code before opening a split terminal.')
      // Windows Terminal windows already open are excluded when the helper looks for the new one.
      const known = (/^TERMINALS=(.*)$/m.exec(probe) || [])[1] || 'none'
      const title = win32.basename(root) || root
      let launchError
      // Windows Terminal treats an unescaped semicolon as a command separator.
      const terminal = spawn('wt.exe', ['-w', 'new', '--title', title, '--suppressApplicationTitle', '-d', root.replace(/;/g, '\\;')], { cwd: root, env, detached: true, stdio: 'ignore' })
      terminal.on('error', error => { launchError = error })
      terminal.on('exit', status => { if (status) launchError = new Error(`Windows Terminal exited with status ${status}`) })
      terminal.unref()
      // The helper waits for the new terminal window and moves nothing until it exists.
      let placed
      try {
        placed = await run(['-Mode', 'Place', '-CodeHandle', code[1], '-Known', known, '-TimeoutMs', '8000', '-Side', side, '-Percent', String(percent)])
      } catch (error) {
        throw launchError || error
      }
      if (smart) {
        const terminalHandle = /^TERMINAL=(\d+)$/m.exec(placed)
        if (!terminalHandle) throw new Error('Smart pairing could not start: the placement did not report the terminal window.')
        try {
          await startPair(code[1], terminalHandle[1])
        } catch (error) {
          throw new Error(`Smart pairing could not start: ${error.message}`)
        }
      }
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
module.exports = { activate, workspaceRoot, createCommand }
'@
    Save-File (Join-Path $Extension 'split.ps1') @'
param(
    [Parameter(Mandatory = $true)][ValidateSet('Probe', 'Place')][string]$Mode,
    [string]$CodeHandle = '0',
    [string]$Known = 'none',
    [int]$TimeoutMs = 8000,
    [ValidateSet('Left', 'Right')][string]$Side = 'Right',
    [ValidateRange(1, 99)][double]$Percent = (100.0 / 3)
)
$ErrorActionPreference = 'Stop'
try {
Add-Type -TypeDefinition @"
using System;
using System.Runtime.InteropServices;
using System.Text;
public static class TbWin {
    [StructLayout(LayoutKind.Sequential)] public struct RECT { public int Left, Top, Right, Bottom; }
    [StructLayout(LayoutKind.Sequential)] public struct MONITORINFO { public int Size; public RECT Monitor, Work; public int Flags; }
    delegate bool EnumProc(IntPtr hwnd, IntPtr lparam);
    [DllImport("user32.dll")] static extern bool EnumWindows(EnumProc proc, IntPtr lparam);
    [DllImport("user32.dll")] static extern bool IsWindowVisible(IntPtr hwnd);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern int GetWindowText(IntPtr hwnd, StringBuilder text, int count);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern int GetClassName(IntPtr hwnd, StringBuilder text, int count);
    [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
    [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr hwnd, out uint pid);
    [DllImport("user32.dll")] static extern bool GetWindowRect(IntPtr hwnd, out RECT rect);
    [DllImport("user32.dll")] static extern bool IsWindow(IntPtr hwnd);
    [DllImport("user32.dll")] static extern bool IsZoomed(IntPtr hwnd);
    [DllImport("user32.dll")] static extern bool IsIconic(IntPtr hwnd);
    [DllImport("user32.dll")] static extern bool ShowWindow(IntPtr hwnd, int command);
    [DllImport("user32.dll")] static extern bool SetWindowPos(IntPtr hwnd, IntPtr after, int x, int y, int cx, int cy, uint flags);
    [DllImport("user32.dll")] static extern IntPtr MonitorFromWindow(IntPtr hwnd, uint flags);
    [DllImport("user32.dll")] static extern bool GetMonitorInfo(IntPtr monitor, ref MONITORINFO info);
    [DllImport("user32.dll")] static extern bool SetForegroundWindow(IntPtr hwnd);
    [DllImport("user32.dll")] static extern void keybd_event(byte key, byte scan, uint flags, UIntPtr extra);
    [DllImport("user32.dll")] static extern bool SetProcessDpiAwarenessContext(IntPtr context);
    [DllImport("dwmapi.dll")] static extern int DwmGetWindowAttribute(IntPtr hwnd, int attribute, out RECT rect, int size);

    public static void EnableDpiAwareness() { SetProcessDpiAwarenessContext(new IntPtr(-4)); }
    public static string OwnerProcess(IntPtr hwnd) {
        uint pid; GetWindowThreadProcessId(hwnd, out pid);
        try { return System.Diagnostics.Process.GetProcessById((int)pid).ProcessName; } catch { return ""; }
    }
    public static bool Exists(IntPtr hwnd) { return IsWindow(hwnd); }
    // Windows Terminal windows are the only visible top-level windows of this class.
    public static long[] Terminals() {
        System.Collections.Generic.List<long> found = new System.Collections.Generic.List<long>();
        EnumWindows(delegate (IntPtr hwnd, IntPtr lparam) {
            if (!IsWindowVisible(hwnd)) return true;
            StringBuilder text = new StringBuilder(256);
            GetClassName(hwnd, text, text.Capacity);
            if (text.ToString() == "CASCADIA_HOSTING_WINDOW_CLASS") found.Add(hwnd.ToInt64());
            return true;
        }, IntPtr.Zero);
        return found.ToArray();
    }
    public static int[] WorkArea(IntPtr hwnd) {
        MONITORINFO info = new MONITORINFO(); info.Size = Marshal.SizeOf(typeof(MONITORINFO));
        if (!GetMonitorInfo(MonitorFromWindow(hwnd, 2), ref info)) throw new Exception("Cannot read the monitor work area.");
        return new int[] { info.Work.Left, info.Work.Top, info.Work.Right - info.Work.Left, info.Work.Bottom - info.Work.Top };
    }
    public static void Restore(IntPtr hwnd) { if (IsIconic(hwnd) || IsZoomed(hwnd)) ShowWindow(hwnd, 9); }
    // Fit the visible frame to the rectangle, compensating for the invisible resize borders Windows adds.
    public static void Place(IntPtr hwnd, int x, int y, int width, int height) {
        RECT outer, visible;
        if (!GetWindowRect(hwnd, out outer)) throw new Exception("Cannot read a window rectangle.");
        if (DwmGetWindowAttribute(hwnd, 9, out visible, Marshal.SizeOf(typeof(RECT))) != 0) visible = outer;
        int left = visible.Left - outer.Left, top = visible.Top - outer.Top;
        int right = outer.Right - visible.Right, bottom = outer.Bottom - visible.Bottom;
        if (!SetWindowPos(hwnd, IntPtr.Zero, x - left, y - top, width + left + right, height + top + bottom, 0x0014)) throw new Exception("Windows refused to move a window.");
    }
    public static int[] Visible(IntPtr hwnd) {
        RECT outer, visible;
        GetWindowRect(hwnd, out outer);
        if (DwmGetWindowAttribute(hwnd, 9, out visible, Marshal.SizeOf(typeof(RECT))) != 0) visible = outer;
        return new int[] { visible.Left, visible.Top, visible.Right - visible.Left, visible.Bottom - visible.Top };
    }
    // Move without resizing, so Windows applies any DPI rescale of the target monitor before the real placement.
    public static void MoveTo(IntPtr hwnd, int x, int y) { SetWindowPos(hwnd, IntPtr.Zero, x, y, 0, 0, 0x0015); }
    public static void Focus(IntPtr hwnd) {
        // A synthetic Alt press lets this background process take the foreground.
        keybd_event(0x12, 0, 0, UIntPtr.Zero); keybd_event(0x12, 0, 2, UIntPtr.Zero);
        SetForegroundWindow(hwnd);
    }
}
"@
[TbWin]::EnableDpiAwareness()
if ($Mode -eq 'Probe') {
    $Owner = [TbWin]::OwnerProcess([TbWin]::GetForegroundWindow())
    if ($Owner -in @('Code', 'Code - Insiders', 'VSCodium')) { [Console]::Out.WriteLine('CODE=' + [TbWin]::GetForegroundWindow().ToInt64()) }
    else { [Console]::Out.WriteLine('NOTCODE') }
    [Console]::Out.WriteLine('TERMINALS=' + (([TbWin]::Terminals() | ForEach-Object { $_.ToString() }) -join ','))
    exit 0
}
$Code = [IntPtr][Int64]$CodeHandle
if (-not [TbWin]::Exists($Code)) { throw 'The VS Code window no longer exists.' }
$Clock = [Diagnostics.Stopwatch]::StartNew()
$Terminal = [IntPtr]::Zero
$Existing = @($Known -split ',' | Where-Object { $_ -match '^\d+$' })
while ($Clock.ElapsedMilliseconds -lt $TimeoutMs) {
    $Fresh = @([TbWin]::Terminals() | Where-Object { $Existing -notcontains $_.ToString() -and $_ -ne $Code.ToInt64() })
    if ($Fresh.Count -gt 0) { $Terminal = [IntPtr][Int64]$Fresh[0]; break }
    Start-Sleep -Milliseconds 100
}
if ($Terminal -eq [IntPtr]::Zero) { throw 'Terminal startup timed out. VS Code was left in place.' }
$Area = [TbWin]::WorkArea($Code)
$X = $Area[0]; $Y = $Area[1]; $Width = $Area[2]; $Height = $Area[3]
if ($Width -lt 3 -or $Height -lt 1) { throw 'The monitor has no usable desktop area.' }
$TerminalWidth = [int][Math]::Max(1, [Math]::Min($Width - 1, [Math]::Ceiling($Width * $Percent / 100)))
$Left = $Width - $TerminalWidth
$TerminalX = if ($Side -eq 'Left') { $X } else { $X + $Left }
$CodeX = if ($Side -eq 'Left') { $X + $TerminalWidth } else { $X }
# An older pair for this VS Code window must stop before its windows move again.
$Older = $null
if ([Threading.EventWaitHandle]::TryOpenExisting("Local\TerminalBeside.Pair.$($Code.ToInt64())", [ref]$Older)) {
    [void]$Older.Set()
    $Older.Dispose()
    Start-Sleep -Milliseconds 200
}
[TbWin]::Restore($Terminal)
[TbWin]::Restore($Code)
# Monitors with different scaling make Windows rescale a window that crosses over, which undoes a single move.
# Land the terminal on the target monitor first, then place each window until its visible frame matches.
[TbWin]::MoveTo($Terminal, $TerminalX + 20, $Y + 20)
Start-Sleep -Milliseconds 250
function Set-Frame([IntPtr]$Handle, [int]$FrameX, [int]$FrameY, [int]$FrameWidth, [int]$FrameHeight) {
    for ($Attempt = 0; $Attempt -lt 8; $Attempt++) {
        [TbWin]::Place($Handle, $FrameX, $FrameY, $FrameWidth, $FrameHeight)
        Start-Sleep -Milliseconds 200
        $Frame = [TbWin]::Visible($Handle)
        if ([Math]::Abs($Frame[0] - $FrameX) -le 1 -and [Math]::Abs($Frame[1] - $FrameY) -le 1 -and [Math]::Abs($Frame[2] - $FrameWidth) -le 1 -and [Math]::Abs($Frame[3] - $FrameHeight) -le 1) { return }
    }
    throw "Windows did not accept the window size (wanted $FrameWidth x $FrameHeight at $FrameX,$FrameY, got $($Frame -join ','))."
}
Set-Frame $Terminal $TerminalX $Y $TerminalWidth $Height
Set-Frame $Code $CodeX $Y $Left $Height
# Electron can apply a late resize after the first placement, so confirm VS Code kept its slot.
Start-Sleep -Milliseconds 400
Set-Frame $Code $CodeX $Y $Left $Height
[TbWin]::Focus($Terminal)
[Console]::Out.WriteLine('OK')
[Console]::Out.WriteLine('TERMINAL=' + $Terminal.ToInt64())
} catch {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 1
}
'@
    Save-File (Join-Path $Extension 'pair.ps1') @'
param(
    [string]$CodeHandle = '0',
    [string]$TerminalHandle = '0'
)
$ErrorActionPreference = 'Stop'
try {
Add-Type -TypeDefinition @"
using System;
using System.Runtime.InteropServices;
using System.Threading;
public static class TbGeometry {
    // Rectangles are {x, y, width, height} in visible frame coordinates.
    // The other window keeps its outer edge and its shared edge follows the changed window.
    // Returns null when the other window would be narrower than minWidth.
    public static int[] FollowResize(int[] changed, int[] other, int minWidth) {
        int x;
        int width;
        if (changed[0] <= other[0]) { x = changed[0] + changed[2]; width = other[0] + other[2] - x; }
        else { x = other[0]; width = changed[0] - other[0]; }
        if (width < minWidth) return null;
        return new int[] { x, changed[1], width, changed[3] };
    }
    public static bool IsAdjacent(int[] a, int[] b, int tolerance) {
        int[] left = a[0] <= b[0] ? a : b;
        int[] right = object.ReferenceEquals(left, a) ? b : a;
        int gap = Math.Abs(right[0] - (left[0] + left[2]));
        int overlap = Math.Min(left[1] + left[3], right[1] + right[3]) - Math.Max(left[1], right[1]);
        return gap <= tolerance && overlap > 0;
    }
    public static bool SameFrame(int[] a, int[] b, int tolerance) {
        for (int i = 0; i < 4; i++) { if (Math.Abs(a[i] - b[i]) > tolerance) return false; }
        return true;
    }
}
public static class TbPair {
    const int MinWidth = 100;
    const int AdjacentTolerance = 2;
    const int FrameTolerance = 1;
    [StructLayout(LayoutKind.Sequential)] struct RECT { public int Left, Top, Right, Bottom; }
    [StructLayout(LayoutKind.Sequential)] struct POINT { public int X, Y; }
    [StructLayout(LayoutKind.Sequential)] struct MSG { public IntPtr hwnd; public uint message; public IntPtr wParam, lParam; public uint time; public POINT pt; }
    [StructLayout(LayoutKind.Sequential)] struct MONITORINFO { public int Size; public RECT Monitor, Work; public int Flags; }
    delegate void WinEventProc(IntPtr hook, uint ev, IntPtr hwnd, int idObject, int idChild, uint thread, uint time);
    [DllImport("user32.dll")] static extern IntPtr SetWinEventHook(uint min, uint max, IntPtr module, WinEventProc proc, uint pid, uint tid, uint flags);
    [DllImport("user32.dll")] static extern bool UnhookWinEvent(IntPtr hook);
    [DllImport("user32.dll")] static extern bool PeekMessage(out MSG msg, IntPtr hwnd, uint min, uint max, uint remove);
    [DllImport("user32.dll")] static extern bool TranslateMessage(ref MSG msg);
    [DllImport("user32.dll")] static extern IntPtr DispatchMessage(ref MSG msg);
    [DllImport("user32.dll")] static extern uint MsgWaitForMultipleObjects(uint count, IntPtr[] handles, bool all, uint milliseconds, uint mask);
    [DllImport("user32.dll")] static extern bool GetWindowRect(IntPtr hwnd, out RECT rect);
    [DllImport("user32.dll")] static extern bool SetWindowPos(IntPtr hwnd, IntPtr after, int x, int y, int cx, int cy, uint flags);
    [DllImport("user32.dll")] static extern bool IsWindow(IntPtr hwnd);
    [DllImport("user32.dll")] static extern bool IsZoomed(IntPtr hwnd);
    [DllImport("user32.dll")] static extern bool IsIconic(IntPtr hwnd);
    [DllImport("user32.dll")] static extern int GetWindowLong(IntPtr hwnd, int index);
    [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr hwnd, out uint pid);
    [DllImport("user32.dll")] static extern IntPtr MonitorFromWindow(IntPtr hwnd, uint flags);
    [DllImport("user32.dll")] static extern bool GetMonitorInfo(IntPtr monitor, ref MONITORINFO info);
    [DllImport("user32.dll")] static extern bool SetProcessDpiAwarenessContext(IntPtr context);
    [DllImport("dwmapi.dll")] static extern int DwmGetWindowAttribute(IntPtr hwnd, int attribute, out RECT rect, int size);

    // The delegate lives in a static field so the garbage collector cannot free it while the hooks are active.
    static WinEventProc callback;
    static IntPtr[] hooks;
    static IntPtr code;
    static IntPtr terminal;
    static int[] lastCode;
    static int[] lastTerminal;
    static string reason;
    static bool applying;
    static bool iconicCode;
    static bool iconicTerminal;
    static EventWaitHandle stop;
    static Mutex owner;

    static int[] Visible(IntPtr hwnd) {
        RECT outer, visible;
        GetWindowRect(hwnd, out outer);
        if (DwmGetWindowAttribute(hwnd, 9, out visible, Marshal.SizeOf(typeof(RECT))) != 0) visible = outer;
        return new int[] { visible.Left, visible.Top, visible.Right - visible.Left, visible.Bottom - visible.Top };
    }
    // Fit the visible frame to the rectangle, compensating for the invisible resize borders Windows adds.
    static void Place(IntPtr hwnd, int[] frame) {
        RECT outer, visible;
        GetWindowRect(hwnd, out outer);
        if (DwmGetWindowAttribute(hwnd, 9, out visible, Marshal.SizeOf(typeof(RECT))) != 0) visible = outer;
        int left = visible.Left - outer.Left, top = visible.Top - outer.Top;
        int right = outer.Right - visible.Right, bottom = outer.Bottom - visible.Bottom;
        SetWindowPos(hwnd, IntPtr.Zero, frame[0] - left, frame[1] - top, frame[2] + left + right, frame[3] + top + bottom, 0x0014);
    }
    // Lifts a window above its neighbours without activating it. Topmost then not topmost reaches the top of the normal group.
    static void Raise(IntPtr hwnd) {
        if (IsIconic(hwnd)) return;
        if ((GetWindowLong(hwnd, -20) & 0x8) != 0) { SetWindowPos(hwnd, IntPtr.Zero, 0, 0, 0, 0, 0x0013); return; }
        SetWindowPos(hwnd, new IntPtr(-1), 0, 0, 0, 0, 0x0013);
        SetWindowPos(hwnd, new IntPtr(-2), 0, 0, 0, 0, 0x0013);
    }
    static bool CoversMonitor(IntPtr hwnd, int[] frame) {
        MONITORINFO info = new MONITORINFO();
        info.Size = Marshal.SizeOf(typeof(MONITORINFO));
        if (!GetMonitorInfo(MonitorFromWindow(hwnd, 2), ref info)) return false;
        int[] monitor = new int[] { info.Monitor.Left, info.Monitor.Top, info.Monitor.Right - info.Monitor.Left, info.Monitor.Bottom - info.Monitor.Top };
        return TbGeometry.SameFrame(frame, monitor, FrameTolerance);
    }
    static void End(string why) { if (reason == null) reason = why; }
    static void Remember(bool isCode, int[] frame) { if (isCode) lastCode = frame; else lastTerminal = frame; }

    static void OnEvent(IntPtr hook, uint ev, IntPtr hwnd, int idObject, int idChild, uint thread, uint time) {
        if (reason != null || idObject != 0 || idChild != 0) return;
        if (hwnd != code && hwnd != terminal) return;
        bool isCode = hwnd == code;
        IntPtr other = isCode ? terminal : code;
        if (ev == 0x8001) { End("closed"); return; }
        if (ev == 0x0003) { Raise(other); Raise(hwnd); return; }
        if (ev != 0x800B || applying) return;
        bool minimized = IsIconic(hwnd);
        bool restored = !minimized && (isCode ? iconicCode : iconicTerminal);
        if (isCode) iconicCode = minimized; else iconicTerminal = minimized;
        // A minimized window reports off-screen coordinates, so it is neither measured nor moved.
        if (minimized) return;
        if (IsZoomed(hwnd)) { End("maximized"); return; }
        int[] now = Visible(hwnd);
        if (CoversMonitor(hwnd, now)) { End("fullscreen"); return; }
        int[] before = isCode ? lastCode : lastTerminal;
        Remember(isCode, now);
        // Adjacency is judged when the minimized window comes back.
        if (IsIconic(other)) return;
        int[] otherFrame = Visible(other);
        if (restored) {
            // Either window may have moved or been resized while this one was minimized.
            if (!TbGeometry.IsAdjacent(now, otherFrame, AdjacentTolerance)) End("dragged-away");
            return;
        }
        if (TbGeometry.SameFrame(now, before, FrameTolerance)) return;
        if (now[2] == before[2] && now[3] == before[3]) {
            // A pure move that is not ours leaves the pair apart, so the link ends.
            if (!TbGeometry.IsAdjacent(now, otherFrame, AdjacentTolerance)) End("dragged-away");
            return;
        }
        int[] target = TbGeometry.FollowResize(now, otherFrame, MinWidth);
        if (target == null) { End("too-narrow"); return; }
        if (TbGeometry.SameFrame(target, otherFrame, FrameTolerance)) return;
        applying = true;
        try { Place(other, target); } finally { applying = false; }
        // Our own placement raises events later. They match the frame stored here and are ignored.
        Remember(!isCode, Visible(other));
    }

    static IntPtr Hook(uint ev, uint pid) {
        IntPtr hook = SetWinEventHook(ev, ev, IntPtr.Zero, callback, pid, 0, 0);
        if (hook == IntPtr.Zero) throw new Exception("SetWinEventHook failed.");
        return hook;
    }
    static uint ProcessOf(IntPtr hwnd) { uint pid; GetWindowThreadProcessId(hwnd, out pid); return pid; }

    // Stops an older helper for the same VS Code window, then installs the hooks on the calling thread.
    public static void Start(long codeHandle, long terminalHandle) {
        SetProcessDpiAwarenessContext(new IntPtr(-4));
        code = new IntPtr(codeHandle);
        terminal = new IntPtr(terminalHandle);
        if (!IsWindow(code) || !IsWindow(terminal)) throw new Exception("A window of the pair no longer exists.");
        string stopName = "Local\\TerminalBeside.Pair." + codeHandle;
        string lockName = "Local\\TerminalBeside.PairLock." + codeHandle;
        EventWaitHandle older;
        if (EventWaitHandle.TryOpenExisting(stopName, out older)) { older.Set(); older.Dispose(); }
        owner = new Mutex(false, lockName);
        try {
            if (!owner.WaitOne(3000)) throw new Exception("The previous pairing did not stop.");
        } catch (AbandonedMutexException) { }
        stop = new EventWaitHandle(false, EventResetMode.ManualReset, stopName);
        lastCode = Visible(code);
        lastTerminal = Visible(terminal);
        callback = OnEvent;
        uint codePid = ProcessOf(code);
        uint terminalPid = ProcessOf(terminal);
        // Location and destroy events are filtered to the two owning processes. Foreground events are global.
        System.Collections.Generic.List<IntPtr> list = new System.Collections.Generic.List<IntPtr>();
        list.Add(Hook(0x0003, 0));
        list.Add(Hook(0x800B, codePid));
        list.Add(Hook(0x8001, codePid));
        if (terminalPid != codePid) { list.Add(Hook(0x800B, terminalPid)); list.Add(Hook(0x8001, terminalPid)); }
        hooks = list.ToArray();
    }

    // Pumps messages until the link ends and returns why.
    public static string Loop() {
        IntPtr[] waits = new IntPtr[] { stop.SafeWaitHandle.DangerousGetHandle() };
        try {
            while (reason == null) {
                if (!IsWindow(code) || !IsWindow(terminal)) { End("closed"); break; }
                uint woke = MsgWaitForMultipleObjects(1, waits, false, 200, 0x04FF);
                if (woke == 0) { End("replaced"); break; }
                MSG msg;
                while (PeekMessage(out msg, IntPtr.Zero, 0, 0, 1)) { TranslateMessage(ref msg); DispatchMessage(ref msg); }
            }
        } finally {
            foreach (IntPtr hook in hooks) UnhookWinEvent(hook);
            stop.Dispose();
            owner.ReleaseMutex();
            owner.Dispose();
        }
        return reason;
    }
}
"@
$Code = [Int64]0
$Terminal = [Int64]0
if (-not [Int64]::TryParse($CodeHandle, [ref]$Code) -or $Code -eq 0) { throw 'A numeric -CodeHandle is required.' }
if (-not [Int64]::TryParse($TerminalHandle, [ref]$Terminal) -or $Terminal -eq 0) { throw 'A numeric -TerminalHandle is required.' }
[TbPair]::Start($Code, $Terminal)
[Console]::Out.WriteLine('READY')
[Console]::Out.WriteLine('ENDED ' + [TbPair]::Loop())
} catch {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 1
}
'@
    Save-File (Join-Path $Extension 'README.md') @'
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

## Smart pairing

The `terminalBeside.smart` setting is on by default. It links the terminal and VS Code as a pair. Focusing or Alt-Tabbing to either window raises both, and resizing one window resizes the other. The link ends when either window closes, is maximized or made fullscreen, or is dragged away from the other.

On Windows, smart pairing needs nothing beyond the installer. While a pair is linked, a small background `powershell.exe` watches both windows and ends by itself when the link ends. After an install or update, reload VS Code. If the helper cannot start, Ctrl+Alt+S shows an error and the windows stay where they were placed. Set `terminalBeside.smart` to `false` to place the windows once.

## Configure width

In VS Code Settings, search for **Terminal Beside: Terminal Width Percent**. Enter a value from 1 to 99. The default is one third of the monitor work area. The setting also works in `settings.json`:

```json
"terminalBeside.terminalWidthPercent": 40
```

This example gives the terminal 40% of the available width and VS Code the remaining 60%.

## Uninstall

Run `code --uninstall-extension MatheusFS-dev.terminal-beside`.
'@
    Save-File (Join-Path $Work 'extension.vsixmanifest') @'
<?xml version="1.0" encoding="utf-8"?>
<PackageManifest Version="2.0.0" xmlns="http://schemas.microsoft.com/developer/vsx-schema/2011">
  <Metadata>
    <Identity Language="en-US" Id="terminal-beside" Version="1.1.0" Publisher="MatheusFS-dev" />
    <DisplayName>Terminal Beside</DisplayName>
    <Description xml:space="preserve">Open a workspace terminal and tile it beside VS Code on Windows.</Description>
    <Tags>terminal,windows</Tags><Categories>Other</Categories><GalleryFlags>Public</GalleryFlags>
    <Properties><Property Id="Microsoft.VisualStudio.Code.Engine" Value="^1.74.0" /><Property Id="Microsoft.VisualStudio.Code.ExtensionKind" Value="ui" /></Properties>
  </Metadata>
  <Installation><InstallationTarget Id="Microsoft.VisualStudio.Code" /></Installation>
  <Dependencies />
  <Assets><Asset Type="Microsoft.VisualStudio.Code.Manifest" Path="extension/package.json" Addressable="true" /><Asset Type="Microsoft.VisualStudio.Code.Readme" Path="extension/README.md" Addressable="true" /><Asset Type="Microsoft.VisualStudio.Code.Icons.Default" Path="extension/icon.png" Addressable="true" /></Assets>
</PackageManifest>
'@
    Save-File (Join-Path $Work '[Content_Types].xml') @'
<?xml version="1.0" encoding="utf-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="png" ContentType="image/png"/><Default Extension="json" ContentType="application/json"/><Default Extension="js" ContentType="application/javascript"/><Default Extension="ps1" ContentType="text/plain"/><Default Extension="vsixmanifest" ContentType="text/xml"/><Default Extension="xml" ContentType="text/xml"/><Default Extension="" ContentType="text/plain"/></Types>
'@
    # Build the archive with forward-slash entry names. Compress-Archive in Windows PowerShell 5.1 writes backslashes.
    Add-Type -AssemblyName System.IO.Compression
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $Package = Join-Path $Work 'terminal-beside.vsix'
    $Archive = [IO.Compression.ZipFile]::Open($Package, [IO.Compression.ZipArchiveMode]::Create)
    try {
        foreach ($Entry in @('extension/package.json', 'extension/extension.js', 'extension/README.md', 'extension/icon.png', 'extension/split.ps1', 'extension/pair.ps1', 'extension.vsixmanifest', '[Content_Types].xml')) {
            [void][IO.Compression.ZipFileExtensions]::CreateEntryFromFile($Archive, (Join-Path $Work ($Entry -replace '/', '\')), $Entry)
        }
    } finally { $Archive.Dispose() }

    Write-ToolboxInfo 'Checking existing Ctrl+Alt+S user bindings.'
    $Conflict = $false
    try {
        $Base = Join-Path $env:APPDATA 'Code\User'
        $Files = @(Join-Path $Base 'keybindings.json')
        $Profiles = Join-Path $Base 'profiles'
        if (Test-Path -LiteralPath $Profiles) {
            foreach ($Folder in Get-ChildItem -LiteralPath $Profiles -Directory) { $Files += Join-Path $Folder.FullName 'keybindings.json' }
        }
        $Strip = { param($Match) if ($Match.Value.StartsWith('"')) { $Match.Value } else { '' } }
        foreach ($File in $Files) {
            if (-not (Test-Path -LiteralPath $File)) { continue }
            $Text = [IO.File]::ReadAllText($File)
            # Preserve quoted strings while removing JSONC comments and trailing commas.
            $Text = [regex]::Replace($Text, '"(?:\\.|[^"\\])*"|//[^\n]*|/\*[\s\S]*?\*/', $Strip)
            $Text = [regex]::Replace($Text, '"(?:\\.|[^"\\])*"|,\s*(?=[}\]])', $Strip)
            if ([string]::IsNullOrWhiteSpace($Text)) { continue }
            # Windows PowerShell 5.1 emits a JSON array as one object. Piping it through ForEach-Object unrolls it.
            foreach ($Binding in @(ConvertFrom-Json -InputObject $Text | ForEach-Object { $_ })) {
                $Key = if ($Binding.PSObject.Properties['key']) { [string]$Binding.key } else { '' }
                $Command = if ($Binding.PSObject.Properties['command']) { [string]$Binding.command } else { '' }
                if ((($Key.ToLowerInvariant().Split('+') | Sort-Object) -join '+') -eq 'alt+ctrl+s' -and $Command -ne 'terminalBeside.openTerminal') { $Conflict = $true }
            }
        }
    } catch { $Conflict = $true }
    if ($Conflict) {
        Write-ToolboxWarn 'An existing Ctrl+Alt+S binding was found, or user bindings could not be parsed. Review Keyboard Shortcuts if the shortcut does not run Terminal Beside: Open Terminal. User bindings were preserved.'
    }

    Write-ToolboxInfo 'Installing MatheusFS-dev.terminal-beside through the VS Code CLI.'
    $InstallLog = (& code --install-extension $Package --force 2>&1 | Out-String)
    if ($LASTEXITCODE -ne 0) {
        Write-ToolboxError 'VS Code extension installation failed. CLI output follows.'
        [Console]::Error.WriteLine($InstallLog.TrimEnd())
        exit 1
    }
    Write-ToolboxSuccess 'Installed VS Code split terminal. Reload existing VS Code windows, then press Ctrl+Alt+S in a local workspace and choose Left or Right. Remove with: code --uninstall-extension MatheusFS-dev.terminal-beside'
} catch {
    Write-ToolboxError $_.Exception.Message
    exit 1
} finally {
    if ($Work -and (Test-Path -LiteralPath $Work)) { Remove-Item -LiteralPath $Work -Recurse -Force -ErrorAction SilentlyContinue }
}
