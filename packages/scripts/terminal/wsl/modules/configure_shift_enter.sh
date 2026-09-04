#!/usr/bin/env bash
set -euo pipefail

module_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
powershell_path="$(wslpath -u 'C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe')"

if [[ ! -x "$powershell_path" ]]; then
    printf '%s\n' 'WARNING: Windows PowerShell is unavailable; Shift+Enter host bindings were skipped.' >&2
    exit 0
fi

script_path="$(wslpath -w "$module_dir/configure_shift_enter.ps1")"
"$powershell_path" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$script_path"
