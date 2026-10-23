[CmdletBinding()]
param(
    [switch]$Undo
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'modules/output.ps1')
Initialize-ToolboxOutput -ToolName 'set-terminal-hotkey'
Write-ToolboxInfo 'Checking the current Ctrl+Alt+T shortcut...'

try {

$ManagedDescription = 'Managed by my-toolbox: opens the Windows default terminal application.'
$ShortcutDirectory = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\My Toolbox'
$ShortcutPath = Join-Path $ShortcutDirectory 'Open Default Terminal.lnk'

if ([string]::IsNullOrWhiteSpace($env:APPDATA)) {
    throw 'APPDATA is unavailable; cannot locate the current user Start Menu.'
}
if ([string]::IsNullOrWhiteSpace($env:ComSpec) -or -not (Test-Path -LiteralPath $env:ComSpec -PathType Leaf)) {
    throw 'ComSpec does not identify an available Windows command processor.'
}

$Shell = New-Object -ComObject WScript.Shell

if (Test-Path -LiteralPath $ShortcutPath) {
    $ExistingShortcut = $Shell.CreateShortcut($ShortcutPath)
    if ($ExistingShortcut.Description -cne $ManagedDescription) {
        throw "The shortcut at '$ShortcutPath' is not managed by my-toolbox; it was left unchanged."
    }

    if ($Undo) {
        Remove-Item -LiteralPath $ShortcutPath -Force
        if (@(Get-ChildItem -LiteralPath $ShortcutDirectory -Force).Count -eq 0) {
            Remove-Item -LiteralPath $ShortcutDirectory -Force
        }
        Write-ToolboxSuccess 'Removed the persistent Ctrl+Alt+T terminal hotkey.'
        return
    }

    $TargetMatches = $ExistingShortcut.TargetPath.Equals($env:ComSpec, [StringComparison]::OrdinalIgnoreCase)
    $HotkeyParts = @($ExistingShortcut.Hotkey.ToUpperInvariant().Split('+') | Sort-Object)
    $HotkeyMatches = [string]::Join('+', $HotkeyParts) -ceq 'ALT+CTRL+T'
    if ($TargetMatches -and $HotkeyMatches) {
        Write-ToolboxSuccess 'Ctrl+Alt+T is already configured to open the Windows default terminal application.'
        return
    }
} elseif ($Undo) {
    Write-ToolboxInfo 'The Ctrl+Alt+T terminal hotkey is not configured; nothing changed.'
    return
}

New-Item -ItemType Directory -Path $ShortcutDirectory -Force | Out-Null
$Shortcut = $Shell.CreateShortcut($ShortcutPath)
$Shortcut.TargetPath = $env:ComSpec
$Shortcut.WorkingDirectory = $env:USERPROFILE
$Shortcut.Hotkey = 'Ctrl+Alt+T'
$Shortcut.Description = $ManagedDescription
$Shortcut.Save()

Write-ToolboxSuccess 'Configured the persistent Ctrl+Alt+T terminal hotkey successfully.'
}
catch {
    Write-ToolboxError "Terminal hotkey update failed: $($_.Exception.Message). Correct the reported issue and retry; any existing shortcut was left unchanged when possible."
    exit 1
}
