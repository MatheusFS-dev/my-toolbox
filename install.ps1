$ErrorActionPreference = 'Stop'

$Repository = 'MatheusFS-dev/my-toolbox'
$DataRoot = Join-Path $env:LOCALAPPDATA 'my-toolbox'
$VersionsRoot = Join-Path $DataRoot 'versions'
$CurrentFile = Join-Path $DataRoot 'current.txt'
$WrapperRoot = Join-Path $DataRoot 'bin'
$WrapperPath = Join-Path $WrapperRoot 'tb.cmd'

if (Test-Path -LiteralPath $CurrentFile) {
    $CurrentVersion = (Get-Content -LiteralPath $CurrentFile -TotalCount 1).Trim()
    Write-Host "my-toolbox $CurrentVersion is already installed. Run tb update to upgrade."
    exit 0
}

if (-not [Environment]::Is64BitOperatingSystem) {
    throw 'my-toolbox requires 64-bit Windows on x64.'
}

$Release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repository/releases/latest"
$Tag = [string]$Release.tag_name
if ([string]::IsNullOrWhiteSpace($Tag)) {
    throw 'Latest my-toolbox release did not contain a tag.'
}
if ($Tag -notmatch '^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$') {
    throw "Release tag is not a safe three-part version: $Tag"
}
$Version = $Tag.Substring(1)
$Archive = 'toolbox-windows-amd64.zip'
$VersionRoot = Join-Path $VersionsRoot $Version
if (Test-Path -LiteralPath $VersionRoot) {
    throw "Version directory already exists without an active installation: $VersionRoot"
}

$TemporaryRoot = Join-Path ([IO.Path]::GetTempPath()) ("my-toolbox-" + [Guid]::NewGuid())
New-Item -ItemType Directory -Path $TemporaryRoot | Out-Null
try {
    $BaseUrl = "https://github.com/$Repository/releases/download/$Tag"
    $ArchivePath = Join-Path $TemporaryRoot $Archive
    $ChecksumPath = "$ArchivePath.sha256"
    Invoke-WebRequest -Uri "$BaseUrl/$Archive" -OutFile $ArchivePath
    Invoke-WebRequest -Uri "$BaseUrl/$Archive.sha256" -OutFile $ChecksumPath

    $ChecksumFields = ((Get-Content -LiteralPath $ChecksumPath -Raw).Trim() -split '\s+')
    if ($ChecksumFields.Count -lt 2 -or $ChecksumFields[1] -ne $Archive) {
        throw "Checksum file does not identify $Archive."
    }
    $ActualChecksum = (Get-FileHash -LiteralPath $ArchivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($ActualChecksum -ne $ChecksumFields[0].ToLowerInvariant()) {
        throw "SHA-256 mismatch for $Archive."
    }

    $Payload = Join-Path $TemporaryRoot 'payload'
    Expand-Archive -LiteralPath $ArchivePath -DestinationPath $Payload
    $Required = @(
        'tb.exe',
        'commands.json',
        'version.txt',
        'packages\agent-workspace-template\source\scripts\windows\install_codex.py',
        'packages\agent-workspace-template\source\scripts\windows\install_claude.py',
        'packages\agent-workspace-template\source\scripts\windows\install_antigravity.py',
        'packages\agent-workspace-template\source\scripts\windows\install_project.py'
    )
    foreach ($RelativePath in $Required) {
        if (-not (Test-Path -LiteralPath (Join-Path $Payload $RelativePath) -PathType Leaf)) {
            throw "Downloaded payload is missing $RelativePath."
        }
    }
    $PayloadVersion = (Get-Content -LiteralPath (Join-Path $Payload 'version.txt') -TotalCount 1).Trim()
    if ($PayloadVersion -ne $Version) {
        throw "Downloaded payload version $PayloadVersion does not match release $Version."
    }

    New-Item -ItemType Directory -Force -Path $VersionsRoot, $WrapperRoot | Out-Null
    $TemporaryWrapper = "$WrapperPath.new"
    @(
        '@echo off',
        'setlocal',
        'set /p TOOLBOX_VERSION=<"%LOCALAPPDATA%\my-toolbox\current.txt"',
        '"%LOCALAPPDATA%\my-toolbox\versions\%TOOLBOX_VERSION%\tb.exe" %*'
    ) | Set-Content -LiteralPath $TemporaryWrapper -Encoding ascii
    Move-Item -LiteralPath $TemporaryWrapper -Destination $WrapperPath
    Move-Item -LiteralPath $Payload -Destination $VersionRoot
    $TemporaryCurrent = Join-Path $DataRoot 'current.txt.new'
    Set-Content -LiteralPath $TemporaryCurrent -Value $Version -Encoding ascii
    Move-Item -LiteralPath $TemporaryCurrent -Destination $CurrentFile
} finally {
    if (Test-Path -LiteralPath $TemporaryRoot) {
        Remove-Item -LiteralPath $TemporaryRoot -Recurse -Force
    }
}

Write-Host "Installed my-toolbox $Version."
if (($env:PATH -split ';') -notcontains $WrapperRoot) {
    Write-Host "Add $WrapperRoot to PATH to run tb."
}
