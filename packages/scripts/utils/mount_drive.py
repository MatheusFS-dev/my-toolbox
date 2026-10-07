#!/usr/bin/env python3
"""Interactively label and mount a non-system Linux filesystem."""

import getpass
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import textwrap


SUPPORTED = {"ext4": "e2label", "vfat": "fatlabel"}
GREY = "\033[90m"
GREEN = "\033[32m"
CYAN = "\033[36m"
YELLOW = "\033[33m"
RESET = "\033[0m"


def run(*args, capture=False, sudo=False):
    """Run a system command and raise on failure.

    Args:
        *args: Command and arguments.
        capture: Return standard output when true.
        sudo: Request elevation when the current process is not root.

    Returns:
        Captured output, or an empty string.
    """
    command = (["sudo"] if sudo and os.geteuid() != 0 else []) + list(args)
    result = subprocess.run(command, check=True, text=True,
                            stdout=subprocess.PIPE if capture else None)
    return result.stdout if capture else ""


def inventory():
    """Return physical disks from lsblk's JSON inventory.

    Returns:
        A list of disk dictionaries with nested partitions.
    """
    output = run("lsblk", "-J", "-o",
                 "NAME,PATH,TYPE,SIZE,MODEL,LABEL,UUID,FSTYPE,MOUNTPOINTS,PKNAME", capture=True)
    return [item for item in json.loads(output)["blockdevices"] if item["type"] == "disk"]


def has_root(item):
    """Check whether an lsblk subtree contains the root filesystem.

    Args:
        item: An lsblk device dictionary.

    Returns:
        True when any node is mounted at /.
    """
    return "/" in (item.get("mountpoints") or []) or any(
        has_root(child) for child in item.get("children") or [])


def flatten_disks(disks, os_disk=None):
    """Add protection and selection flags to physical disk rows.

    Args:
        disks: Nested lsblk disk dictionaries.
        os_disk: Optional OS disk path for controlled tests.

    Returns:
        A flat list of device dictionaries.
    """
    rows = []
    for disk in disks:
        protected = has_root(disk) or disk.get("path") == os_disk

        def visit(item, depth):
            row = dict(item)
            row["depth"] = depth
            row["protected"] = protected
            row["selectable"] = bool(not protected and item.get("uuid") and
                                     item.get("fstype") in SUPPORTED and
                                     item.get("type") in ("disk", "part"))
            rows.append(row)
            for child in item.get("children") or []:
                visit(child, depth + 1)

        visit(disk, 0)
    return rows


def validate_target(value):
    """Validate a mount target and indicate when it is outside /media/.

    Args:
        value: User supplied absolute path.

    Returns:
        The normalized path and a warning flag.

    Raises:
        ValueError: If the path is unsafe or not absolute.
    """
    if not value or not os.path.isabs(value) or "\x00" in value or any(c.isspace() for c in value):
        raise ValueError("Use an absolute path without whitespace.")
    path = Path(os.path.normpath(value))
    if path in (Path("/"), Path("/media")):
        raise ValueError("Choose a dedicated directory, not / or /media.")
    for part in (path, *path.parents):
        if part.is_symlink():
            raise ValueError(f"Mount path contains a symlink: {part}")
    warning = path != Path("/media") and Path("/media") not in path.parents
    return str(path), warning


def validate_label(label, fstype):
    """Validate a filesystem label before changing the device.

    Args:
        label: Requested new label.
        fstype: The lsblk filesystem type.

    Returns:
        The valid label.

    Raises:
        ValueError: If the type or label is unsupported.
    """
    if fstype not in SUPPORTED:
        raise ValueError(f"Unsupported filesystem: {fstype}")
    limit = 16 if fstype == "ext4" else 11
    if not label or len(label.encode("utf-8")) > limit or "/" in label or "\x00" in label:
        raise ValueError(f"{fstype} labels must be 1–{limit} bytes without '/' or NUL.")
    if fstype == "vfat" and (not label.isascii() or not re.fullmatch(r"[A-Za-z0-9 _-]+", label)):
        raise ValueError("FAT32 labels may contain ASCII letters, digits, spaces, _ and -.")
    return label


def update_fstab(text, uuid, target, fstype, persistent):
    """Replace this UUID's fstab row, preserving unrelated rows.

    Args:
        text: Existing fstab content.
        uuid: Selected filesystem UUID.
        target: New mount target.
        fstype: Filesystem type.
        persistent: Whether the mount should survive reboot.

    Returns:
        Updated fstab text.
    """
    sources = {f"UUID={uuid}", f"/dev/disk/by-uuid/{uuid}"}
    lines = [line for line in text.splitlines() if
             not (line.strip() and not line.lstrip().startswith("#") and
                  line.split()[0] in sources)]
    if persistent:
        lines.append(f"UUID={uuid} {target} {fstype} defaults,nofail 0 2")
    return "\n".join(lines) + "\n"


def check_fstab_conflicts(text, uuid, old_mount, target, device=None, label=None):
    """Refuse fstab aliases that cannot be safely replaced by UUID.

    Args:
        text: Existing fstab content.
        uuid: Selected filesystem UUID.
        old_mount: Current mount path, if any.
        target: Requested mount path.
        device: Selected device path.
        label: Current filesystem label.

    Raises:
        RuntimeError: If a conflicting entry needs manual review.
    """
    selected_sources = {f"UUID={uuid}", f"/dev/disk/by-uuid/{uuid}"}
    aliases = {device, f"LABEL={label}" if label else None}
    for line in text.splitlines():
        fields = line.split()
        if len(fields) < 2 or line.lstrip().startswith("#"):
            continue
        source, mount = fields[:2]
        if source in selected_sources:
            continue
        if source in aliases or mount in {old_mount, target}:
            raise RuntimeError(
                f"Conflicting /etc/fstab entry: {source} {mount}. Review it manually before continuing.")


def current_mount(row):
    """Get the active mount path from an lsblk row.

    Args:
        row: An lsblk device dictionary.

    Returns:
        The mount path or None.
    """
    return next((value for value in row.get("mountpoints") or [] if value), None)


def check_target_available(target, old_mount):
    """Refuse targets already mounted or containing unrelated files.

    Args:
        target: New mount path.
        old_mount: Current mount path, if any.

    Raises:
        RuntimeError: If the target would hide another filesystem or files.
    """
    path = Path(target)
    if target == old_mount:
        return
    if old_mount and Path(old_mount) in path.parents:
        raise RuntimeError("New target cannot be inside the current mount.")
    if os.path.ismount(target):
        raise RuntimeError(f"Target is already mounted: {target}")
    if path.exists() and (not path.is_dir() or any(path.iterdir())):
        raise RuntimeError("Target must be a new or empty directory.")


def requires_unmount(old_mount, target, old_label, new_label):
    """Decide whether a label or mount path change requires unmounting.

    Args:
        old_mount: Current mount path, if any.
        target: Requested mount path.
        old_label: Current filesystem label.
        new_label: Requested filesystem label.

    Returns:
        True when the existing mount must be released.
    """
    return bool(old_mount and (old_mount != target or old_label != new_label))


def busy_processes(mount_path):
    """Find processes keeping a filesystem busy.

    Args:
        mount_path: Current mount path.

    Returns:
        A list of PID and command name pairs.
    """
    command = (["sudo"] if os.geteuid() != 0 else []) + ["fuser", "-m", mount_path]
    result = subprocess.run(command, text=True, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE, check=False)
    if result.returncode not in (0, 1):
        raise RuntimeError(f"Could not check processes using {mount_path}: {result.stderr.strip()}")
    pids = []
    for token in result.stdout.split():
        match = re.match(r"(\d+)", token)
        if match:
            pid = int(match.group(1))
            try:
                name = Path(f"/proc/{pid}/comm").read_text().strip()
            except OSError:
                name = "unknown"
            pids.append((pid, name))
    return pids


def ensure_not_busy(mount_path):
    """Stop before unmounting a filesystem used by other processes.

    Args:
        mount_path: Current mount path.

    Raises:
        RuntimeError: If processes still use the filesystem.
    """
    processes = busy_processes(mount_path)
    if processes:
        listed = ", ".join(f"{pid} ({name})" for pid, name in processes[:12])
        remaining = f", and {len(processes) - 12} more" if len(processes) > 12 else ""
        raise RuntimeError(
            f"Cannot unmount {mount_path}; it is in use by {listed}{remaining}. "
            "Run 'cd ~' in terminals using this drive, close applications using it, then retry.")


def display(rows, width=None, color=None):
    """Print grouped disk details with aligned fields and status colors.

    Args:
        rows: Flat inventory rows.
        width: Optional display width for controlled rendering.
        color: Override automatic terminal color detection when set.
    """
    width = max(32, min(72, width or shutil.get_terminal_size((80, 24)).columns))
    if color is None:
        color = sys.stdout.isatty() and "NO_COLOR" not in os.environ

    def emit(line, tone=None):
        print(f"{tone}{line}{RESET}" if color and tone else line)

    def field(name, value, prefix, tone=None):
        label = f"{prefix}{name:<12}"
        chunks = textwrap.wrap(str(value), width=max(1, width - len(label)),
                               break_long_words=True, break_on_hyphens=False) or ["-"]
        emit(label + chunks[0], tone)
        for chunk in chunks[1:]:
            emit(f"{prefix}{'':12}{chunk}", tone)

    def mount_field(value, prefix, tone=None):
        emit(f"{prefix}Mounted at", tone)
        value_prefix = f"{prefix}  "
        chunks = textwrap.wrap(str(value), width=max(1, width - len(value_prefix)),
                               break_long_words=True, break_on_hyphens=False) or ["-"]
        for chunk in chunks:
            emit(value_prefix + chunk, tone)

    print("\nPhysical disks and filesystems")
    for line in textwrap.wrap(
            "Green: selectable   Gray: OS disk (locked)   Yellow: unsupported or unformatted",
            width=width):
        print(line)
    for line in textwrap.wrap("UUID: stable filesystem ID used for persistent mounts.", width=width):
        print(line)
    print()
    group_open = False
    for index, row in enumerate(rows, 1):
        tone = GREY if row["protected"] else GREEN if row["selectable"] else YELLOW
        if row["depth"] == 0:
            if group_open:
                emit("╰─", group_tone)
                print()
            group_tone = GREY if row["protected"] else CYAN
            status = "OS disk · locked" if row["protected"] else "physical disk"
            emit(f"╭─ [{index}] {row['path']}  ·  {status}", group_tone)
            field("Size", row.get("size") or "-", "│  ", GREY if row["protected"] else None)
            field("Model", row.get("model") or "-", "│  ", GREY if row["protected"] else None)
            if row.get("children"):
                field("Contents", "Partitions listed below", "│  ",
                      GREY if row["protected"] else None)
            group_open = True
        else:
            emit("│", GREY if row["protected"] else None)
            status = ("OS locked" if row["protected"] else
                      "selectable" if row["selectable"] else "not mountable")
            emit(f"│  ├─ [{index}] {row['path']}  ·  {status}", tone)
        if row["depth"] == 0 and row.get("children"):
            continue
        prefix = "│  " if row["depth"] == 0 else "│  │  "
        detail_tone = GREY if row["protected"] else None
        if row["depth"] > 0:
            field("Size", row.get("size") or "-", prefix, detail_tone)
        field("Format", row.get("fstype") or "-", prefix, detail_tone)
        field("Label", row.get("label") or "-", prefix, detail_tone)
        field("UUID", row.get("uuid") or "-", prefix, detail_tone)
        mount_field(current_mount(row) or "-", prefix, detail_tone)
    if group_open:
        emit("╰─", group_tone)
        print()


def ask_yes_no(prompt, default=False):
    """Read a yes or no answer.

    Args:
        prompt: Question text.
        default: Value returned for an empty answer.

    Returns:
        True for yes, false for no.
    """
    while True:
        answer = input(f"{prompt} [{'Y/n' if default else 'y/N'}]: ").strip().lower()
        if not answer:
            return default
        if answer in ("y", "yes"):
            return True
        if answer in ("n", "no"):
            return False
        print("Enter yes or no.")


def sudo_read(path):
    """Read a root-owned text file.

    Args:
        path: File path.

    Returns:
        File contents.
    """
    return run("cat", path, capture=True, sudo=True)


def verify_fstab(path):
    """Check fstab as root and explain any warnings concisely.

    Args:
        path: Candidate fstab file path.

    Raises:
        RuntimeError: If findmnt reports a validation error.
    """
    command = (["sudo"] if os.geteuid() != 0 else []) + [
        "findmnt", "--verify", "--tab-file", path]
    result = subprocess.run(command, capture_output=True, text=True, check=False)
    output = result.stdout + result.stderr
    if result.returncode:
        raise RuntimeError(f"/etc/fstab validation failed:\n{output.strip()}")
    print("Persistent mount configuration: valid.")
    for line in output.splitlines():
        if "[W]" not in line:
            continue
        if "non-bind mount source /swap.img is a directory or regular file" in line:
            print("Note: the existing /swap.img entry is a swap file; findmnt warns about it.")
        else:
            print(f"fstab warning: {line.strip()}")


def write_fstab(content):
    """Validate and atomically install new fstab content.

    Args:
        content: Complete replacement fstab text.
    """
    with tempfile.NamedTemporaryFile(mode="w", delete=False) as handle:
        handle.write(content)
        temporary = handle.name
    staged = f"/etc/.fstab.mount-drive-{os.getpid()}.tmp"
    try:
        verify_fstab(temporary)
        run("install", "-m", "644", temporary, staged, sudo=True)
        run("mv", "-f", staged, "/etc/fstab", sudo=True)
        run("systemctl", "daemon-reload", sudo=True)
    finally:
        os.unlink(temporary)
        run("rm", "-f", staged, sudo=True)


def apply_change(row, target, label, persistent, chmod_all):
    """Apply a confirmed mount configuration to one filesystem.

    Args:
        row: Selected inventory row.
        target: Validated mount path.
        label: Validated filesystem label.
        persistent: Add an fstab entry when true.
        chmod_all: Recursively set mode 777 when true.
    """
    fresh = next((item for item in flatten_disks(inventory()) if item["path"] == row["path"]), None)
    if not fresh or not fresh["selectable"] or fresh["uuid"] != row["uuid"]:
        raise RuntimeError("Device changed or became protected. No changes made.")
    target, _ = validate_target(target)
    old_mount = current_mount(fresh)
    must_unmount = requires_unmount(old_mount, target, fresh.get("label") or "", label)
    if old_mount and len([p for p in fresh.get("mountpoints") or [] if p]) > 1:
        raise RuntimeError("Filesystem has multiple mounts; unmount them manually first.")
    check_target_available(target, old_mount)
    if not shutil.which(SUPPORTED[fresh["fstype"]]):
        raise RuntimeError(f"Missing label tool: {SUPPORTED[fresh['fstype']]}")
    if old_mount and (must_unmount or chmod_all):
        nested = run("findmnt", "-rn", "-o", "TARGET", "-R", old_mount, capture=True).splitlines()
        if len(nested) > 1:
            raise RuntimeError("Source has nested mounts; unmount them manually first.")
    if must_unmount:
        ensure_not_busy(old_mount)
    old_fstab = sudo_read("/etc/fstab")
    check_fstab_conflicts(old_fstab, fresh["uuid"], old_mount, target,
                          fresh["path"], fresh.get("label"))
    new_fstab = update_fstab(old_fstab, fresh["uuid"], target, fresh["fstype"], persistent)
    if must_unmount:
        try:
            run("umount", old_mount, sudo=True)
        except subprocess.CalledProcessError as error:
            ensure_not_busy(old_mount)
            raise RuntimeError(f"Could not unmount {old_mount}; no changes were made.") from error
    try:
        if label != (fresh.get("label") or ""):
            run(SUPPORTED[fresh["fstype"]], fresh["path"], label, sudo=True)
        if not old_mount or must_unmount:
            run("mkdir", "-p", target, sudo=True)
            run("mount", "-t", fresh["fstype"], "-U", fresh["uuid"], target, sudo=True)
    except (subprocess.CalledProcessError, OSError):
        if must_unmount:
            print(f"Mount failed. Trying to restore the previous mount at {old_mount}.", file=sys.stderr)
            run("mount", "-t", fresh["fstype"], "-U", fresh["uuid"], old_mount, sudo=True)
        raise
    mounted = run("findmnt", "-n", "-o", "UUID", "--mountpoint", target, capture=True).strip()
    if mounted != fresh["uuid"]:
        raise RuntimeError("Mount verification failed; fstab and permissions were not changed.")
    if new_fstab != old_fstab:
        write_fstab(new_fstab)
    if chmod_all:
        run("chmod", "-R", "777", "--", target, sudo=True)


def main():
    """Run the interactive disk selection and mount workflow."""
    rows = flatten_disks(inventory())
    display(rows)
    if "--list" in sys.argv:
        return
    try:
        number = int(input("\nSelect a device number (or 0 to cancel): "))
        if number == 0:
            return
        if not 1 <= number <= len(rows) or not rows[number - 1]["selectable"]:
            raise ValueError("That device cannot be selected.")
        row = rows[number - 1]
        print(f"\nSelected [{number}] {row['path']} ({row['fstype']}, {row.get('size') or 'unknown size'})")
        print("The label is the drive name shown by Linux. Its UUID will stay the same.")
        label = input(f"New filesystem label [{row.get('label') or '-'}]: ").strip() or row.get("label")
        label = validate_label(label, row["fstype"])
        user = os.environ.get("SUDO_USER") or getpass.getuser()
        suggested = f"/media/{user}/{label}"
        print("The mount path is the folder where this drive's files will appear.")
        target = input(f"Full mount path [{suggested}]: ").strip() or suggested
        target, outside = validate_target(target)
        if outside:
            print("WARNING: This path is outside /media/, where removable drives are usually mounted.")
        persistent = ask_yes_no("Save this mount in /etc/fstab for future boots?")
        chmod_all = ask_yes_no("Give everyone full access to all files (chmod -R 777)?")
        if chmod_all:
            print("WARNING: This recursively changes permissions on every file and directory on the drive.")
        print("\nPlanned changes")
        print(f"  Device       {row['path']}")
        print(f"  UUID         {row['uuid']} (unchanged)")
        print(f"  Label        {row.get('label') or '-'} → {label}")
        print(f"  Mount path   {current_mount(row) or '-'} → {target}")
        print(f"  Future boots {'save in /etc/fstab' if persistent else 'do not save'}")
        print(f"  Permissions  {'chmod -R 777' if chmod_all else 'leave as they are'}")
        if requires_unmount(current_mount(row), target, row.get("label") or "", label):
            print("  Note         The current mount must be released; open files may block it.")
        if not ask_yes_no("Apply these changes?", default=True):
            print("Cancelled.")
            return
        apply_change(row, target, label, persistent, chmod_all)
        print(f"Mounted {row['path']} at {target}.")
    except (EOFError, KeyboardInterrupt):
        print("\nCancelled.")
    except (ValueError, RuntimeError, subprocess.CalledProcessError, OSError) as error:
        print(f"Error: {error}", file=sys.stderr)
        raise SystemExit(1) from error


if __name__ == "__main__":
    main()
