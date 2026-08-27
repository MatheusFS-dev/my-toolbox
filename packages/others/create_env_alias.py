"""Create persistent Bash and Zsh aliases for Python virtual environments."""

import datetime
import os
import re
import shutil
import sys
import tempfile
from pathlib import Path

ALIAS_PATTERN_TEMPLATE = r"^[ \t]*alias[ \t]+{name}=.*$"


def resolve_venv(input_path: Path) -> Path:
    """Resolve and validate a venv root or a project containing .venv.

    Args:
        input_path (Path): Existing venv root or project directory. A venv must
            contain bin/activate and bin/python; a project must contain a valid
            .venv child with those files.

    Returns:
        Path: Absolute, symlink-resolved venv root.

    Raises:
        FileNotFoundError: If input_path is not an existing directory.
        ValueError: If neither input_path nor input_path/.venv is a valid venv.

    Examples:
        resolve_venv(Path("/work/project")) returns /work/project/.venv when the
        project contains a valid virtual environment.
    """
    path = input_path.expanduser()
    if not path.is_dir():
        raise FileNotFoundError(f"Path does not exist or is not a directory: {path}")
    path = path.resolve()
    candidates = [path, path / ".venv"]
    for candidate in candidates:
        python_path = candidate / "bin" / "python"
        if (
            (candidate / "bin" / "activate").is_file()
            and python_path.is_file()
            and os.access(python_path, os.X_OK)
        ):
            return candidate.resolve()
    raise ValueError(
        f"Expected bin/activate and executable bin/python in {path} or {path / '.venv'}"
    )


def validate_alias_name(alias_name: str) -> None:
    """Validate one shell alias name.

    Args:
        alias_name (str): Candidate name. It must start with a letter or
            underscore and then contain only letters, digits, underscores, or
            hyphens.

    Returns:
        None.

    Raises:
        ValueError: If alias_name does not satisfy the accepted syntax.
    """
    if re.fullmatch(r"[A-Za-z_][A-Za-z0-9_-]*", alias_name) is None:
        raise ValueError(
            "Alias name must start with a letter or underscore and contain only "
            "letters, digits, underscores, and hyphens"
        )


def shell_single_quote(value: str) -> str:
    """Quote a literal value for a POSIX shell single-quoted string.

    Args:
        value (str): Literal text to quote. Embedded single quotes are preserved
            through the standard close-quote, escaped-quote, reopen sequence.

    Returns:
        str: Shell-safe single-quoted representation.

    Raises:
        TypeError: If value is not a string.

    Examples:
        shell_single_quote("a b") returns a single-quoted value containing a b.
    """
    if not isinstance(value, str):
        raise TypeError("value must be a string")
    return "'" + value.replace("'", "'\"'\"'") + "'"


def prepare_alias_updates(
    venv: Path,
    alias_name: str,
    shells: list[str],
    home: Path,
) -> tuple[dict[Path, str], list[Path]]:
    """Prepare shell file contents without mutating the filesystem.

    Args:
        venv (Path): Validated venv root containing bin/activate.
        alias_name (str): Validated public alias name.
        shells (list[str]): Explicit shell selection containing bash, zsh, or
            both values. Each selected shell maps to .bashrc or .zshrc.
        home (Path): Home directory containing the selected shell files. Missing
            shell files are planned as new files.

    Returns:
        tuple[dict[Path, str], list[Path]]: Planned complete file contents and
        sorted files whose alias definitions would be replaced.

    Raises:
        ValueError: If venv is invalid, alias_name is invalid, or shells contains
            unsupported or duplicate values.
        OSError: If an existing shell file cannot be read.

    Examples:
        prepare_alias_updates(venv, "ml", ["bash"], home) plans only .bashrc.
    """
    validate_alias_name(alias_name)
    python_path = venv / "bin" / "python"
    if (
        not (venv / "bin" / "activate").is_file()
        or not python_path.is_file()
        or not os.access(python_path, os.X_OK)
    ):
        raise ValueError(f"Invalid venv root: {venv}")
    if not shells or len(shells) != len(set(shells)):
        raise ValueError("Shell selection must be explicit and contain no duplicates")
    unsupported = sorted(set(shells) - {"bash", "zsh"})
    if unsupported:
        raise ValueError(f"Unsupported shell selection: {', '.join(unsupported)}")

    activation_path = venv.resolve() / "bin" / "activate"
    activation_command = "source " + shell_single_quote(str(activation_path))
    alias_line = f"alias {alias_name}=" + shell_single_quote(activation_command)
    pattern = re.compile(
        ALIAS_PATTERN_TEMPLATE.format(name=re.escape(alias_name)),
        flags=re.MULTILINE,
    )
    shell_files = {"bash": home / ".bashrc", "zsh": home / ".zshrc"}
    updates: dict[Path, str] = {}
    conflicts: list[Path] = []
    for shell in shells:
        path = shell_files[shell]
        if path.is_symlink():
            raise ValueError(f"Shell configuration path must not be a symlink: {path}")
        original = path.read_text(encoding="utf-8") if path.exists() else ""
        if pattern.search(original):
            conflicts.append(path)
            updated = pattern.sub(lambda _: alias_line, original)
        else:
            separator = "" if not original or original.endswith("\n") else "\n"
            updated = (
                original
                + separator
                + f"\n# Python environment alias: {alias_name}\n{alias_line}\n"
            )
        updates[path] = updated
    return updates, sorted(conflicts)


def atomic_write_text(path: Path, content: str) -> None:
    """Atomically replace one UTF-8 text file in its own directory.

    Args:
        path (Path): Destination file. Existing permission bits are preserved;
            new files use normal user umask permissions.
        content (str): Complete UTF-8 file content.

    Returns:
        None.

    Raises:
        ValueError: If path is a symbolic link. Configuration targets must be
            regular files so replacement cannot silently remove a link.
        OSError: If the parent, temporary write, permission update, or atomic
            replacement fails.
    """
    if path.is_symlink():
        raise ValueError(f"Shell configuration path must not be a symlink: {path}")
    path.parent.mkdir(parents=True, exist_ok=True)
    existing_mode = path.stat().st_mode & 0o777 if path.exists() else None
    descriptor, temporary_name = tempfile.mkstemp(
        prefix=f".{path.name}.",
        dir=path.parent,
        text=True,
    )
    temporary_path = Path(temporary_name)
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8", newline="") as stream:
            stream.write(content)
            stream.flush()
            os.fsync(stream.fileno())
        if existing_mode is not None:
            temporary_path.chmod(existing_mode)
        else:
            current_umask = os.umask(0)
            os.umask(current_umask)
            temporary_path.chmod(0o666 & ~current_umask)
        os.replace(temporary_path, path)
    finally:
        if temporary_path.exists():
            temporary_path.unlink()


def apply_alias_updates(
    updates: dict[Path, str],
    create_backups: bool,
) -> list[Path]:
    """Apply prepared alias updates atomically per file.

    Args:
        updates (dict[Path, str]): Complete destination contents returned by
            prepare_alias_updates.
        create_backups (bool): When true, copy each existing destination to a
            unique timestamped sibling before any update. When false, no backups
            are created.

    Returns:
        list[Path]: Backup paths created in destination order.

    Raises:
        FileExistsError: If a generated backup name already exists.
        OSError: If backup creation or an atomic file update fails. Each file
            replacement is atomic, but this function does not promise rollback
            across multiple shell files.
    """
    backups: list[Path] = []
    timestamp = datetime.datetime.now(datetime.timezone.utc).strftime(
        "%Y%m%d-%H%M%S-%f"
    )
    if create_backups:
        for path in updates:
            if not path.exists():
                continue
            backup = path.with_name(f"{path.name}.bak.{timestamp}")
            if backup.exists():
                raise FileExistsError(f"Backup path already exists: {backup}")
            shutil.copy2(path, backup)
            backups.append(backup)
    for path, content in updates.items():
        atomic_write_text(path, content)
    return backups


def prompt_yes_no(question: str, default: bool | None) -> bool:
    """Prompt for one explicit yes/no answer.

    Args:
        question (str): Prompt text without the answer suffix.
        default (bool | None): True makes Enter mean yes, False makes Enter mean
            no, and None rejects Enter and requires an explicit answer.

    Returns:
        bool: True for yes and false for no.

    Raises:
        ValueError: If the answer is empty without a default or is not yes/no.
        EOFError: If interactive input closes before an answer.
    """
    suffix = " [Y/n]" if default is True else " [y/N]" if default is False else " [y/n]"
    answer = input(question + suffix + ": ").strip().lower()
    if not answer:
        if default is None:
            raise ValueError("An explicit yes/no answer is required")
        return default
    if answer in {"y", "yes"}:
        return True
    if answer in {"n", "no"}:
        return False
    raise ValueError(f"Invalid yes/no answer: {answer}")


def run_interactive() -> None:
    """Collect alias settings, preview changes, and apply confirmed updates.

    Args:
        None.

    Returns:
        None.

    Raises:
        FileNotFoundError: If the selected path does not exist.
        ValueError: If venv, alias, shell, or confirmation input is invalid.
        OSError: If shell files or backups cannot be read or written.
        EOFError: If interactive input closes before all required answers.
    """
    path_answer = input("Venv root or project containing .venv: ").strip()
    if not path_answer:
        raise ValueError("An explicit venv or project path is required")
    venv = resolve_venv(Path(path_answer))
    default_alias = venv.parent.name if venv.name == ".venv" else venv.name
    alias_name = input(f"Alias name [{default_alias}]: ").strip() or default_alias
    shell_choice = input("Shell selection (bash, zsh, or both): ").strip().lower()
    shell_map = {"bash": ["bash"], "zsh": ["zsh"], "both": ["bash", "zsh"]}
    if shell_choice not in shell_map:
        raise ValueError("Shell selection must be bash, zsh, or both")
    updates, conflicts = prepare_alias_updates(
        venv,
        alias_name,
        shell_map[shell_choice],
        Path.home(),
    )
    activation = "source " + shell_single_quote(str(venv / "bin" / "activate"))
    print("\nPreview:")
    print(f"  Alias: {alias_name}")
    print(f"  Command: {activation}")
    print("  Shell files:")
    for path in updates:
        print(f"    - {path}")
    if conflicts:
        print("Existing alias definitions:")
        for path in conflicts:
            print(f"  - {path}")
        if not prompt_yes_no("Replace the existing alias definitions?", None):
            print("No changes made.")
            return
        create_backups = prompt_yes_no("Create backups before replacement?", True)
    else:
        create_backups = False
        if not prompt_yes_no("Apply these changes?", None):
            print("No changes made.")
            return
    backups = apply_alias_updates(updates, create_backups)
    for backup in backups:
        print(f"[INFO] Backup created: {backup}")
    for path in updates:
        print(f"[OK] Updated {path}")


if __name__ == "__main__":
    if len(sys.argv) != 1:
        raise SystemExit("create-env-alias does not accept arguments")
    run_interactive()
