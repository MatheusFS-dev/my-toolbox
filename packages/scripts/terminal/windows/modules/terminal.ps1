Set-StrictMode -Version 2.0

$script:ProjectTemplateTerminalGuid = '{a656786d-5a89-5878-85d8-3e304c6e5682}'

function Get-WindowsTerminalFragmentPath {
    param([string]$LocalAppData = $env:LOCALAPPDATA)
    return Join-Path $LocalAppData 'Microsoft\Windows Terminal\Fragments\project-template\profile.json'
}

function Get-WindowsTerminalSettingsPath {
    param([string]$LocalAppData = $env:LOCALAPPDATA)
    $packaged = Join-Path $LocalAppData 'Packages\Microsoft.WindowsTerminal_8wekyb3d8bbwe\LocalState\settings.json'
    $unpackaged = Join-Path $LocalAppData 'Microsoft\Windows Terminal\settings.json'
    if ([IO.File]::Exists($packaged) -or [IO.Directory]::Exists((Split-Path -Parent $packaged))) { return $packaged }
    return $unpackaged
}

function Write-WindowsTerminalFragment {
    param(
        [string]$Path = (Get-WindowsTerminalFragmentPath),
        [string]$CommandLine = '%ProgramFiles%\PowerShell\7\pwsh.exe'
    )
    $commandLineJson = $CommandLine | ConvertTo-Json -Compress
    $content = @"
{
  "profiles": [
    {
      "name": "PowerShell 7 (project-template)",
      "guid": "{a656786d-5a89-5878-85d8-3e304c6e5682}",
      "commandline": $commandLineJson,
      "font": {
        "face": "FiraCode Nerd Font Mono"
      }
    }
  ]
}
"@
    Write-AtomicText -Path $Path -Content ($content + "`r`n")
}

function Find-JsonStringEnd {
    param([string]$Text, [int]$Start)
    if ($Start -ge $Text.Length -or $Text[$Start] -ne '"') { throw 'Expected a JSON string.' }
    $escaped = $false
    for ($index = $Start + 1; $index -lt $Text.Length; $index++) {
        $character = $Text[$index]
        if ($escaped) { $escaped = $false; continue }
        if ($character -eq '\') { $escaped = $true; continue }
        if ($character -eq '"') { return $index }
    }
    throw 'Unterminated JSON string.'
}

function Skip-JsoncTrivia {
    param([string]$Text, [int]$Start)
    $index = $Start
    while ($index -lt $Text.Length) {
        if ([char]::IsWhiteSpace($Text[$index])) { $index++; continue }
        if ($index + 1 -lt $Text.Length -and $Text[$index] -eq '/' -and $Text[$index + 1] -eq '/') {
            $index += 2
            while ($index -lt $Text.Length -and $Text[$index] -notin @("`r", "`n")) { $index++ }
            continue
        }
        if ($index + 1 -lt $Text.Length -and $Text[$index] -eq '/' -and $Text[$index + 1] -eq '*') {
            $end = $Text.IndexOf('*/', $index + 2, [StringComparison]::Ordinal)
            if ($end -lt 0) { throw 'Unterminated JSONC block comment.' }
            $index = $end + 2
            continue
        }
        break
    }
    return $index
}

function Find-TopLevelDefaultProfileValue {
    param([string]$Text)
    $root = Skip-JsoncTrivia -Text $Text -Start 0
    if ($root -ge $Text.Length -or $Text[$root] -ne '{') { throw 'Windows Terminal settings must be a top-level JSON object.' }
    $depth = 1
    $index = $root + 1
    while ($index -lt $Text.Length -and $depth -gt 0) {
        $index = Skip-JsoncTrivia -Text $Text -Start $index
        if ($index -ge $Text.Length) { break }
        $character = $Text[$index]
        if ($character -eq '"') {
            $stringEnd = Find-JsonStringEnd -Text $Text -Start $index
            if ($depth -eq 1 -and $Text.Substring($index, $stringEnd - $index + 1) -eq '"defaultProfile"') {
                $colon = Skip-JsoncTrivia -Text $Text -Start ($stringEnd + 1)
                if ($colon -lt $Text.Length -and $Text[$colon] -eq ':') {
                    $valueStart = Skip-JsoncTrivia -Text $Text -Start ($colon + 1)
                    if ($valueStart -ge $Text.Length -or $Text[$valueStart] -ne '"') { throw 'defaultProfile must be a JSON string.' }
                    $valueEnd = Find-JsonStringEnd -Text $Text -Start $valueStart
                    return [pscustomobject]@{ Root = $root; Start = $valueStart; Length = $valueEnd - $valueStart + 1 }
                }
            }
            $index = $stringEnd + 1
            continue
        }
        if ($character -in @('{', '[')) { $depth++ }
        elseif ($character -in @('}', ']')) { $depth-- }
        $index++
    }
    if ($depth -ne 0) { throw 'Malformed Windows Terminal JSONC settings.' }
    return [pscustomobject]@{ Root = $root; Start = -1; Length = 0 }
}

function Set-WindowsTerminalDefaultProfile {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [string]$Guid = $script:ProjectTemplateTerminalGuid
    )

    $encoding = New-Object Text.UTF8Encoding($false)
    $text = '{}'
    if ([IO.File]::Exists($Path)) {
        $encoding = (Get-TextEncodingInfo -Path $Path).Encoding
        $text = [IO.File]::ReadAllText($Path)
    }
    Assert-JsoncDocument -Text $text -RootType Object
    $location = Find-TopLevelDefaultProfileValue -Text $text
    $quotedGuid = '"' + $Guid + '"'
    if ($location.Start -ge 0) {
        $updated = $text.Remove($location.Start, $location.Length).Insert($location.Start, $quotedGuid)
    }
    else {
        $afterRoot = Skip-JsoncTrivia -Text $text -Start ($location.Root + 1)
        $newline = if ($text -match "`r`n") { "`r`n" } else { "`n" }
        if ($afterRoot -lt $text.Length -and $text[$afterRoot] -eq '}') {
            $insertion = $newline + '  "defaultProfile": ' + $quotedGuid + $newline
        }
        else {
            $insertion = $newline + '  "defaultProfile": ' + $quotedGuid + ','
        }
        $updated = $text.Insert($location.Root + 1, $insertion)
    }
    Write-AtomicText -Path $Path -Content $updated -Encoding $encoding
}
