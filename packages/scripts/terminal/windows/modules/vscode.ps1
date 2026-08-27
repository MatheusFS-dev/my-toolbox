Set-StrictMode -Version 2.0

function Get-VSCodeKeybindingsPath {
    param([string]$AppData = $env:APPDATA)
    return Join-Path $AppData 'Code\User\keybindings.json'
}

function Find-KeybindingsStringEnd {
    param([string]$Text, [int]$Start)
    $escaped = $false
    for ($index = $Start + 1; $index -lt $Text.Length; $index++) {
        if ($escaped) { $escaped = $false; continue }
        if ($Text[$index] -eq '\') { $escaped = $true; continue }
        if ($Text[$index] -eq '"') { return $index }
    }
    throw 'Unterminated JSONC string.'
}

function Skip-KeybindingsTrivia {
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

function Get-KeybindingsArrayInfo {
    param([string]$Text)
    $rootStart = Skip-KeybindingsTrivia -Text $Text -Start 0
    if ($rootStart -ge $Text.Length -or $Text[$rootStart] -ne '[') { throw 'VS Code keybindings must be a top-level JSON array.' }
    $stack = New-Object System.Collections.Stack
    $stack.Push('[')
    $objects = New-Object System.Collections.ArrayList
    $objectStart = -1
    $lastTopLevelToken = $null
    $rootEnd = -1
    $index = $rootStart + 1
    while ($index -lt $Text.Length) {
        $index = Skip-KeybindingsTrivia -Text $Text -Start $index
        if ($index -ge $Text.Length) { break }
        $character = $Text[$index]
        if ($character -eq '"') { $index = (Find-KeybindingsStringEnd -Text $Text -Start $index) + 1; continue }
        if ($character -in @('{', '[')) {
            if ($stack.Count -eq 1 -and $character -eq '{') { $objectStart = $index }
            $stack.Push($character)
            $index++
            continue
        }
        if ($character -in @('}', ']')) {
            if ($stack.Count -eq 0) { throw 'Unexpected closing token in keybindings JSONC.' }
            $opening = [char]$stack.Peek()
            if (($character -eq '}' -and $opening -ne '{') -or ($character -eq ']' -and $opening -ne '[')) {
                throw 'Mismatched tokens in keybindings JSONC.'
            }
            [void]$stack.Pop()
            if ($character -eq '}' -and $stack.Count -eq 1 -and $objectStart -ge 0) {
                [void]$objects.Add([pscustomobject]@{ Start = $objectStart; Length = $index - $objectStart + 1 })
                $objectStart = -1
                $lastTopLevelToken = 'object'
            }
            elseif ($character -eq ']' -and $stack.Count -eq 0) { $rootEnd = $index; break }
            $index++
            continue
        }
        if ($stack.Count -eq 1 -and $character -eq ',') { $lastTopLevelToken = 'comma' }
        $index++
    }
    if ($stack.Count -ne 0 -or $rootEnd -lt 0) { throw 'Malformed or incomplete keybindings JSONC.' }
    $trailing = Skip-KeybindingsTrivia -Text $Text -Start ($rootEnd + 1)
    if ($trailing -ne $Text.Length) { throw 'Unexpected content after the keybindings array.' }
    return [pscustomobject]@{ RootStart = $rootStart; RootEnd = $rootEnd; Objects = @($objects); LastToken = $lastTopLevelToken }
}

function Assert-KeybindingsJsoncValid {
    param([string]$Text)
    Assert-JsoncDocument -Text $Text -RootType Array
}

function Get-ShiftEnterBindingText {
    return @'
  {
    "key": "shift+enter",
    "command": "workbench.action.terminal.sendSequence",
    "args": { "text": "\u001b[200~\n\u001b[201~" },
    "when": "terminalFocus"
  }
'@
}

function Set-VSCodeShiftEnterBinding {
    param([string]$Path = (Get-VSCodeKeybindingsPath))
    try {
        if (-not [IO.File]::Exists($Path)) {
            Write-AtomicText -Path $Path -Content ("[`r`n" + (Get-ShiftEnterBindingText) + "`r`n]`r`n")
            return New-FeatureResult -Status Success -Message 'Created the VS Code Shift+Enter terminal binding.'
        }
        $encoding = (Get-TextEncodingInfo -Path $Path).Encoding
        $text = [IO.File]::ReadAllText($Path)
        Assert-KeybindingsJsoncValid -Text $text
        $info = Get-KeybindingsArrayInfo -Text $text
        $binding = Get-ShiftEnterBindingText
        $matching = $null
        foreach ($object in $info.Objects) {
            $objectText = $text.Substring($object.Start, $object.Length)
            if ($objectText -match '(?is)"key"\s*:\s*"shift\+enter"' -and $objectText -match '(?is)"when"\s*:\s*"terminalFocus"') {
                $matching = $object
                break
            }
        }
        if ($matching) {
            $updated = $text.Remove($matching.Start, $matching.Length).Insert($matching.Start, $binding.TrimEnd("`r", "`n"))
        }
        else {
            $newline = if ($text -match "`r`n") { "`r`n" } else { "`n" }
            $separator = if ($info.Objects.Count -gt 0 -and $info.LastToken -ne 'comma') { ',' } else { '' }
            $updated = $text.Insert($info.RootEnd, $separator + $newline + $binding + $newline)
        }
        Write-AtomicText -Path $Path -Content $updated -Encoding $encoding
        return New-FeatureResult -Status Success -Message 'Configured the VS Code Shift+Enter terminal binding.'
    }
    catch {
        return New-FeatureResult -Status Warning -Message ("Refused to overwrite malformed VS Code keybindings: {0}" -f $_.Exception.Message)
    }
}
