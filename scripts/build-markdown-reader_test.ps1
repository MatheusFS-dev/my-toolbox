$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot 'build-markdown-reader-checks.ps1')

function Assert-Rejected {
    param([scriptblock]$Action, [string]$ErrorPattern)

    try {
        & $Action
    } catch {
        if ($_.Exception.Message -notmatch $ErrorPattern) { throw }
        return
    }
    throw 'Expected reader validation to reject this input.'
}

$Failures = [System.Collections.Generic.List[string]]::new()
$Checks = 0
function Invoke-Check {
    param([string]$Name, [scriptblock]$Action)

    $script:Checks++
    try { & $Action } catch { $Failures.Add("${Name}: $($_.Exception.Message)") }
}

# Exercise the production validators with deterministic inspector output. No
# Rust build, PE binary, inspector installation, or inherited flags are needed.
$SystemDLLs = @(
    'KERNEL32.dll', 'USER32.dll', 'ADVAPI32.dll', 'SHELL32.dll', 'ole32.dll',
    'WS2_32.dll', 'bcrypt.dll', 'ntdll.dll', 'api-ms-win-core-synch-l1-2-0.dll'
)
foreach ($DLL in $SystemDLLs) {
    Invoke-Check "allow LLVM system import $DLL" {
        Assert-ReaderStaticRuntimeImports "Import {`n  Name: $DLL`n  Symbol: WindowsAPI (0)`n}"
    }
    Invoke-Check "allow dumpbin system import $DLL" {
        Assert-ReaderStaticRuntimeImports "Image has the following dependencies:`n    $DLL`n"
    }
}
$RuntimeDLLs = @(
    'msvcm90.dll', 'MSVCM80D.DLL', 'msvcr120.dll', 'msvcp140.dll',
    'msvcp140_1.dll', 'msvcp140_atomic_wait.dll', 'msvcp140_codecvt_ids.dll',
    'vcruntime140.dll', 'vcruntime140_1.dll', 'VCRUNTIME140D.DLL',
    'ucrtbase.dll', 'ucrtbased.dll', 'api-ms-win-crt-runtime-l1-1-0.dll',
    'concrt140.dll', 'vcomp140.dll', 'vccorlib140.dll', 'vcamp140.dll',
    'mfc140.dll', 'mfc140u.dll', 'mfc140ud.dll', 'mfc140chs.dll',
    'mfcm140.dll', 'mfcm140u.dll', 'mfcmifc80.dll', 'atl90.dll'
)
foreach ($DLL in $RuntimeDLLs) {
    Invoke-Check "reject LLVM runtime import $DLL" {
        Assert-Rejected {
            Assert-ReaderStaticRuntimeImports "Import {`n  Name: KERNEL32.dll`n}`nImport {`n  Name: $DLL`n}"
        } 'Reader imports a dynamic VC/UCRT runtime DLL:'
    }
    Invoke-Check "reject dumpbin runtime import $DLL" {
        Assert-Rejected {
            Assert-ReaderStaticRuntimeImports "Image has the following dependencies:`n    KERNEL32.dll`n    $DLL`n"
        } 'Reader imports a dynamic VC/UCRT runtime DLL:'
    }
}
Invoke-Check 'reject empty import inspection' {
    Assert-Rejected { Assert-ReaderStaticRuntimeImports '' } 'PE import inspection returned no DLL names'
}
Invoke-Check 'allow absent encoded flags' {
    Assert-ReaderRustFlagsEnvironment @{ RUSTFLAGS = '-C opt-level=3' }
}
# Supplying a process-environment snapshot also represents an explicitly empty
# variable on PowerShell versions where setting an empty env value removes it.
foreach ($EncodedFlags in @('', '-C target-feature=-crt-static')) {
    Invoke-Check "reject present encoded flags [$EncodedFlags]" {
        Assert-Rejected {
            Assert-ReaderRustFlagsEnvironment @{ CARGO_ENCODED_RUSTFLAGS = $EncodedFlags }
        } 'Unset CARGO_ENCODED_RUSTFLAGS'
    }
}
if ($Failures.Count -gt 0) {
    throw "$($Failures.Count) of $Checks reader checks failed:`n$($Failures -join "`n")"
}
Write-Output "$Checks reader checks passed."
