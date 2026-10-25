param(
    [string]$TerminalSettingsPath,
    [string]$VSCodeKeybindingsPath,
    [string]$BackupRoot
)

$ErrorActionPreference = 'Stop'
$WindowsModuleRoot = Join-Path (Split-Path -Parent $PSScriptRoot) '..\windows\modules'
. (Join-Path $WindowsModuleRoot 'shared.ps1')
. (Join-Path $WindowsModuleRoot 'backup.ps1')
. (Join-Path $WindowsModuleRoot 'terminal.ps1')
. (Join-Path $WindowsModuleRoot 'vscode.ps1')

function Invoke-WslShiftEnterSetup {
    param(
        [string]$TerminalSettingsPath = (Get-WindowsTerminalSettingsPath),
        [string]$VSCodeKeybindingsPath = (Get-VSCodeKeybindingsPath),
        [string]$BackupRoot = (Join-Path $env:LOCALAPPDATA 'project-template\wsl-backups')
    )

    $terminalDirectory = Split-Path -Parent $TerminalSettingsPath
    $vscodeDirectory = Split-Path -Parent $VSCodeKeybindingsPath
    $terminalAvailable = [IO.Directory]::Exists($terminalDirectory)
    $vscodeAvailable = [IO.Directory]::Exists($vscodeDirectory)
    $pathMap = [ordered]@{}
    if ($terminalAvailable -and [IO.File]::Exists($TerminalSettingsPath)) {
        $pathMap['WindowsTerminal/settings.json'] = $TerminalSettingsPath
    }
    if ($vscodeAvailable -and [IO.File]::Exists($VSCodeKeybindingsPath)) {
        $pathMap['VSCode/keybindings.json'] = $VSCodeKeybindingsPath
    }
    $backupPath = ''
    if ($pathMap.Count -gt 0) {
        $backupPath = New-WindowsConfigBackup -PathMap $pathMap -BackupRoot $BackupRoot
    }

    $messages = New-Object Collections.Generic.List[string]
    $warning = $false
    if ($terminalAvailable) {
        $terminalResult = Set-WindowsTerminalShiftEnterBinding -Path $TerminalSettingsPath
        $messages.Add($terminalResult.Message)
        $warning = $warning -or $terminalResult.Status -eq 'Warning'
    }
    else {
        $messages.Add('Windows Terminal settings were not found; skipped its Shift+Enter binding.')
        $warning = $true
    }
    if ($vscodeAvailable) {
        $vscodeResult = Set-VSCodeShiftEnterBinding -Path $VSCodeKeybindingsPath
        $messages.Add($vscodeResult.Message)
        $warning = $warning -or $vscodeResult.Status -eq 'Warning'
    }
    else {
        $messages.Add('VS Code settings were not found; skipped its Shift+Enter binding.')
        $warning = $true
    }
    if ($backupPath) {
        $messages.Add("Original settings were backed up to $backupPath.")
    }
    $status = if ($warning) { 'Warning' } else { 'Success' }
    return New-FeatureResult -Status $status -Message ([string]::Join(' ', $messages))
}

if ($MyInvocation.InvocationName -ne '.') {
    try {
        $arguments = @{}
        if ($TerminalSettingsPath) { $arguments.TerminalSettingsPath = $TerminalSettingsPath }
        if ($VSCodeKeybindingsPath) { $arguments.VSCodeKeybindingsPath = $VSCodeKeybindingsPath }
        if ($BackupRoot) { $arguments.BackupRoot = $BackupRoot }
        $result = Invoke-WslShiftEnterSetup @arguments
        Write-Output ("{0}: {1}" -f $result.Status, $result.Message)
        exit 0
    }
    catch {
        [Console]::Error.WriteLine("Error: $($_.Exception.Message)")
        exit 1
    }
}
