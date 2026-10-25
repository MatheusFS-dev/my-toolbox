Set-StrictMode -Version 2.0

function Get-StarshipConfigPath {
    param([string]$UserHome = [Environment]::GetFolderPath('UserProfile'))
    return Join-Path $UserHome '.config\starship.toml'
}

function Set-ProjectTemplateStarshipConfig {
    param(
        [string]$Path = (Get-StarshipConfigPath),
        [int]$ScanTimeout = 200
    )

    $startMarker = '# >>> project-template:windows:scan-timeout >>>'
    $endMarker = '# <<< project-template:windows:scan-timeout <<<'
    $existing = ''
    $encoding = New-Object Text.UTF8Encoding($false)
    if ([IO.File]::Exists($Path)) {
        $encoding = (Get-TextEncodingInfo -Path $Path).Encoding
        $existing = [IO.File]::ReadAllText($Path)
    }

    $newline = if (-not $existing -or $existing -match "`r`n") { "`r`n" } else { "`n" }
    $managedPattern = '(?ms)^[ \t]*' + [regex]::Escape($startMarker) + '\r?\n.*?^[ \t]*' +
        [regex]::Escape($endMarker) + '[ \t]*(?:\r?\n)?'
    $hasManagedBlock = [regex]::IsMatch($existing, $managedPattern)
    $preserved = [regex]::Replace($existing, $managedPattern, '', 1)
    $rootSectionEnd = $preserved.IndexOf('[')
    $rootSection = if ($rootSectionEnd -ge 0) { $preserved.Substring(0, $rootSectionEnd) } else { $preserved }

    # Respect a timeout explicitly maintained by the user.
    if (-not $hasManagedBlock -and $rootSection -match '(?m)^[ \t]*scan_timeout[ \t]*=') { return }

    $preserved = $preserved.TrimStart("`r", "`n")
    $block = $startMarker + $newline + "scan_timeout = $ScanTimeout" + $newline + $endMarker
    $updated = if ($preserved.Length -gt 0) { $block + $newline + $newline + $preserved } else { $block + $newline }
    Write-AtomicText -Path $Path -Content $updated -Encoding $encoding
}
