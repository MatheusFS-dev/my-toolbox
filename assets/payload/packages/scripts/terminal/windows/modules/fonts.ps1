Set-StrictMode -Version 2.0

function Invoke-FiraCodeDownload {
    param([string]$Uri, [string]$Destination)
    Invoke-WebRequest -UseBasicParsing -Uri $Uri -OutFile $Destination
}

function Expand-FiraCodeArchive {
    param([string]$Archive, [string]$Destination)
    Expand-Archive -LiteralPath $Archive -DestinationPath $Destination -Force
}

function Set-CurrentUserFontRegistration {
    param([string]$Name, [string]$Value, [string]$Path)
    if (-not (Test-Path -LiteralPath $Path)) { New-Item -Path $Path -Force | Out-Null }
    New-ItemProperty -Path $Path -Name $Name -Value $Value -PropertyType String -Force | Out-Null
}

function Install-FiraCodeNerdFont {
    param(
        [string]$FontDirectory = (Join-Path $env:LOCALAPPDATA 'Microsoft\Windows\Fonts'),
        [string]$RegistryPath = 'HKCU:\Software\Microsoft\Windows NT\CurrentVersion\Fonts',
        [string]$WorkingDirectory = ([IO.Path]::GetTempPath()),
        [scriptblock]$Downloader = ${function:Invoke-FiraCodeDownload},
        [scriptblock]$ArchiveExpander = ${function:Expand-FiraCodeArchive},
        [scriptblock]$RegistryWriter = ${function:Set-CurrentUserFontRegistration}
    )

    $downloadUri = 'https://github.com/ryanoasis/nerd-fonts/releases/latest/download/FiraCode.zip'
    $operationDirectory = Join-Path $WorkingDirectory ('project-template-firacode-{0}' -f [guid]::NewGuid().ToString('N'))
    $archivePath = Join-Path $operationDirectory 'FiraCode.zip'
    $extractPath = Join-Path $operationDirectory 'expanded'
    try {
        [IO.Directory]::CreateDirectory($operationDirectory) | Out-Null
        & $Downloader $downloadUri $archivePath
        & $ArchiveExpander $archivePath $extractPath
        $fontFiles = @(Get-ChildItem -LiteralPath $extractPath -Recurse -File -Filter 'FiraCode*NerdFontMono*.ttf')
        if ($fontFiles.Count -eq 0) {
            return New-FeatureResult -Status Warning -Message 'The official archive contained no FiraCode Nerd Font Mono faces.'
        }

        [IO.Directory]::CreateDirectory($FontDirectory) | Out-Null
        foreach ($fontFile in $fontFiles) {
            $destination = Join-Path $FontDirectory $fontFile.Name
            $copyRequired = $true
            if ([IO.File]::Exists($destination)) {
                $sourceHash = (Get-FileHash -LiteralPath $fontFile.FullName -Algorithm SHA256).Hash
                $destinationHash = (Get-FileHash -LiteralPath $destination -Algorithm SHA256).Hash
                $copyRequired = $sourceHash -ne $destinationHash
            }
            if ($copyRequired) { [IO.File]::Copy($fontFile.FullName, $destination, $true) }
            & $RegistryWriter ($fontFile.BaseName + ' (TrueType)') $destination $RegistryPath
        }
        return New-FeatureResult -Status Success -Message ("Installed and registered {0} FiraCode Nerd Font Mono faces for the current user." -f $fontFiles.Count)
    }
    catch {
        return New-FeatureResult -Status Warning -Message $_.Exception.Message
    }
    finally {
        if ([IO.Directory]::Exists($operationDirectory)) {
            Remove-Item -LiteralPath $operationDirectory -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}
