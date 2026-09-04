Set-StrictMode -Version 2.0

function Test-CommandOrPath {
    param([string[]]$Commands, [string[]]$Paths)
    foreach ($command in $Commands) {
        if (Get-Command $command -ErrorAction SilentlyContinue) { return $true }
    }
    foreach ($path in $Paths) {
        if ($path -and [IO.File]::Exists($path)) { return $true }
    }
    return $false
}

function Set-SummaryResult {
    param($Summary, [string]$Name, [string]$Status, [string]$Message)
    $Summary[$Name] = New-FeatureResult -Status $Status -Message $Message
}

function Write-WindowsSetupSummary {
    param($Summary, [AllowEmptyString()][string]$BackupPath)
    Write-Output ''
    Write-Output 'Windows terminal setup summary'
    Write-Output '------------------------------'
    foreach ($name in $Summary.Keys) {
        $result = $Summary[$name]
        Write-Output ('{0,-24} {1,-8} {2}' -f $name, $result.Status, $result.Message)
    }
    if ($BackupPath) { Write-Output "Backup: $BackupPath" }
    else { Write-Output 'Backup: unavailable (see warnings above)' }
    Write-Output ''
    Write-Output 'Restart Windows Terminal to load the new profile and PowerShell configuration.'
    Write-Output 'If needed, choose Windows Terminal as the OS default terminal application in Windows Terminal > Settings > Startup.'
}

function Resolve-PowerShell7Dependencies {
    param(
        [Parameter(Mandatory = $true)]$Selection,
        [Parameter(Mandatory = $true)]$Summary,
        [Parameter(Mandatory = $true)][bool]$PowerShell7Available
    )

    if ($PowerShell7Available) {
        return [pscustomobject]@{ ConfigureTerminal = $true; ConfigureProfile = $true }
    }

    $summaryNames = [ordered]@{
        Starship = 'Starship'
        Eza = 'eza'
        FzfTab = 'PSFzf Tab completion'
        WordNavigation = 'Word navigation'
        GitWrapper = 'Git wrapper'
        Venv = 'venv'
        AutoStartZellij = 'Zellij auto-start'
    }
    foreach ($feature in $summaryNames.Keys) {
        if (-not $Selection[$feature]) { continue }
        $Selection[$feature] = $false
        $summaryName = $summaryNames[$feature]
        if ($feature -in @('Starship', 'Eza') -and $Summary[$summaryName].Status -eq 'Success') {
            Set-SummaryResult $Summary $summaryName Warning 'Installed, but profile integration was skipped because PowerShell 7 is unavailable.'
        }
        else {
            Set-SummaryResult $Summary $summaryName Skipped 'PowerShell 7 is unavailable.'
        }
    }
    Set-SummaryResult $Summary 'PowerShell profile' Warning 'PowerShell 7 installation failed and pwsh.exe is unavailable.'
    return [pscustomobject]@{ ConfigureTerminal = $false; ConfigureProfile = $false }
}

function Invoke-WindowsTerminalSetup {
    param(
        [string[]]$Arguments = @(),
        [scriptblock]$Prompt = ${function:Read-YesNo},
        [scriptblock]$WingetInvoker = ${function:Invoke-WingetInstall}
    )

    $options = Parse-WindowsSetupArguments -Arguments $Arguments
    if ($options.ShowHelp) {
        Write-Output (Get-WindowsSetupUsage)
        return 0
    }

    $preflight = Test-WindowsSetupPreflight
    if (-not $preflight.Success) {
        foreach ($problem in $preflight.Problems) { [Console]::Error.WriteLine("Error: $problem") }
        return 1
    }

    $selection = Select-WindowsFeatures -Options $options -Prompt $Prompt
    $summary = [ordered]@{}
    foreach ($name in @(
        'Windows Terminal', 'PowerShell 7', 'FiraCode Nerd Font', 'Starship', 'Zellij',
        'Zellij clipboard', 'eza', 'fzf', 'PSFzf Tab completion', 'Word navigation',
        'Git wrapper', 'venv', 'VS Code Shift+Enter', 'Zellij auto-start', 'PowerShell profile'
    )) {
        Set-SummaryResult -Summary $summary -Name $name -Status Skipped -Message 'Not selected.'
    }

    Write-Output 'Installing mandatory infrastructure and selected tools with WinGet...'
    $packageIds = @(Get-WindowsPackagePlan -Selection $selection)
    $packageResults = Install-WindowsPackages -PackageIds $packageIds -WingetInvoker $WingetInvoker
    $packageSummaryNames = [ordered]@{
        'Microsoft.WindowsTerminal' = 'Windows Terminal'
        'Microsoft.PowerShell' = 'PowerShell 7'
        'Starship.Starship' = 'Starship'
        'Zellij.Zellij' = 'Zellij'
        'eza-community.eza' = 'eza'
        'junegunn.fzf' = 'fzf'
    }
    foreach ($id in $packageResults.Keys) {
        $summary[$packageSummaryNames[$id]] = $packageResults[$id]
    }

    $powerShell7Installation = Get-PowerShell7Installation
    $powerShell7Path = $powerShell7Installation.ExecutablePath
    $powerShell7Available = $powerShell7Installation.Available
    $powerShellDependencyState = Resolve-PowerShell7Dependencies -Selection $selection -Summary $summary -PowerShell7Available $powerShell7Available

    if ($selection.Font) { $summary['FiraCode Nerd Font'] = Install-FiraCodeNerdFont }

    if ($selection.ZellijCopy -and $summary['Zellij'].Status -ne 'Success') {
        Set-SummaryResult $summary 'Zellij clipboard' Skipped 'Zellij installation failed.'
        $selection.ZellijCopy = $false
    }
    if ($selection.AutoStartZellij -and $summary['Zellij'].Status -ne 'Success') {
        Set-SummaryResult $summary 'Zellij auto-start' Skipped 'Zellij installation failed.'
        $selection.AutoStartZellij = $false
    }
    if ($selection.FzfTab -and $summary['fzf'].Status -ne 'Success') {
        Set-SummaryResult $summary 'PSFzf Tab completion' Skipped 'fzf installation failed.'
        $selection.FzfTab = $false
    }
    if ($selection.Starship -and $summary['Starship'].Status -ne 'Success') { $selection.Starship = $false }
    if ($selection.Eza -and $summary['eza'].Status -ne 'Success') { $selection.Eza = $false }

    if ($selection.FzfTab) {
        $psfzf = Install-PSFzfForPowerShell7 -PowerShell7Path $powerShell7Path
        $summary['PSFzf Tab completion'] = $psfzf
        if ($psfzf.Status -ne 'Success') { $selection.FzfTab = $false }
    }

    $profilePath = Get-PowerShell7ProfilePath
    $starshipConfigPath = Get-StarshipConfigPath
    $terminalSettingsPath = Get-WindowsTerminalSettingsPath
    $terminalFragmentPath = Get-WindowsTerminalFragmentPath
    $zellijPath = Get-ZellijConfigPath
    $vscodePath = Get-VSCodeKeybindingsPath
    $backupPath = ''
    try {
        $backupPath = New-WindowsConfigBackup -PathMap ([ordered]@{
            'PowerShell/profile.ps1' = $profilePath
            'Starship/starship.toml' = $starshipConfigPath
            'WindowsTerminal/settings.json' = $terminalSettingsPath
            'WindowsTerminal/Fragments/project-template/profile.json' = $terminalFragmentPath
            'Zellij/config.kdl' = $zellijPath
            'VSCode/keybindings.json' = $vscodePath
        })
    }
    catch {
        [Console]::Error.WriteLine("WARNING: configuration backup failed: $($_.Exception.Message)")
    }

    if ($summary['Windows Terminal'].Status -eq 'Success' -and $powerShellDependencyState.ConfigureTerminal) {
        try {
            Write-WindowsTerminalFragment -Path $terminalFragmentPath -CommandLine $powerShell7Installation.TerminalCommandLine
            Set-WindowsTerminalDefaultProfile -Path $terminalSettingsPath
            $summary['Windows Terminal'] = New-FeatureResult Success 'Installed and configured with the project PowerShell 7 profile as default.'
        }
        catch {
            $summary['Windows Terminal'] = New-FeatureResult Warning $_.Exception.Message
        }
    }
    elseif ($summary['Windows Terminal'].Status -eq 'Success') {
        $summary['Windows Terminal'] = New-FeatureResult Warning 'Installed, but its PowerShell 7 profile was not configured because PowerShell 7 is unavailable.'
    }

    if ($selection.Starship) {
        try {
            Set-ProjectTemplateStarshipConfig -Path $starshipConfigPath
        }
        catch {
            Set-SummaryResult $summary 'Starship' Warning "Installed, but scan timeout configuration failed: $($_.Exception.Message)"
        }
    }

    $gitAvailable = Test-CommandOrPath -Commands @('git.exe') -Paths @()
    if ($selection.GitWrapper -and -not $gitAvailable) {
        $selection.GitWrapper = $false
        Set-SummaryResult $summary 'Git wrapper' Warning 'Git is unavailable and is not installed automatically.'
    }
    $vscodeAvailable = Test-CommandOrPath -Commands @('code.cmd', 'code.exe') -Paths @(
        (Join-Path $env:LOCALAPPDATA 'Programs\Microsoft VS Code\Code.exe'),
        (Join-Path $env:ProgramFiles 'Microsoft VS Code\Code.exe')
    )
    if ($selection.VscodeShiftEnter -and -not $vscodeAvailable) {
        $selection.VscodeShiftEnter = $false
        Set-SummaryResult $summary 'VS Code Shift+Enter' Warning 'VS Code is unavailable and is not installed automatically.'
    }

    $profileSelections = [ordered]@{
        Starship = $selection.Starship
        Eza = $selection.Eza
        FzfTab = $selection.FzfTab
        WordNavigation = $selection.WordNavigation
        GitWrapper = $selection.GitWrapper
        Venv = $selection.Venv
        AutoStartZellij = $selection.AutoStartZellij
    }
    if ($powerShellDependencyState.ConfigureProfile -and ($profileSelections.Values | Where-Object { $_ }).Count -gt 0) {
        try {
            Set-ProjectTemplatePowerShellProfile -Path $profilePath -Selection $profileSelections
            $policyResult = Resolve-ProfileExecutionPolicy -Prompt $Prompt
            $summary['PowerShell profile'] = $policyResult
            foreach ($mapping in @(
                @('Starship', 'Starship'), @('Eza', 'eza'), @('FzfTab', 'PSFzf Tab completion'),
                @('WordNavigation', 'Word navigation'), @('GitWrapper', 'Git wrapper'),
                @('Venv', 'venv'), @('AutoStartZellij', 'Zellij auto-start')
            )) {
                if ($profileSelections[$mapping[0]] -and $summary[$mapping[1]].Status -ne 'Warning') {
                    Set-SummaryResult $summary $mapping[1] Success 'Configured in the PowerShell 7 profile.'
                }
            }
        }
        catch {
            Set-SummaryResult $summary 'PowerShell profile' Warning $_.Exception.Message
        }
    }

    if ($selection.ZellijCopy) {
        try {
            Set-ProjectTemplateZellijConfig -Path $zellijPath
            Set-SummaryResult $summary 'Zellij clipboard' Success 'Configured native system clipboard behavior.'
        }
        catch { Set-SummaryResult $summary 'Zellij clipboard' Warning $_.Exception.Message }
    }
    if ($selection.VscodeShiftEnter) { $summary['VS Code Shift+Enter'] = Set-VSCodeShiftEnterBinding -Path $vscodePath }

    Write-WindowsSetupSummary -Summary $summary -BackupPath $backupPath
    return 0
}
