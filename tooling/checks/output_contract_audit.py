#!/usr/bin/env python3
"""Reject catalog tools that bypass their runtime output contract."""
import json
from pathlib import Path
import re
import sys


ROOT = Path(__file__).resolve().parents[2]


def catalog_entrypoints():
    """Return unique list-visible script entrypoints from the command catalog.

    Returns:
        list[Path]: Repository-relative script paths.
    """
    catalog = json.loads((ROOT / "assets/payload/commands.json").read_text(encoding="utf-8"))
    paths = set()
    for command in catalog["commands"]:
        if command.get("visibility") != "list":
            continue
        for entrypoint in command.get("entrypoints", {}).values():
            if entrypoint[0] in ("bash-script", "powershell-script", "python-script"):
                paths.add(Path(entrypoint[1]))
    return sorted(paths)


def audit_entrypoint(path):
    """Return contract violations for one catalog entrypoint.

    Args:
        path (Path): Repository-relative entrypoint path.

    Returns:
        list[str]: Human-readable violations.
    """
    text = (ROOT / path if path.name.startswith("install-local") else ROOT / "assets/payload" / path).read_text(encoding="utf-8")
    suffix = path.suffix.lower()
    if suffix == ".sh" and not any(
        marker in text for marker in ("toolbox_output.sh", "toolbox_output.py", "tb_output_init")
    ):
        return [f"{path}: Bash entrypoint does not initialize the shared output helper"]
    if suffix == ".ps1" and "output.ps1" not in text:
        return [f"{path}: PowerShell entrypoint does not load output.ps1"]
    if suffix == ".py" and not any(
        marker in text for marker in ("TerminalOutput", "legacy_print", "run_installer", "toolbox_adapter")
    ):
        return [f"{path}: Python entrypoint does not use a native output helper"]
    return []


def audit_runtime_sources():
    """Return violations in sources whose human output must be fully wrapped.

    Returns:
        list[str]: Human-readable violations.
    """
    violations = []
    stress_paths = (
        ROOT / "assets/payload/packages/stress_gpu_runtime/stress_gpu_runtime/__main__.py",
        ROOT / "assets/payload/packages/stress_gpu_runtime/stress_gpu_runtime/config.py",
    )
    for path in stress_paths:
        for number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
            if re.search(r"\bprint\s*\(", line):
                violations.append(f"{path.relative_to(ROOT)}:{number}: raw print bypasses TerminalOutput")

    ssh_helper = ROOT / "assets/payload/packages/ssh_tools/direct_link.py"
    ssh_source = ssh_helper.read_text(encoding="utf-8")
    if "terminal_output" not in ssh_source:
        violations.append("packages/ssh_tools/direct_link.py: missing shared terminal output helper")
    for number, line in enumerate(ssh_source.splitlines(), 1):
        if re.search(r"\bprint\s*\(", line):
            violations.append(f"packages/ssh_tools/direct_link.py:{number}: raw print bypasses TerminalOutput")

    git_sync = ROOT / "internal/features/gitsync/git_sync.go"
    if 'Selection (required)' in git_sync.read_text(encoding="utf-8"):
        violations.append("internal/features/gitsync/git_sync.go: redundant selection prompt after a menu")
    for number, line in enumerate(git_sync.read_text(encoding="utf-8").splitlines(), 1):
        if re.search(r"fmt\.F(?:print|printf|println)\s*\(", line):
            violations.append(
                f"{git_sync.relative_to(ROOT)}:{number}: raw human output bypasses toolOutput"
            )

    for name in ("alacritty", "kitty"):
        setup = ROOT / f"assets/payload/packages/scripts/terminal/{name}/setup_{name}.sh"
        if re.search(r'tb_prompt\s+["\']Selection', setup.read_text(encoding="utf-8")):
            violations.append(f"{setup.relative_to(ROOT)}: redundant selection prompt after a menu")

    module_roots = (
        ROOT / "assets/payload/packages/scripts/terminal/alacritty/modules",
        ROOT / "assets/payload/packages/scripts/terminal/kitty/modules",
        ROOT / "assets/payload/packages/scripts/terminal/wsl/modules",
    )
    for module_root in module_roots:
        for path in module_root.glob("*.sh"):
            text = path.read_text(encoding="utf-8")
            if any(token in text for token in ("tb_info", "tb_warn", "tb_error")):
                if "shared.sh" not in text and path.name != "shared.sh":
                    violations.append(
                        f"{path.relative_to(ROOT)}: contract call is not backed by the module shared helper"
                    )
    return violations


def main():
    """Run the source audit and report every violation.

    Returns:
        int: Zero when all scoped sources use the contract, otherwise one.
    """
    violations = []
    for path in catalog_entrypoints() + [Path("install-local.sh"), Path("install-local.ps1")]:
        violations.extend(audit_entrypoint(path))
    violations.extend(audit_runtime_sources())
    if violations:
        sys.stderr.write("\n".join(violations) + "\n")
        return 1
    print("output contract source audit passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
