Set-StrictMode -Version 2.0

function New-ArgumentError {
    param([string]$Message)
    $exception = New-Object ArgumentException($Message)
    $exception.Data['ExitCode'] = 2
    return $exception
}

function Parse-WindowsSetupArguments {
    param([Parameter(Position = 0, ValueFromRemainingArguments = $true)][string[]]$Arguments = @())

    $skip = [ordered]@{
        Font = $false; Starship = $false; Zellij = $false; ZellijCopy = $false
        Eza = $false; Fzf = $false; FzfTab = $false; WordNavigation = $false
        GitWrapper = $false; Venv = $false; VscodeShiftEnter = $false
    }
    $assumeYes = $false
    $showHelp = $false
    $autoStartZellij = $false

    foreach ($argument in $Arguments) {
        switch ($argument) {
            { $_ -in @('-y', '--yes') } { $assumeYes = $true; break }
            '--autostart-zellij' { $autoStartZellij = $true; break }
            '--skip-font' { $skip.Font = $true; break }
            '--skip-starship' { $skip.Starship = $true; break }
            '--skip-zellij' { $skip.Zellij = $true; break }
            '--skip-zellij-copy' { $skip.ZellijCopy = $true; break }
            '--skip-eza' { $skip.Eza = $true; break }
            '--skip-fzf' { $skip.Fzf = $true; break }
            '--skip-fzf-tab' { $skip.FzfTab = $true; break }
            '--skip-word-navigation' { $skip.WordNavigation = $true; break }
            '--skip-git-wrapper' { $skip.GitWrapper = $true; break }
            '--skip-venv' { $skip.Venv = $true; break }
            '--skip-vscode-shift-enter' { $skip.VscodeShiftEnter = $true; break }
            { $_ -in @('-h', '--help') } { $showHelp = $true; break }
            default { throw (New-ArgumentError "Unknown argument: $argument") }
        }
    }

    return [pscustomobject]@{
        AssumeYes       = $assumeYes
        ShowHelp        = $showHelp
        AutoStartZellij = $autoStartZellij
        Skip             = $skip
    }
}

function Select-WindowsFeatures {
    param(
        [Parameter(Mandatory = $true)]$Options,
        [scriptblock]$Prompt = ${function:Read-YesNo}
    )

    $labels = [ordered]@{
        Font = 'FiraCode Nerd Font Mono'
        Starship = 'Starship prompt'
        Zellij = 'Zellij'
        ZellijCopy = 'Zellij system clipboard integration'
        Eza = 'eza ls replacement'
        Fzf = 'fzf'
        FzfTab = 'PSFzf-backed Tab completion'
        WordNavigation = 'Ctrl+Left/Right word navigation'
        GitWrapper = 'git clone auto-directory wrapper'
        Venv = 'parent-directory virtual environment helper'
        VscodeShiftEnter = 'VS Code terminal Shift+Enter binding'
    }
    $selected = [ordered]@{}
    foreach ($feature in $labels.Keys) {
        if ($Options.Skip[$feature]) { $selected[$feature] = $false }
        elseif ($Options.AssumeYes) { $selected[$feature] = $true }
        else { $selected[$feature] = [bool](& $Prompt "Enable $($labels[$feature])?" $true) }
    }

    if (-not $selected.Zellij) {
        $selected.ZellijCopy = $false
        $selected.AutoStartZellij = $false
    }
    elseif ($Options.AutoStartZellij) {
        $selected.AutoStartZellij = $true
    }
    elseif ($Options.AssumeYes) {
        $selected.AutoStartZellij = $false
    }
    else {
        $selected.AutoStartZellij = [bool](& $Prompt 'Auto-start Zellij in interactive Windows Terminal sessions?' $false)
    }

    if (-not $selected.Fzf) { $selected.FzfTab = $false }
    return $selected
}

function Get-WindowsSetupUsage {
    return @'
Usage: powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\setup_windows.ps1 [OPTIONS]

Options:
  -y, --yes                    Enable all compatible optional features
  --autostart-zellij           Auto-start Zellij in Windows Terminal
  --skip-font                  Skip FiraCode Nerd Font installation
  --skip-starship              Skip Starship
  --skip-zellij               Skip Zellij, clipboard, and auto-start setup
  --skip-zellij-copy          Skip Zellij clipboard setup
  --skip-eza                   Skip eza
  --skip-fzf                   Skip fzf, PSFzf, and fzf-tab setup
  --skip-fzf-tab               Skip PSFzf-backed Tab completion
  --skip-word-navigation       Skip Ctrl+Left/Right bindings
  --skip-git-wrapper           Skip the Git clone wrapper
  --skip-venv                  Skip the venv helper
  --skip-vscode-shift-enter    Skip the VS Code terminal binding
  -h, --help                   Show this help and exit
'@
}
