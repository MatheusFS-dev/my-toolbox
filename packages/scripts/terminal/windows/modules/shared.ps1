Set-StrictMode -Version 2.0

function Get-TextEncodingInfo {
    param([Parameter(Mandatory = $true)][string]$Path)

    $bytes = [IO.File]::ReadAllBytes($Path)
    $hasBom = $bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF
    return [pscustomobject]@{
        HasUtf8Bom = $hasBom
        Encoding   = New-Object Text.UTF8Encoding($hasBom)
    }
}

function Write-AtomicText {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Content,
        [Text.Encoding]$Encoding = (New-Object Text.UTF8Encoding($false))
    )

    $directory = Split-Path -Parent $Path
    if (-not $directory) { $directory = (Get-Location).Path }
    [IO.Directory]::CreateDirectory($directory) | Out-Null
    $temporaryPath = Join-Path $directory ('.project-template-{0}.tmp' -f [guid]::NewGuid().ToString('N'))
    try {
        [IO.File]::WriteAllText($temporaryPath, $Content, $Encoding)
        if ([IO.File]::Exists($Path)) {
            $replaceBackup = $temporaryPath + '.replace-backup'
            try {
                [IO.File]::Replace($temporaryPath, $Path, $replaceBackup)
            }
            finally {
                if ([IO.File]::Exists($replaceBackup)) { [IO.File]::Delete($replaceBackup) }
            }
        }
        else {
            [IO.File]::Move($temporaryPath, $Path)
        }
    }
    finally {
        if ([IO.File]::Exists($temporaryPath)) { [IO.File]::Delete($temporaryPath) }
    }
}

function Write-ManagedBlock {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$Feature,
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Content,
        [string]$CommentPrefix = '#'
    )

    $startMarker = "$CommentPrefix >>> project-template:windows:$Feature >>>"
    $endMarker = "$CommentPrefix <<< project-template:windows:$Feature <<<"
    $existing = ''
    $encoding = New-Object Text.UTF8Encoding($false)
    if ([IO.File]::Exists($Path)) {
        $encoding = (Get-TextEncodingInfo -Path $Path).Encoding
        $existing = [IO.File]::ReadAllText($Path)
    }

    $pattern = '(?ms)^[ \t]*' + [regex]::Escape($startMarker) + '\r?\n.*?^[ \t]*' + [regex]::Escape($endMarker) + '[ \t]*(?:\r?\n)?'
    $preserved = [regex]::Replace($existing, $pattern, '')
    $preserved = $preserved.TrimEnd("`r", "`n")
    $newline = if (-not $existing -or $existing -match "`r`n") { "`r`n" } else { "`n" }
    $block = $startMarker + $newline + $Content.TrimEnd("`r", "`n") + $newline + $endMarker + $newline
    $updated = if ($preserved.Length -gt 0) { $preserved + $newline + $newline + $block } else { $block }
    Write-AtomicText -Path $Path -Content $updated -Encoding $encoding
}

function New-FeatureResult {
    param(
        [ValidateSet('Success', 'Warning', 'Skipped')][string]$Status,
        [string]$Message = ''
    )
    return [pscustomobject]@{ Status = $Status; Message = $Message }
}

function Read-YesNo {
    param([string]$Message, [bool]$Default = $true)
    $suffix = if ($Default) { '[Y/n]' } else { '[y/N]' }
    $answer = Read-Host "$Message $suffix"
    if ([string]::IsNullOrWhiteSpace($answer)) { return $Default }
    return $answer -match '^(?i:y|yes)$'
}

function Find-JsoncStringEnd {
    param([string]$Text, [int]$Start)
    if ($Start -ge $Text.Length -or $Text[$Start] -ne '"') { throw 'Expected a JSON string.' }
    $escaped = $false
    for ($index = $Start + 1; $index -lt $Text.Length; $index++) {
        if ($escaped) { $escaped = $false; continue }
        if ($Text[$index] -eq '\') { $escaped = $true; continue }
        if ($Text[$index] -eq '"') { return $index }
    }
    throw 'Unterminated JSON string.'
}

function Convert-JsoncToStrictJson {
    param([Parameter(Mandatory = $true)][string]$Text)

    $withoutComments = New-Object Text.StringBuilder
    $index = 0
    while ($index -lt $Text.Length) {
        if ($Text[$index] -eq '"') {
            $end = Find-JsoncStringEnd -Text $Text -Start $index
            [void]$withoutComments.Append($Text.Substring($index, $end - $index + 1))
            $index = $end + 1
            continue
        }
        if ($index + 1 -lt $Text.Length -and $Text[$index] -eq '/' -and $Text[$index + 1] -eq '/') {
            while ($index -lt $Text.Length -and $Text[$index] -notin @("`r", "`n")) {
                [void]$withoutComments.Append(' ')
                $index++
            }
            continue
        }
        if ($index + 1 -lt $Text.Length -and $Text[$index] -eq '/' -and $Text[$index + 1] -eq '*') {
            $end = $Text.IndexOf('*/', $index + 2, [StringComparison]::Ordinal)
            if ($end -lt 0) { throw 'Unterminated JSONC block comment.' }
            while ($index -lt $end + 2) {
                if ($Text[$index] -in @("`r", "`n")) { [void]$withoutComments.Append($Text[$index]) }
                else { [void]$withoutComments.Append(' ') }
                $index++
            }
            continue
        }
        [void]$withoutComments.Append($Text[$index])
        $index++
    }

    $commentFree = $withoutComments.ToString()
    $strictJson = New-Object Text.StringBuilder
    $index = 0
    while ($index -lt $commentFree.Length) {
        if ($commentFree[$index] -eq '"') {
            $end = Find-JsoncStringEnd -Text $commentFree -Start $index
            [void]$strictJson.Append($commentFree.Substring($index, $end - $index + 1))
            $index = $end + 1
            continue
        }
        if ($commentFree[$index] -eq ',') {
            $lookAhead = $index + 1
            while ($lookAhead -lt $commentFree.Length -and [char]::IsWhiteSpace($commentFree[$lookAhead])) { $lookAhead++ }
            if ($lookAhead -lt $commentFree.Length -and $commentFree[$lookAhead] -in @('}', ']')) {
                [void]$strictJson.Append(' ')
                $index++
                continue
            }
        }
        [void]$strictJson.Append($commentFree[$index])
        $index++
    }
    return $strictJson.ToString()
}

function Assert-JsoncDocument {
    param(
        [Parameter(Mandatory = $true)][string]$Text,
        [ValidateSet('Object', 'Array')][string]$RootType
    )
    $strictJson = Convert-JsoncToStrictJson -Text $Text
    $null = $strictJson | ConvertFrom-Json -ErrorAction Stop
    $trimmed = $strictJson.TrimStart()
    if (-not $trimmed) { throw 'Expected a JSON document.' }
    if ($RootType -eq 'Object' -and $trimmed[0] -ne '{') {
        throw 'Expected a top-level JSON object.'
    }
    if ($RootType -eq 'Array' -and $trimmed[0] -ne '[') {
        throw 'Expected a top-level JSON array.'
    }
}
