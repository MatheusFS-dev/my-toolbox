import io
import json
import sys
import unittest
import os
import signal
import subprocess
import time
from pathlib import Path
from tempfile import TemporaryDirectory
from unittest.mock import patch

from monitor_runtime.__main__ import main
from monitor_runtime.config import default_config, save_json_atomic


class MainTests(unittest.TestCase):
    def test_run_request_emits_handshake_and_final_outcome(self):
        with TemporaryDirectory() as directory:
            root = Path(directory)
            script = root / "ok.py"
            script.write_text("print('ok')\n", encoding="utf-8")
            config = default_config()
            config["reports_enabled"] = False
            config_path = root / "config.json"
            save_json_atomic(config_path, config)
            request = {"protocol_version": 1, "type": "run", "scripts": [str(script)], "interpreter": sys.executable, "config_path": str(config_path)}
            output = io.StringIO()
            with patch("sys.stdin", io.StringIO(json.dumps(request) + "\n")), patch("sys.stdout", output):
                code = main()
            events = [json.loads(line) for line in output.getvalue().splitlines()]
            self.assertEqual(code, 0)
            self.assertEqual(events[0]["type"], "handshake")
            self.assertEqual(events[-1]["type"], "final_outcome")

    def test_sigint_finalizes_run_and_returns_130(self):
        with TemporaryDirectory() as directory:
            root = Path(directory)
            ready = root / "ready"
            script = root / "wait.py"
            script.write_text("from pathlib import Path\nimport time\nPath('ready').touch()\ntime.sleep(60)\n", encoding="utf-8")
            config = default_config()
            config["reports_enabled"] = False
            config["notifications"] = {key: False for key in config["notifications"]}
            config_path = root / "config.json"
            save_json_atomic(config_path, config)
            request = {"protocol_version": 1, "type": "run", "scripts": [str(script)], "interpreter": sys.executable, "config_path": str(config_path)}
            environment = dict(os.environ)
            environment["PYTHONPATH"] = str(Path(__file__).parents[1])
            process = subprocess.Popen(
                [sys.executable, "-m", "monitor_runtime"], stdin=subprocess.PIPE,
                stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
                env=environment, start_new_session=True,
            )
            process.stdin.write(json.dumps(request) + "\n")
            process.stdin.close()
            for _ in range(200):
                if ready.exists():
                    break
                time.sleep(0.01)
            process.send_signal(signal.SIGINT)
            output = process.stdout.read()
            error = process.stderr.read()
            process.wait(timeout=10)
            process.stdout.close()
            process.stderr.close()
            self.assertEqual(process.returncode, 130, error)
            events = [json.loads(line) for line in output.splitlines()]
            self.assertEqual(events[-1]["outcome"], "cancelled")

    def test_custom_title_reaches_completion_email_and_final_event(self):
        """Preserve the selected title through runtime notifications and events."""
        with TemporaryDirectory() as directory:
            root = Path(directory)
            script = root / "ok.py"
            script.write_text("print('ok')\n", encoding="utf-8")
            config = default_config()
            config["recipients"] = ["alerts@example.com"]
            config["reports_enabled"] = False
            config["notifications"] = {key: key == "completion" for key in config["notifications"]}
            config_path = root / "config.json"
            credentials_path = root / "credentials.json"
            save_json_atomic(config_path, config)
            save_json_atomic(credentials_path, {
                "host": "smtp.example.com", "port": 465, "security": "tls",
                "sender": "sender@example.com", "password": "secret",
            })
            request = {
                "protocol_version": 1, "type": "run", "scripts": [str(script)],
                "titles": ["Experiment A"], "interpreter": sys.executable,
                "config_path": str(config_path), "credentials_path": str(credentials_path),
            }
            output = io.StringIO()
            sent = []
            with patch("sys.stdin", io.StringIO(json.dumps(request) + "\n")), patch("sys.stdout", output), patch("monitor_runtime.__main__.send_message", side_effect=lambda _, message: sent.append(message)):
                code = main()
            events = [json.loads(line) for line in output.getvalue().splitlines()]
            self.assertEqual(code, 0)
            self.assertEqual(sent[0]["Subject"], "Experiment A — Completed")
            self.assertEqual(events[-1]["title"], "Experiment A")
