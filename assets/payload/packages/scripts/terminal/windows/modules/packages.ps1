Set-StrictMode -Version 2.0

function Get-WindowsPackagePlan {
    param([Parameter(Mandatory = $true)]$Selection)

    Write-Output 'Microsoft.WindowsTerminal'
    Write-Output 'Microsoft.PowerShell'
    if ($Selection.Starship) { Write-Output 'Starship.Starship' }
    if ($Selection.Zellij) { Write-Output 'Zellij.Zellij' }
    if ($Selection.Eza) { Write-Output 'eza-community.eza' }
    if ($Selection.Fzf) { Write-Output 'junegunn.fzf' }
}

function Invoke-WingetInstall {
    param([Parameter(Mandatory = $true)][string]$Id)
    & winget.exe install --id $Id --exact --source winget --accept-source-agreements --accept-package-agreements | Out-Host
    return $LASTEXITCODE
}

function Install-WindowsPackages {
    param(
        [Parameter(Mandatory = $true)][string[]]$PackageIds,
        [scriptblock]$WingetInvoker = ${function:Invoke-WingetInstall}
    )

    $results = [ordered]@{}
    foreach ($id in $PackageIds) {
        try {
            $exitCode = & $WingetInvoker $id
            if ([int]$exitCode -in @(0, -1978335189)) {
                $results[$id] = New-FeatureResult -Status Success -Message 'Installed or already current.'
            }
            else {
                $results[$id] = New-FeatureResult -Status Warning -Message "WinGet exited with status $exitCode."
            }
        }
        catch {
            $results[$id] = New-FeatureResult -Status Warning -Message $_.Exception.Message
        }
    }
    return $results
}

function Resolve-PowerShell7Command {
    return Get-Command pwsh.exe -ErrorAction SilentlyContinue
}

function Get-PowerShell7Installation {
    param(
        [string]$ProgramFilesPath = $env:ProgramFiles,
        [string]$LocalAppDataPath = $env:LOCALAPPDATA,
        [scriptblock]$CommandResolver = ${function:Resolve-PowerShell7Command}
    )

    $msiPath = Join-Path $ProgramFilesPath 'PowerShell\7\pwsh.exe'
    if ([IO.File]::Exists($msiPath)) {
        return [pscustomobject]@{
            Available           = $true
            ExecutablePath      = $msiPath
            TerminalCommandLine = '%ProgramFiles%\PowerShell\7\pwsh.exe'
        }
    }

    $storeAliasPath = Join-Path $LocalAppDataPath 'Microsoft\WindowsApps\pwsh.exe'
    if ([IO.File]::Exists($storeAliasPath)) {
        return [pscustomobject]@{
            Available           = $true
            ExecutablePath      = $storeAliasPath
            TerminalCommandLine = '%LOCALAPPDATA%\Microsoft\WindowsApps\pwsh.exe'
        }
    }

    $command = & $CommandResolver
    $resolvedPath = if ($command -is [string]) { $command } elseif ($command) { $command.Source } else { $null }
    if ($resolvedPath -and [IO.File]::Exists($resolvedPath)) {
        return [pscustomobject]@{
            Available           = $true
            ExecutablePath      = $resolvedPath
            TerminalCommandLine = 'pwsh.exe'
        }
    }

    return [pscustomobject]@{
        Available           = $false
        ExecutablePath      = $null
        TerminalCommandLine = $null
    }
}

function Invoke-PowerShell7Command {
    param([string]$Executable, [string]$Command)
    & $Executable -NoLogo -NoProfile -NonInteractive -Command $Command | Out-Host
    return $LASTEXITCODE
}

function Install-PSFzfForPowerShell7 {
    param(
        [string]$PowerShell7Path = (Join-Path $env:ProgramFiles 'PowerShell\7\pwsh.exe'),
        [scriptblock]$PowerShellInvoker = ${function:Invoke-PowerShell7Command}
    )

    if (-not [IO.File]::Exists($PowerShell7Path)) {
        return New-FeatureResult -Status Warning -Message 'PowerShell 7 is unavailable; PSFzf was not installed.'
    }
    $command = @'
$ErrorActionPreference = 'Stop'
if (Get-Command Install-PSResource -ErrorAction SilentlyContinue) {
    Install-PSResource PSFzf -Scope CurrentUser -TrustRepository -AcceptLicense -ErrorAction Stop
}
else {
    Install-Module PSFzf -Scope CurrentUser -Force -AllowClobber -ErrorAction Stop
}
'@
    $exitCode = & $PowerShellInvoker $PowerShell7Path $command
    if ([int]$exitCode -eq 0) {
        return New-FeatureResult -Status Success -Message 'PSFzf installed for PowerShell 7 at current-user scope.'
    }
    return New-FeatureResult -Status Warning -Message "PSFzf installation exited with status $exitCode."
}
