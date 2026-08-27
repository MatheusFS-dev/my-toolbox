Set-StrictMode -Version 2.0

function Test-WindowsSetupPreflight {
    param(
        [int]$BuildNumber = [Environment]::OSVersion.Version.Build,
        [PlatformID]$Platform = [Environment]::OSVersion.Platform,
        [AllowNull()][string]$WingetPath = $(
            $command = Get-Command winget.exe -ErrorAction SilentlyContinue
            if ($command) { $command.Source } else { $null }
        )
    )

    $problems = New-Object System.Collections.Generic.List[string]
    if ($Platform -ne [PlatformID]::Win32NT) {
        $problems.Add('This installer supports native Windows only.')
    }
    if ($BuildNumber -lt 17763) {
        $problems.Add('Windows 10 build 17763 or newer (with ConPTY), or Windows 11, is required.')
    }
    if ([string]::IsNullOrWhiteSpace($WingetPath)) {
        $problems.Add('WinGet is required. Install or update App Installer from Microsoft Store.')
    }
    return [pscustomobject]@{
        Success  = $problems.Count -eq 0
        Problems = @($problems)
    }
}
