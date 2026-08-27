$ErrorActionPreference = 'Stop'

$RepositoryRoot = Split-Path -Parent $PSScriptRoot
$Installer = Join-Path $RepositoryRoot 'install.ps1'
$TestRoot = Join-Path ([IO.Path]::GetTempPath()) ("my-toolbox-installer-test-" + [Guid]::NewGuid())
$Payload = Join-Path $TestRoot 'payload'
$Downloads = Join-Path $TestRoot 'downloads'
$OriginalLocalAppData = $env:LOCALAPPDATA
$OriginalPath = $env:PATH
New-Item -ItemType Directory -Path $Payload, $Downloads | Out-Null
$TemporaryBefore = @(
    Get-ChildItem ([IO.Path]::GetTempPath()) -Directory -Filter 'my-toolbox-*' |
        ForEach-Object FullName
)

try {
    Copy-Item -LiteralPath (Join-Path $RepositoryRoot 'commands.json') -Destination (Join-Path $Payload 'commands.json')
    Set-Content -LiteralPath (Join-Path $Payload 'version.txt') -Value '0.1.5' -Encoding ascii
    Set-Content -LiteralPath (Join-Path $Payload 'tb.exe') -Value 'fixture' -Encoding ascii
    $Catalog = Get-Content -LiteralPath (Join-Path $RepositoryRoot 'commands.json') -Raw | ConvertFrom-Json
    foreach ($Command in $Catalog.commands) {
        if ($Command.protocol -eq 'builtin') {
            continue
        }
        $Entrypoint = $Command.entrypoints.'windows-amd64'
        if ($null -eq $Entrypoint) {
            continue
        }
        foreach ($RelativePath in $Entrypoint[1..($Entrypoint.Count - 1)]) {
            $Path = Join-Path $Payload $RelativePath
            New-Item -ItemType Directory -Force -Path (Split-Path -Parent $Path) | Out-Null
            Set-Content -LiteralPath $Path -Value 'fixture' -Encoding ascii
        }
    }
    $Archive = Join-Path $Downloads 'toolbox-windows-amd64.zip'
    Compress-Archive -Path (Join-Path $Payload '*') -DestinationPath $Archive
    $Digest = (Get-FileHash -LiteralPath $Archive -Algorithm SHA256).Hash.ToLowerInvariant()
    Set-Content -LiteralPath "$Archive.sha256" -Value "$Digest  toolbox-windows-amd64.zip" -Encoding ascii
    $script:FixtureArchive = $Archive

    function Invoke-RestMethod {
        param(
            [string]$Uri
        )
        return [pscustomobject]@{ tag_name = 'v0.1.5' }
    }
    function Invoke-WebRequest {
        param(
            [string]$Uri,
            [string]$OutFile
        )
        if ($Uri.EndsWith('.sha256')) {
            Copy-Item -LiteralPath "$script:FixtureArchive.sha256" -Destination $OutFile
        } else {
            Copy-Item -LiteralPath $script:FixtureArchive -Destination $OutFile
        }
    }

    $env:LOCALAPPDATA = Join-Path $TestRoot 'localappdata'
    $env:PATH = 'C:\Windows\System32'
    $Output = (& $Installer *>&1 | Out-String)
    foreach ($Text in @(
        ' __  __ __   __  _____ ___   ___  _     ____   _____  __',
        '[INFO] Stage 1/7: prerequisites',
        '[INFO] Stage 2/7: release lookup',
        '[INFO] Stage 3/7: download',
        '[INFO] Stage 4/7: checksum',
        '[INFO] Stage 5/7: extraction/validation',
        '[INFO] Stage 6/7: installation',
        '[INFO] Stage 7/7: activation',
        '[OK] Stage 7/7: activation',
        'Add '
    )) {
        if (-not $Output.Contains($Text)) {
            throw "Installer output is missing '$Text'. Output: $Output"
        }
    }
    if ($Output.Contains([char]27)) {
        throw 'Redirected installer output contains ANSI escapes.'
    }
    $InstalledTool = Join-Path $env:LOCALAPPDATA 'my-toolbox\versions\0.1.5\packages\others\create_project_template.py'
    if (-not (Test-Path -LiteralPath $InstalledTool -PathType Leaf)) {
        throw 'Installer did not install the fixture payload.'
    }
    $TemporaryAfter = @(
        Get-ChildItem ([IO.Path]::GetTempPath()) -Directory -Filter 'my-toolbox-*' |
            ForEach-Object FullName
    )
    $LeakedTemporaryPaths = @($TemporaryAfter | Where-Object { $_ -notin $TemporaryBefore })
    if ($LeakedTemporaryPaths.Count -gt 0) {
        throw "Installer left temporary directories behind: $LeakedTemporaryPaths"
    }

    Remove-Item -LiteralPath (Join-Path $env:LOCALAPPDATA 'my-toolbox') -Recurse -Force
    Set-Content -LiteralPath "$script:FixtureArchive.sha256" -Value "$('0' * 64)  toolbox-windows-amd64.zip" -Encoding ascii
    $Failure = ''
    try {
        & $Installer
    } catch {
        $Failure = ($_ | Out-String)
    }
    if (-not $Failure.Contains('[FAIL] Stage 4/7: checksum')) {
        throw "Checksum failure did not identify its active stage. Error: $Failure"
    }
    if (Test-Path -LiteralPath (Join-Path $env:LOCALAPPDATA 'my-toolbox\versions\0.1.5')) {
        throw 'Checksum failure created a version directory.'
    }
    $TemporaryAfterFailure = @(
        Get-ChildItem ([IO.Path]::GetTempPath()) -Directory -Filter 'my-toolbox-*' |
            ForEach-Object FullName
    )
    $LeakedTemporaryPaths = @(
        $TemporaryAfterFailure | Where-Object { $_ -notin $TemporaryBefore }
    )
    if ($LeakedTemporaryPaths.Count -gt 0) {
        throw "Failed installer left temporary directories behind: $LeakedTemporaryPaths"
    }
} finally {
    $env:LOCALAPPDATA = $OriginalLocalAppData
    $env:PATH = $OriginalPath
    if (Test-Path -LiteralPath $TestRoot) {
        Remove-Item -LiteralPath $TestRoot -Recurse -Force
    }
}
