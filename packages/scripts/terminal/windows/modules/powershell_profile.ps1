Set-StrictMode -Version 2.0

function Get-PowerShell7ProfilePath {
    param([string]$DocumentsPath = [Environment]::GetFolderPath([Environment+SpecialFolder]::MyDocuments))
    return Join-Path $DocumentsPath 'PowerShell\profile.ps1'
}

function Get-ProjectTemplateProfileBlock {
    param([Parameter(Mandatory = $true)][string]$Feature)

    switch ($Feature) {
        'starship' {
            return @'
if (Get-Command starship -ErrorAction SilentlyContinue) {
    Invoke-Expression (& starship init powershell)
}
'@
        }
        'eza' {
            return @'
if (Get-Command eza -ErrorAction SilentlyContinue) {
    Remove-Item Alias:ls -Force -ErrorAction SilentlyContinue
    function global:ls { & eza -1 --icons=auto @args }
}
'@
        }
        'fzf-tab' {
            return @'
if ((Get-Command fzf.exe -ErrorAction SilentlyContinue) -and
    (Get-Module -ListAvailable -Name PSFzf)) {
    Import-Module PSFzf -ErrorAction SilentlyContinue
    if ((Get-Command Set-PSReadLineKeyHandler -ErrorAction SilentlyContinue) -and
        (Get-Command Invoke-FzfTabCompletion -ErrorAction SilentlyContinue)) {
        Set-PSReadLineKeyHandler -Key Tab -ScriptBlock { Invoke-FzfTabCompletion }
    }
}
'@
        }
        'word-navigation' {
            return @'
if (Get-Command Set-PSReadLineKeyHandler -ErrorAction SilentlyContinue) {
    Set-PSReadLineKeyHandler -Chord Ctrl+LeftArrow -Function BackwardWord
    Set-PSReadLineKeyHandler -Chord Ctrl+RightArrow -Function ForwardWord
}
'@
        }
        'git-wrapper' {
            return @'
function global:_projectTemplateGitCloneTarget {
    param([object[]]$CloneArguments)
    $optionsWithValues = @(
        '-b', '--branch', '-o', '--origin', '-u', '--upload-pack', '--template',
        '--reference', '--reference-if-able', '--separate-git-dir', '-j', '--jobs',
        '--depth', '--shallow-since', '--shallow-exclude', '-c', '--config',
        '--filter', '--server-option'
    )
    $repository = $null
    $target = $null
    for ($index = 0; $index -lt $CloneArguments.Count; $index++) {
        $argument = [string]$CloneArguments[$index]
        if ($argument -eq '--') {
            if ($index + 1 -lt $CloneArguments.Count) { $repository = [string]$CloneArguments[++$index] }
            if ($index + 1 -lt $CloneArguments.Count) { $target = [string]$CloneArguments[++$index] }
            break
        }
        if ($argument -in $optionsWithValues) { $index++; continue }
        if ($argument -match '^--[^=]+=') { continue }
        if ($argument.StartsWith('-')) { continue }
        if ($null -eq $repository) { $repository = $argument }
        elseif ($null -eq $target) { $target = $argument }
    }
    if ($target) { return $target }
    if (-not $repository) { return $null }
    $leaf = $repository.TrimEnd('/', '\') -replace '^.*[:/\\]', ''
    return ($leaf -replace '\.git$', '')
}

function global:git {
    param([Parameter(ValueFromRemainingArguments = $true)][object[]]$GitArguments)
    if ($GitArguments.Count -eq 0 -or [string]$GitArguments[0] -ne 'clone') {
        & git.exe @GitArguments
        return
    }
    $cloneArguments = if ($GitArguments.Count -gt 1) { @($GitArguments[1..($GitArguments.Count - 1)]) } else { @() }
    $target = _projectTemplateGitCloneTarget -CloneArguments $cloneArguments
    $startingDirectory = (Get-Location).Path
    & git.exe @GitArguments
    $status = $LASTEXITCODE
    if ($status -eq 0 -and $target) {
        $resolvedTarget = if ([IO.Path]::IsPathRooted($target)) { $target } else { Join-Path $startingDirectory $target }
        if ([IO.Directory]::Exists($resolvedTarget)) {
            Set-Location -LiteralPath $resolvedTarget
            Write-Host "git-clone-cd: changed directory to $resolvedTarget"
        }
    }
}
'@
        }
        'venv' {
            return @'
function global:venv {
    $directory = (Get-Location).Path
    $activationPath = $null
    while ($true) {
        $candidate = Join-Path $directory '.venv\Scripts\Activate.ps1'
        if ([IO.File]::Exists($candidate)) { $activationPath = $candidate; break }
        $parent = [IO.Directory]::GetParent($directory)
        if ($null -eq $parent) { break }
        $directory = $parent.FullName
    }
    if (-not $activationPath) {
        Write-Warning 'venv: no .venv\Scripts\Activate.ps1 was found in this directory or its parents.'
        return
    }
    $environmentRoot = [IO.Directory]::GetParent([IO.Directory]::GetParent($activationPath).FullName).FullName
    if ($env:VIRTUAL_ENV -and [string]::Equals(
        [IO.Path]::GetFullPath($env:VIRTUAL_ENV),
        [IO.Path]::GetFullPath($environmentRoot),
        [StringComparison]::OrdinalIgnoreCase
    )) {
        Write-Host "venv: already active: $environmentRoot"
        return
    }
    . $activationPath
    Write-Host "venv: activated $environmentRoot"
}
'@
        }
        'zellij-autostart' {
            return @'
function global:_projectTemplateShouldAutoStartZellij {
    param([string[]]$CommandLineArguments = [Environment]::GetCommandLineArgs())
    if (-not $env:WT_SESSION -or $env:ZELLIJ) { return $false }
    if ($env:TERM_PROGRAM -eq 'vscode' -or $env:VSCODE_INJECTION -or $env:VSCODE_PID) { return $false }
    if (-not [Environment]::UserInteractive -or $Host.Name -ne 'ConsoleHost') { return $false }
    foreach ($argument in $CommandLineArguments) {
        if ($argument -match '^(?i:-(?:file|f|command|c|encodedcommand|e|noninteractive|noni))$') { return $false }
    }
    return $true
}
if ((_projectTemplateShouldAutoStartZellij) -and (Get-Command zellij -ErrorAction SilentlyContinue)) {
    & zellij
}
'@
        }
        default { throw "Unknown PowerShell profile feature: $Feature" }
    }
}

function Set-ProjectTemplatePowerShellProfile {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)]$Selection
    )
    $mapping = [ordered]@{
        Starship = 'starship'
        Eza = 'eza'
        FzfTab = 'fzf-tab'
        WordNavigation = 'word-navigation'
        GitWrapper = 'git-wrapper'
        Venv = 'venv'
        AutoStartZellij = 'zellij-autostart'
    }
    foreach ($selectionName in $mapping.Keys) {
        if ($Selection[$selectionName]) {
            $feature = $mapping[$selectionName]
            Write-ManagedBlock -Path $Path -Feature $feature -Content (Get-ProjectTemplateProfileBlock -Feature $feature)
        }
    }
}

function Get-ProfileExecutionPolicyState {
    param($PolicyList = (Get-ExecutionPolicy -List))
    $policies = @{}
    foreach ($entry in $PolicyList) { $policies[[string]$entry.Scope] = [string]$entry.ExecutionPolicy }
    foreach ($scope in @('MachinePolicy', 'UserPolicy')) {
        $policy = $policies[$scope]
        if (-not $policy -or $policy -eq 'Undefined') { continue }
        if ($policy -in @('Restricted', 'AllSigned')) {
            return [pscustomobject]@{ Status = 'Blocked'; Message = "$scope is $policy and may prevent this unsigned profile from loading." }
        }
        return [pscustomobject]@{ Status = 'Ready'; Message = "$scope permits this locally created profile ($policy)." }
    }

    $persistentPolicy = 'Restricted'
    foreach ($scope in @('CurrentUser', 'LocalMachine')) {
        if ($policies[$scope] -and $policies[$scope] -ne 'Undefined') {
            $persistentPolicy = $policies[$scope]
            break
        }
    }
    if ($persistentPolicy -in @('Restricted', 'AllSigned')) {
        return [pscustomobject]@{ Status = 'NeedsChange'; Message = "The persistent execution policy is $persistentPolicy." }
    }
    return [pscustomobject]@{ Status = 'Ready'; Message = "The persistent execution policy is $persistentPolicy." }
}

function Resolve-ProfileExecutionPolicy {
    param(
        [scriptblock]$Prompt = ${function:Read-YesNo},
        [scriptblock]$Setter = { Set-ExecutionPolicy -Scope CurrentUser -ExecutionPolicy RemoteSigned -Force }
    )
    $state = Get-ProfileExecutionPolicyState
    if ($state.Status -eq 'Blocked') { return New-FeatureResult -Status Warning -Message $state.Message }
    if ($state.Status -eq 'NeedsChange') {
        if (-not (& $Prompt 'Set the CurrentUser execution policy to RemoteSigned so PowerShell profiles can load?' $false)) {
            return New-FeatureResult -Status Warning -Message 'Execution policy was left unchanged; the profile may not load.'
        }
        try { & $Setter; return New-FeatureResult -Status Success -Message 'CurrentUser execution policy set to RemoteSigned.' }
        catch { return New-FeatureResult -Status Warning -Message $_.Exception.Message }
    }
    return New-FeatureResult -Status Success -Message $state.Message
}
