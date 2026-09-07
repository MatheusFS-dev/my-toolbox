$ErrorActionPreference = 'Stop'

$RepositoryRoot = Split-Path -Parent $PSScriptRoot
$Installer = Join-Path $RepositoryRoot 'install.ps1'
$TestRoot = Join-Path ([IO.Path]::GetTempPath()) ("my-toolbox-installer-test-" + [Guid]::NewGuid())
$Payload = Join-Path $TestRoot 'payload'
$Downloads = Join-Path $TestRoot 'downloads'
$Documents = Join-Path $TestRoot 'Documents'
$OriginalLocalAppData = $env:LOCALAPPDATA
$OriginalPath = $env:PATH
$OriginalPathExt = $env:PATHEXT
$OriginalLastExitCode = $global:LASTEXITCODE
$OriginalProcessorArchitecture = $env:PROCESSOR_ARCHITECTURE
$OriginalProcessorArchitectureW6432 = $env:PROCESSOR_ARCHITEW6432
$OriginalUpdate = $env:TOOLBOX_UPDATE
$OriginalExpectedVersion = $env:TOOLBOX_EXPECTED_VERSION
New-Item -ItemType Directory -Path $Payload, $Downloads, $Documents | Out-Null
$TemporaryBefore = @(
    Get-ChildItem ([IO.Path]::GetTempPath()) -Directory -Filter 'my-toolbox-*' |
        ForEach-Object FullName
)

try {
    Copy-Item -LiteralPath (Join-Path $RepositoryRoot 'commands.json') -Destination (Join-Path $Payload 'commands.json')
    $MacroRoot = Join-Path $Payload 'packages\macros\autohotkey'
    New-Item -ItemType Directory -Path $MacroRoot -Force | Out-Null
    Copy-Item -LiteralPath (Join-Path $RepositoryRoot 'packages\macros\autohotkey\press_key_after_x_ms.ahk') -Destination $MacroRoot
    Copy-Item -LiteralPath (Join-Path $RepositoryRoot 'packages\macros\autohotkey\press_key_after_x_ms.json') -Destination $MacroRoot
    Copy-Item -LiteralPath (Join-Path $RepositoryRoot 'completions') -Destination (Join-Path $Payload 'completions') -Recurse
    Set-Content -LiteralPath (Join-Path $Payload 'version.txt') -Value '0.1.5' -Encoding ascii
    Set-Content -LiteralPath (Join-Path $Payload 'tb.exe') -Value 'fixture' -Encoding ascii
    New-Item -ItemType Directory -Path (Join-Path $Payload 'libexec') | Out-Null
    Set-Content -LiteralPath (Join-Path $Payload 'libexec\tb-markdown-reader.exe') -Value 'reader fixture' -Encoding ascii
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
    $ArticleRoot = Join-Path $Payload 'packages\search\articles\test'
    New-Item -ItemType Directory -Force -Path $ArticleRoot | Out-Null
    Set-Content -LiteralPath (Join-Path $ArticleRoot 'fixture.md') -Value '# Fixture Guide' -Encoding utf8
    $Archive = Join-Path $Downloads 'toolbox-windows-amd64.zip'
    Compress-Archive -Path (Join-Path $Payload '*') -DestinationPath $Archive
    $Digest = (Get-FileHash -LiteralPath $Archive -Algorithm SHA256).Hash.ToLowerInvariant()
    Set-Content -LiteralPath "$Archive.sha256" -Value "$Digest  toolbox-windows-amd64.zip" -Encoding ascii
    $global:ToolboxInstallerFixtureArchive = $Archive

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
            Copy-Item -LiteralPath "$global:ToolboxInstallerFixtureArchive.sha256" -Destination $OutFile
        } else {
            Copy-Item -LiteralPath $global:ToolboxInstallerFixtureArchive -Destination $OutFile
        }
    }

    function Invoke-TestInstaller {
        param(
            [Parameter(Mandatory = $true)]
            [scriptblock]$UserPathWriter,
            [scriptblock]$ReaderValidator = {
                param([string]$ReaderPath, [string[]]$ReaderArguments)
                Assert-StagedReaderInvocation $ReaderPath $ReaderArguments
                return 0
            },
            [switch]$UseProductionReaderValidator
        )

        $ReaderParameters = @{}
        if (-not $UseProductionReaderValidator) {
            $ReaderParameters.ReaderValidator = $ReaderValidator
        }
        & $Installer @ReaderParameters `
            -UserPathReader { $global:ToolboxInstallerTestUserPath } `
            -UserPathWriter $UserPathWriter `
            -DocumentsPathReader { $global:ToolboxInstallerTestDocuments } `
            -CommandReader {
                param([string]$Name)
                if ($Name -in @('py', 'python')) {
                    throw "Windows bootstrap unexpectedly probed $Name."
                }
                return Get-Command $Name -ErrorAction SilentlyContinue
            }
    }

    function Write-Output {
        param([Parameter(ValueFromPipeline = $true)]$InputObject)

        process {
            if ($UpdateCase -eq 'status' -and $InputObject -ceq '[OK] Installed my-toolbox 0.1.5.') {
                throw 'Injected status output failure.'
            }
            Microsoft.PowerShell.Utility\Write-Output -InputObject $InputObject
        }
    }

    function Assert-StagedReaderInvocation {
        param([string]$ReaderPath, [string[]]$ReaderArguments)

        $Data = Join-Path $env:LOCALAPPDATA 'my-toolbox'
        $Staging = @(Get-ChildItem -LiteralPath (Join-Path $Data 'versions') -Directory -Filter '.install-0.1.5-*')
        if ($Staging.Count -ne 1 -or $ReaderPath -cne (Join-Path $Staging[0].FullName 'libexec\tb-markdown-reader.exe') -or
            $ReaderArguments.Count -ne 1 -or $ReaderArguments[0] -cne '--tb-self-check' -or
            -not (Test-Path -LiteralPath $ReaderPath -PathType Leaf)) {
            throw 'Reader validator did not receive the exact staged executable and self-check argument.'
        }
        if ((Test-Path -LiteralPath (Join-Path $Data 'versions\0.1.5')) -or (Test-Path -LiteralPath (Join-Path $Data 'current.txt'))) {
            throw 'Reader validator ran after version or current publication.'
        }
        $global:ToolboxInstallerReaderChecks += $ReaderPath
    }

    function Get-InstallerSnapshot {
        param([string]$Root)

        return (@(Get-ChildItem -LiteralPath $Root -Recurse -Force | Sort-Object FullName | ForEach-Object {
            $Relative = $_.FullName.Substring($Root.Length)
            if ($_.PSIsContainer) {
                "directory $Relative"
            } else {
                "file $Relative $((Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash)"
            }
        }) -join "`n")
    }

    $global:ToolboxInstallerReaderChecks = @()
    $MissingReaderPayload = Join-Path $TestRoot 'payload-without-reader'
    Copy-Item -LiteralPath $Payload -Destination $MissingReaderPayload -Recurse
    Remove-Item -LiteralPath (Join-Path $MissingReaderPayload 'libexec\tb-markdown-reader.exe')
    $MissingReaderArchive = Join-Path $Downloads 'toolbox-windows-amd64-missing-reader.zip'
    Compress-Archive -Path (Join-Path $MissingReaderPayload '*') -DestinationPath $MissingReaderArchive
    $MissingReaderDigest = (Get-FileHash -LiteralPath $MissingReaderArchive -Algorithm SHA256).Hash.ToLowerInvariant()
    Set-Content -LiteralPath "$MissingReaderArchive.sha256" -Value "$MissingReaderDigest  toolbox-windows-amd64.zip" -Encoding ascii
    $ReaderArchives = @{ missing = $MissingReaderArchive }
    foreach ($LinkCase in @('symlink', 'directory-symlink')) {
        $LinkArchive = Join-Path $Downloads "toolbox-windows-amd64-$LinkCase.zip"
        Copy-Item -LiteralPath $Archive -Destination $LinkArchive
        $Zip = [IO.Compression.ZipFile]::Open($LinkArchive, [IO.Compression.ZipArchiveMode]::Update)
        try {
            foreach ($Entry in @($Zip.Entries | Where-Object { $_.FullName.Replace('\', '/').StartsWith('libexec/') })) {
                $Entry.Delete()
            }
            $TargetName = if ($LinkCase -eq 'symlink') { 'reader-target.exe' } else { 'reader-target/tb-markdown-reader.exe' }
            $Writer = [IO.StreamWriter]::new($Zip.CreateEntry($TargetName).Open())
            try { $Writer.Write('reader fixture') } finally { $Writer.Dispose() }
            $LinkName = if ($LinkCase -eq 'symlink') { 'libexec/tb-markdown-reader.exe' } else { 'libexec' }
            $LinkTarget = if ($LinkCase -eq 'symlink') { '../reader-target.exe' } else { 'reader-target' }
            $LinkEntry = $Zip.CreateEntry($LinkName)
            $LinkEntry.ExternalAttributes = -1610612736 # Unix symbolic-link type (0xA000 << 16).
            $Writer = [IO.StreamWriter]::new($LinkEntry.Open())
            try { $Writer.Write($LinkTarget) } finally { $Writer.Dispose() }
        } finally {
            $Zip.Dispose()
        }
        $LinkDigest = (Get-FileHash -LiteralPath $LinkArchive -Algorithm SHA256).Hash.ToLowerInvariant()
        Set-Content -LiteralPath "$LinkArchive.sha256" -Value "$LinkDigest  toolbox-windows-amd64.zip" -Encoding ascii
        $ReaderArchives[$LinkCase] = $LinkArchive
    }
    foreach ($ReaderCase in @('invalid-executable', 'missing', 'failed', 'validator-error', 'symlink', 'directory-symlink')) {
        foreach ($InstallationState in @('fresh', 'existing')) {
            $CaseRoot = Join-Path $TestRoot "reader-$ReaderCase-$InstallationState"
            $env:LOCALAPPDATA = Join-Path $CaseRoot 'localappdata'
            $global:ToolboxInstallerTestDocuments = Join-Path $CaseRoot 'Documents'
            New-Item -ItemType Directory -Path $env:LOCALAPPDATA, $global:ToolboxInstallerTestDocuments | Out-Null
            $Data = Join-Path $env:LOCALAPPDATA 'my-toolbox'
            if ($InstallationState -eq 'existing') {
                # Without current.txt, bootstrap can stage alongside an older
                # version, wrapper, completions, and profiles.
                foreach ($RelativePath in @('versions\0.1.4\tb.exe', 'bin\tb.cmd', 'completions\tb.ps1')) {
                    $ExistingPath = Join-Path $Data $RelativePath
                    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $ExistingPath) | Out-Null
                    Set-Content -LiteralPath $ExistingPath -Value "original $RelativePath" -Encoding ascii
                }
                foreach ($Profile in @('WindowsPowerShell\profile.ps1', 'PowerShell\profile.ps1')) {
                    $ExistingPath = Join-Path $global:ToolboxInstallerTestDocuments $Profile
                    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $ExistingPath) | Out-Null
                    Set-Content -LiteralPath $ExistingPath -Value '# original profile' -Encoding ascii
                }
                $Before = Get-InstallerSnapshot $CaseRoot
            }
            $env:PATH = 'C:\Windows\System32'
            $global:ToolboxInstallerTestUserPath = 'C:\Persisted\Reader'
            $global:ToolboxInstallerFixtureArchive = if ($ReaderArchives.ContainsKey($ReaderCase)) { $ReaderArchives[$ReaderCase] } else { $Archive }
            $ChecksBeforeFailure = $global:ToolboxInstallerReaderChecks.Count
            $ReaderParameters = @{}
            if ($ReaderCase -eq 'invalid-executable') {
                $ReaderParameters.UseProductionReaderValidator = $true
                $global:LASTEXITCODE = 0
            } else {
                $ReaderParameters.ReaderValidator = {
                    param([string]$ReaderPath, [string[]]$ReaderArguments)
                    Assert-StagedReaderInvocation $ReaderPath $ReaderArguments
                    if ($ReaderCase -eq 'validator-error') { throw 'Injected reader startup failure.' }
                    if ($ReaderCase -in @('symlink', 'directory-symlink')) { return 0 }
                    return 23
                }
            }
            $Failure = ''
            try {
                Invoke-TestInstaller @ReaderParameters -UserPathWriter { param([string]$Value) throw 'Validation reached PATH publication.' } | Out-Null
            } catch {
                $Failure = $_.Exception.Message
            }
            if (-not $Failure.Contains('[FAIL] Stage 5/7: extraction/validation')) {
                throw "$ReaderCase reader did not fail staging for $InstallationState installation. Error: $Failure"
            }
            if ($ReaderCase -eq 'missing' -and -not $Failure.Contains('libexec\tb-markdown-reader.exe')) {
                throw "Missing reader failure did not identify its path. Error: $Failure"
            }
            if ($ReaderCase -in @('symlink', 'directory-symlink') -and -not $Failure.Contains('unsafe bundled reader path')) {
                throw "Reader link did not fail the safety check explicitly. Error: $Failure"
            }
            $ExpectedChecks = if ($ReaderCase -in @('failed', 'validator-error')) { 1 } else { 0 }
            if ($global:ToolboxInstallerReaderChecks.Count -ne ($ChecksBeforeFailure + $ExpectedChecks)) {
                throw "$ReaderCase invoked reader validation an unexpected number of times."
            }
            if ((Test-Path -LiteralPath (Join-Path $Data 'current.txt')) -or (Test-Path -LiteralPath (Join-Path $Data 'versions\0.1.5')) -or
                @(Get-ChildItem -LiteralPath (Join-Path $Data 'versions') -Force -Filter '.install-*').Count -ne 0) {
                throw 'Reader validation failure left publication or staging files.'
            }
            if ($InstallationState -eq 'existing') {
                if ((Get-InstallerSnapshot $CaseRoot) -cne $Before) {
                    throw 'Reader validation failure changed existing installation or profile files.'
                }
            } elseif ((Test-Path -LiteralPath (Join-Path $Data 'bin')) -or (Test-Path -LiteralPath (Join-Path $Data 'completions')) -or
                @(Get-ChildItem -LiteralPath $global:ToolboxInstallerTestDocuments -Recurse -Force).Count -ne 0) {
                throw 'Fresh reader validation failure published wrapper, completion, or profile files.'
            }
            if ($env:PATH -cne 'C:\Windows\System32' -or $global:ToolboxInstallerTestUserPath -cne 'C:\Persisted\Reader') {
                throw 'Reader validation failure changed process or user PATH.'
            }
        }
    }
    $global:ToolboxInstallerFixtureArchive = $Archive

    foreach ($UpdateCase in @('failed', 'missing', 'mismatch', 'missing-expected', 'completion', 'status', 'current-locked', 'same', 'success', 'foreign-version')) {
        $CaseRoot = Join-Path $TestRoot "update-$UpdateCase"
        $env:LOCALAPPDATA = Join-Path $CaseRoot 'localappdata'
        $global:ToolboxInstallerTestDocuments = Join-Path $CaseRoot 'Documents'
        $Data = Join-Path $env:LOCALAPPDATA 'my-toolbox'
        $OldVersion = Join-Path $Data 'versions\0.1.4'
        New-Item -ItemType Directory -Force -Path (Split-Path -Parent $OldVersion), (Join-Path $Data 'bin'), $global:ToolboxInstallerTestDocuments | Out-Null
        Copy-Item -LiteralPath $Payload -Destination $OldVersion -Recurse
        Set-Content -LiteralPath (Join-Path $OldVersion 'version.txt') -Value '0.1.4' -Encoding ascii
        Set-Content -LiteralPath (Join-Path $Data 'current.txt') -Value '0.1.4' -Encoding ascii
        Set-Content -LiteralPath (Join-Path $Data 'bin\tb.cmd') -Value 'old wrapper' -Encoding ascii
        Copy-Item -LiteralPath (Join-Path $Payload 'completions') -Destination (Join-Path $Data 'completions') -Recurse
        if ($UpdateCase -eq 'completion') {
            New-Item -ItemType Directory -Path (Join-Path $global:ToolboxInstallerTestDocuments 'PowerShell') | Out-Null
            Set-Content -LiteralPath (Join-Path $global:ToolboxInstallerTestDocuments 'PowerShell\profile.ps1') -Value '# >>> my-toolbox completion >>>' -Encoding ascii
        }
        $Before = Get-InstallerSnapshot $CaseRoot
        $OldVersionBefore = Get-InstallerSnapshot $OldVersion
        $WrapperBefore = [IO.File]::ReadAllText((Join-Path $Data 'bin\tb.cmd'))
        $CurrentBefore = [IO.File]::ReadAllText((Join-Path $Data 'current.txt'))
        $env:PATH = 'C:\Windows\System32'
        $global:ToolboxInstallerTestUserPath = 'C:\Persisted\Update'
        $env:TOOLBOX_UPDATE = '1'
        $env:TOOLBOX_EXPECTED_VERSION = switch ($UpdateCase) {
            'same' { '0.1.4' }
            'mismatch' { '0.1.6' }
            'missing-expected' { '' }
            default { '0.1.5' }
        }
        $global:ToolboxInstallerFixtureArchive = if ($UpdateCase -eq 'missing') { $MissingReaderArchive } else { $Archive }
        $UpdateChecks = [Collections.Generic.List[string]]::new()
        $CurrentLock = $null
        if ($UpdateCase -eq 'current-locked') {
            $CurrentLock = [IO.File]::Open((Join-Path $Data 'current.txt'), [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::Read)
        }
        $Failure = ''
        try {
            Invoke-TestInstaller -UserPathWriter { throw 'Update unexpectedly changed user PATH.' } -ReaderValidator {
                param([string]$ReaderPath, [string[]]$ReaderArguments)
                if ($ReaderPath -notlike (Join-Path $Data 'versions\.install-0.1.5-*\libexec\tb-markdown-reader.exe') -or
                    $ReaderArguments.Count -ne 1 -or $ReaderArguments[0] -cne '--tb-self-check') {
                    throw 'Update did not validate the staged reader.'
                }
                if ((Get-InstallerSnapshot $OldVersion) -cne $OldVersionBefore -or
                    [IO.File]::ReadAllText((Join-Path $Data 'current.txt')) -cne $CurrentBefore -or
                    [IO.File]::ReadAllText((Join-Path $Data 'bin\tb.cmd')) -cne $WrapperBefore -or
                    (Test-Path -LiteralPath (Join-Path $Data 'versions\0.1.5'))) {
                    throw 'Update changed the active installation before reader validation.'
                }
                $UpdateChecks.Add($ReaderPath)
                if ($UpdateCase -eq 'failed') { return 23 }
                if ($UpdateCase -eq 'foreign-version') {
                    Copy-Item -LiteralPath $Payload -Destination (Join-Path $Data 'versions\0.1.5') -Recurse
                    Set-Content -LiteralPath (Join-Path $Data 'current.txt') -Value '0.1.5' -Encoding ascii
                }
                return 0
            } | Out-Null
        } catch {
            $Failure = $_.Exception.Message
        } finally {
            if ($null -ne $CurrentLock) { $CurrentLock.Dispose() }
        }
        if ($UpdateCase -eq 'foreign-version') {
            if (-not $Failure.Contains('[FAIL] Stage 6/7:') -or
                -not (Test-Path -LiteralPath (Join-Path $Data 'versions\0.1.5\tb.exe')) -or
                (Get-Content -LiteralPath (Join-Path $Data 'current.txt') -Raw).Trim() -cne '0.1.5') {
                throw "Rollback deleted another transaction's active binary: $Failure"
            }
        } elseif ($UpdateCase -in @('success', 'same')) {
            if ($Failure.Length -gt 0) { throw "Update $UpdateCase failed: $Failure" }
            if ($UpdateCase -eq 'same') {
                if ((Get-InstallerSnapshot $CaseRoot) -cne $Before -or $UpdateChecks.Count -ne 0) {
                    throw 'Same-version update changed the installation.'
                }
            } elseif ($UpdateChecks.Count -ne 1 -or
                (Get-Content -LiteralPath (Join-Path $Data 'current.txt') -Raw).Trim() -cne '0.1.5' -or
                -not (Test-Path -LiteralPath (Join-Path $Data 'versions\0.1.5\libexec\tb-markdown-reader.exe')) -or
                (Get-InstallerSnapshot $OldVersion) -cne $OldVersionBefore -or
                [IO.File]::ReadAllText((Join-Path $Data 'bin\tb.cmd')) -cne $WrapperBefore) {
                throw 'Successful update did not validate, switch current, and retain the previous version and wrapper.'
            }
        } else {
            $Stage = switch ($UpdateCase) { 'mismatch' { 2 }; 'missing-expected' { 1 }; { $_ -in @('completion', 'status', 'current-locked') } { 7 }; default { 5 } }
            if (-not $Failure.Contains("[FAIL] Stage $Stage/7:") -or (Get-InstallerSnapshot $CaseRoot) -cne $Before) {
                throw "Update $UpdateCase did not preserve the installation at stage $Stage. Error: $Failure"
            }
        }
        if ($env:PATH -cne 'C:\Windows\System32' -or $global:ToolboxInstallerTestUserPath -cne 'C:\Persisted\Update' -or
            @(Get-ChildItem -LiteralPath (Join-Path $Data 'versions') -Force -Filter '.install-*').Count -ne 0) {
            throw 'Update changed PATH or leaked staging files.'
        }
    }
    $UpdateCase = ''
    $ConcurrentRoot = Join-Path $TestRoot 'concurrent-update'
    $env:LOCALAPPDATA = Join-Path $ConcurrentRoot 'localappdata'
    $Data = Join-Path $env:LOCALAPPDATA 'my-toolbox'
    $OldVersion = Join-Path $Data 'versions\0.1.4'
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $OldVersion), (Join-Path $Data 'bin'), (Join-Path $ConcurrentRoot 'Documents') | Out-Null
    Copy-Item -LiteralPath $Payload -Destination $OldVersion -Recurse
    Set-Content -LiteralPath (Join-Path $Data 'current.txt') -Value '0.1.4' -Encoding ascii
    Set-Content -LiteralPath (Join-Path $Data 'bin\tb.cmd') -Value 'old wrapper' -Encoding ascii
    $RunConcurrentInstaller = {
        param([string]$Installer, [string]$CaseRoot, [string]$FixtureArchive, [string]$Name)
        $ErrorActionPreference = 'Stop'
        $env:LOCALAPPDATA = Join-Path $CaseRoot 'localappdata'
        $env:TOOLBOX_UPDATE = '1'
        $env:TOOLBOX_EXPECTED_VERSION = '0.1.5'
        function Invoke-RestMethod { param([string]$Uri) return [pscustomobject]@{ tag_name = 'v0.1.5' } }
        function Invoke-WebRequest {
            param([string]$Uri, [string]$OutFile)
            $Source = if ($Uri.EndsWith('.sha256')) { "$FixtureArchive.sha256" } else { $FixtureArchive }
            Copy-Item -LiteralPath $Source -Destination $OutFile
        }
        try {
            & $Installer -UserPathReader { '' } -UserPathWriter { throw 'Update changed user PATH.' } `
                -DocumentsPathReader { Join-Path $CaseRoot 'Documents' } -ReaderValidator {
                    param([string]$ReaderPath, [string[]]$ReaderArguments)
                    Set-Content -LiteralPath (Join-Path $CaseRoot "$Name.reader") -Value $ReaderPath
                    if ($Name -eq 'first') {
                        while (-not (Test-Path -LiteralPath (Join-Path $CaseRoot 'release'))) { Start-Sleep -Milliseconds 20 }
                    }
                    return 0
                } | Out-Null
            [pscustomobject]@{ Succeeded = $true; Failure = '' }
        } catch {
            [pscustomobject]@{ Succeeded = $false; Failure = $_.Exception.Message }
        }
    }
    $First = Start-Job -ScriptBlock $RunConcurrentInstaller -ArgumentList $Installer, $ConcurrentRoot, $Archive, 'first'
    $Second = $null
    try {
        $Deadline = [DateTime]::UtcNow.AddSeconds(30)
        while (-not (Test-Path -LiteralPath (Join-Path $ConcurrentRoot 'first.reader')) -and
            $First.State -eq 'Running' -and [DateTime]::UtcNow -lt $Deadline) { Start-Sleep -Milliseconds 20 }
        if (-not (Test-Path -LiteralPath (Join-Path $ConcurrentRoot 'first.reader'))) {
            throw "First concurrent updater did not reach validation: $(Receive-Job $First)"
        }
        $Second = Start-Job -ScriptBlock $RunConcurrentInstaller -ArgumentList $Installer, $ConcurrentRoot, $Archive, 'second'
        if ($null -eq (Wait-Job $Second -Timeout 30)) { throw 'Concurrent updater did not fail promptly.' }
        $SecondResult = Receive-Job $Second
        if ($SecondResult.Succeeded -or (Test-Path -LiteralPath (Join-Path $ConcurrentRoot 'second.reader'))) {
            throw 'Concurrent updater entered the locked transaction.'
        }
        Set-Content -LiteralPath (Join-Path $ConcurrentRoot 'release') -Value ''
        if ($null -eq (Wait-Job $First -Timeout 30)) { throw 'Active updater did not finish.' }
        $FirstResult = Receive-Job $First
        $Current = (Get-Content -LiteralPath (Join-Path $Data 'current.txt') -Raw).Trim()
        if (-not $FirstResult.Succeeded -or $Current -cne '0.1.5' -or
            -not (Test-Path -LiteralPath (Join-Path $Data "versions\$Current\tb.exe"))) {
            throw "Concurrent update left current pointing to a missing binary: $($FirstResult.Failure)"
        }
    } finally {
        Set-Content -LiteralPath (Join-Path $ConcurrentRoot 'release') -Value ''
        @($First, $Second) | Where-Object { $null -ne $_ } | Stop-Job
        @($First, $Second) | Where-Object { $null -ne $_ } | Remove-Job
    }
    $env:TOOLBOX_UPDATE = $null
    $env:TOOLBOX_EXPECTED_VERSION = $null
    $global:ToolboxInstallerFixtureArchive = $Archive

    $env:LOCALAPPDATA = ''
    $env:PROCESSOR_ARCHITECTURE = 'ARM64'
    $env:PROCESSOR_ARCHITEW6432 = ''
    $Failure = ''
    try {
        & $Installer `
            -UserPathReader { '' } `
            -UserPathWriter { param([string]$Value) } `
            -DocumentsPathReader { '' } `
            -CommandReader { param([string]$Name) return $null }
    } catch {
        $Failure = $_.Exception.Message
    }
    foreach ($Text in @(
        '[FAIL] Stage 1/7: prerequisites',
        'Missing PowerShell capabilities: Invoke-RestMethod, Invoke-WebRequest, Get-FileHash.',
        'Install Windows PowerShell 5.1 or PowerShell 7 to provide the missing capabilities.',
        'my-toolbox requires 64-bit Windows on x64.',
        'LOCALAPPDATA is not set to an absolute path.',
        'The current user Documents known folder could not be resolved.'
    )) {
        if (-not $Failure.Contains($Text)) {
            throw "Aggregated Windows prerequisite report is missing '$Text'. Error: $Failure"
        }
    }
    if (Test-Path -LiteralPath (Join-Path $TestRoot 'my-toolbox')) {
        throw 'Windows Stage 1 failure created toolbox files.'
    }
    $env:PROCESSOR_ARCHITECTURE = $OriginalProcessorArchitecture
    $env:PROCESSOR_ARCHITEW6432 = $OriginalProcessorArchitectureW6432

    $NestedLocalAppData = Join-Path $TestRoot 'nested-localappdata'
    $NestedDocuments = Join-Path $TestRoot 'nested-documents'
    New-Item -ItemType Directory -Path (Join-Path $NestedLocalAppData 'my-toolbox'), $NestedDocuments | Out-Null
    Set-Content -LiteralPath (Join-Path $NestedLocalAppData 'my-toolbox\versions') -Value 'type conflict' -Encoding ascii
    New-Item -ItemType Directory -Path (Join-Path $NestedDocuments 'PowerShell\profile.ps1') | Out-Null
    $env:LOCALAPPDATA = $NestedLocalAppData
    $Failure = ''
    try {
        & $Installer `
            -UserPathReader { '' } `
            -UserPathWriter { param([string]$Value) } `
            -DocumentsPathReader { $NestedDocuments } `
            -CommandReader { param([string]$Name) return [pscustomobject]@{ Source = $Name } }
    } catch {
        $Failure = $_.Exception.Message
    }
    foreach ($Text in @(
        "Toolbox versions path is not writable: $(Join-Path $NestedLocalAppData 'my-toolbox\versions')",
        "PowerShell profile has unsupported type: $(Join-Path $NestedDocuments 'PowerShell\profile.ps1')"
    )) {
        if (-not $Failure.Contains($Text)) {
            throw "Nested Windows prerequisite report is missing '$Text'. Error: $Failure"
        }
    }
    if (Test-Path -LiteralPath (Join-Path $NestedLocalAppData 'my-toolbox\current.txt')) {
        throw 'Nested Windows Stage 1 failure created activation files.'
    }

    $env:LOCALAPPDATA = Join-Path $TestRoot 'localappdata'
    $env:PATH = 'C:\Windows\System32'
    $env:PATHEXT = '.COM;.EXE;.BAT;.CMD'
    $global:ToolboxInstallerTestDocuments = $Documents
    $WindowsPowerShellProfile = Join-Path $Documents 'WindowsPowerShell\profile.ps1'
    $PowerShellProfile = Join-Path $Documents 'PowerShell\profile.ps1'
    New-Item -ItemType Directory -Path (Split-Path -Parent $WindowsPowerShellProfile) | Out-Null
    # Construct the non-ASCII text without a source-file literal so Windows
    # PowerShell 5.1 cannot decode the test fixture itself through its legacy code page.
    $UnrelatedProfileText = '$caf' + [char]0x00E9 + " = 'unrelated'"
    $UnrelatedProfileBytes = ([Text.UTF8Encoding]::new($false)).GetBytes($UnrelatedProfileText)
    [IO.File]::WriteAllBytes($WindowsPowerShellProfile, $UnrelatedProfileBytes)
    $global:ToolboxInstallerTestUserPath = 'C:\Persisted\One;;C:\Persisted\Two'
    $PathWriter = { param([string]$Value) $global:ToolboxInstallerTestUserPath = $Value }
    $ChecksBeforeSuccess = $global:ToolboxInstallerReaderChecks.Count
    $Output = (Invoke-TestInstaller -UserPathWriter $PathWriter *>&1 | Out-String)
    if ($global:ToolboxInstallerReaderChecks.Count -ne ($ChecksBeforeSuccess + 1) -or
        -not (Test-Path -LiteralPath (Join-Path $env:LOCALAPPDATA 'my-toolbox\versions\0.1.5\libexec\tb-markdown-reader.exe'))) {
        throw 'Successful installation did not validate and publish the bundled reader.'
    }
    $ExpectedBannerLines = @(
        '###>   ###>##>   ##>    ########> ######>  ######> ##>     ######>  ######> ##>  ##>',
        '####> ####|<##> ##+]    <==##+==]##+===##>##+===##>##|     ##+==##>##+===##><##>##+]',
        '##+####+##| <####+]        ##|   ##|   ##|##|   ##|##|     ######+]##|   ##| <###+]',
        '##|<##+]##|  <##+]         ##|   ##|   ##|##|   ##|##|     ##+==##>##|   ##| ##+##>',
        '##| <=] ##|   ##|          ##|   <######+]<######+]#######>######+]<######+]##+] ##>',
        '<=]     <=]   <=]          <=]    <=====]  <=====] <======]<=====]  <=====] <=]  <=]'
    ) | ForEach-Object {
        $_.Replace('#', [char]0x2588).Replace('>', [char]0x2557).Replace('<', [char]0x255A).
            Replace('|', [char]0x2551).Replace('+', [char]0x2554).Replace(']', [char]0x255D).
            Replace('=', [char]0x2550)
    }
    foreach ($Text in $ExpectedBannerLines + @(
        '[INFO] Stage 1/7: prerequisites',
        '[INFO] Stage 2/7: release lookup',
        '[INFO] Stage 3/7: download',
        '[INFO] Stage 4/7: checksum',
        '[INFO] Stage 5/7: extraction/validation',
        '[INFO] Stage 6/7: installation',
        '[INFO] Stage 7/7: activation',
        '[OK] Stage 7/7: activation'
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
    $InstalledArticles = Join-Path $env:LOCALAPPDATA 'my-toolbox\versions\0.1.5\packages\search\articles'
    if (-not (Test-Path -LiteralPath $InstalledArticles -PathType Container)) {
        throw 'Installer did not install the article library.'
    }
    foreach ($Completion in @('_tb', 'tb.bash', 'tb.ps1')) {
        $ExpectedCompletion = Join-Path $RepositoryRoot "completions\$Completion"
        $InstalledCompletion = Join-Path $env:LOCALAPPDATA "my-toolbox\completions\$Completion"
        if (-not [Collections.StructuralComparisons]::StructuralEqualityComparer.Equals([IO.File]::ReadAllBytes($ExpectedCompletion), [IO.File]::ReadAllBytes($InstalledCompletion))) {
            throw "Installer did not publish completion asset $Completion."
        }
    }
    $ManagedBlock = "# >>> my-toolbox completion >>>`r`n. (Join-Path `$env:LOCALAPPDATA 'my-toolbox\completions\tb.ps1')`r`n# <<< my-toolbox completion <<<`r`n"
    $ExpectedWindowsPowerShellProfile = "$UnrelatedProfileText`r`n$ManagedBlock"
    $ExpectedPowerShellProfile = $ManagedBlock
    if ([IO.File]::ReadAllText($WindowsPowerShellProfile) -cne $ExpectedWindowsPowerShellProfile) {
        throw 'Installer did not preserve and activate the Windows PowerShell profile exactly.'
    }
    if ([IO.File]::ReadAllText($PowerShellProfile) -cne $ExpectedPowerShellProfile) {
        throw 'Installer did not preserve and activate the PowerShell profile exactly.'
    }
    $PublishedProfileBytes = [IO.File]::ReadAllBytes($WindowsPowerShellProfile)
    if ($PublishedProfileBytes.Length -lt 3 -or $PublishedProfileBytes[0] -ne 0xEF -or $PublishedProfileBytes[1] -ne 0xBB -or $PublishedProfileBytes[2] -ne 0xBF) {
        throw 'Installer did not make the BOM-less UTF-8 profile compatible with Windows PowerShell 5.1.'
    }
    for ($Index = 0; $Index -lt $UnrelatedProfileBytes.Length; $Index++) {
        if ($PublishedProfileBytes[$Index + 3] -ne $UnrelatedProfileBytes[$Index]) {
            throw 'Installer changed unrelated BOM-less UTF-8 profile bytes.'
        }
    }
    $ProfileTokens = $null
    $ProfileErrors = $null
    [void][Management.Automation.Language.Parser]::ParseFile($WindowsPowerShellProfile, [ref]$ProfileTokens, [ref]$ProfileErrors)
    if ($ProfileErrors.Count -gt 0) {
        throw "Installed Windows PowerShell profile does not parse: $ProfileErrors"
    }
    $WrapperRoot = Join-Path $env:LOCALAPPDATA 'my-toolbox\bin'
    $ExpectedUserPath = "C:\Persisted\One;;C:\Persisted\Two;$WrapperRoot"
    if ($global:ToolboxInstallerTestUserPath -cne $ExpectedUserPath) {
        throw "Installer user PATH = '$global:ToolboxInstallerTestUserPath', want '$ExpectedUserPath'."
    }
    $ExpectedProcessPath = "C:\Windows\System32;$WrapperRoot"
    if ($env:PATH -cne $ExpectedProcessPath) {
        throw "Installer process PATH = '$env:PATH', want '$ExpectedProcessPath'."
    }
    if ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT) {
        $InstalledCommand = Get-Command tb -ErrorAction Stop
        if (-not [string]::Equals($InstalledCommand.Source, (Join-Path $WrapperRoot 'tb.cmd'), [StringComparison]::OrdinalIgnoreCase)) {
            throw "Get-Command tb resolved '$($InstalledCommand.Source)'."
        }
    }

    # The active-pointer shortcut preserves an already complete installation,
    # even when a downloaded reader would fail. Update staging is a later task.
    $CurrentBefore = [IO.File]::ReadAllText((Join-Path $env:LOCALAPPDATA 'my-toolbox\current.txt'))
    $InstallationBefore = Get-InstallerSnapshot $env:LOCALAPPDATA
    $ProfilesBefore = Get-InstallerSnapshot $Documents
    Invoke-TestInstaller -UserPathWriter $PathWriter -ReaderValidator { throw 'Active install unexpectedly validated a new reader.' } | Out-Null
    if ([IO.File]::ReadAllText((Join-Path $env:LOCALAPPDATA 'my-toolbox\current.txt')) -cne $CurrentBefore -or
        (Get-InstallerSnapshot $env:LOCALAPPDATA) -cne $InstallationBefore -or (Get-InstallerSnapshot $Documents) -cne $ProfilesBefore -or
        $env:PATH -cne $ExpectedProcessPath -or $global:ToolboxInstallerTestUserPath -cne $ExpectedUserPath) {
        throw 'Active installation shortcut changed existing installation, current pointer, profiles, or PATH.'
    }

    $env:PATH = 'C:\Windows\System32'
    $global:ToolboxInstallerTestUserPath = 'C:\Persisted\Repair'
    Remove-Item -LiteralPath (Join-Path $env:LOCALAPPDATA 'my-toolbox\completions') -Recurse -Force
    [IO.File]::WriteAllBytes($WindowsPowerShellProfile, $UnrelatedProfileBytes)
    Remove-Item -LiteralPath $PowerShellProfile -Force
    $RepairOutput = (Invoke-TestInstaller -UserPathWriter $PathWriter *>&1 | Out-String)
    if (-not $RepairOutput.Contains('is already installed')) {
        throw "Existing installation did not take the repair path. Output: $RepairOutput"
    }
    if ($global:ToolboxInstallerTestUserPath -cne "C:\Persisted\Repair;$WrapperRoot") {
        throw "Existing installation did not repair user PATH: '$global:ToolboxInstallerTestUserPath'."
    }
    if ($env:PATH -cne "C:\Windows\System32;$WrapperRoot") {
        throw "Existing installation did not repair process PATH: '$env:PATH'."
    }
    if ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT -and $null -eq (Get-Command tb -ErrorAction SilentlyContinue)) {
        throw 'Get-Command tb did not resolve after repairing an existing installation.'
    }
    if ([IO.File]::ReadAllText($WindowsPowerShellProfile) -cne $ExpectedWindowsPowerShellProfile -or [IO.File]::ReadAllText($PowerShellProfile) -cne $ExpectedPowerShellProfile) {
        throw 'Existing installation did not repair PowerShell completion activation.'
    }
    foreach ($Completion in @('_tb', 'tb.bash', 'tb.ps1')) {
        if (-not (Test-Path -LiteralPath (Join-Path $env:LOCALAPPDATA "my-toolbox\completions\$Completion") -PathType Leaf)) {
            throw "Existing installation did not repair completion asset $Completion."
        }
    }
    foreach ($ProfilePath in @($WindowsPowerShellProfile, $PowerShellProfile)) {
        $ProfileText = [IO.File]::ReadAllText($ProfilePath)
        if ([regex]::Matches($ProfileText, [regex]::Escape('# >>> my-toolbox completion >>>')).Count -ne 1 -or [regex]::Matches($ProfileText, [regex]::Escape('# <<< my-toolbox completion <<<')).Count -ne 1) {
            throw "Installer duplicated a managed completion block in $ProfilePath."
        }
    }

    if ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT) {
        $QuotedVariant = '"' + $WrapperRoot.ToUpperInvariant() + '\"'
        $global:ToolboxInstallerTestUserPath = "C:\Before;$QuotedVariant;;C:\After"
        $env:PATH = "C:\Windows\System32;$($WrapperRoot.ToUpperInvariant())\"
        Invoke-TestInstaller -UserPathWriter $PathWriter | Out-Null
        if ($global:ToolboxInstallerTestUserPath -cne "C:\Before;$QuotedVariant;;C:\After") {
            throw "Installer duplicated or changed a user PATH variant: '$global:ToolboxInstallerTestUserPath'."
        }
        if ($env:PATH -cne "C:\Windows\System32;$($WrapperRoot.ToUpperInvariant())\") {
            throw "Installer duplicated or changed a process PATH variant: '$env:PATH'."
        }
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
    [IO.File]::WriteAllBytes($WindowsPowerShellProfile, $UnrelatedProfileBytes)
    Remove-Item -LiteralPath $PowerShellProfile -Force
    Remove-Item -LiteralPath (Split-Path -Parent $PowerShellProfile)
    $env:PATH = 'C:\Windows\System32'
    $global:ToolboxInstallerTestUserPath = 'C:\Persisted\Failure'
    $Failure = ''
    try {
        Invoke-TestInstaller -UserPathWriter { param([string]$Value) throw 'Injected user PATH persistence failure.' }
    } catch {
        $Failure = ($_ | Out-String)
    }
    if (-not $Failure.Contains('[FAIL] Stage 7/7: activation')) {
        throw "PATH persistence failure did not identify Stage 7. Error: $Failure"
    }
    if (Test-Path -LiteralPath (Join-Path $env:LOCALAPPDATA 'my-toolbox\versions\0.1.5')) {
        throw 'PATH persistence failure did not roll back the version directory.'
    }
    if (Test-Path -LiteralPath (Join-Path $env:LOCALAPPDATA 'my-toolbox\bin\tb.cmd')) {
        throw 'PATH persistence failure did not roll back the wrapper.'
    }
    if (Test-Path -LiteralPath (Join-Path $env:LOCALAPPDATA 'my-toolbox\current.txt')) {
        throw 'PATH persistence failure did not roll back activation.'
    }
    if ([IO.File]::ReadAllText($WindowsPowerShellProfile) -cne $UnrelatedProfileText -or (Test-Path -LiteralPath (Split-Path -Parent $PowerShellProfile))) {
        throw 'PATH persistence failure did not restore PowerShell profiles.'
    }
    if (Test-Path -LiteralPath (Join-Path $env:LOCALAPPDATA 'my-toolbox\completions')) {
        throw 'PATH persistence failure did not roll back completion assets.'
    }

    $MalformedProfile = "unrelated`r`n# >>> my-toolbox completion >>>`r`n"
    [IO.File]::WriteAllText($WindowsPowerShellProfile, $MalformedProfile, [Text.UTF8Encoding]::new($false))
    $Failure = ''
    try {
        Invoke-TestInstaller -UserPathWriter $PathWriter
    } catch {
        $Failure = ($_ | Out-String)
    }
    if (-not $Failure.Contains('Malformed my-toolbox completion markers')) {
        throw "Malformed completion markers did not fail explicitly. Error: $Failure"
    }
    if ([IO.File]::ReadAllText($WindowsPowerShellProfile) -cne $MalformedProfile) {
        throw 'Malformed-marker failure changed the Windows PowerShell profile.'
    }
    if (Test-Path -LiteralPath (Join-Path $env:LOCALAPPDATA 'my-toolbox\versions\0.1.5')) {
        throw 'Malformed-marker failure did not roll back the version directory.'
    }

    Set-Content -LiteralPath "$global:ToolboxInstallerFixtureArchive.sha256" -Value "$('0' * 64)  toolbox-windows-amd64.zip" -Encoding ascii
    $Failure = ''
    try {
        Invoke-TestInstaller -UserPathWriter $PathWriter
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

    $MissingPayload = Join-Path $TestRoot 'payload-without-articles'
    Copy-Item -LiteralPath $Payload -Destination $MissingPayload -Recurse
    Remove-Item -LiteralPath (Join-Path $MissingPayload 'packages\search\articles') -Recurse -Force
    $MissingArchive = Join-Path $Downloads 'toolbox-windows-amd64-missing-articles.zip'
    Compress-Archive -Path (Join-Path $MissingPayload '*') -DestinationPath $MissingArchive
    $MissingDigest = (Get-FileHash -LiteralPath $MissingArchive -Algorithm SHA256).Hash.ToLowerInvariant()
    Set-Content -LiteralPath "$MissingArchive.sha256" -Value "$MissingDigest  toolbox-windows-amd64.zip" -Encoding ascii
    $global:ToolboxInstallerFixtureArchive = $MissingArchive
    Remove-Item -LiteralPath (Join-Path $env:LOCALAPPDATA 'my-toolbox') -Recurse -Force -ErrorAction SilentlyContinue
    [IO.File]::WriteAllText($WindowsPowerShellProfile, $UnrelatedProfileText, [Text.UTF8Encoding]::new($false))
    $Failure = ''
    try {
        Invoke-TestInstaller -UserPathWriter $PathWriter
    } catch {
        $Failure = ($_ | Out-String)
    }
    if (-not $Failure.Contains('[FAIL] Stage 5/7: extraction/validation') -or -not $Failure.Contains('packages\search\articles')) {
        throw "Missing article library did not fail validation explicitly. Error: $Failure"
    }
} finally {
    $env:TOOLBOX_UPDATE = $OriginalUpdate
    $env:TOOLBOX_EXPECTED_VERSION = $OriginalExpectedVersion
    Remove-Variable -Name ToolboxInstallerFixtureArchive -Scope Global -ErrorAction SilentlyContinue
    Remove-Variable -Name ToolboxInstallerTestUserPath -Scope Global -ErrorAction SilentlyContinue
    Remove-Variable -Name ToolboxInstallerTestDocuments -Scope Global -ErrorAction SilentlyContinue
    Remove-Variable -Name ToolboxInstallerReaderChecks -Scope Global -ErrorAction SilentlyContinue
    $env:LOCALAPPDATA = $OriginalLocalAppData
    $env:PATH = $OriginalPath
    $env:PATHEXT = $OriginalPathExt
    $global:LASTEXITCODE = $OriginalLastExitCode
    $env:PROCESSOR_ARCHITECTURE = $OriginalProcessorArchitecture
    $env:PROCESSOR_ARCHITEW6432 = $OriginalProcessorArchitectureW6432
    if (Test-Path -LiteralPath $TestRoot) {
        Remove-Item -LiteralPath $TestRoot -Recurse -Force
    }
}
