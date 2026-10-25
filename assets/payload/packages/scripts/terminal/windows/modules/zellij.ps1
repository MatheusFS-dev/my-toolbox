Set-StrictMode -Version 2.0

function Get-ZellijConfigPath {
    param([string]$AppData = $env:APPDATA)
    return Join-Path $AppData 'zellij\config.kdl'
}

function Set-ProjectTemplateZellijConfig {
    param([string]$Path = (Get-ZellijConfigPath))
    $content = @'
mouse_mode true
copy_on_select true
copy_clipboard "system"
'@
    Write-ManagedBlock -Path $Path -Feature 'clipboard' -Content $content -CommentPrefix '//'
}
