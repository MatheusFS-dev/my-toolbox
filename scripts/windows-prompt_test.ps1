$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

$repositoryRoot = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
. (Join-Path $repositoryRoot 'packages/scripts/terminal/windows/modules/shared.ps1')
. (Join-Path $repositoryRoot 'packages/scripts/terminal/windows/modules/wsl_path_prompt.ps1')
. (Join-Path $repositoryRoot 'packages/scripts/terminal/windows/modules/output.ps1')

$OriginalOutput = [Console]::Out
$MenuWriter = New-Object IO.StringWriter
$PreviousNoColor = $env:NO_COLOR
try {
    $env:NO_COLOR = '1'
    [Console]::SetOut($MenuWriter)
    Initialize-ToolboxOutput -ToolName 'test-tool'
    $script:ToolboxClock = { '09:32:23' }
    Write-ToolboxMenu -Question 'Choose an option' -Options @('1. First', '2. Second')
} finally {
    [Console]::SetOut($OriginalOutput)
    $env:NO_COLOR = $PreviousNoColor
}
$ExpectedMenu = "`n[09:32:23 test-tool PROMPT] Choose an option`n  1. First`n  2. Second`n"
if ($MenuWriter.ToString().Replace("`r`n", "`n") -cne $ExpectedMenu) {
    throw "Menu output was not one labeled block: $($MenuWriter.ToString())"
}

$script:answers = @('maybe', 'no')
$script:answerIndex = 0
function Read-Host {
    param([string]$Prompt)
    $script:answerIndex++
    return $script:answers[$script:answerIndex - 1]
}

$PromptWriter = New-Object IO.StringWriter
$PreviousNoColor = $env:NO_COLOR
try {
    $env:NO_COLOR = '1'
    [Console]::SetOut($PromptWriter)
    $result = @(Read-YesNo -Message 'Continue?' -Default $true)
} finally {
    [Console]::SetOut($OriginalOutput)
    $env:NO_COLOR = $PreviousNoColor
}
if ($script:answerIndex -ne 2) { throw "Read-YesNo prompted $script:answerIndex times instead of 2." }
if ($result[-1] -ne $false) { throw 'Read-YesNo did not accept no after an invalid answer.' }
if ($PromptWriter.ToString() -notmatch 'Enter y or n') {
    throw 'Read-YesNo did not display a corrective warning.'
}

$script:answers = @('')
$script:answerIndex = 0
$result = @(Read-YesNo -Message 'Continue?' -Default $true 3>&1)
if ($result[-1] -ne $true) { throw 'Read-YesNo changed the yes default.' }

$script:pathAnswers = @('', 'relative', '/missing', '/project')
$script:pathIndex = 0
$PathWriter = New-Object IO.StringWriter
$PreviousNoColor = $env:NO_COLOR
try {
    $env:NO_COLOR = '1'
    [Console]::SetOut($PathWriter)
    $pathResult = @(
        Read-WslDirectory -InitialPath '' -WslExe 'unused.exe' `
            -Prompt {
                $script:pathIndex++
                return $script:pathAnswers[$script:pathIndex - 1]
            } `
            -DirectoryExists { param($Executable, $Path) return $Path -eq '/project' }
    )
} finally {
    [Console]::SetOut($OriginalOutput)
    $env:NO_COLOR = $PreviousNoColor
}
if ($script:pathIndex -ne 4) { throw "Read-WslDirectory prompted $script:pathIndex times instead of 4." }
if ($pathResult[-1] -ne '/project') { throw 'Read-WslDirectory did not accept the valid retried path.' }
if ($PathWriter.ToString() -notmatch 'absolute path' -or $PathWriter.ToString() -notmatch 'does not exist') {
    throw 'Read-WslDirectory did not report invalid interactive paths.'
}

try {
    $null = Read-WslDirectory -InitialPath 'relative' -WslExe 'unused.exe' `
        -DirectoryExists { return $true }
    throw 'Read-WslDirectory accepted an invalid command-line path.'
}
catch {
    if ($_.Exception.Message -notmatch 'absolute path') { throw }
}

Write-Output 'Windows prompt checks passed.'
