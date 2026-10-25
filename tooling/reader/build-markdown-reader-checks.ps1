function Assert-ReaderRustFlagsEnvironment {
    param(
        [System.Collections.IDictionary]$EnvironmentVariables = [Environment]::GetEnvironmentVariables()
    )

    # Cargo uses encoded flags whenever the variable exists, including an
    # explicitly empty value. Windows variable names are case-insensitive even
    # when the dictionary returned by GetEnvironmentVariables is case-sensitive.
    foreach ($VariableName in $EnvironmentVariables.Keys) {
        if ($VariableName -ieq 'CARGO_ENCODED_RUSTFLAGS') {
            throw 'Unset CARGO_ENCODED_RUSTFLAGS before building the reader.'
        }
    }
}

function Assert-ReaderStaticRuntimeImports {
    param([string]$ImportText)

    if ($ImportText -notmatch '(?im)\b[\w.-]+\.dll\b') {
        throw 'PE import inspection returned no DLL names.'
    }
    # Include legacy managed CRT, AMP, MFC (including managed interfaces and
    # language resources), and versioned ATL alongside VC/UCRT libraries.
    $RuntimeFamilies = @(
        'api-ms-win-crt-[\w.-]+', 'ucrtbase(?:d)?',
        '(?:msvc[rpm]|vcruntime|concrt|vcomp|vccorlib|vcamp)[\w.-]*',
        'mfc(?:m(?:ifc)?)?[0-9][\w.-]*', 'atl[0-9][\w.-]*'
    )
    $RuntimePattern = '(?i)\b(?:' + ($RuntimeFamilies -join '|') + ')\.dll\b'
    if ($ImportText -match $RuntimePattern) {
        throw "Reader imports a dynamic VC/UCRT runtime DLL: $($Matches[0])"
    }
}
