$ErrorActionPreference = 'Stop'

$RepositoryRoot = Split-Path -Parent $PSScriptRoot
$ModuleRoot = Join-Path $RepositoryRoot 'packages\scripts\terminal\windows\modules'
. (Join-Path $ModuleRoot 'shared.ps1')
. (Join-Path $ModuleRoot 'terminal.ps1')
. (Join-Path $ModuleRoot 'vscode.ps1')
$WslModuleRoot = Join-Path $RepositoryRoot 'packages\scripts\terminal\wsl\modules'
. (Join-Path $WslModuleRoot 'configure_shift_enter.ps1')

$TestRoot = Join-Path ([IO.Path]::GetTempPath()) ("my-toolbox-wsl-shift-enter-" + [Guid]::NewGuid())
[IO.Directory]::CreateDirectory($TestRoot) | Out-Null

try {
    $TerminalPath = Join-Path $TestRoot 'terminal\settings.json'
    [IO.Directory]::CreateDirectory((Split-Path -Parent $TerminalPath)) | Out-Null
    $TerminalOriginal = @'
{
  // keep terminal comment
  "actions": [
    { "command": "copy", "keys": "ctrl+shift+c" },
    { "command": "paste", "keys": "shift+enter" }
  ],
  "theme": "dark"
}
'@
    [IO.File]::WriteAllText($TerminalPath, $TerminalOriginal)
    $TerminalResult = Set-WindowsTerminalShiftEnterBinding -Path $TerminalPath
    if ($TerminalResult.Status -ne 'Success') { throw "Windows Terminal setup failed: $($TerminalResult.Message)" }
    $TerminalUpdated = [IO.File]::ReadAllText($TerminalPath)
    foreach ($Text in @('// keep terminal comment', '"theme": "dark"', '"action": "sendInput"', '"keys": "shift+enter"', '\u001b[200~\n\u001b[201~')) {
        if (-not $TerminalUpdated.Contains($Text)) { throw "Windows Terminal settings are missing '$Text'." }
    }
    if ([regex]::Matches($TerminalUpdated, '"keys"\s*:\s*"shift\+enter"').Count -ne 1) {
        throw 'Windows Terminal setup duplicated the Shift+Enter binding.'
    }
    $TerminalResult = Set-WindowsTerminalShiftEnterBinding -Path $TerminalPath
    if ([IO.File]::ReadAllText($TerminalPath) -cne $TerminalUpdated) {
        throw 'Repeated Windows Terminal setup changed an already-correct document.'
    }

    $VSCodePath = Join-Path $TestRoot 'vscode\keybindings.json'
    [IO.Directory]::CreateDirectory((Split-Path -Parent $VSCodePath)) | Out-Null
    $VSCodeOriginal = @'
[
  // keep vscode comment
  { "key": "ctrl+k", "command": "example.command" },
  { "key": "shift+enter", "command": "wrong.command", "when": "terminalFocus" }
]
'@
    [IO.File]::WriteAllText($VSCodePath, $VSCodeOriginal)
    $VSCodeResult = Set-VSCodeShiftEnterBinding -Path $VSCodePath
    if ($VSCodeResult.Status -ne 'Success') { throw "VS Code setup failed: $($VSCodeResult.Message)" }
    $VSCodeUpdated = [IO.File]::ReadAllText($VSCodePath)
    foreach ($Text in @('// keep vscode comment', '"command": "example.command"', '"command": "workbench.action.terminal.sendSequence"', '\u001b[200~\n\u001b[201~')) {
        if (-not $VSCodeUpdated.Contains($Text)) { throw "VS Code keybindings are missing '$Text'." }
    }
    if ([regex]::Matches($VSCodeUpdated, '"key"\s*:\s*"shift\+enter"').Count -ne 1) {
        throw 'VS Code setup duplicated the Shift+Enter binding.'
    }
    $VSCodeResult = Set-VSCodeShiftEnterBinding -Path $VSCodePath
    if ([IO.File]::ReadAllText($VSCodePath) -cne $VSCodeUpdated) {
        throw 'Repeated VS Code setup changed an already-correct document.'
    }

    $CreatedTerminalPath = Join-Path $TestRoot 'created-terminal\settings.json'
    $CreatedVSCodePath = Join-Path $TestRoot 'created-vscode\keybindings.json'
    [IO.Directory]::CreateDirectory((Split-Path -Parent $CreatedTerminalPath)) | Out-Null
    [IO.Directory]::CreateDirectory((Split-Path -Parent $CreatedVSCodePath)) | Out-Null
    $null = Set-WindowsTerminalShiftEnterBinding -Path $CreatedTerminalPath
    $null = Set-VSCodeShiftEnterBinding -Path $CreatedVSCodePath
    if (-not [IO.File]::Exists($CreatedTerminalPath) -or -not [IO.File]::Exists($CreatedVSCodePath)) {
        throw 'Shift+Enter setup did not create missing configuration files for installed applications.'
    }
    Assert-JsoncDocument -Text ([IO.File]::ReadAllText($CreatedTerminalPath)) -RootType Object
    Assert-JsoncDocument -Text ([IO.File]::ReadAllText($CreatedVSCodePath)) -RootType Array

    foreach ($Malformed in @(
        @{ Path = (Join-Path $TestRoot 'bad-terminal.json'); Kind = 'terminal' },
        @{ Path = (Join-Path $TestRoot 'bad-vscode.json'); Kind = 'vscode' }
    )) {
        [IO.File]::WriteAllText($Malformed.Path, '{ malformed')
        if ($Malformed.Kind -eq 'terminal') {
            $Result = Set-WindowsTerminalShiftEnterBinding -Path $Malformed.Path
        }
        else {
            $Result = Set-VSCodeShiftEnterBinding -Path $Malformed.Path
        }
        if ($Result.Status -ne 'Warning') { throw "$($Malformed.Kind) malformed input was not reported as a warning." }
        if ([IO.File]::ReadAllText($Malformed.Path) -cne '{ malformed') { throw "$($Malformed.Kind) malformed input was changed." }
    }

    $BackupTerminalPath = Join-Path $TestRoot 'backup-source\terminal.json'
    $BackupVSCodePath = Join-Path $TestRoot 'backup-source\keybindings.json'
    $BackupRoot = Join-Path $TestRoot 'backups'
    [IO.Directory]::CreateDirectory((Split-Path -Parent $BackupTerminalPath)) | Out-Null
    [IO.File]::WriteAllText($BackupTerminalPath, '{ "actions": [] }')
    [IO.File]::WriteAllText($BackupVSCodePath, '[]')
    $SetupResult = Invoke-WslShiftEnterSetup `
        -TerminalSettingsPath $BackupTerminalPath `
        -VSCodeKeybindingsPath $BackupVSCodePath `
        -BackupRoot $BackupRoot
    if ($SetupResult.Status -ne 'Success') { throw "Combined WSL host setup failed: $($SetupResult.Message)" }
    $BackupDirectory = Get-ChildItem -LiteralPath $BackupRoot -Directory | Select-Object -First 1
    if ($null -eq $BackupDirectory) { throw 'Combined WSL host setup did not create a backup.' }
    $BackedUpContent = @(Get-ChildItem -LiteralPath $BackupDirectory.FullName -File -Recurse | ForEach-Object { [IO.File]::ReadAllText($_.FullName) })
    if ('{ "actions": [] }' -notin $BackedUpContent -or '[]' -notin $BackedUpContent) {
        throw 'Combined WSL host setup did not preserve both original configuration files in its backup.'
    }

    $MissingRoot = Join-Path $TestRoot 'missing-applications'
    $MissingResult = Invoke-WslShiftEnterSetup `
        -TerminalSettingsPath (Join-Path $MissingRoot 'terminal\settings.json') `
        -VSCodeKeybindingsPath (Join-Path $MissingRoot 'vscode\keybindings.json') `
        -BackupRoot (Join-Path $MissingRoot 'backups')
    if ($MissingResult.Status -ne 'Warning') { throw 'Missing host applications were not reported as a warning.' }
    if ([IO.Directory]::Exists($MissingRoot)) { throw 'Missing host applications caused configuration paths to be created.' }
}
finally {
    Remove-Item -LiteralPath $TestRoot -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Output 'WSL Shift+Enter configuration checks passed.'
