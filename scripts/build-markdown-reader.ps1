param(
    [Parameter(Mandatory = $true, Position = 0)]
    [ValidateNotNullOrEmpty()]
    [string]$OutputDirectory
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

if ($env:OS -ne 'Windows_NT' -or [Environment]::GetEnvironmentVariable('PROCESSOR_ARCHITECTURE') -ne 'AMD64') {
    throw 'Reader builds require a native x64 Windows host.'
}
if (Test-Path -LiteralPath $OutputDirectory -PathType Leaf) {
    throw 'Output directory must be a directory path.'
}
$OutputDirectory = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($OutputDirectory)
$RepositoryRoot = Split-Path -Parent $PSScriptRoot
$ManifestPath = Join-Path $RepositoryRoot 'third_party/tb-markdown-reader/Cargo.toml'
$TargetDirectory = $env:CARGO_TARGET_DIR
if ([string]::IsNullOrEmpty($TargetDirectory)) {
    $TargetDirectory = Join-Path $RepositoryRoot 'third_party/tb-markdown-reader/target'
}
$TargetDirectory = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($TargetDirectory)
[void](Get-Command cargo -ErrorAction Stop)
$Inspector = Get-Command llvm-readobj -ErrorAction SilentlyContinue
$UseLLVM = $null -ne $Inspector
if (-not $UseLLVM) {
    $Inspector = Get-Command dumpbin -ErrorAction SilentlyContinue
}
if ($null -eq $Inspector) {
    throw 'Reader build requires llvm-readobj or dumpbin to inspect PE imports.'
}

$PreviousRustFlags = $env:RUSTFLAGS
$PreviousEncodedRustFlags = $env:CARGO_ENCODED_RUSTFLAGS
try {
    # Encoded flags take precedence over RUSTFLAGS; reject this override so the
    # requested static CRT setting cannot be silently ignored.
    if (-not [string]::IsNullOrEmpty($PreviousEncodedRustFlags)) {
        throw 'Unset CARGO_ENCODED_RUSTFLAGS before building the reader.'
    }
    $env:RUSTFLAGS = "$PreviousRustFlags -C target-feature=+crt-static".Trim()
    & cargo build --locked --release --target x86_64-pc-windows-msvc --manifest-path $ManifestPath --target-dir $TargetDirectory
    if ($LASTEXITCODE -ne 0) { throw 'Reader Cargo build failed.' }
} finally {
    $env:RUSTFLAGS = $PreviousRustFlags
}
$Reader = Join-Path $TargetDirectory 'x86_64-pc-windows-msvc/release/tb-markdown-reader.exe'
$ReaderItem = Get-Item -LiteralPath $Reader
if ($ReaderItem.PSIsContainer -or $ReaderItem.Length -eq 0 -or ($ReaderItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
    throw "Cargo did not produce a regular reader executable: $Reader"
}
if ($UseLLVM) {
    $Imports = & $Inspector.Source --coff-imports $Reader 2>&1
} else {
    $Imports = & $Inspector.Source /nologo /dependents $Reader 2>&1
}
if ($LASTEXITCODE -ne 0) { throw "PE import inspection failed: $Imports" }
$ImportText = $Imports -join "`n"
if ($ImportText -notmatch '(?im)\b[\w.-]+\.dll\b') {
    throw 'PE import inspection returned no DLL names.'
}
# Static CRT binaries may still import kernel32, user32, and other Windows API
# libraries. Only dynamically linked Microsoft C/C++ runtime libraries fail.
if ($ImportText -match '(?i)\b(?:api-ms-win-crt-[\w.-]+|ucrtbase(?:d)?|msvcr[\w.-]*|msvcp[\w.-]*|vcruntime[\w.-]*|concrt[\w.-]*|vcomp[\w.-]*|vccorlib[\w.-]*)\.dll\b') {
    throw "Reader imports a dynamic VC/UCRT runtime DLL: $($Matches[0])"
}
& $Reader --tb-self-check
if ($LASTEXITCODE -ne 0) { throw 'Reader native self-check failed.' }
$LibexecDirectory = Join-Path $OutputDirectory 'libexec'
[void](New-Item -ItemType Directory -Force -Path $LibexecDirectory)
Copy-Item -LiteralPath $Reader -Destination (Join-Path $LibexecDirectory 'tb-markdown-reader.exe') -Force
