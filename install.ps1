param(
    [Parameter(DontShow = $true)]
    [scriptblock]$UserPathReader = { [Environment]::GetEnvironmentVariable('Path', 'User') },
    [Parameter(DontShow = $true)]
    [scriptblock]$UserPathWriter = { param([string]$Value) [Environment]::SetEnvironmentVariable('Path', $Value, 'User') },
    [Parameter(DontShow = $true)]
    [scriptblock]$DocumentsPathReader = { [Environment]::GetFolderPath([Environment+SpecialFolder]::MyDocuments) }
)

$ErrorActionPreference = 'Stop'

$Repository = 'MatheusFS-dev/my-toolbox'
$DataRoot = Join-Path $env:LOCALAPPDATA 'my-toolbox'
$VersionsRoot = Join-Path $DataRoot 'versions'
$CurrentFile = Join-Path $DataRoot 'current.txt'
$WrapperRoot = Join-Path $DataRoot 'bin'
$WrapperPath = Join-Path $WrapperRoot 'tb.cmd'
$CompletionRoot = Join-Path $DataRoot 'completions'
$TemporaryRoot = $null
$TemporaryCurrent = $null
$TemporaryWrapper = $null
$StagingPayload = $null
$CurrentStage = 0
$CurrentStageName = ''
$PublishedVersion = $false
$PublishedWrapper = $false
$Activated = $false
$SavedWrapper = $null
$CompletionPublished = $false
$CompletionReplaced = $false
$SavedCompletionRoot = $null
$PublishedProfiles = @()
$InstallSucceeded = $false

function Write-Status {
    <#
    .SYNOPSIS
    Writes one installer status line with optional interactive-terminal color.
    .PARAMETER Kind
    INFO, OK, or FAIL. The value selects the label and interactive color.
    .PARAMETER Message
    Plain ASCII status text. Redirected output always remains uncolored.
    .OUTPUTS
    System.String when output is redirected; otherwise writes directly to the host.
    #>
    param(
        [Parameter(Mandatory = $true)]
        [ValidateSet('INFO', 'OK', 'FAIL')]
        [string]$Kind,
        [Parameter(Mandatory = $true)]
        [string]$Message
    )

    $Text = "[$Kind] $Message"
    if ([Console]::IsOutputRedirected) {
        Write-Output $Text
        return
    }
    $Color = switch ($Kind) {
        'INFO' { 'Cyan' }
        'OK' { 'Green' }
        'FAIL' { 'Red' }
    }
    Write-Host $Text -ForegroundColor $Color
}

function Start-Stage {
    <#
    .SYNOPSIS
    Marks and reports the active installer stage.
    .PARAMETER Number
    One-based stage number in the fixed seven-stage bootstrap sequence.
    .PARAMETER Name
    Stable stage label used by INFO, OK, and FAIL messages.
    .OUTPUTS
    None.
    #>
    param(
        [Parameter(Mandatory = $true)]
        [int]$Number,
        [Parameter(Mandatory = $true)]
        [string]$Name
    )

    $script:CurrentStage = $Number
    $script:CurrentStageName = $Name
    Write-Status -Kind INFO -Message "Stage $CurrentStage/7: $CurrentStageName"
}

function Complete-Stage {
    <#
    .SYNOPSIS
    Reports successful completion of the current installer stage.
    .OUTPUTS
    None.
    #>
    Write-Status -Kind OK -Message "Stage $CurrentStage/7: $CurrentStageName"
}

function Get-NormalizedPathEntry {
    <#
    .SYNOPSIS
    Normalizes a PATH entry for identity comparison only.
    .PARAMETER Entry
    A single PATH entry that may be quoted or end in a directory separator.
    .OUTPUTS
    System.String.
    #>
    param(
        [AllowEmptyString()]
        [string]$Entry
    )

    $Normalized = $Entry.Trim()
    if ($Normalized.Length -ge 2 -and $Normalized[0] -eq '"' -and $Normalized[$Normalized.Length - 1] -eq '"') {
        $Normalized = $Normalized.Substring(1, $Normalized.Length - 2)
    }
    return $Normalized.TrimEnd([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar)
}

function Add-PathEntry {
    <#
    .SYNOPSIS
    Appends one PATH entry unless an equivalent entry is already present.
    .PARAMETER Value
    Existing PATH text to preserve byte-for-byte as the result prefix.
    .PARAMETER Entry
    Directory to append when absent.
    .OUTPUTS
    System.String.
    #>
    param(
        [AllowNull()]
        [AllowEmptyString()]
        [string]$Value,
        [Parameter(Mandatory = $true)]
        [string]$Entry
    )

    $Existing = if ($null -eq $Value) { '' } else { $Value }
    $NormalizedEntry = Get-NormalizedPathEntry -Entry $Entry
    foreach ($Candidate in ($Existing -split ';')) {
        if ([string]::Equals((Get-NormalizedPathEntry -Entry $Candidate), $NormalizedEntry, [StringComparison]::OrdinalIgnoreCase)) {
            return $Existing
        }
    }
    if ([string]::IsNullOrEmpty($Existing) -or $Existing.EndsWith(';')) {
        return $Existing + $Entry
    }
    return $Existing + ';' + $Entry
}

function Enable-ToolboxPath {
    <#
    .SYNOPSIS
    Activates the toolbox wrapper for future and current processes.
    .OUTPUTS
    None.
    #>
    $UserPath = & $UserPathReader
    $UpdatedUserPath = Add-PathEntry -Value $UserPath -Entry $WrapperRoot
    if ($UpdatedUserPath -cne $UserPath) {
        & $UserPathWriter $UpdatedUserPath | Out-Null
    }
    $env:PATH = Add-PathEntry -Value $env:PATH -Entry $WrapperRoot
}

function New-CompletionProfileState {
    <#
    .SYNOPSIS
    Builds and validates one exact PowerShell profile activation candidate.
    .PARAMETER Path
    CurrentUserAllHosts profile path whose unrelated bytes must remain unchanged.
    .PARAMETER SourceLine
    Exact ASCII command that loads the stable toolbox completion registration.
    .OUTPUTS
    PSCustomObject containing original bytes, candidate bytes, and path state for publication and rollback.
    #>
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [string]$SourceLine
    )

    $StartMarker = '# >>> my-toolbox completion >>>'
    $EndMarker = '# <<< my-toolbox completion <<<'
    $Existed = Test-Path -LiteralPath $Path -PathType Leaf
    if (-not $Existed -and (Test-Path -LiteralPath $Path)) {
        throw "PowerShell profile is not a regular file: $Path"
    }
    if ($Existed) {
        $OriginalBytes = [IO.File]::ReadAllBytes($Path)
    } else {
        $OriginalBytes = [byte[]]::new(0)
    }
    $Encoding = [Text.Encoding]::Default
    $PreambleLength = 0
    $AddUtf8Preamble = $false
    $Text = $null
    if ($OriginalBytes.Length -ge 3 -and $OriginalBytes[0] -eq 0xEF -and $OriginalBytes[1] -eq 0xBB -and $OriginalBytes[2] -eq 0xBF) {
        $Encoding = [Text.UTF8Encoding]::new($true, $true)
        $PreambleLength = 3
    } elseif ($OriginalBytes.Length -ge 2 -and $OriginalBytes[0] -eq 0xFF -and $OriginalBytes[1] -eq 0xFE) {
        $Encoding = [Text.UnicodeEncoding]::new($false, $true, $true)
        $PreambleLength = 2
    } elseif ($OriginalBytes.Length -ge 2 -and $OriginalBytes[0] -eq 0xFE -and $OriginalBytes[1] -eq 0xFF) {
        $Encoding = [Text.UnicodeEncoding]::new($true, $true, $true)
        $PreambleLength = 2
    } else {
        # PowerShell 7 writes BOM-less UTF-8 by default. Decode it strictly
        # before using the Windows PowerShell system-code-page fallback.
        $Utf8Encoding = [Text.UTF8Encoding]::new($false, $true)
        try {
            $Text = $Utf8Encoding.GetString($OriginalBytes)
            $Encoding = $Utf8Encoding
            # Windows PowerShell 5.1 requires the UTF-8 preamble to decode
            # non-ASCII script files as UTF-8 when it loads the profile.
            $AddUtf8Preamble = $true
        } catch [Text.DecoderFallbackException] {
            $Text = $Encoding.GetString($OriginalBytes)
        }
    }
    if ($null -eq $Text) {
        $Text = $Encoding.GetString($OriginalBytes, $PreambleLength, $OriginalBytes.Length - $PreambleLength)
    }
    $StartOccurrences = [regex]::Matches($Text, [regex]::Escape($StartMarker)).Count
    $EndOccurrences = [regex]::Matches($Text, [regex]::Escape($EndMarker)).Count
    $StartLines = [regex]::Matches($Text, '(?m)^' + [regex]::Escape($StartMarker) + '\r?$').Count
    $EndLines = [regex]::Matches($Text, '(?m)^' + [regex]::Escape($EndMarker) + '\r?$').Count
    if ($StartOccurrences -ne $StartLines -or $EndOccurrences -ne $EndLines -or $StartLines -ne $EndLines -or $StartLines -gt 1) {
        throw "Malformed my-toolbox completion markers in $Path."
    }

    $BlockWithoutTerminator = "$StartMarker`r`n$SourceLine`r`n$EndMarker"
    if ($StartLines -eq 1) {
        $StartIndex = $Text.IndexOf($StartMarker, [StringComparison]::Ordinal)
        $EndIndex = $Text.IndexOf($EndMarker, $StartIndex, [StringComparison]::Ordinal)
        $ManagedBlock = $Text.Substring($StartIndex, $EndIndex + $EndMarker.Length - $StartIndex)
        if ($ManagedBlock -cne $BlockWithoutTerminator) {
            throw "Malformed my-toolbox completion block in $Path."
        }
        $CandidateBytes = $OriginalBytes
    } else {
        $Separator = if ($OriginalBytes.Length -eq 0) { '' } else { "`r`n" }
        $AddedBytes = $Encoding.GetBytes("$Separator$BlockWithoutTerminator`r`n")
        $CandidateBytes = [byte[]]::new($OriginalBytes.Length + $AddedBytes.Length)
        [Array]::Copy($OriginalBytes, 0, $CandidateBytes, 0, $OriginalBytes.Length)
        [Array]::Copy($AddedBytes, 0, $CandidateBytes, $OriginalBytes.Length, $AddedBytes.Length)
    }
    $CandidatePreambleLength = $PreambleLength
    if ($AddUtf8Preamble) {
        # Prefixing the preamble leaves every unrelated original byte intact
        # while making the resulting profile portable across both runtimes.
        $Utf8Preamble = [byte[]](0xEF, 0xBB, 0xBF)
        $BytesWithPreamble = [byte[]]::new($Utf8Preamble.Length + $CandidateBytes.Length)
        [Array]::Copy($Utf8Preamble, 0, $BytesWithPreamble, 0, $Utf8Preamble.Length)
        [Array]::Copy($CandidateBytes, 0, $BytesWithPreamble, $Utf8Preamble.Length, $CandidateBytes.Length)
        $CandidateBytes = $BytesWithPreamble
        $CandidatePreambleLength = $Utf8Preamble.Length
    }

    $Tokens = $null
    $ParseErrors = $null
    $CandidateText = $Encoding.GetString($CandidateBytes, $CandidatePreambleLength, $CandidateBytes.Length - $CandidatePreambleLength)
    [void][Management.Automation.Language.Parser]::ParseInput($CandidateText, [ref]$Tokens, [ref]$ParseErrors)
    if ($ParseErrors.Count -gt 0) {
        throw "PowerShell profile candidate is invalid for ${Path}: $ParseErrors"
    }
    return [pscustomobject]@{
        Path = $Path
        Parent = Split-Path -Parent $Path
        ParentExisted = Test-Path -LiteralPath (Split-Path -Parent $Path) -PathType Container
        Existed = $Existed
        OriginalBytes = $OriginalBytes
        CandidateBytes = $CandidateBytes
    }
}

function Enable-ToolboxCompletion {
    <#
    .SYNOPSIS
    Publishes stable completion assets and exact PowerShell profile blocks.
    .DESCRIPTION
    Validates both profile candidates before publication. Existing managed
    assets and profile bytes are left unchanged when already current, while
    missing or stale managed assets are replaced transactionally.
    .OUTPUTS
    None.
    #>
    foreach ($CompletionAsset in @('_tb', 'tb.bash', 'tb.ps1')) {
        if (-not (Test-Path -LiteralPath (Join-Path $VersionRoot "completions\$CompletionAsset") -PathType Leaf)) {
            throw "Installed version is missing completion asset $CompletionAsset."
        }
    }

    $Documents = [string](& $DocumentsPathReader)
    if ([string]::IsNullOrWhiteSpace($Documents)) {
        throw 'The current user Documents known folder could not be resolved.'
    }
    $CompletionSourceLine = ". (Join-Path `$env:LOCALAPPDATA 'my-toolbox\completions\tb.ps1')"
    $ProfileStates = @(
        New-CompletionProfileState -Path (Join-Path $Documents 'WindowsPowerShell\profile.ps1') -SourceLine $CompletionSourceLine
        New-CompletionProfileState -Path (Join-Path $Documents 'PowerShell\profile.ps1') -SourceLine $CompletionSourceLine
    )

    $CompletionAssetsMatch = Test-Path -LiteralPath $CompletionRoot -PathType Container
    if (-not $CompletionAssetsMatch -and (Test-Path -LiteralPath $CompletionRoot)) {
        throw "Completion path is not a directory: $CompletionRoot"
    }
    if ($CompletionAssetsMatch) {
        foreach ($CompletionAsset in @('_tb', 'tb.bash', 'tb.ps1')) {
            $VersionAsset = Join-Path $VersionRoot "completions\$CompletionAsset"
            $StableAsset = Join-Path $CompletionRoot $CompletionAsset
            if (-not (Test-Path -LiteralPath $StableAsset -PathType Leaf) -or -not [Collections.StructuralComparisons]::StructuralEqualityComparer.Equals([IO.File]::ReadAllBytes($VersionAsset), [IO.File]::ReadAllBytes($StableAsset))) {
                $CompletionAssetsMatch = $false
                break
            }
        }
    }
    if (-not $CompletionAssetsMatch) {
        if (Test-Path -LiteralPath $CompletionRoot -PathType Container) {
            $script:SavedCompletionRoot = Join-Path $DataRoot ('.completions-previous-' + [Guid]::NewGuid())
            Move-Item -LiteralPath $CompletionRoot -Destination $SavedCompletionRoot
            $script:CompletionReplaced = $true
        } else {
            $script:CompletionPublished = $true
        }
        Copy-Item -LiteralPath (Join-Path $VersionRoot 'completions') -Destination $CompletionRoot -Recurse
    }

    foreach ($ProfileState in $ProfileStates) {
        if ([Collections.StructuralComparisons]::StructuralEqualityComparer.Equals($ProfileState.OriginalBytes, $ProfileState.CandidateBytes)) {
            continue
        }
        New-Item -ItemType Directory -Force -Path $ProfileState.Parent | Out-Null
        $script:PublishedProfiles += $ProfileState
        [IO.File]::WriteAllBytes($ProfileState.Path, $ProfileState.CandidateBytes)
    }
}

function Expand-ToolboxArchive {
    <#
    .SYNOPSIS
    Extracts a toolbox ZIP while preserving validated symbolic links.
    .PARAMETER ArchivePath
    Existing release ZIP file to read.
    .PARAMETER Destination
    New extraction root. Every archive path and symbolic-link target must stay
    within this directory.
    .OUTPUTS
    None.
    #>
    param(
        [Parameter(Mandatory = $true)]
        [string]$ArchivePath,
        [Parameter(Mandatory = $true)]
        [string]$Destination
    )

    Add-Type -AssemblyName System.IO.Compression.FileSystem
    New-Item -ItemType Directory -Path $Destination | Out-Null
    $Root = [IO.Path]::GetFullPath($Destination)
    $RootPrefix = $Root.TrimEnd([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar
    $ArchiveObject = [IO.Compression.ZipFile]::OpenRead($ArchivePath)
    $SymbolicLinks = @()
    try {
        foreach ($Entry in $ArchiveObject.Entries) {
            $ArchiveName = $Entry.FullName.Replace('\', '/')
            if ([string]::IsNullOrEmpty($ArchiveName)) {
                continue
            }
            $RelativePath = $ArchiveName.Replace('/', [IO.Path]::DirectorySeparatorChar)
            $EntryPath = [IO.Path]::GetFullPath((Join-Path $Root $RelativePath))
            if (-not $EntryPath.StartsWith($RootPrefix, [StringComparison]::OrdinalIgnoreCase)) {
                throw "Archive contains unsafe path $ArchiveName."
            }
            $UnixType = (($Entry.ExternalAttributes -shr 16) -band 0xF000)
            if ($UnixType -eq 0xA000) {
                $SymbolicLinks += [pscustomobject]@{ Entry = $Entry; Path = $EntryPath; Name = $ArchiveName }
                continue
            }
            if ($ArchiveName.EndsWith('/')) {
                New-Item -ItemType Directory -Force -Path $EntryPath | Out-Null
                continue
            }
            $Parent = Split-Path -Parent $EntryPath
            New-Item -ItemType Directory -Force -Path $Parent | Out-Null
            $InputStream = $Entry.Open()
            $OutputStream = $null
            try {
                $OutputStream = [IO.File]::Open($EntryPath, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
                $InputStream.CopyTo($OutputStream)
            } finally {
                if ($null -ne $OutputStream) {
                    $OutputStream.Dispose()
                }
                $InputStream.Dispose()
            }
        }
        foreach ($Link in $SymbolicLinks) {
            if ($Link.Entry.Length -gt 4096) {
                throw "Archive symbolic link $($Link.Name) has an oversized target."
            }
            $InputStream = $Link.Entry.Open()
            $Reader = $null
            try {
                $Reader = [IO.StreamReader]::new($InputStream, [Text.UTF8Encoding]::new($false), $true, 1024, $false)
                $Target = $Reader.ReadToEnd()
            } finally {
                if ($null -ne $Reader) {
                    $Reader.Dispose()
                } else {
                    $InputStream.Dispose()
                }
            }
            if ([string]::IsNullOrEmpty($Target) -or $Target.Contains([char]0) -or [IO.Path]::IsPathRooted($Target)) {
                throw "Archive symbolic link $($Link.Name) has an unsafe target."
            }
            $ResolvedTarget = [IO.Path]::GetFullPath((Join-Path (Split-Path -Parent $Link.Path) $Target))
            if ($ResolvedTarget -ne $Root -and -not $ResolvedTarget.StartsWith($RootPrefix, [StringComparison]::OrdinalIgnoreCase)) {
                throw "Archive symbolic link $($Link.Name) escapes the extraction root."
            }
            New-Item -ItemType Directory -Force -Path (Split-Path -Parent $Link.Path) | Out-Null
            New-Item -ItemType SymbolicLink -Path $Link.Path -Target $Target | Out-Null
        }
    } finally {
        $ArchiveObject.Dispose()
    }
}

@'
 __  __ __   __  _____ ___   ___  _     ____   _____  __
|  \/  |\ \ / / |_   _/ _ \ / _ \| |   | __ ) / _ \ \/ /
| |\/| | \ V /    | || | | | | | | |   |  _ \| | | \  /
| |  | |  | |     | || |_| | |_| | |___| |_) | |_| /  \
|_|  |_|  |_|     |_| \___/ \___/|_____|____/ \___/_/\_\
'@ | Write-Output

try {
    Start-Stage -Number 1 -Name 'prerequisites'
    if (Test-Path -LiteralPath $CurrentFile) {
        $CurrentVersion = (Get-Content -LiteralPath $CurrentFile -TotalCount 1).Trim()
        Write-Status -Kind INFO -Message "my-toolbox $CurrentVersion is already installed. Run tb update to upgrade."
        Complete-Stage
        $VersionRoot = Join-Path $VersionsRoot $CurrentVersion
        Start-Stage -Number 7 -Name 'activation'
        Enable-ToolboxCompletion
        Enable-ToolboxPath
        Complete-Stage
        $InstallSucceeded = $true
        return
    }
    if (-not [Environment]::Is64BitOperatingSystem) {
        throw 'my-toolbox requires 64-bit Windows on x64.'
    }
    foreach ($CommandName in @('Invoke-RestMethod', 'Invoke-WebRequest', 'Get-FileHash')) {
        if ($null -eq (Get-Command $CommandName -ErrorAction SilentlyContinue)) {
            throw "$CommandName is required to install my-toolbox."
        }
    }
    Complete-Stage

    Start-Stage -Number 2 -Name 'release lookup'
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
    Complete-Stage

    Start-Stage -Number 3 -Name 'download'
    $TemporaryRoot = Join-Path ([IO.Path]::GetTempPath()) ("my-toolbox-" + [Guid]::NewGuid())
    New-Item -ItemType Directory -Path $TemporaryRoot | Out-Null
    $BaseUrl = "https://github.com/$Repository/releases/download/$Tag"
    $ArchivePath = Join-Path $TemporaryRoot $Archive
    $ChecksumPath = "$ArchivePath.sha256"
    Invoke-WebRequest -Uri "$BaseUrl/$Archive" -OutFile $ArchivePath
    Invoke-WebRequest -Uri "$BaseUrl/$Archive.sha256" -OutFile $ChecksumPath
    Complete-Stage

    Start-Stage -Number 4 -Name 'checksum'
    $ChecksumFields = ((Get-Content -LiteralPath $ChecksumPath -Raw).Trim() -split '\s+')
    if ($ChecksumFields.Count -lt 2 -or $ChecksumFields[1] -ne $Archive) {
        throw "Checksum file does not identify $Archive."
    }
    $ActualChecksum = (Get-FileHash -LiteralPath $ArchivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($ActualChecksum -ne $ChecksumFields[0].ToLowerInvariant()) {
        throw "SHA-256 mismatch for $Archive."
    }
    Complete-Stage

    Start-Stage -Number 5 -Name 'extraction/validation'
    New-Item -ItemType Directory -Force -Path $VersionsRoot | Out-Null
    $StagingPayload = Join-Path $VersionsRoot (".install-$Version-" + [Guid]::NewGuid())
    $Payload = $StagingPayload
    Expand-ToolboxArchive -ArchivePath $ArchivePath -Destination $Payload
    $Required = @(
        'tb.exe',
        'commands.json',
        'version.txt',
        'completions\_tb',
        'completions\tb.bash',
        'completions\tb.ps1',
        'packages\agent-workspace-template\source\scripts\windows\install_codex.py',
        'packages\agent-workspace-template\source\scripts\windows\install_claude.py',
        'packages\agent-workspace-template\source\scripts\windows\install_antigravity.py',
        'packages\agent-workspace-template\source\scripts\windows\install_project.py',
        'packages\scripts\terminal\windows\setup_windows.ps1',
        'packages\scripts\terminal\windows\set_vscode_wsl_cwd.ps1',
        'packages\others\create_project_template.py'
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
    Complete-Stage

    Start-Stage -Number 6 -Name 'installation'
    New-Item -ItemType Directory -Force -Path $VersionsRoot, $WrapperRoot | Out-Null
    $TemporaryWrapper = Join-Path $WrapperRoot ('.tb-' + [Guid]::NewGuid() + '.cmd')
    @(
        '@echo off',
        'setlocal',
        'set /p TOOLBOX_VERSION=<"%LOCALAPPDATA%\my-toolbox\current.txt"',
        '"%LOCALAPPDATA%\my-toolbox\versions\%TOOLBOX_VERSION%\tb.exe" %*'
    ) | Set-Content -LiteralPath $TemporaryWrapper -Encoding ascii
    if (Test-Path -LiteralPath $WrapperPath -PathType Container) {
        throw "Wrapper path is an existing directory: $WrapperPath"
    }
    if (Test-Path -LiteralPath $WrapperPath) {
        $SavedWrapperCandidate = Join-Path $WrapperRoot ('.tb-previous-' + [Guid]::NewGuid() + '.cmd')
        Move-Item -LiteralPath $WrapperPath -Destination $SavedWrapperCandidate
        $SavedWrapper = $SavedWrapperCandidate
    }
    $PublishedVersion = $true
    Move-Item -LiteralPath $StagingPayload -Destination $VersionRoot
    $StagingPayload = $null
    $PublishedWrapper = $true
    Move-Item -LiteralPath $TemporaryWrapper -Destination $WrapperPath
    $TemporaryWrapper = $null
    Complete-Stage

    Start-Stage -Number 7 -Name 'activation'
    Enable-ToolboxCompletion
    $TemporaryCurrent = Join-Path $DataRoot 'current.txt.new'
    Set-Content -LiteralPath $TemporaryCurrent -Value $Version -Encoding ascii
    $Activated = $true
    Move-Item -LiteralPath $TemporaryCurrent -Destination $CurrentFile
    $TemporaryCurrent = $null
    Enable-ToolboxPath
    Write-Status -Kind OK -Message "Installed my-toolbox $Version."
    Complete-Stage
    $InstallSucceeded = $true
} catch {
    $Failure = "[FAIL] Stage $CurrentStage/7: $CurrentStageName"
    Write-Status -Kind FAIL -Message "Stage $CurrentStage/7: $CurrentStageName"
    throw "$Failure. $($_.Exception.Message)"
} finally {
    if (-not $InstallSucceeded) {
        foreach ($ProfileState in $PublishedProfiles) {
            if ($ProfileState.Existed) {
                [IO.File]::WriteAllBytes($ProfileState.Path, $ProfileState.OriginalBytes)
            } elseif (Test-Path -LiteralPath $ProfileState.Path) {
                Remove-Item -LiteralPath $ProfileState.Path -Force
            }
        }
        foreach ($ProfileState in $PublishedProfiles) {
            if (-not $ProfileState.ParentExisted -and (Test-Path -LiteralPath $ProfileState.Parent -PathType Container)) {
                Remove-Item -LiteralPath $ProfileState.Parent -ErrorAction SilentlyContinue
            }
        }
        if ($CompletionPublished -and (Test-Path -LiteralPath $CompletionRoot)) {
            Remove-Item -LiteralPath $CompletionRoot -Recurse -Force
        }
        if ($CompletionReplaced) {
            if (Test-Path -LiteralPath $CompletionRoot) {
                Remove-Item -LiteralPath $CompletionRoot -Recurse -Force
            }
            Move-Item -LiteralPath $SavedCompletionRoot -Destination $CompletionRoot
        }
        if ($Activated -and (Test-Path -LiteralPath $CurrentFile)) {
            Remove-Item -LiteralPath $CurrentFile -Force
        }
        if ($PublishedWrapper -and (Test-Path -LiteralPath $WrapperPath)) {
            Remove-Item -LiteralPath $WrapperPath -Force
        }
        if ($null -ne $SavedWrapper -and (Test-Path -LiteralPath $SavedWrapper)) {
            Move-Item -LiteralPath $SavedWrapper -Destination $WrapperPath
        }
        if ($PublishedVersion -and (Test-Path -LiteralPath $VersionRoot)) {
            Remove-Item -LiteralPath $VersionRoot -Recurse -Force
        }
    } elseif ($null -ne $SavedWrapper -and (Test-Path -LiteralPath $SavedWrapper)) {
        Remove-Item -LiteralPath $SavedWrapper -Force
    }
    if ($InstallSucceeded -and $null -ne $SavedCompletionRoot -and (Test-Path -LiteralPath $SavedCompletionRoot)) {
        Remove-Item -LiteralPath $SavedCompletionRoot -Recurse -Force
    }
    if ($null -ne $TemporaryCurrent -and (Test-Path -LiteralPath $TemporaryCurrent)) {
        Remove-Item -LiteralPath $TemporaryCurrent -Force
    }
    if ($null -ne $TemporaryWrapper -and (Test-Path -LiteralPath $TemporaryWrapper)) {
        Remove-Item -LiteralPath $TemporaryWrapper -Force
    }
    if ($null -ne $StagingPayload -and (Test-Path -LiteralPath $StagingPayload)) {
        Remove-Item -LiteralPath $StagingPayload -Recurse -Force
    }
    if ($null -ne $TemporaryRoot -and (Test-Path -LiteralPath $TemporaryRoot)) {
        Remove-Item -LiteralPath $TemporaryRoot -Recurse -Force
    }
}
