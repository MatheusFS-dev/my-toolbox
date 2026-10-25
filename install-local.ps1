$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$RepositoryRoot = $PSScriptRoot
. (Join-Path $RepositoryRoot 'assets/payload/packages/scripts/terminal/windows/modules/output.ps1')
Initialize-ToolboxOutput -ToolName 'install-local'

function Show-PathGuide {
    Write-Output ('')
    Write-Output ('If tb is not recognized, add it to PATH:')
    Write-Output ('')
    Write-Output ('  Current PowerShell session')
    Write-Output ('  $env:Path = "$env:LOCALAPPDATA\my-toolbox\bin;$env:Path"')
    Write-Output ('')
    Write-Output ('  Persist for future PowerShell sessions')
    Write-Output ('  New-Item -ItemType Directory -Force (Split-Path $PROFILE) | Out-Null')
    Write-Output ('  Add-Content -Path $PROFILE -Value ''$env:Path = "$env:LOCALAPPDATA\my-toolbox\bin;$env:Path"''')
    Write-Output ('')
}

$Stage = $null
try {
    if ($env:OS -ne 'Windows_NT' -or [Environment]::GetEnvironmentVariable('PROCESSOR_ARCHITECTURE') -ne 'AMD64') {
        throw 'Local installation supports native Windows x64.'
    }
    if ($null -eq (Get-Command go -ErrorAction SilentlyContinue)) {
        throw 'Go 1.25.8 is required. Install it and retry.'
    }
    $DataRoot = Join-Path $env:LOCALAPPDATA 'my-toolbox'
    $VersionsRoot = Join-Path $DataRoot 'versions'
    $CurrentFile = Join-Path $DataRoot 'current.txt'
    $WrapperRoot = Join-Path $DataRoot 'bin'
    $Wrapper = Join-Path $WrapperRoot 'tb.cmd'
    $Current = ''
    if (Test-Path -LiteralPath $CurrentFile) {
        $Current = (Get-Content -LiteralPath $CurrentFile -TotalCount 1).Trim()
    }
    $BaseVersion = '0.0.0'
    if ($Current -match '^([0-9]+\.[0-9]+\.[0-9]+)') { $BaseVersion = $Matches[1] }
    $Version = '{0}-local.{1}' -f $BaseVersion, [DateTime]::UtcNow.ToString('yyyyMMddHHmmss')
    $VersionRoot = Join-Path $VersionsRoot $Version
    if (Test-Path -LiteralPath $VersionRoot) { throw "Local version $Version already exists. Retry after one second." }
    [void](New-Item -ItemType Directory -Force -Path $VersionsRoot, $WrapperRoot)
    $Stage = Join-Path $VersionsRoot ('.install-{0}.{1}' -f $Version, [Guid]::NewGuid().ToString('N'))
    [void](New-Item -ItemType Directory -Path $Stage)
    $Libexec = Join-Path $Stage 'libexec'
    [void](New-Item -ItemType Directory -Path $Libexec)

    Write-ToolboxInfo 'Checking the local source and bundled Markdown reader...'
    $InstalledReader = [string]$env:TOOLBOX_READER
    if (-not $InstalledReader -and $Current) {
        $InstalledReader = Join-Path (Join-Path $VersionsRoot $Current) 'libexec/tb-markdown-reader.exe'
    }
    if ($InstalledReader -and (Test-Path -LiteralPath $InstalledReader -PathType Leaf)) {
        Copy-Item -LiteralPath $InstalledReader -Destination (Join-Path $Libexec 'tb-markdown-reader.exe')
    } else {
        Write-ToolboxInfo 'Building the bundled Markdown reader...'
        & (Join-Path $RepositoryRoot 'tooling/reader/build-markdown-reader.ps1') $Stage
        if ($LASTEXITCODE -ne 0) { throw 'Markdown reader build failed; the active version was not changed.' }
    }
    & (Join-Path $Libexec 'tb-markdown-reader.exe') --tb-self-check | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Markdown reader self-check failed; the active version was not changed.' }
    Copy-Item -LiteralPath (Join-Path $RepositoryRoot 'assets/reader/fork-markdown-reader/LICENSE') -Destination (Join-Path $Libexec 'tb-markdown-reader-LICENSE')

    Write-ToolboxInfo 'Building and staging the local toolbox...'
    Copy-Item -LiteralPath (Join-Path $RepositoryRoot 'assets/payload/commands.json') -Destination $Stage
    Copy-Item -LiteralPath (Join-Path $RepositoryRoot 'assets/payload/completions') -Destination $Stage -Recurse
    $Packages = Join-Path $Stage 'packages'
    [void](New-Item -ItemType Directory -Path $Packages)
    Get-ChildItem -LiteralPath (Join-Path $RepositoryRoot 'assets/payload/packages') | Where-Object { $_.Name -ne 'search' } | ForEach-Object {
        Copy-Item -LiteralPath $_.FullName -Destination $Packages -Recurse
    }
    $Search = Join-Path $Packages 'search'
    [void](New-Item -ItemType Directory -Path $Search)
    Copy-Item -LiteralPath (Join-Path $RepositoryRoot 'assets/payload/packages/search/articles') -Destination $Search -Recurse
    Get-ChildItem -LiteralPath $Packages -Force -Recurse | Where-Object { $_.Name -eq '.git' -or $_.Name -eq '__pycache__' } | ForEach-Object {
        Remove-Item -LiteralPath $_.FullName -Recurse -Force
    }
    [IO.File]::WriteAllText((Join-Path $Stage 'version.txt'), "$Version`n", [Text.UTF8Encoding]::new($false))
    Push-Location $RepositoryRoot
    try {
        & go build -trimpath -ldflags "-s -w -X main.version=$Version" -o (Join-Path $Stage 'tb.exe') ./cmd/tb
    } finally {
        Pop-Location
    }
    if ($LASTEXITCODE -ne 0) { throw 'Go build failed; the active version was not changed.' }
    & (Join-Path $Stage 'tb.exe') help | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Local toolbox smoke test failed; the active version was not changed.' }

    Write-ToolboxInfo 'Activating the local toolbox version...'
    Move-Item -LiteralPath $Stage -Destination $VersionRoot
    $Stage = $null
    if (-not (Test-Path -LiteralPath $Wrapper)) {
        $WrapperText = "@echo off`r`nset /p TOOLBOX_VERSION=<`"%LOCALAPPDATA%\my-toolbox\current.txt`"`r`n`"%LOCALAPPDATA%\my-toolbox\versions\%TOOLBOX_VERSION%\tb.exe`" %*`r`n"
        [IO.File]::WriteAllText($Wrapper, $WrapperText, [Text.Encoding]::ASCII)
    }
    $Candidate = Join-Path $DataRoot ('.current.' + [Guid]::NewGuid().ToString('N'))
    [IO.File]::WriteAllText($Candidate, "$Version`n", [Text.Encoding]::ASCII)
    Move-Item -LiteralPath $Candidate -Destination $CurrentFile -Force
    $PathParts = @([Environment]::GetEnvironmentVariable('Path', 'User') -split ';')
    if ($PathParts -notcontains $WrapperRoot) {
        [Environment]::SetEnvironmentVariable('Path', (($PathParts + $WrapperRoot) -join ';').Trim(';'), 'User')
    }
    if (($env:Path -split ';') -notcontains $WrapperRoot) { $env:Path += ";$WrapperRoot" }
    Show-PathGuide
    Write-ToolboxSuccess "Installed local toolbox $Version. Run tb version to verify it."
} catch {
    Write-ToolboxError $_.Exception.Message
    exit 1
} finally {
    if ($Stage -and (Test-Path -LiteralPath $Stage)) { Remove-Item -LiteralPath $Stage -Recurse -Force }
}
