Set-StrictMode -Version 2.0

function New-WindowsConfigBackup {
    param(
        [Parameter(Mandatory = $true)]$PathMap,
        [string]$BackupRoot = (Join-Path $env:LOCALAPPDATA 'project-template\windows-backups'),
        [string]$Timestamp = (Get-Date -Format 'yyyyMMdd-HHmmss')
    )

    [IO.Directory]::CreateDirectory($BackupRoot) | Out-Null
    $backupPath = Join-Path $BackupRoot $Timestamp
    $suffix = 0
    while ([IO.Directory]::Exists($backupPath)) {
        $suffix++
        $backupPath = Join-Path $BackupRoot ("$Timestamp-$PID-$suffix")
    }
    [IO.Directory]::CreateDirectory($backupPath) | Out-Null

    foreach ($relativePath in $PathMap.Keys) {
        $sourcePath = [string]$PathMap[$relativePath]
        if ([string]::IsNullOrWhiteSpace($sourcePath) -or -not [IO.File]::Exists($sourcePath)) { continue }
        $destinationPath = Join-Path $backupPath $relativePath
        [IO.Directory]::CreateDirectory((Split-Path -Parent $destinationPath)) | Out-Null
        [IO.File]::Copy($sourcePath, $destinationPath, $true)
    }
    return $backupPath
}
