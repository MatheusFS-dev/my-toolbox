param(
    [switch]$Undo,
    [string]$WslPath
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

function Find-JsonStringEnd {
    param([Parameter(Mandatory = $true)][string]$Text, [Parameter(Mandatory = $true)][int]$Start)

    $escaped = $false
    for ($index = $Start + 1; $index -lt $Text.Length; $index++) {
        if ($escaped) { $escaped = $false; continue }
        if ($Text[$index] -eq '\') { $escaped = $true; continue }
        if ($Text[$index] -eq '"') { return $index }
    }
    throw 'Unterminated JSONC string.'
}

function Skip-JsoncTrivia {
    param([Parameter(Mandatory = $true)][string]$Text, [Parameter(Mandatory = $true)][int]$Start)

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

function Read-JsoncValue {
    param([Parameter(Mandatory = $true)][string]$Text, [Parameter(Mandatory = $true)][int]$Start)

    if ($Start -ge $Text.Length) { throw 'Expected a JSONC value.' }
    if ($Text[$Start] -eq '"') {
        $end = Find-JsonStringEnd -Text $Text -Start $Start
        $null = $Text.Substring($Start, $end - $Start + 1) | ConvertFrom-Json -ErrorAction Stop
        return [pscustomobject]@{ End = $end + 1 }
    }
    if ($Text[$Start] -eq '{') {
        $object = Get-JsoncObjectInfo -Text $Text -Start $Start
        return [pscustomobject]@{ End = $object.End + 1 }
    }
    if ($Text[$Start] -eq '[') {
        $index = Skip-JsoncTrivia -Text $Text -Start ($Start + 1)
        if ($index -lt $Text.Length -and $Text[$index] -eq ']') {
            return [pscustomobject]@{ End = $index + 1 }
        }
        while ($index -lt $Text.Length) {
            $value = Read-JsoncValue -Text $Text -Start $index
            $index = Skip-JsoncTrivia -Text $Text -Start $value.End
            if ($index -lt $Text.Length -and $Text[$index] -eq ']') {
                return [pscustomobject]@{ End = $index + 1 }
            }
            if ($index -ge $Text.Length -or $Text[$index] -ne ',') {
                throw 'Expected a comma between JSONC array values.'
            }
            $index = Skip-JsoncTrivia -Text $Text -Start ($index + 1)
            if ($index -lt $Text.Length -and $Text[$index] -eq ']') {
                return [pscustomobject]@{ End = $index + 1 }
            }
        }
        throw 'Unterminated JSONC array.'
    }
    else {
        $index = $Start
        while ($index -lt $Text.Length) {
            if ($Text[$index] -in @(',', '}', ']') -or [char]::IsWhiteSpace($Text[$index])) { break }
            if ($index + 1 -lt $Text.Length -and $Text[$index] -eq '/' -and $Text[$index + 1] -in @('/', '*')) { break }
            $index++
        }
        $token = $Text.Substring($Start, $index - $Start)
        if ($token -cnotmatch '^(?:true|false|null|-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?)$') {
            throw "Invalid JSONC value: $token"
        }
        return [pscustomobject]@{ End = $index }
    }
}

function Get-JsoncObjectInfo {
    param([Parameter(Mandatory = $true)][string]$Text, [Parameter(Mandatory = $true)][int]$Start)

    if ($Start -ge $Text.Length -or $Text[$Start] -ne '{') { throw 'Expected a JSONC object.' }
    $properties = New-Object System.Collections.ArrayList
    $propertyNames = New-Object 'System.Collections.Generic.HashSet[string]' ([StringComparer]::Ordinal)
    $index = Skip-JsoncTrivia -Text $Text -Start ($Start + 1)
    $trailingComma = $false
    while ($index -lt $Text.Length -and $Text[$index] -ne '}') {
        if ($Text[$index] -ne '"') { throw 'Expected a JSONC property name.' }
        $keyEnd = Find-JsonStringEnd -Text $Text -Start $index
        $key = $Text.Substring($index, $keyEnd - $index + 1) | ConvertFrom-Json -ErrorAction Stop
        if (-not $propertyNames.Add([string]$key)) { throw "Duplicate JSONC property name: $key" }
        $colon = Skip-JsoncTrivia -Text $Text -Start ($keyEnd + 1)
        if ($colon -ge $Text.Length -or $Text[$colon] -ne ':') { throw "Expected ':' after JSONC property name." }
        $valueStart = Skip-JsoncTrivia -Text $Text -Start ($colon + 1)
        if ($valueStart -ge $Text.Length) { throw 'Expected a JSONC property value.' }
        $value = Read-JsoncValue -Text $Text -Start $valueStart
        $valueEnd = $value.End
        [void]$properties.Add([pscustomobject]@{ Name = $key; Start = $index; ValueStart = $valueStart; ValueEnd = $valueEnd })
        $index = Skip-JsoncTrivia -Text $Text -Start $valueEnd
        if ($index -lt $Text.Length -and $Text[$index] -eq ',') {
            $trailingComma = $true
            $index = Skip-JsoncTrivia -Text $Text -Start ($index + 1)
        }
        else { $trailingComma = $false }
        if ($index -lt $Text.Length -and $Text[$index] -ne '}' -and -not $trailingComma) {
            throw 'Expected a comma between JSONC properties.'
        }
    }
    if ($index -ge $Text.Length -or $Text[$index] -ne '}') { throw 'Unterminated JSONC object.' }
    return [pscustomobject]@{ Start = $Start; End = $index; Properties = @($properties); TrailingComma = $trailingComma }
}

function Find-JsoncProperty {
    param($ObjectInfo, [Parameter(Mandatory = $true)][string]$Name)
    return @($ObjectInfo.Properties | Where-Object { $_.Name -ceq $Name } | Select-Object -First 1)[0]
}

function Add-JsoncObjectProperty {
    param(
        [Parameter(Mandatory = $true)][string]$Text,
        [Parameter(Mandatory = $true)]$ObjectInfo,
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][string]$Value,
        [Parameter(Mandatory = $true)][string]$Newline,
        [string]$Indent = '  '
    )

    $quotedName = $Name | ConvertTo-Json -Compress
    if ($ObjectInfo.Properties.Count -eq 0) {
        $insertion = $Newline + $Indent + $quotedName + ': ' + $Value + $Newline
    }
    else {
        $separator = if ($ObjectInfo.TrailingComma) { '' } else { ',' }
        $suffix = if ($ObjectInfo.TrailingComma) { ',' } else { '' }
        $insertion = $separator + $Newline + $Indent + $quotedName + ': ' + $Value + $suffix
    }
    return $Text.Insert($ObjectInfo.End, $insertion)
}

function Remove-JsoncObjectProperty {
    param(
        [Parameter(Mandatory = $true)][string]$Text,
        [Parameter(Mandatory = $true)]$ObjectInfo,
        [Parameter(Mandatory = $true)]$Property
    )

    $propertyIndex = [array]::IndexOf($ObjectInfo.Properties, $Property)
    if ($propertyIndex -lt 0) { throw 'The JSONC property does not belong to the supplied object.' }

    $afterProperty = Skip-JsoncTrivia -Text $Text -Start $Property.ValueEnd
    if ($afterProperty -lt $Text.Length -and $Text[$afterProperty] -eq ',') {
        return $Text.Remove($Property.Start, $afterProperty - $Property.Start + 1)
    }

    if ($propertyIndex -gt 0) {
        $previousProperty = $ObjectInfo.Properties[$propertyIndex - 1]
        $separator = Skip-JsoncTrivia -Text $Text -Start $previousProperty.ValueEnd
        if ($separator -ge $Text.Length -or $Text[$separator] -ne ',') { throw 'Expected a comma before the JSONC property.' }
        return $Text.Remove($separator, $Property.ValueEnd - $separator)
    }

    return $Text.Remove($Property.Start, $Property.ValueEnd - $Property.Start)
}

function Read-TextFile {
    param([Parameter(Mandatory = $true)][string]$Path)
    $bytes = [IO.File]::ReadAllBytes($Path)
    $bomLength = 0
    if ($bytes.Length -ge 4 -and $bytes[0] -eq 0x00 -and $bytes[1] -eq 0x00 -and $bytes[2] -eq 0xFE -and $bytes[3] -eq 0xFF) {
        $encoding = New-Object Text.UTF32Encoding($true, $true, $true)
        $bomLength = 4
    }
    elseif ($bytes.Length -ge 4 -and $bytes[0] -eq 0xFF -and $bytes[1] -eq 0xFE -and $bytes[2] -eq 0x00 -and $bytes[3] -eq 0x00) {
        $encoding = New-Object Text.UTF32Encoding($false, $true, $true)
        $bomLength = 4
    }
    elseif ($bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF) {
        $encoding = New-Object Text.UTF8Encoding($true, $true)
        $bomLength = 3
    }
    elseif ($bytes.Length -ge 2 -and $bytes[0] -eq 0xFF -and $bytes[1] -eq 0xFE) {
        $encoding = New-Object Text.UnicodeEncoding($false, $true, $true)
        $bomLength = 2
    }
    elseif ($bytes.Length -ge 2 -and $bytes[0] -eq 0xFE -and $bytes[1] -eq 0xFF) {
        $encoding = New-Object Text.UnicodeEncoding($true, $true, $true)
        $bomLength = 2
    }
    else {
        $encoding = New-Object Text.UTF8Encoding($false, $true)
    }
    $text = $encoding.GetString($bytes, $bomLength, $bytes.Length - $bomLength)
    return [pscustomobject]@{ Encoding = $encoding; Text = $text }
}

function Write-AtomicText {
    param([string]$Path, [string]$Content, [Text.Encoding]$Encoding)
    $directory = Split-Path -Parent $Path
    [IO.Directory]::CreateDirectory($directory) | Out-Null
    $temporaryPath = Join-Path $directory ('.project-template-{0}.tmp' -f [guid]::NewGuid().ToString('N'))
    try {
        [IO.File]::WriteAllText($temporaryPath, $Content, $Encoding)
        if ([IO.File]::Exists($Path)) {
            $replaceBackup = $temporaryPath + '.replace-backup'
            try { [IO.File]::Replace($temporaryPath, $Path, $replaceBackup) }
            finally { if ([IO.File]::Exists($replaceBackup)) { [IO.File]::Delete($replaceBackup) } }
        }
        else { [IO.File]::Move($temporaryPath, $Path) }
    }
    finally { if ([IO.File]::Exists($temporaryPath)) { [IO.File]::Delete($temporaryPath) } }
}

function New-SettingsBackup {
    param([Parameter(Mandatory = $true)][string]$Path)
    $stamp = [DateTime]::UtcNow.ToString('yyyyMMdd-HHmmss-fffffff')
    $backupPath = '{0}.project-template-{1}-{2}.bak' -f $Path, $stamp, [guid]::NewGuid().ToString('N').Substring(0, 8)
    [IO.File]::Copy($Path, $backupPath, $false)
    return $backupPath
}

function Set-VSCodeWslProfile {
    param([Parameter(Mandatory = $true)][string]$SettingsPath, [Parameter(Mandatory = $true)][string]$WslPath)

    $exists = [IO.File]::Exists($SettingsPath)
    $encoding = New-Object Text.UTF8Encoding($false, $true)
    $text = "{`r`n}`r`n"
    if ($exists) {
        $file = Read-TextFile -Path $SettingsPath
        $encoding = $file.Encoding
        $text = $file.Text
    }

    $rootStart = Skip-JsoncTrivia -Text $text -Start 0
    if ($rootStart -ge $text.Length -or $text[$rootStart] -ne '{') { throw 'VS Code settings must be a top-level JSON object.' }
    $root = Get-JsoncObjectInfo -Text $text -Start $rootStart
    $afterRoot = Skip-JsoncTrivia -Text $text -Start ($root.End + 1)
    if ($afterRoot -ne $text.Length) { throw 'Unexpected content after the VS Code settings object.' }

    $newline = if ($text.Contains("`r`n")) { "`r`n" } else { "`n" }
    $profileValue = [ordered]@{
        path = '${env:windir}\System32\wsl.exe'
        args = @('--cd', $WslPath)
    } | ConvertTo-Json -Compress

    $profilesProperty = Find-JsoncProperty -ObjectInfo $root -Name 'terminal.integrated.profiles.windows'
    if ($profilesProperty) {
        if ($text[$profilesProperty.ValueStart] -ne '{') { throw 'terminal.integrated.profiles.windows must be a JSON object.' }
        $profiles = Get-JsoncObjectInfo -Text $text -Start $profilesProperty.ValueStart
        $managedProfile = Find-JsoncProperty -ObjectInfo $profiles -Name 'WSL (project-template)'
        if ($managedProfile) {
            $updated = $text.Remove($managedProfile.ValueStart, $managedProfile.ValueEnd - $managedProfile.ValueStart).Insert($managedProfile.ValueStart, $profileValue)
        }
        else {
            $updated = Add-JsoncObjectProperty -Text $text -ObjectInfo $profiles -Name 'WSL (project-template)' -Value $profileValue -Newline $newline -Indent '    '
        }
    }
    else {
        $profilesValue = '{' + $newline + '    "WSL (project-template)": ' + $profileValue + $newline + '  }'
        $updated = Add-JsoncObjectProperty -Text $text -ObjectInfo $root -Name 'terminal.integrated.profiles.windows' -Value $profilesValue -Newline $newline -Indent '  '
    }

    if ($updated -ceq $text) { return $null }
    $backupPath = $null
    if ($exists) { $backupPath = New-SettingsBackup -Path $SettingsPath }
    Write-AtomicText -Path $SettingsPath -Content $updated -Encoding $encoding
    return $backupPath
}

function Remove-VSCodeWslProfile {
    param([Parameter(Mandatory = $true)][string]$SettingsPath)

    if (-not [IO.File]::Exists($SettingsPath)) { return $false }
    $file = Read-TextFile -Path $SettingsPath
    $rootStart = Skip-JsoncTrivia -Text $file.Text -Start 0
    if ($rootStart -ge $file.Text.Length -or $file.Text[$rootStart] -ne '{') { throw 'VS Code settings must be a top-level JSON object.' }
    $root = Get-JsoncObjectInfo -Text $file.Text -Start $rootStart
    $afterRoot = Skip-JsoncTrivia -Text $file.Text -Start ($root.End + 1)
    if ($afterRoot -ne $file.Text.Length) { throw 'Unexpected content after the VS Code settings object.' }

    $profilesProperty = Find-JsoncProperty -ObjectInfo $root -Name 'terminal.integrated.profiles.windows'
    if (-not $profilesProperty) { return $false }
    if ($file.Text[$profilesProperty.ValueStart] -ne '{') { throw 'terminal.integrated.profiles.windows must be a JSON object.' }
    $profiles = Get-JsoncObjectInfo -Text $file.Text -Start $profilesProperty.ValueStart
    $managedProfile = Find-JsoncProperty -ObjectInfo $profiles -Name 'WSL (project-template)'
    if (-not $managedProfile) { return $false }

    $updated = Remove-JsoncObjectProperty -Text $file.Text -ObjectInfo $profiles -Property $managedProfile
    Write-AtomicText -Path $SettingsPath -Content $updated -Encoding $file.Encoding
    return $true
}

function Get-VSCodeWslFolderUri {
    param(
        [Parameter(Mandatory = $true)][string]$WslExe,
        [Parameter(Mandatory = $true)][string]$WslPath
    )

    $distribution = (& $WslExe --exec /bin/sh -lc 'printf %s "$WSL_DISTRO_NAME"' | Out-String).Trim()
    if ($LASTEXITCODE -ne 0) { throw 'Could not determine the default WSL distribution.' }
    if ([string]::IsNullOrWhiteSpace($distribution)) { throw 'The default WSL distribution did not report its name.' }

    $segments = @($WslPath.TrimStart('/') -split '/' | Where-Object { $_.Length -gt 0 } | ForEach-Object { [Uri]::EscapeDataString($_) })
    $encodedPath = $segments -join '/'
    return 'vscode-remote://wsl+' + [Uri]::EscapeDataString($distribution) + '/' + $encodedPath
}

function Open-VSCodeWslFolder {
    param(
        [Parameter(Mandatory = $true)][string]$WslExe,
        [Parameter(Mandatory = $true)][string]$WslPath
    )

    $folderUri = Get-VSCodeWslFolderUri -WslExe $WslExe -WslPath $WslPath
    $codeCommand = Get-Command code -CommandType Application -ErrorAction Stop | Select-Object -First 1
    & $codeCommand.Source --folder-uri $folderUri
    if ($LASTEXITCODE -ne 0) { throw "VS Code failed to open the WSL workspace (exit code $LASTEXITCODE)." }
    return $folderUri
}

try {
    if ($args.Count -ne 0) { throw 'Unexpected arguments.' }
    $settingsPath = Join-Path $env:APPDATA 'Code\User\settings.json'

    if ($Undo) {
        if ($PSBoundParameters.ContainsKey('WslPath')) { throw '-WslPath cannot be used with -Undo.' }
        if (Remove-VSCodeWslProfile -SettingsPath $settingsPath) {
            Write-Output 'Removed the managed VS Code WSL terminal profile.'
        }
        else {
            Write-Output 'No managed VS Code WSL terminal profile was found.'
        }
        exit 0
    }

    if ([string]::IsNullOrWhiteSpace($WslPath)) { $WslPath = Read-Host -Prompt 'Enter the default WSL directory' }
    if ([string]::IsNullOrWhiteSpace($wslPath)) { throw 'A WSL directory is required.' }
    if (-not $wslPath.StartsWith('/')) { throw 'The WSL directory must be an absolute path beginning with /.' }

    $wslExe = Join-Path $env:windir 'System32\wsl.exe'
    & $wslExe --exec test -d $wslPath
    if ($LASTEXITCODE -ne 0) { throw "The WSL directory does not exist in the default distribution: $wslPath" }

    $backupPath = Set-VSCodeWslProfile -SettingsPath $settingsPath -WslPath $wslPath
    $folderUri = Open-VSCodeWslFolder -WslExe $wslExe -WslPath $wslPath
    Write-Output "Configured WSL directory: $wslPath"
    Write-Output "Opened VS Code workspace: $folderUri"
    if ($backupPath) { Write-Output "Backup: $backupPath" }
    else { Write-Output 'Backup: none (settings were created or already current)' }
    exit 0
}
catch {
    [Console]::Error.WriteLine("Error: $($_.Exception.Message)")
    exit 1
}
