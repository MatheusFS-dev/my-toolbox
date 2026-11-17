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
    Save-File (Join-Path $Extension 'package.json') @'
{
  "name": "split-terminal",
  "displayName": "Toolbox Split Terminal",
  "description": "Open Windows Terminal at the workspace root and tile it beside VS Code.",
  "version": "1.0.0",
  "publisher": "my-toolbox",
  "engines": { "vscode": "^1.74.0" },
  "main": "./extension.js",
  "extensionKind": ["ui"],
  "capabilities": { "untrustedWorkspaces": { "supported": true }, "virtualWorkspaces": false },
  "contributes": {
    "commands": [{ "command": "myToolbox.openSplitTerminal", "title": "Toolbox: Open Split Terminal" }],
    "keybindings": [{ "command": "myToolbox.openSplitTerminal", "key": "ctrl+alt+s", "when": "isWindows" }]
  }
}
'@
    Save-File (Join-Path $Extension 'extension.js') @'
'use strict'
const childProcess = require('node:child_process')
const { promisify } = require('node:util')
const { basename, join } = require('node:path')

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
  const powershell = ['-NoLogo', '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-File', helper]
  const run = async args => {
    try {
      return String((await execFile('powershell.exe', [...powershell, ...args], { env, timeout: 20000, maxBuffer: 1024 * 1024, windowsHide: true })).stdout).trim()
    } catch (error) {
      const detail = String(error.stderr || '').trim() || error.message
      throw new Error(error.code === 'ENOENT' ? 'powershell.exe is missing. Re-run the macro installer.' : detail)
    }
  }
  let busy = false
  return async () => {
    if (busy) return
    busy = true
    try {
      const root = await workspaceRoot(vscode)
      if (!root) return
      if (platform !== 'win32') throw new Error('Split terminal requires Windows.')
      const probe = await run(['-Mode', 'Probe'])
      const code = /^CODE=(\d+)$/m.exec(probe)
      if (!code) throw new Error('Focus VS Code before opening a split terminal.')
      // Windows Terminal windows already open are excluded when the helper looks for the new one.
      const known = (/^TERMINALS=(.*)$/m.exec(probe) || [])[1] || 'none'
      const title = basename(root) || root
      let launchError
      // Windows Terminal treats an unescaped semicolon as a command separator.
      const terminal = spawn('wt.exe', ['-w', 'new', '--title', title, '--suppressApplicationTitle', '-d', root.replace(/;/g, '\\;')], { cwd: root, env, detached: true, stdio: 'ignore' })
      terminal.on('error', error => { launchError = error })
      terminal.on('exit', status => { if (status) launchError = new Error(`Windows Terminal exited with status ${status}`) })
      terminal.unref()
      // The helper waits for the new terminal window and moves nothing until it exists.
      try {
        await run(['-Mode', 'Place', '-CodeHandle', code[1], '-Known', known, '-TimeoutMs', '8000'])
      } catch (error) {
        throw launchError || error
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
  context.subscriptions.push(vscode.commands.registerCommand('myToolbox.openSplitTerminal', createCommand(vscode)))
}
module.exports = { activate, workspaceRoot, createCommand }
'@
    Save-File (Join-Path $Extension 'split.ps1') @'
param(
    [Parameter(Mandatory = $true)][ValidateSet('Probe', 'Place')][string]$Mode,
    [string]$CodeHandle = '0',
    [string]$Known = 'none',
    [int]$TimeoutMs = 8000
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
$Left = [int][Math]::Floor($Width * 2 / 3)
[TbWin]::Restore($Terminal)
[TbWin]::Restore($Code)
# Monitors with different scaling make Windows rescale a window that crosses over, which undoes a single move.
# Land the terminal on the target monitor first, then place each window until its visible frame matches.
[TbWin]::MoveTo($Terminal, $X + $Left + 20, $Y + 20)
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
Set-Frame $Terminal ($X + $Left) $Y ($Width - $Left) $Height
Set-Frame $Code $X $Y $Left $Height
[TbWin]::Focus($Terminal)
[Console]::Out.WriteLine('OK')
} catch {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 1
}
'@
    Save-File (Join-Path $Work 'extension.vsixmanifest') @'
<?xml version="1.0" encoding="utf-8"?>
<PackageManifest Version="2.0.0" xmlns="http://schemas.microsoft.com/developer/vsx-schema/2011">
  <Metadata>
    <Identity Language="en-US" Id="split-terminal" Version="1.0.0" Publisher="my-toolbox" />
    <DisplayName>Toolbox Split Terminal</DisplayName>
    <Description xml:space="preserve">Open a workspace terminal and tile it beside VS Code on Windows.</Description>
    <Tags>terminal,windows</Tags><Categories>Other</Categories><GalleryFlags>Public</GalleryFlags>
    <Properties><Property Id="Microsoft.VisualStudio.Code.Engine" Value="^1.74.0" /><Property Id="Microsoft.VisualStudio.Code.ExtensionKind" Value="ui" /></Properties>
  </Metadata>
  <Installation><InstallationTarget Id="Microsoft.VisualStudio.Code" /></Installation>
  <Dependencies />
  <Assets><Asset Type="Microsoft.VisualStudio.Code.Manifest" Path="extension/package.json" Addressable="true" /></Assets>
</PackageManifest>
'@
    Save-File (Join-Path $Work '[Content_Types].xml') @'
<?xml version="1.0" encoding="utf-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="json" ContentType="application/json"/><Default Extension="js" ContentType="application/javascript"/><Default Extension="ps1" ContentType="text/plain"/><Default Extension="vsixmanifest" ContentType="text/xml"/><Default Extension="xml" ContentType="text/xml"/><Default Extension="" ContentType="text/plain"/></Types>
'@
    # Build the archive with forward-slash entry names. Compress-Archive in Windows PowerShell 5.1 writes backslashes.
    Add-Type -AssemblyName System.IO.Compression
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $Package = Join-Path $Work 'split-terminal.vsix'
    $Archive = [IO.Compression.ZipFile]::Open($Package, [IO.Compression.ZipArchiveMode]::Create)
    try {
        foreach ($Entry in @('extension/package.json', 'extension/extension.js', 'extension/split.ps1', 'extension.vsixmanifest', '[Content_Types].xml')) {
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
                if ((($Key.ToLowerInvariant().Split('+') | Sort-Object) -join '+') -eq 'alt+ctrl+s' -and $Command -ne 'myToolbox.openSplitTerminal') { $Conflict = $true }
            }
        }
    } catch { $Conflict = $true }
    if ($Conflict) {
        Write-ToolboxWarn 'An existing Ctrl+Alt+S binding was found, or user bindings could not be parsed. Review Keyboard Shortcuts if the shortcut does not run Toolbox: Open Split Terminal. User bindings were preserved.'
    }

    Write-ToolboxInfo 'Installing my-toolbox.split-terminal through the VS Code CLI.'
    $InstallLog = (& code --install-extension $Package --force 2>&1 | Out-String)
    if ($LASTEXITCODE -ne 0) {
        Write-ToolboxError 'VS Code extension installation failed. CLI output follows.'
        [Console]::Error.WriteLine($InstallLog.TrimEnd())
        exit 1
    }
    Write-ToolboxSuccess 'Installed VS Code split terminal. Reload existing VS Code windows, then press Ctrl+Alt+S in a local workspace. Remove with: code --uninstall-extension my-toolbox.split-terminal'
} catch {
    Write-ToolboxError $_.Exception.Message
    exit 1
} finally {
    if ($Work -and (Test-Path -LiteralPath $Work)) { Remove-Item -LiteralPath $Work -Recurse -Force -ErrorAction SilentlyContinue }
}
