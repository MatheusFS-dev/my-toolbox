$ErrorActionPreference = 'Stop'
$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$moduleRoot = Join-Path $scriptRoot 'modules'

foreach ($module in @(
    'output.ps1', 'shared.ps1', 'arguments.ps1', 'preflight.ps1', 'packages.ps1', 'backup.ps1',
    'fonts.ps1', 'terminal.ps1', 'starship.ps1', 'powershell_profile.ps1', 'zellij.ps1', 'vscode.ps1',
    'installer.ps1'
)) {
    . (Join-Path $moduleRoot $module)
}
Initialize-ToolboxOutput -ToolName 'setup-windows'

if ($MyInvocation.InvocationName -ne '.') {
    try {
        Write-ToolboxInfo 'Checking Windows terminal prerequisites...'
        $result = @(Invoke-WindowsTerminalSetup -Arguments $args)
        if ($result.Count -eq 0) { $exitCode = 0 }
        else {
            $exitCode = [int]$result[-1]
            if ($result.Count -gt 1) { $result[0..($result.Count - 2)] | Write-Output }
        }
        if ($exitCode -eq 0) { Write-ToolboxSuccess 'Windows terminal setup completed successfully.' }
        exit $exitCode
    }
    catch {
        $exitCode = 1
        if ($_.Exception.Data.Contains('ExitCode')) { $exitCode = [int]$_.Exception.Data['ExitCode'] }
        Write-ToolboxError "Windows terminal setup failed: $($_.Exception.Message). Correct the reported issue and retry; earlier completed changes may remain."
        if ($exitCode -eq 2) { Write-ToolboxError (Get-WindowsSetupUsage) }
        exit $exitCode
    }
}
