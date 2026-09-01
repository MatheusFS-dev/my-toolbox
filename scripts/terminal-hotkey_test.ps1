$ErrorActionPreference = 'Stop'

$RepositoryRoot = Split-Path -Parent $PSScriptRoot
$ScriptPath = Join-Path $RepositoryRoot 'packages\scripts\terminal\windows\set_terminal_hotkey.ps1'
$TestRoot = Join-Path ([IO.Path]::GetTempPath()) ("my-toolbox-terminal-hotkey-" + [Guid]::NewGuid())
$OriginalAppData = $env:APPDATA
$ShortcutPath = Join-Path $TestRoot 'Microsoft\Windows\Start Menu\Programs\My Toolbox\Open Default Terminal.lnk'
$ManagedDescription = 'Managed by my-toolbox: opens the Windows default terminal application.'

New-Item -ItemType Directory -Path $TestRoot | Out-Null

try {
    $env:APPDATA = $TestRoot
    & $ScriptPath | Out-Null

    if (-not (Test-Path -LiteralPath $ShortcutPath -PathType Leaf)) {
        throw "Terminal hotkey shortcut was not created at $ShortcutPath."
    }

    $Shell = New-Object -ComObject WScript.Shell
    $Shortcut = $Shell.CreateShortcut($ShortcutPath)
    if (-not $Shortcut.TargetPath.Equals($env:ComSpec, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Shortcut target = '$($Shortcut.TargetPath)', want '$env:ComSpec'."
    }
    $HotkeyParts = @($Shortcut.Hotkey.ToUpperInvariant().Split('+') | Sort-Object)
    if ([string]::Join('+', $HotkeyParts) -cne 'ALT+CTRL+T') {
        throw "Shortcut hotkey = '$($Shortcut.Hotkey)', want 'Ctrl+Alt+T'."
    }
    if ($Shortcut.Description -cne $ManagedDescription) {
        throw "Shortcut description = '$($Shortcut.Description)', want the my-toolbox ownership marker."
    }

    $InitialWriteTime = (Get-Item -LiteralPath $ShortcutPath).LastWriteTimeUtc
    Start-Sleep -Milliseconds 1100
    & $ScriptPath | Out-Null
    if ((Get-Item -LiteralPath $ShortcutPath).LastWriteTimeUtc -ne $InitialWriteTime) {
        throw 'Repeated setup rewrote an already-correct shortcut.'
    }

    & $ScriptPath -Undo | Out-Null
    if (Test-Path -LiteralPath $ShortcutPath) {
        throw 'Undo did not remove the managed terminal hotkey shortcut.'
    }

    $ShortcutDirectory = Split-Path -Parent $ShortcutPath
    New-Item -ItemType Directory -Path $ShortcutDirectory -Force | Out-Null
    $Unrelated = $Shell.CreateShortcut($ShortcutPath)
    $Unrelated.TargetPath = $env:ComSpec
    $Unrelated.Hotkey = 'Ctrl+Alt+Y'
    $Unrelated.Description = 'Unrelated shortcut'
    $Unrelated.Save()

    $SetupFailure = ''
    try {
        & $ScriptPath | Out-Null
    } catch {
        $SetupFailure = $_.Exception.Message
    }
    if (-not $SetupFailure.Contains('not managed by my-toolbox')) {
        throw "Setup did not reject an unrelated shortcut. Error: $SetupFailure"
    }
    if ($Shell.CreateShortcut($ShortcutPath).Description -cne 'Unrelated shortcut') {
        throw 'Setup changed an unrelated shortcut.'
    }

    $UndoFailure = ''
    try {
        & $ScriptPath -Undo | Out-Null
    } catch {
        $UndoFailure = $_.Exception.Message
    }
    if (-not $UndoFailure.Contains('not managed by my-toolbox')) {
        throw "Undo did not reject an unrelated shortcut. Error: $UndoFailure"
    }
    if (-not (Test-Path -LiteralPath $ShortcutPath -PathType Leaf)) {
        throw 'Undo removed an unrelated shortcut.'
    }
} finally {
    $env:APPDATA = $OriginalAppData
    Remove-Item -LiteralPath $TestRoot -Recurse -Force -ErrorAction SilentlyContinue
}
