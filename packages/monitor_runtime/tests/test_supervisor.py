import json
import os
import sys
import unittest
import subprocess
import time
import types
from pathlib import Path
from tempfile import TemporaryDirectory
from unittest.mock import patch

from monitor_runtime.config import default_config
from monitor_runtime.supervisor import _gpu_metrics, _process_metrics, supervise_script, terminate_process_group


class SupervisorTests(unittest.TestCase):
    def test_process_metrics_include_raw_target_and_total_system_ram(self):
        process = types.SimpleNamespace(
            pid=os.getpid(),
            children=lambda recursive: [],
            cpu_percent=lambda interval: 12.5,
            is_running=lambda: True,
            memory_info=lambda: types.SimpleNamespace(rss=125_000_000),
        )
        psutil = types.SimpleNamespace(
            Process=lambda pid: process,
            virtual_memory=lambda: types.SimpleNamespace(total=32_000_000_000),
        )
        with patch.dict(sys.modules, {"psutil": psutil}), patch("monitor_runtime.supervisor._gpu_metrics", return_value={}):
            sample = _process_metrics(os.getpid(), time.monotonic())
        self.assertEqual(sample["ram_bytes"], 125_000_000)
        self.assertEqual(sample["system_ram_total_bytes"], 32_000_000_000)
        self.assertAlmostEqual(sample["ram_mib"], 119.21, places=2)

    @patch("monitor_runtime.supervisor.shutil.which", return_value="/usr/bin/nvidia-smi")
    @patch("monitor_runtime.supervisor.subprocess.check_output")
    def test_gpu_metrics_fall_back_to_clearly_scoped_system_totals(self, check_output, _which):
        check_output.side_effect = [
            "",
            "# gpu pid type sm mem enc dec command\n# Idx # C/G % % % % name\n",
            "0, GPU A, 20, 100, 1000\n1, GPU B, 40, 200, 2000\n",
        ]
        sample = _gpu_metrics({123})
        self.assertEqual(sample["gpu_scope"], "system-wide")
        self.assertEqual(sample["gpu_percent"], 30)
        self.assertEqual(sample["gpu_memory_mib"], 300)
        self.assertEqual(sample["gpu_memory_total_mib"], 3000)

    @patch("monitor_runtime.supervisor.shutil.which", return_value="/usr/bin/nvidia-smi")
    @patch("monitor_runtime.supervisor.subprocess.check_output")
    def test_gpu_metrics_keep_target_scope_when_process_data_exists(self, check_output, _which):
        check_output.side_effect = ["123, 256\n", "0 123 C 35 0 0 0 python\n"]
        sample = _gpu_metrics({123})
        self.assertEqual(sample["gpu_scope"], "target")
        self.assertEqual(sample["gpu_percent"], 35)
        self.assertEqual(sample["gpu_memory_mib"], 256)
        self.assertIsNone(sample["gpu_memory_total_mib"])

    @patch("monitor_runtime.supervisor.shutil.which", return_value="/usr/bin/nvidia-smi")
    @patch("monitor_runtime.supervisor.subprocess.check_output")
    def test_gpu_metrics_keep_target_memory_when_pmon_fails(self, check_output, _which):
        check_output.side_effect = ["123, 256\n", subprocess.CalledProcessError(1, "nvidia-smi")]
        sample = _gpu_metrics({123})
        self.assertEqual(sample["gpu_scope"], "target")
        self.assertEqual(sample["gpu_memory_mib"], 256)

    @patch("monitor_runtime.supervisor.shutil.which", return_value="/usr/bin/nvidia-smi")
    @patch("monitor_runtime.supervisor.subprocess.check_output")
    def test_gpu_metrics_ignore_entire_malformed_global_rows(self, check_output, _which):
        check_output.side_effect = [
            "",
            "# no target rows\n",
            "0, GPU A, 90, bad, 1000\n1, GPU B, 20, 200, 2000\n",
        ]
        sample = _gpu_metrics({123})
        self.assertEqual(sample["gpu_percent"], 20)
        self.assertEqual(sample["gpu_memory_mib"], 200)
    def test_termination_returns_without_waiting_for_timeout_after_clean_exit(self):
        process = subprocess.Popen([sys.executable, "-c", "import time; time.sleep(60)"], start_new_session=True)
        started = time.monotonic()
        terminate_process_group(process, timeout=2)
        self.assertLess(time.monotonic() - started, 1)

    def test_termination_escalates_for_resistant_descendants(self):
        with TemporaryDirectory() as directory:
            child_pid = Path(directory) / "child.pid"
            child_ready = Path(directory) / "child.ready"
            process = subprocess.Popen(
                [sys.executable, "-u", "-c",
                 "import subprocess,time,sys; p=subprocess.Popen([sys.executable,'-c',\"import signal,time,sys; signal.signal(signal.SIGTERM, signal.SIG_IGN); open(sys.argv[1],'w').close(); time.sleep(60)\",sys.argv[2]]); open(sys.argv[1],'w').write(str(p.pid)); time.sleep(60)",
                 str(child_pid), str(child_ready)],
                start_new_session=True,
            )
            for _ in range(100):
                if child_pid.exists() and child_ready.exists():
                    break
                import time
                time.sleep(0.02)
            pid = int(child_pid.read_text(encoding="utf-8"))
            terminate_process_group(process, timeout=0.2)
            state = "R"
            for _ in range(50):
                try:
                    state = Path("/proc/{}/stat".format(pid)).read_text(encoding="utf-8").split()[2]
                except FileNotFoundError:
                    state = None
                if state in (None, "Z"):
                    break
                import time
                time.sleep(0.02)
            self.assertIn(state, (None, "Z"))

    def test_success_uses_selected_interpreter_and_writes_labeled_output(self):
        with TemporaryDirectory() as directory:
            script = Path(directory) / "success.py"
            script.write_text("import sys\nprint(sys.executable)\n", encoding="utf-8")
            config = default_config()
            config["reports_enabled"] = False
            events = []
            result = supervise_script(str(script), sys.executable, config, events.append)
            self.assertEqual(result["outcome"], "success")
            output = Path(result["run_directory"]) / "output.log"
            content = output.read_text(encoding="utf-8")
            self.assertIn("[attempt 1 stdout]", content)
            printed = content.split("] ", 1)[1].strip()
            self.assertEqual(os.path.realpath(printed), os.path.realpath(sys.executable))

    def test_exact_retry_exhaustion_preserves_each_failed_attempt(self):
        with TemporaryDirectory() as directory:
            script = Path(directory) / "fail.py"
            counter = Path(directory) / "counter.txt"
            script.write_text(
                "from pathlib import Path\n"
                "p=Path('counter.txt')\n"
                "n=int(p.read_text())+1 if p.exists() else 1\n"
                "p.write_text(str(n))\n"
                "print('attempt-{}'.format(n))\n"
                "raise SystemExit(7)\n",
                encoding="utf-8",
            )
            config = default_config()
            config["reports_enabled"] = False
            config["restart"].update({"crash_retries": 2, "base_delay_seconds": 0})
            result = supervise_script(str(script), sys.executable, config, lambda event: None)
            self.assertEqual(result["outcome"], "failed")
            self.assertEqual(counter.read_text(encoding="utf-8"), "3")
            crash_logs = list((Path(result["run_directory"]) / "crash_logs").glob("attempt-*.log"))
            self.assertEqual(len(crash_logs), 3)
            second = sorted(crash_logs)[1].read_text(encoding="utf-8")
            self.assertIn("attempt-2", second)
            self.assertNotIn("attempt-1", second)

    def test_rapid_crashes_send_one_code_error_email_only_after_retry_exhaustion(self):
        with TemporaryDirectory() as directory:
            script = Path(directory) / "broken.py"
            script.write_text("print('broken')\nraise SystemExit(7)\n", encoding="utf-8")
            config = default_config()
            config["reports_enabled"] = False
            config["restart"].update({"crash_retries": 2, "base_delay_seconds": 0, "rapid_crash_seconds": 60})
            config["notifications"] = {key: key in ("possible_code_error", "final_failure") for key in config["notifications"]}
            events = []
            notifications = []
            timeline = []

            def sink(event):
                events.append(event)
                timeline.append(("event", event["type"], event.get("restart")))

            def notifier(kind, title, output_lines, metrics, graph_paths=None):
                notifications.append((kind, title, output_lines, metrics))
                timeline.append(("email", kind, None))
                return True

            result = supervise_script(str(script), sys.executable, config, sink, notifier, "Broken training")

            self.assertEqual(result["outcome"], "failed")
            self.assertEqual([item[0] for item in notifications], ["possible_code_error"])
            terminal_decision = timeline.index(("event", "restart_decision", False))
            code_error_email = timeline.index(("email", "possible_code_error", None))
            self.assertGreater(code_error_email, terminal_decision)
            details = notifications[0][3]
            self.assertEqual(details["exit_code"], 7)
            self.assertEqual(details["attempt"], 3)
            self.assertEqual(details["remaining_retries"], 0)
            self.assertEqual(details["script"], str(script.resolve()))
            self.assertLess(details["attempt_duration_seconds"], 60)
            warnings = [event for event in events if event.get("state") == "possible_code_error_warning"]
            self.assertEqual(len(warnings), 1)

    def test_recovered_rapid_crash_does_not_send_code_error_email(self):
        with TemporaryDirectory() as directory:
            script = Path(directory) / "recover.py"
            script.write_text(
                "from pathlib import Path\n"
                "marker = Path('attempt.txt')\n"
                "attempt = int(marker.read_text()) + 1 if marker.exists() else 1\n"
                "marker.write_text(str(attempt))\n"
                "if attempt == 1:\n"
                "    raise SystemExit(4)\n",
                encoding="utf-8",
            )
            config = default_config()
            config["reports_enabled"] = False
            config["restart"].update({"crash_retries": 1, "base_delay_seconds": 0, "rapid_crash_seconds": 60})
            config["notifications"] = {key: key == "possible_code_error" for key in config["notifications"]}
            notifications = []

            def notifier(kind, title, output_lines, metrics, graph_paths=None):
                notifications.append(kind)
                return True

            result = supervise_script(str(script), sys.executable, config, lambda event: None, notifier)

            self.assertEqual(result["outcome"], "success")
            self.assertEqual(notifications, [])

    def test_output_and_crash_log_keep_full_lines_while_protocol_output_is_bounded(self):
        with TemporaryDirectory() as directory:
            script = Path(directory) / "long_failure.py"
            full_line = "x" * 5000
            script.write_text("print({!r})\nraise SystemExit(2)\n".format(full_line), encoding="utf-8")
            config = default_config()
            config["reports_enabled"] = False
            config["restart"]["crash_retries"] = 0
            config["notifications"] = {key: False for key in config["notifications"]}
            events = []

            result = supervise_script(str(script), sys.executable, config, events.append)

            output = (Path(result["run_directory"]) / "output.log").read_text(encoding="utf-8")
            crash = (Path(result["run_directory"]) / "crash_logs" / "attempt-001.log").read_text(encoding="utf-8")
            displayed = next(event["text"] for event in events if event["type"] == "target_output")
            self.assertIn(full_line, output)
            self.assertIn(full_line, crash)
            self.assertEqual(len(displayed), 4096)

    def test_fast_process_output_is_fully_drained_after_exit(self):
        with TemporaryDirectory() as directory:
            script = Path(directory) / "many_lines.py"
            script.write_text(
                "for number in range(5000):\n"
                "    print('line-{}'.format(number))\n"
                "raise SystemExit(3)\n",
                encoding="utf-8",
            )
            config = default_config()
            config["reports_enabled"] = False
            config["restart"]["crash_retries"] = 0
            config["notifications"] = {key: False for key in config["notifications"]}

            result = supervise_script(str(script), sys.executable, config, lambda event: None)

            output = (Path(result["run_directory"]) / "output.log").read_text(encoding="utf-8")
            crash = (Path(result["run_directory"]) / "crash_logs" / "attempt-001.log").read_text(encoding="utf-8")
            self.assertEqual(output.count("[attempt 1 stdout]"), 5000)
            self.assertIn("line-4999", output)
            self.assertIn("line-4999", crash)

    def test_queue_stops_after_first_terminal_failure(self):
        from monitor_runtime.supervisor import run_queue

        with TemporaryDirectory() as directory:
            first = Path(directory) / "first.py"
            second = Path(directory) / "second.py"
            marker = Path(directory) / "second-ran"
            first.write_text("raise SystemExit(2)\n", encoding="utf-8")
            second.write_text("open('second-ran','w').close()\n", encoding="utf-8")
            config = default_config()
            config["reports_enabled"] = False
            config["restart"]["crash_retries"] = 0
            outcome = run_queue([str(first), str(second)], sys.executable, config, lambda event: None)
            self.assertEqual(outcome, 1)
            self.assertFalse(marker.exists())

    def test_memory_limit_restart_is_budget_neutral_and_reports_its_threshold(self):
        with TemporaryDirectory() as directory:
            script = Path(directory) / "memory_then_success.py"
            script.write_text(
                "from pathlib import Path\n"
                "import time\n"
                "counter = Path('memory-attempt.txt')\n"
                "attempt = int(counter.read_text()) + 1 if counter.exists() else 1\n"
                "counter.write_text(str(attempt))\n"
                "if attempt == 1:\n"
                "    allocation = bytearray(64 * 1024 * 1024)\n"
                "    time.sleep(10)\n"
                "print('finished')\n",
                encoding="utf-8",
            )
            config = default_config()
            config["reports_enabled"] = False
            config["sampling_interval_seconds"] = 0.02
            config["notifications"] = {key: False for key in config["notifications"]}
            config["restart"].update({
                "crash_retries": 0,
                "memory_aware": True,
                "memory_limit_gb": 0.04,
                "scheduled_interval_minutes": 0,
            })
            events = []
            limited_pid = []

            def metrics(pid, started):
                counter = Path(directory) / "memory-attempt.txt"
                if not limited_pid and counter.exists() and counter.read_text() == "1":
                    limited_pid.append(pid)
                ram_mib = 64 if limited_pid == [pid] else 8
                return {
                    "elapsed_seconds": time.monotonic() - started,
                    "cpu_percent": 2,
                    "ram_bytes": ram_mib * 1024 * 1024,
                    "ram_mib": ram_mib,
                    "system_ram_total_bytes": 16_000_000_000,
                    "gpu_percent": None,
                    "gpu_memory_mib": None,
                    "gpu_memory_total_mib": None,
                    "gpu_scope": None,
                }

            with patch("monitor_runtime.supervisor._process_metrics", side_effect=metrics):
                result = supervise_script(str(script), sys.executable, config, events.append)

            decisions = [event for event in events if event["type"] == "restart_decision"]
            self.assertEqual(result["outcome"], "success")
            self.assertEqual(result["attempts"], 2)
            self.assertEqual(result["crash_restarts"], 0)
            self.assertEqual(result["memory_restarts"], 1)
            self.assertEqual(decisions[0]["reason"], "memory_limit")
            self.assertGreaterEqual(decisions[0]["ram_mib"], 40)
            self.assertEqual(decisions[0]["memory_limit_gb"], 0.04)
            self.assertGreaterEqual(decisions[0]["ram_bytes"], 40_000_000)

    def test_target_virtual_environment_remains_separate_from_supervisor(self):
        with TemporaryDirectory() as directory:
            root = Path(directory)
            target_venv = root / "target-venv"
            subprocess.run([sys.executable, "-m", "venv", str(target_venv)], check=True)
            target_python = target_venv / "bin" / "python"
            site_packages = subprocess.check_output(
                [str(target_python), "-c", "import sysconfig; print(sysconfig.get_paths()['purelib'])"],
                text=True,
            ).strip()
            Path(site_packages, "target_only_marker.py").write_text("VALUE = 'target-only'\n", encoding="utf-8")
            script = root / "marker.py"
            script.write_text("import sys, target_only_marker\nprint(target_only_marker.VALUE)\nprint(sys.executable)\n", encoding="utf-8")
            config = default_config()
            config["reports_enabled"] = False
            config["notifications"] = {key: False for key in config["notifications"]}
            events = []
            result = supervise_script(str(script), str(target_python), config, events.append)
            self.assertEqual(result["outcome"], "success")
            created = next(event for event in events if event["type"] == "run_created")
            self.assertEqual(created["interpreter"], os.path.realpath(target_python))
            self.assertIn("target-only", (Path(result["run_directory"]) / "output.log").read_text(encoding="utf-8"))
