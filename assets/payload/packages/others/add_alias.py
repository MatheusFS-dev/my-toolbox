"""Add persistent shell aliases to managed Bash and Zsh configuration blocks."""

import os
import re
import sys
import tempfile
from pathlib import Path
from typing import Optional

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from toolbox_output import TerminalOutput


BLOCK_START = "# >>> TB ALIASES >>>"
BLOCK_END = "# <<< TB ALIASES <<<"
ALIAS_NAME_PATTERN = re.compile(r"[A-Za-z_][A-Za-z0-9_-]*\Z")


def validate_alias_name(alias_name: str) -> None:
    """Validate an alias name accepted by Bash and Zsh.

    Args:
        alias_name: Candidate alias name.

    Raises:
        ValueError: If the name has unsupported shell syntax.
    """
    if ALIAS_NAME_PATTERN.fullmatch(alias_name) is None:
        raise ValueError(
            "Alias name must start with a letter or underscore and contain only "
            "letters, digits, underscores, and hyphens."
        )


def validate_alias_command(command: str) -> None:
    """Validate one command that can be represented by a shell alias.

    Args:
        command: Command text entered by the user.

    Raises:
        ValueError: If the command is empty or spans multiple lines.
    """
    if not command:
        raise ValueError("Alias command is required.")
    if "\x00" in command or "\n" in command or "\r" in command:
        raise ValueError("Alias command must be a single line.")


def shell_single_quote(value: str) -> str:
    """Return a POSIX-shell single-quoted literal.

    Args:
        value: Literal value to quote.

    Returns:
        A shell expression that expands to value.
    """
    return "'" + value.replace("'", "'\"'\"'") + "'"


def prepare_alias_update(content: str, alias_name: str, command: str) -> str:
    """Add or replace an alias inside one managed ``TB ALIASES`` block.

    Args:
        content: Existing shell configuration text.
        alias_name: Valid alias name to add or update.
        command: Valid single-line command for the alias.

    Returns:
        Updated shell configuration text with exactly one managed alias block.

    Raises:
        ValueError: If input is invalid or the existing managed block is malformed.
    """
    validate_alias_name(alias_name)
    validate_alias_command(command)
    newline = "\r\n" if "\r\n" in content else "\n"
    alias_line = "alias {0}={1}".format(alias_name, shell_single_quote(command))
    lines = content.splitlines()
    starts = [index for index, line in enumerate(lines) if line == BLOCK_START]
    ends = [index for index, line in enumerate(lines) if line == BLOCK_END]

    if not starts and not ends:
        suffix = content
        if suffix and not suffix.endswith(("\n", "\r")):
            suffix += newline
        if suffix and not suffix.endswith(newline * 2):
            suffix += newline
        return suffix + newline.join([BLOCK_START, alias_line, BLOCK_END]) + newline
    if len(starts) != 1 or len(ends) != 1 or starts[0] >= ends[0]:
        raise ValueError("Existing TB ALIASES block is malformed.")

    alias_pattern = re.compile(
        r"^[ \t]*alias[ \t]+{0}=.*$".format(re.escape(alias_name))
    )
    managed_lines = []
    replaced = False
    for line in lines[starts[0] + 1 : ends[0]]:
        if alias_pattern.fullmatch(line):
            if not replaced:
                managed_lines.append(alias_line)
                replaced = True
            continue
        managed_lines.append(line)
    if not replaced:
        managed_lines.append(alias_line)

    updated_lines = (
        lines[: starts[0] + 1]
        + managed_lines
        + lines[ends[0] :]
    )
    return newline.join(updated_lines) + newline


def prepare_alias_updates(
    alias_name: str,
    command: str,
    shells: list[str],
    home: Path,
) -> dict[Path, str]:
    """Prepare complete shell configuration updates without writing them.

    Args:
        alias_name: Alias name to add or update.
        command: Single-line alias command.
        shells: Selected shell names, using ``bash`` and/or ``zsh``.
        home: Home directory that contains the shell configuration files.

    Returns:
        Mapping of selected shell configuration paths to their updated text.

    Raises:
        ValueError: If selections are invalid or a shell configuration is a symlink.
        OSError: If an existing configuration cannot be read.
    """
    validate_alias_name(alias_name)
    validate_alias_command(command)
    if not shells or len(shells) != len(set(shells)):
        raise ValueError("Select Bash, Zsh, or both shell configuration files.")
    unsupported = sorted(set(shells) - {"bash", "zsh"})
    if unsupported:
        raise ValueError("Unsupported shell selection: {0}".format(", ".join(unsupported)))

    paths = {"bash": home / ".bashrc", "zsh": home / ".zshrc"}
    updates = {}
    for shell in shells:
        path = paths[shell]
        if path.is_symlink():
            raise ValueError("Shell configuration path must not be a symlink: {0}".format(path))
        original = path.read_text(encoding="utf-8") if path.exists() else ""
        updates[path] = prepare_alias_update(original, alias_name, command)
    return updates


def atomic_write_text(path: Path, content: str) -> None:
    """Atomically replace a regular UTF-8 shell configuration file.

    Args:
        path: Destination shell configuration file.
        content: Complete updated file content.

    Raises:
        ValueError: If path is a symbolic link.
        OSError: If the replacement cannot be completed.
    """
    if path.is_symlink():
        raise ValueError("Shell configuration path must not be a symlink: {0}".format(path))
    path.parent.mkdir(parents=True, exist_ok=True)
    existing_mode = path.stat().st_mode & 0o777 if path.exists() else None
    descriptor, temporary_name = tempfile.mkstemp(prefix=".{0}.".format(path.name), dir=path.parent)
    temporary_path = Path(temporary_name)
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8", newline="") as stream:
            stream.write(content)
            stream.flush()
            os.fsync(stream.fileno())
        if existing_mode is not None:
            temporary_path.chmod(existing_mode)
        os.replace(temporary_path, path)
    finally:
        if temporary_path.exists():
            temporary_path.unlink()


def run_interactive(
    home: Optional[Path] = None,
    output=None,
    input_function=None,
) -> None:
    """Prompt for an alias and optionally write managed Bash and Zsh updates.

    Args:
        home: Optional home-directory override for testing.
        output: Optional toolbox terminal output helper.
        input_function: Optional no-argument input reader.

    Raises:
        OSError: If selected shell configuration files cannot be read or written.
        ValueError: If existing managed configuration is malformed.
    """
    output = output or TerminalOutput(sys.stdout, "add-alias")
    reader = input_function or input
    while True:
        alias_name = output.ask_required("Alias name (required)", reader)
        try:
            validate_alias_name(alias_name)
        except ValueError as error:
            output.warn(str(error))
            continue
        break
    while True:
        command = output.ask_required("Alias command (required)", reader)
        try:
            validate_alias_command(command)
        except ValueError as error:
            output.warn(str(error))
            continue
        break

    shells = []
    if output.ask_yes_no("Add this alias to ~/.bashrc?", False, reader):
        shells.append("bash")
    if output.ask_yes_no("Add this alias to ~/.zshrc?", False, reader):
        shells.append("zsh")
    if not shells:
        output.info("No shell configuration files selected. No changes made.")
        return

    updates = prepare_alias_updates(alias_name, command, shells, home or Path.home())
    output.info("Updating managed TB ALIASES blocks...")
    for path, content in updates.items():
        atomic_write_text(path, content)
    output.success("Added alias {0} to {1}.".format(alias_name, ", ".join(str(path) for path in updates)))


if __name__ == "__main__":
    output = TerminalOutput(sys.stdout, "add-alias")
    if len(sys.argv) != 1:
        output.error("add-alias does not accept command-line arguments.")
        raise SystemExit(2)
    run_interactive(output=output)
