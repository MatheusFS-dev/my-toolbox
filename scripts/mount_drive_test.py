"""Isolated checks for the packaged mount-drive utility."""

import importlib.util
from pathlib import Path
import unittest
from unittest import mock


SCRIPT = Path(__file__).resolve().parents[1] / "packages/scripts/utils/mount_drive.py"
spec = importlib.util.spec_from_file_location("mount_drive", SCRIPT)
mount_drive = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mount_drive)


class MountDriveTests(unittest.TestCase):
    """Check disk protection and persistent mount edits without touching devices."""

    def test_os_disk_is_locked(self):
        disks = [
            {"type": "disk", "path": "/dev/sda", "mountpoints": [None], "children": [
                {"type": "part", "path": "/dev/sda1", "uuid": "root", "fstype": "ext4", "mountpoints": ["/"]}],
             "uuid": None, "fstype": None},
            {"type": "disk", "path": "/dev/sdb", "mountpoints": [None], "children": [
                {"type": "part", "path": "/dev/sdb1", "uuid": "data", "fstype": "ext4", "mountpoints": [None]}],
             "uuid": None, "fstype": None},
        ]
        rows = mount_drive.flatten_disks(disks)
        self.assertFalse(rows[1]["selectable"])
        self.assertTrue(rows[3]["selectable"])

    def test_fstab_replaces_only_selected_uuid(self):
        old = "UUID=data /media/old ext4 defaults 0 2\nUUID=root / ext4 defaults 0 1\n"
        new = mount_drive.update_fstab(old, "data", "/media/new", "ext4", True)
        self.assertIn("UUID=root / ext4 defaults 0 1", new)
        self.assertNotIn("/media/old", new)
        self.assertIn("UUID=data /media/new ext4 defaults,nofail 0 2", new)

    def test_cancelled_selection_makes_no_changes(self):
        disk = {"type": "disk", "path": "/dev/sdb", "uuid": "data", "fstype": "ext4",
                "label": "data", "size": "1G", "mountpoints": [None]}
        with mock.patch.object(mount_drive, "inventory", return_value=[disk]), \
             mock.patch.object(mount_drive, "display"), \
             mock.patch("builtins.input", return_value="0"), \
             mock.patch.object(mount_drive, "apply_change") as apply:
            mount_drive.main()
        apply.assert_not_called()

    def test_unsafe_target_is_rejected(self):
        for target in ("/", "/media", "relative", "/media/a b"):
            with self.subTest(target=target), self.assertRaises(ValueError):
                mount_drive.validate_target(target)

    def test_unexpected_fstab_alias_is_rejected(self):
        with self.assertRaises(RuntimeError):
            mount_drive.check_fstab_conflicts("LABEL=data /media/data ext4 defaults 0 2\n",
                                               "uuid", None, "/media/new", "/dev/sdb1", "data")


if __name__ == "__main__":
    unittest.main()
