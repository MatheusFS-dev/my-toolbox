$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

$repositoryRoot = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
. (Join-Path $repositoryRoot 'packages/scripts/terminal/windows/modules/shared.ps1')
. (Join-Path $repositoryRoot 'packages/scripts/terminal/windows/modules/wsl_path_prompt.ps1')

$script:answers = @('maybe', 'no')
$script:answerIndex = 0
function Read-Host {
    param([string]$Prompt)
    $script:answerIndex++
    return $script:answers[$script:answerIndex - 1]
}

$result = @(Read-YesNo -Message 'Continue?' -Default $true 3>&1)
if ($script:answerIndex -ne 2) { throw "Read-YesNo prompted $script:answerIndex times instead of 2." }
if ($result[-1] -ne $false) { throw 'Read-YesNo did not accept no after an invalid answer.' }
if (($result | Out-String) -notmatch 'Enter yes, y, no, n') {
    throw 'Read-YesNo did not display a corrective warning.'
}

$script:answers = @('')
$script:answerIndex = 0
$result = @(Read-YesNo -Message 'Continue?' -Default $true 3>&1)
if ($result[-1] -ne $true) { throw 'Read-YesNo changed the yes default.' }

$script:pathAnswers = @('', 'relative', '/missing', '/project')
$script:pathIndex = 0
$pathResult = @(
    Read-WslDirectory -InitialPath '' -WslExe 'unused.exe' `
        -Prompt {
            $script:pathIndex++
            return $script:pathAnswers[$script:pathIndex - 1]
        } `
        -DirectoryExists { param($Executable, $Path) return $Path -eq '/project' } 3>&1
)
if ($script:pathIndex -ne 4) { throw "Read-WslDirectory prompted $script:pathIndex times instead of 4." }
if ($pathResult[-1] -ne '/project') { throw 'Read-WslDirectory did not accept the valid retried path.' }
if (($pathResult | Out-String) -notmatch 'absolute path' -or ($pathResult | Out-String) -notmatch 'does not exist') {
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
