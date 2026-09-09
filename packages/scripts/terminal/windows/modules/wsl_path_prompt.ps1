Set-StrictMode -Version 2.0

function Read-WslDirectory {
    param(
        [AllowEmptyString()][string]$InitialPath,
        [Parameter(Mandatory = $true)][string]$WslExe,
        [scriptblock]$Prompt = { Read-Host -Prompt 'Enter the default WSL directory' },
        [scriptblock]$DirectoryExists = {
            param([string]$Executable, [string]$Path)
            & $Executable --exec test -d $Path
            return $LASTEXITCODE -eq 0
        }
    )

    if (-not [string]::IsNullOrWhiteSpace($InitialPath)) {
        if (-not $InitialPath.StartsWith('/')) {
            throw 'The WSL directory must be an absolute path beginning with /.'
        }
        if (-not (& $DirectoryExists $WslExe $InitialPath)) {
            throw "The WSL directory does not exist in the default distribution: $InitialPath"
        }
        return $InitialPath
    }

    while ($true) {
        $candidate = [string](& $Prompt)
        if ([string]::IsNullOrWhiteSpace($candidate)) {
            Write-Warning 'A WSL directory is required.'
            continue
        }
        if (-not $candidate.StartsWith('/')) {
            Write-Warning 'The WSL directory must be an absolute path beginning with /.'
            continue
        }
        if (-not (& $DirectoryExists $WslExe $candidate)) {
            Write-Warning "The WSL directory does not exist in the default distribution: $candidate"
            continue
        }
        return $candidate
    }
}
