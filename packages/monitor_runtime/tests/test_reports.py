import csv
import json
import struct
import unittest
from pathlib import Path
from tempfile import TemporaryDirectory

from monitor_runtime.reports import write_reports


class ReportTests(unittest.TestCase):
    def test_reports_have_stable_machine_and_human_readable_schemas(self):
        with TemporaryDirectory() as directory:
            root = Path(directory)
            sample = {"elapsed_seconds": 1, "cpu_percent": 2, "ram_mib": 3, "gpu_percent": None, "gpu_memory_mib": None}
            write_reports(root, [sample], [{"type": "attempt_started", "attempt": 1}], {"outcome": "success"})
            with (root / "samples.csv").open(newline="", encoding="utf-8") as stream:
                rows = list(csv.DictReader(stream))
            self.assertEqual(rows[0]["ram_mib"], "3")
            self.assertEqual(json.loads((root / "summary.json").read_text(encoding="utf-8"))["outcome"], "success")
            self.assertIn("outcome: success", (root / "summary.txt").read_text(encoding="utf-8"))

    def test_resource_plots_use_increased_canvas(self):
        """Keep attached plots within the approved larger dimensions."""
        with TemporaryDirectory() as directory:
            root = Path(directory)
            samples = [
                {"elapsed_seconds": 0, "cpu_percent": 1, "ram_mib": 2, "gpu_percent": 3, "gpu_memory_mib": 4},
                {"elapsed_seconds": 1, "cpu_percent": 2, "ram_mib": 3, "gpu_percent": 4, "gpu_memory_mib": 5},
            ]
            write_reports(root, samples, [], {"outcome": "success"})
            with (root / "cpu.png").open("rb") as stream:
                stream.seek(16)
                width, height = struct.unpack(">II", stream.read(8))
            self.assertGreaterEqual(width, 850)
            self.assertLessEqual(width, 1050)
            self.assertGreaterEqual(height, 450)
            self.assertLessEqual(height, 600)
