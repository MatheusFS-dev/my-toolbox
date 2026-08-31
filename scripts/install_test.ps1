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
New-Item -ItemType Directory -Path $Payload, $Downloads, $Documents | Out-Null
$TemporaryBefore = @(
    Get-ChildItem ([IO.Path]::GetTempPath()) -Directory -Filter 'my-toolbox-*' |
        ForEach-Object FullName
)

try {
    Copy-Item -LiteralPath (Join-Path $RepositoryRoot 'commands.json') -Destination (Join-Path $Payload 'commands.json')
    Copy-Item -LiteralPath (Join-Path $RepositoryRoot 'completions') -Destination (Join-Path $Payload 'completions') -Recurse
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
            [scriptblock]$UserPathWriter
        )

        & $Installer -UserPathReader { $global:ToolboxInstallerTestUserPath } -UserPathWriter $UserPathWriter -DocumentsPathReader { $global:ToolboxInstallerTestDocuments }
    }

    $env:LOCALAPPDATA = Join-Path $TestRoot 'localappdata'
    $env:PATH = 'C:\Windows\System32'
    $env:PATHEXT = '.COM;.EXE;.BAT;.CMD'
    $global:ToolboxInstallerTestDocuments = $Documents
    $WindowsPowerShellProfile = Join-Path $Documents 'WindowsPowerShell\profile.ps1'
    $PowerShellProfile = Join-Path $Documents 'PowerShell\profile.ps1'
    New-Item -ItemType Directory -Path (Split-Path -Parent $WindowsPowerShellProfile) | Out-Null
    [IO.File]::WriteAllText($WindowsPowerShellProfile, "`$café = 'unrelated'", [Text.UTF8Encoding]::new($false))
    $global:ToolboxInstallerTestUserPath = 'C:\Persisted\One;;C:\Persisted\Two'
    $PathWriter = { param([string]$Value) $global:ToolboxInstallerTestUserPath = $Value }
    $Output = (Invoke-TestInstaller -UserPathWriter $PathWriter *>&1 | Out-String)
    foreach ($Text in @(
        ' __  __ __   __  _____ ___   ___  _     ____   _____  __',
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
    foreach ($Completion in @('_tb', 'tb.bash', 'tb.ps1')) {
        $ExpectedCompletion = Join-Path $RepositoryRoot "completions\$Completion"
        $InstalledCompletion = Join-Path $env:LOCALAPPDATA "my-toolbox\completions\$Completion"
        if (-not [Collections.StructuralComparisons]::StructuralEqualityComparer.Equals([IO.File]::ReadAllBytes($ExpectedCompletion), [IO.File]::ReadAllBytes($InstalledCompletion))) {
            throw "Installer did not publish completion asset $Completion."
        }
    }
    $ManagedBlock = "# >>> my-toolbox completion >>>`r`n. (Join-Path `$env:LOCALAPPDATA 'my-toolbox\completions\tb.ps1')`r`n# <<< my-toolbox completion <<<`r`n"
    $ExpectedWindowsPowerShellProfile = "`$café = 'unrelated'`r`n$ManagedBlock"
    $ExpectedPowerShellProfile = $ManagedBlock
    if ([IO.File]::ReadAllText($WindowsPowerShellProfile) -cne $ExpectedWindowsPowerShellProfile) {
        throw 'Installer did not preserve and activate the Windows PowerShell profile exactly.'
    }
    if ([IO.File]::ReadAllText($PowerShellProfile) -cne $ExpectedPowerShellProfile) {
        throw 'Installer did not preserve and activate the PowerShell profile exactly.'
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

    $env:PATH = 'C:\Windows\System32'
    $global:ToolboxInstallerTestUserPath = 'C:\Persisted\Repair'
    Remove-Item -LiteralPath (Join-Path $env:LOCALAPPDATA 'my-toolbox\completions') -Recurse -Force
    [IO.File]::WriteAllText($WindowsPowerShellProfile, "`$café = 'unrelated'", [Text.UTF8Encoding]::new($false))
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
    [IO.File]::WriteAllText($WindowsPowerShellProfile, "`$café = 'unrelated'", [Text.UTF8Encoding]::new($false))
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
    if ([IO.File]::ReadAllText($WindowsPowerShellProfile) -cne "`$café = 'unrelated'" -or (Test-Path -LiteralPath (Split-Path -Parent $PowerShellProfile))) {
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
} finally {
    Remove-Variable -Name ToolboxInstallerFixtureArchive -Scope Global -ErrorAction SilentlyContinue
    Remove-Variable -Name ToolboxInstallerTestUserPath -Scope Global -ErrorAction SilentlyContinue
    Remove-Variable -Name ToolboxInstallerTestDocuments -Scope Global -ErrorAction SilentlyContinue
    $env:LOCALAPPDATA = $OriginalLocalAppData
    $env:PATH = $OriginalPath
    $env:PATHEXT = $OriginalPathExt
    if (Test-Path -LiteralPath $TestRoot) {
        Remove-Item -LiteralPath $TestRoot -Recurse -Force
    }
}
