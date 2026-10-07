"""Dashboard, bounded statistics, transactional export and CLI tests."""
import io
import tempfile
import unittest
from pathlib import Path
from unittest.mock import Mock, patch

from stress_gpu_runtime.reporting import summarize_sample, render_dashboard, format_report, save_results, Dashboard, prompt_save
from stress_gpu_runtime.__main__ import main


class ReportingTests(unittest.TestCase):
    """Verify measured results and optional exports."""

    def configuration(self):
        """Return a deterministic GPU description.

        Args:
            None.
        Returns:
            dict: A short two-board run.
        Raises:
            None.
        """
        return {"devices": [{"uuid": "GPU-a", "ordinal": 0, "name": "First", "total_bytes": 1000, "free_bytes": 900, "slowdown_celsius": 80},
                            {"uuid": "GPU-b", "ordinal": 1, "name": "Second", "total_bytes": 2000, "free_bytes": 1800, "slowdown_celsius": None}],
                "load_percent": 25, "duration_seconds": 2}

    def test_online_statistics_temperature_and_missing_values(self):
        """Accumulate exact summaries without retaining every observation.

        Args:
            None.
        Returns:
            None.
        Raises:
            AssertionError: If reported metrics or memory bounds are wrong.
        """
        state = {}
        for elapsed, temperature in ((0, 70), (2, 90)):
            summarize_sample(state, {"uuid": "GPU-a", "elapsed_seconds": elapsed, "temperature_celsius": temperature,
                                     "utilization_percent": None if elapsed == 0 else 20, "thermal_throttling": elapsed > 0})
        self.assertEqual(state["GPU-a"]["metrics"]["temperature_celsius"]["sum"], 160)
        report = format_report(self.configuration(), {"elapsed_seconds": 2, "stop_reason": "duration completed", "errors": []}, state)
        self.assertIn("80.0", report)
        self.assertIn("90.0", report)
        self.assertIn("+10.00", report)
        self.assertIn("threshold reached", report)
        self.assertIn("thermal", report)
        self.assertIn("No observations", report)
        for elapsed in range(10000):
            summarize_sample(state, {"uuid": "GPU-a", "elapsed_seconds": elapsed, "temperature_celsius": 60})
        self.assertNotIn("samples", state["GPU-a"])
        self.assertLess(len(str(state)), 2000)
        dashboard = render_dashboard(self.configuration(), state, 2)
        self.assertIn("25", dashboard)
        self.assertIn("N/A", dashboard)
        self.assertIn("GPU-b", dashboard)

    def test_dashboard_replaces_and_restores_terminal(self):
        """Replace TTY frames and restore the cursor after errors.

        Args:
            None.
        Returns:
            None.
        Raises:
            AssertionError: If terminal state remains changed.
        """
        output = io.StringIO()
        with self.assertRaises(RuntimeError):
            with Dashboard(output, interactive=True) as dashboard:
                dashboard.draw("first")
                dashboard.draw("second")
                raise RuntimeError("stop")
        self.assertIn("\x1b[2J\x1b[H", output.getvalue())
        self.assertTrue(output.getvalue().endswith("\x1b[?25h\n"))
        plain = io.StringIO()
        with Dashboard(plain, interactive=False) as dashboard:
            dashboard.draw("frame")
        self.assertNotIn("\x1b", plain.getvalue())

    def test_export_overwrite_refusal_and_rollback(self):
        """Keep old exports intact when publishing fails.

        Args:
            None.
        Returns:
            None.
        Raises:
            AssertionError: If existing files are damaged.
        """
        with tempfile.TemporaryDirectory() as folder:
            spool = io.StringIO('{"uuid":"GPU-a"}\n')
            paths = save_results(folder, "first", spool)
            self.assertEqual(Path(paths[0]).read_text(), "first")
            with self.assertRaises(FileExistsError):
                save_results(folder, "second", spool)
            import os
            replace = os.replace
            calls = 0
            def fail_second(source, destination):
                """Fail publishing the samples after report publication.

                Args:
                    source (str): Staged path.
                    destination (str): Destination path.
                Returns:
                    None.
                Raises:
                    OSError: On the second publication call.
                """
                nonlocal calls
                calls += 1
                if calls == 2:
                    raise OSError("disk failed")
                return replace(source, destination)
            with patch("stress_gpu_runtime.reporting.os.replace", side_effect=fail_second):
                with self.assertRaises(OSError):
                    save_results(folder, "second", spool, overwrite=True)
            self.assertEqual(Path(paths[0]).read_text(), "first")
            self.assertEqual(Path(paths[1]).read_text(), '{"uuid":"GPU-a"}\n')

    def test_save_prompts_decline_retry_and_interruption(self):
        """Ask for a path only after acceptance and recover from write failure.

        Args:
            None.
        Returns:
            None.
        Raises:
            AssertionError: If the prompt flow loses collected results.
        """
        input_fn = Mock(return_value="n")
        prompt_save("report", io.StringIO(), input_fn, io.StringIO())
        self.assertEqual(input_fn.call_count, 1)
        with tempfile.TemporaryDirectory() as folder:
            bad = Path(folder) / "file"
            bad.write_text("existing")
            output = io.StringIO()
            prompt_save("report", io.StringIO("sample\n"), Mock(side_effect=["y", str(bad), str(Path(folder) / "good")]), output)
            self.assertIn("Could not save", output.getvalue())
            self.assertEqual((Path(folder) / "good/report.txt").read_text(), "report")
        prompt_save("report", io.StringIO(), Mock(side_effect=KeyboardInterrupt), io.StringIO())

    def test_mocked_cli_collects_samples_and_final_report(self):
        """Exercise a full CLI session without GPU dependencies.

        Args:
            None.
        Returns:
            None.
        Raises:
            AssertionError: If the CLI loses report output or cleanup.
        """
        config = self.configuration()
        output = io.StringIO()
        nvml = Mock()
        def session(settings, sampler, on_sample):
            """Emit a sample for each GPU.

            Args:
                settings (dict): Selected run configuration.
                sampler (callable): Unused telemetry callback.
                on_sample (callable): Sample consumer.
            Returns:
                dict: Successful result.
            Raises:
                None.
            """
            for device in settings["devices"]:
                on_sample({"uuid": device["uuid"], "elapsed_seconds": 1, "temperature_celsius": 60})
            return {"stop_reason": "duration completed", "elapsed_seconds": 2, "errors": []}
        with patch("stress_gpu_runtime.__main__.load_dependencies", return_value=(Mock(), nvml)), \
             patch("stress_gpu_runtime.__main__.discover_devices", return_value=config["devices"]), \
             patch("stress_gpu_runtime.__main__.prompt_configuration", return_value=config), \
             patch("stress_gpu_runtime.__main__.run_session", side_effect=session), \
             patch("sys.stdout", output), patch("builtins.input", return_value="n"), patch("sys.argv", ["stress-gpu"]):
            self.assertEqual(main(), 0)
        self.assertIn("Final report", output.getvalue())
        self.assertIn("Second", output.getvalue())
        nvml.nvmlShutdown.assert_called_once()

class ExportInterruptionTests(unittest.TestCase):
    """Exercise interruption after a successful atomic replacement."""

    def test_interruption_after_replace_restores_old_outputs(self):
        """Restore both original files even if bookkeeping is interrupted.

        Args:
            None.
        Returns:
            None.
        Raises:
            AssertionError: If an original export is lost.
        """
        import os
        with tempfile.TemporaryDirectory() as folder:
            paths = save_results(folder, "old report", io.StringIO("old samples"))
            original = os.replace
            calls = 0
            def interrupted_replace(source, target):
                """Raise after publishing the first output.

                Args:
                    source (str): Source file.
                    target (str): Destination file.
                Returns:
                    None.
                Raises:
                    KeyboardInterrupt: After the first successful publication.
                """
                nonlocal calls
                original(source, target)
                calls += 1
                if calls == 1:
                    raise KeyboardInterrupt
            with patch("stress_gpu_runtime.reporting.os.replace", side_effect=interrupted_replace):
                with self.assertRaises(KeyboardInterrupt):
                    save_results(folder, "new report", io.StringIO("new samples"), overwrite=True)
            self.assertEqual(Path(paths[0]).read_text(), "old report")
            self.assertEqual(Path(paths[1]).read_text(), "old samples")

    def test_failed_rollback_preserves_recovery_backups(self):
        """Keep backups on disk if rollback cannot restore an output.

        Args:
            None.
        Returns:
            None.
        Raises:
            AssertionError: If backup files are discarded after recovery failure.
        """
        with tempfile.TemporaryDirectory() as folder:
            save_results(folder, "old report", io.StringIO("old samples"))
            with patch("stress_gpu_runtime.reporting.os.replace", side_effect=OSError("disk failed")):
                with self.assertRaisesRegex(OSError, "backup"):
                    save_results(folder, "new report", io.StringIO("new samples"), overwrite=True)
            backups = list(Path(folder).glob(".stress-gpu-export-*/report.txt.backup"))
            self.assertEqual(len(backups), 1)
            self.assertEqual(backups[0].read_text(), "old report")

    def test_unknown_thermal_telemetry_is_not_reported_as_zero(self):
        """Represent entirely unsupported thermal telemetry as unavailable.

        Args:
            None.
        Returns:
            None.
        Raises:
            AssertionError: If no telemetry becomes a zero result.
        """
        state = {}
        summarize_sample(state, {"uuid": "GPU-a", "elapsed_seconds": 0, "thermal_throttling": None})
        config = {"devices": [{"uuid": "GPU-a", "ordinal": 0, "name": "Board"}], "load_percent": 100, "duration_seconds": 1}
        report = format_report(config, {"elapsed_seconds": 1, "stop_reason": "duration completed", "errors": []}, state)
        self.assertIn("Samples with thermal throttling: N/A", report)
