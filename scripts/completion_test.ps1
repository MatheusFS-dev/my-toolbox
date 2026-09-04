$ErrorActionPreference = 'Stop'

$RepositoryRoot = Split-Path -Parent $PSScriptRoot
$TestRoot = Join-Path ([IO.Path]::GetTempPath()) ("my-toolbox-completion-" + [Guid]::NewGuid())
$BinRoot = Join-Path $TestRoot 'bin'
New-Item -ItemType Directory -Path $BinRoot | Out-Null

try {
    if ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT) {
        @'
@echo off
if not "%1"=="__complete" exit /b 2
if not "%2"=="" exit /b 2
echo help
echo install-codex
echo install-gh
echo list
echo uninstall
echo update
echo version
'@ | Set-Content -LiteralPath (Join-Path $BinRoot 'tb.cmd') -Encoding ASCII
    } else {
        @'
#!/bin/sh
if [ "$#" -ne 1 ] || [ "$1" != "__complete" ]; then exit 2; fi
printf '%s\n' help install-codex install-gh list uninstall update version
'@ | Set-Content -LiteralPath (Join-Path $BinRoot 'tb') -Encoding ASCII
        & chmod 755 (Join-Path $BinRoot 'tb')
    }
    $env:PATH = "$BinRoot$([IO.Path]::PathSeparator)$env:PATH"

    . (Join-Path $RepositoryRoot 'completions\tb.ps1')

    $FirstInput = 'tb in'
    $First = [Management.Automation.CommandCompletion]::CompleteInput($FirstInput, $FirstInput.Length, $null).CompletionMatches.CompletionText
    if ([string]::Join("`n", $First) -cne "install-codex`ninstall-gh") {
        throw "PowerShell first-argument completion = $([string]::Join(', ', $First))."
    }

    $LaterInput = 'tb install-codex --'
    $Later = [Management.Automation.CommandCompletion]::CompleteInput($LaterInput, $LaterInput.Length, $null).CompletionMatches.CompletionText
    $ToolboxCandidates = @('help', 'install-codex', 'install-gh', 'list', 'uninstall', 'update', 'version')
    if (@($Later | Where-Object { $_ -in $ToolboxCandidates }).Count -ne 0) {
        throw "PowerShell later-argument completion returned toolbox candidates: $([string]::Join(', ', $Later))."
    }
} finally {
    Remove-Item -LiteralPath $TestRoot -Recurse -Force -ErrorAction SilentlyContinue
}
