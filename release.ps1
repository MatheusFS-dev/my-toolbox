# Interactively run or observe the existing release controller on Windows.

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'assets/payload/packages/scripts/terminal/windows/modules/output.ps1')
Initialize-ToolboxOutput release

if ($args.Count -ne 0) {
    Write-ToolboxError 'release.ps1 is interactive. Run it without arguments.'
    exit 2
}

$GitCommand = Get-Command git.exe -ErrorAction SilentlyContinue
if (-not $GitCommand) {
    Write-ToolboxError 'Git for Windows is required.'
    exit 1
}
$GitRoot = Split-Path -Parent (Split-Path -Parent $GitCommand.Source)
$BashPath = Join-Path $GitRoot 'bin/bash.exe'
if (-not (Test-Path -LiteralPath $BashPath -PathType Leaf)) {
    Write-ToolboxError 'Git Bash is required. Install Git for Windows with Git Bash.'
    exit 1
}

while ($true) {
    $ControllerArguments = $null
    Write-ToolboxMenu 'Choose release action' @(
        '1. Release current changes'
        '2. Watch an existing release'
    )
    $Action = [Console]::ReadLine()
    if ($null -eq $Action) {
        Write-ToolboxError 'Release action input closed.'
        exit 1
    }
    switch ($Action) {
        '1' {
            $Message = Read-ToolboxRequired 'Release message (required)'
            $ControllerArguments = @('--message', $Message)
            break
        }
        '2' {
            $Sha = Read-ToolboxRequired 'Commit SHA to watch (required)'
            $ControllerArguments = @('--watch', $Sha)
            break
        }
        default {
            Write-ToolboxWarn 'Choose 1 to release current changes or 2 to watch an existing release.'
        }
    }
    if ($null -ne $ControllerArguments) { break }
}

$PreviousLocation = Get-Location
try {
    Set-Location -LiteralPath $PSScriptRoot
    & $BashPath 'tooling/release/release.sh' @ControllerArguments
    exit $LASTEXITCODE
} finally {
    Set-Location -LiteralPath $PreviousLocation.Path
}
