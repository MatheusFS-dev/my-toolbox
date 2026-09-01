"""Copy the packaged project template recursively into an existing directory."""

import os
import shutil
import stat
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Optional


@dataclass(frozen=True)
class TemplateEntry:
    """Describe one dynamically discovered template entry.

    Attributes:
        relative_path (Path): Path relative to the packaged template root.
        kind (str): One of directory, file, or symlink.
    """

    relative_path: Path
    kind: str


def discover_template(source: Path) -> list[TemplateEntry]:
    """Discover every directory, file, and symlink in a template tree.

    Args:
        source (Path): Existing template root. Discovery reads the filesystem on
            every call, includes dotfiles and empty directories, and does not
            follow directory symlinks.

    Returns:
        list[TemplateEntry]: Entries sorted by relative POSIX path.

    Raises:
        FileNotFoundError: If source is not an existing directory.
        OSError: If a directory cannot be scanned.

    Examples:
        discover_template(Path("template")) includes newly committed files
        without requiring a manifest update.
    """
    if not source.is_dir():
        raise FileNotFoundError(f"Template directory does not exist: {source}")
    entries: list[TemplateEntry] = []

    def visit(directory: Path) -> None:
        """Recursively scan one real directory without following symlinks.

        Args:
            directory (Path): Directory under source to scan.

        Returns:
            None.

        Raises:
            OSError: If directory entries cannot be inspected.
        """
        with os.scandir(directory) as children:
            for child in sorted(children, key=lambda item: item.name):
                path = Path(child.path)
                relative = path.relative_to(source)
                if child.is_symlink():
                    entries.append(TemplateEntry(relative, "symlink"))
                elif child.is_dir(follow_symlinks=False):
                    entries.append(TemplateEntry(relative, "directory"))
                    visit(path)
                elif child.is_file(follow_symlinks=False):
                    entries.append(TemplateEntry(relative, "file"))
                else:
                    raise ValueError(f"Unsupported template entry type: {relative}")

    visit(source)
    return sorted(entries, key=lambda entry: entry.relative_path.as_posix())


def path_kind(path: Path) -> Optional[str]:
    """Return the copy-relevant type for one destination path.

    Args:
        path (Path): Destination candidate. Broken symlinks are recognized as
            symlinks instead of treated as missing.

    Returns:
        Optional[str]: directory, file, symlink, or None when missing.

    Raises:
        ValueError: If path exists with an unsupported filesystem type.
        OSError: If path metadata cannot be read.
    """
    if path.is_symlink():
        return "symlink"
    if not path.exists():
        return None
    if path.is_dir():
        return "directory"
    if path.is_file():
        return "file"
    raise ValueError(f"Unsupported destination entry type: {path}")


def preflight_copy(
    source: Path,
    destination: Path,
    entries: list[TemplateEntry],
) -> list[Path]:
    """Validate a recursive merge and list same-type overwrite conflicts.

    Args:
        source (Path): Existing packaged template root.
        destination (Path): Explicit existing destination directory.
        entries (list[TemplateEntry]): Fresh discovery result for source.

    Returns:
        list[Path]: Sorted relative file and symlink conflicts. Existing
        directories are merge points rather than overwrite conflicts.

    Raises:
        FileNotFoundError: If source or destination is not an existing directory.
        ValueError: If source and destination overlap or any source/destination
            entry has a file, directory, or symlink type mismatch.
        OSError: If path resolution or metadata inspection fails.

    Examples:
        preflight_copy(source, destination, discover_template(source)) validates
        the complete tree before the first copy.
    """
    if not source.is_dir():
        raise FileNotFoundError(f"Template directory does not exist: {source}")
    if not destination.is_dir():
        raise FileNotFoundError(
            f"Destination does not exist or is not a directory: {destination}"
        )
    resolved_source = source.resolve()
    resolved_destination = destination.resolve()
    if (
        resolved_source == resolved_destination
        or resolved_source in resolved_destination.parents
        or resolved_destination in resolved_source.parents
    ):
        raise ValueError(
            f"Source and destination overlap: {resolved_source} and "
            f"{resolved_destination}"
        )

    conflicts: list[Path] = []
    mismatches: list[Path] = []
    for entry in entries:
        destination_path = resolved_destination / entry.relative_path
        destination_kind = path_kind(destination_path)
        if destination_kind is None:
            continue
        if destination_kind != entry.kind:
            mismatches.append(entry.relative_path)
        elif entry.kind in {"file", "symlink"}:
            conflicts.append(entry.relative_path)
    if mismatches:
        listed = ", ".join(path.as_posix() for path in sorted(mismatches))
        raise ValueError(f"Source/destination type mismatch: {listed}")
    return sorted(conflicts)


def copy_template(
    source: Path,
    destination: Path,
    entries: list[TemplateEntry],
    overwrite: bool,
) -> None:
    """Merge a preflighted template tree into a destination.

    Args:
        source (Path): Packaged template root used to discover entries.
        destination (Path): Existing destination directory.
        entries (list[TemplateEntry]): Preflighted dynamic discovery result.
        overwrite (bool): True replaces existing same-type files and symlinks;
            False rejects such conflicts. Directories always merge, missing
            entries are always copied, and destination-only paths are untouched.

    Returns:
        None.

    Raises:
        FileExistsError: If overwrite is false and a same-type conflict exists.
        ValueError: If an entry kind is unsupported.
        OSError: If directory creation, copying, chmod, unlinking, or symlink
            creation fails. Completed copies are not rolled back.

    Examples:
        copy_template(source, destination, entries, overwrite=True) applies all
        confirmed same-type replacements.
    """
    for entry in sorted(
        (item for item in entries if item.kind == "directory"),
        key=lambda item: len(item.relative_path.parts),
    ):
        source_path = source / entry.relative_path
        destination_path = destination / entry.relative_path
        if not destination_path.exists():
            destination_path.mkdir()
            destination_path.chmod(stat.S_IMODE(source_path.stat().st_mode))

    for entry in (item for item in entries if item.kind != "directory"):
        source_path = source / entry.relative_path
        destination_path = destination / entry.relative_path
        destination_path.parent.mkdir(parents=True, exist_ok=True)
        existing_kind = path_kind(destination_path)
        if existing_kind is not None and not overwrite:
            raise FileExistsError(f"Destination conflict: {entry.relative_path}")
        if entry.kind == "file":
            shutil.copy2(source_path, destination_path)
        elif entry.kind == "symlink":
            if existing_kind is not None:
                destination_path.unlink()
            target = os.readlink(source_path)
            target_path = source_path.parent / target
            os.symlink(
                target, destination_path, target_is_directory=target_path.is_dir()
            )
        else:
            raise ValueError(f"Unsupported template entry kind: {entry.kind}")


def prompt_yes_no(question: str) -> bool:
    """Prompt for an explicit yes/no answer without an Enter default.

    Args:
        question (str): Prompt text without the answer suffix.

    Returns:
        bool: True for yes and false for no.

    Raises:
        ValueError: If the answer is empty or not yes/no.
        EOFError: If input closes before an answer.
    """
    answer = input(question + " [y/n]: ").strip().lower()
    if answer in {"y", "yes"}:
        return True
    if answer in {"n", "no"}:
        return False
    raise ValueError("An explicit yes/no answer is required")


def run_interactive() -> None:
    """Prompt for a destination and copy the current packaged template tree.

    Args:
        None.

    Returns:
        None.

    Raises:
        FileNotFoundError: If the packaged template or destination is missing.
        ValueError: If Enter is used for the destination, paths overlap, entry
            types mismatch, or confirmation is invalid.
        OSError: If discovery or copying fails.
        EOFError: If input closes before required answers.
    """
    destination_text = input("Existing destination directory: ").strip()
    if not destination_text:
        raise ValueError("Destination cannot be empty")
    destination = Path(destination_text).expanduser()
    if not destination.is_dir():
        raise FileNotFoundError(
            f"Destination does not exist or is not a directory: {destination}"
        )
    destination = destination.resolve()
    source = Path(__file__).resolve().parent / "template"
    entries = discover_template(source)
    conflicts = preflight_copy(source, destination, entries)
    if conflicts:
        print("Conflicting paths:")
        for conflict in conflicts:
            print(f"  - {conflict.as_posix()}")
        if not prompt_yes_no("Overwrite these same-type entries?"):
            print("No changes made.")
            return
    copy_template(source, destination, entries, overwrite=bool(conflicts))
    print(f"[OK] Copied project template into {destination}")


if __name__ == "__main__":
    if len(sys.argv) != 1:
        raise SystemExit("create-project-template does not accept arguments")
    run_interactive()
