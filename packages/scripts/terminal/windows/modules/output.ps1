Set-StrictMode -Version Latest

$script:ToolboxOutputStarted = $false
$script:ToolboxOutputName = ''
$script:ToolboxClock = { [DateTime]::Now.ToString('HH:mm:ss') }

function Initialize-ToolboxOutput {
    param([Parameter(Mandatory = $true)][string]$ToolName)

    $script:ToolboxOutputName = $ToolName
    $script:ToolboxOutputStarted = $false
}

function Test-ToolboxColor {
    if (Test-Path Env:NO_COLOR) { return $false }
    if ([Console]::IsOutputRedirected) { return $false }
    return $Host.Name -eq 'ConsoleHost'
}

function Test-Toolbox256Color {
    if (-not (Test-ToolboxColor)) { return $false }
    return $env:TERM -like '*256color*' -or
        -not [string]::IsNullOrWhiteSpace($env:COLORTERM) -or
        -not [string]::IsNullOrWhiteSpace($env:WT_SESSION)
}

function Write-ToolboxLine {
    param(
        [Parameter(Mandatory = $true)][ValidateSet('INFO', 'PROMPT', 'WARN', 'ERROR', 'SUCCESS')][string]$Level,
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Message
    )

    if (-not $script:ToolboxOutputStarted) {
        [Console]::Out.WriteLine('')
        $script:ToolboxOutputStarted = $true
    }
    foreach ($MessageLine in ($Message -split "`r?`n")) {
        $Rendered = '[{0} {1} {2}] {3}' -f (& $script:ToolboxClock), $script:ToolboxOutputName, $Level, $MessageLine
        if (-not (Test-ToolboxColor) -or $Level -eq 'INFO') {
            [Console]::Out.WriteLine($Rendered)
            continue
        }
        if ($Level -eq 'WARN' -and (Test-Toolbox256Color)) {
            $Escape = [char]27
            [Console]::Out.WriteLine("${Escape}[38;5;208m${Rendered}${Escape}[0m")
            continue
        }
        $Color = switch ($Level) {
            'PROMPT' { 'Cyan' }
            'WARN' { 'Yellow' }
            'ERROR' { 'Red' }
            'SUCCESS' { 'Green' }
        }
        Write-Host $Rendered -ForegroundColor $Color
    }
}

function Write-ToolboxInfo { param([string]$Message) Write-ToolboxLine INFO $Message }
function Write-ToolboxPrompt { param([string]$Message) Write-ToolboxLine PROMPT $Message }
function Write-ToolboxMenu {
    param([string]$Question, [string[]]$Options)

    Write-ToolboxPrompt $Question
    foreach ($Option in $Options) {
        $Rendered = "  $Option"
        if (Test-ToolboxColor) { Write-Host $Rendered -ForegroundColor Cyan }
        else { [Console]::Out.WriteLine($Rendered) }
    }
}
function Write-ToolboxWarn { param([string]$Message) Write-ToolboxLine WARN $Message }
function Write-ToolboxError { param([string]$Message) Write-ToolboxLine ERROR $Message }
function Write-ToolboxSuccess { param([string]$Message) Write-ToolboxLine SUCCESS $Message }

function Read-ToolboxYesNo {
    param(
        [Parameter(Mandatory = $true)][string]$Question,
        [Parameter(Mandatory = $true)][bool]$Default
    )

    $DefaultName = if ($Default) { 'Y' } else { 'n' }
    $Marker = if ($Default) { 'Y/n' } else { 'y/N' }
    while ($true) {
        Write-ToolboxPrompt "$Question [$Marker] (default: $DefaultName):"
        $Answer = Read-Host
        if ([string]::IsNullOrWhiteSpace($Answer)) { return $Default }
        switch ($Answer.Trim().ToLowerInvariant()) {
            { $_ -in @('y', 'yes') } { return $true }
            { $_ -in @('n', 'no') } { return $false }
            default { Write-ToolboxWarn "Enter y or n, or press Enter for $DefaultName." }
        }
    }
}

function Read-ToolboxRequired {
    param([Parameter(Mandatory = $true)][string]$Question)

    while ($true) {
        Write-ToolboxPrompt "${Question}: "
        $Answer = Read-Host
        if (-not [string]::IsNullOrWhiteSpace($Answer)) { return $Answer.Trim() }
        Write-ToolboxWarn 'A value is required; enter a value to continue.'
    }
}
