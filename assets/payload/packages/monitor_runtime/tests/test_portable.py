import contextlib
import io
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

from portable_monitor import run


class PortableTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.script = self.root / 'target with spaces.py'
        self.config = self.root / 'config.json'
        self.config.write_text(json.dumps({'sender': 'sender@example.com', 'app_password': 'secret', 'recipients': ['to@example.com']}))

    def invoke(self, code, args=None):
        self.script.write_text(code)
        with patch('portable_monitor.emailer.smtplib.SMTP_SSL') as smtp:
            smtp.return_value.__enter__.return_value.send_message.return_value = {}
            result = run(str(self.script), args, str(self.config))
        return result, smtp

    def test_success_runs_once_with_arguments_and_sends_message(self):
        marker = self.root / 'runs'
        result, smtp = self.invoke('import sys\nfrom pathlib import Path\np=Path(sys.argv[1])\np.write_text(p.read_text() + "x" if p.exists() else "x")\nassert sys.argv[2] == "hello world"\n', [str(marker), 'hello world'])
        self.assertEqual(result, 0)
        self.assertEqual(marker.read_text(), 'x')
        smtp.assert_called_once()
        message = smtp.return_value.__enter__.return_value.send_message.call_args.args[0]
        self.assertIn('Exit code: 0', message.get_content())
        self.assertNotIn('secret', message.as_string())

    def test_failure_and_signal_status(self):
        result, smtp = self.invoke('raise SystemExit(7)')
        self.assertEqual(result, 7)
        self.assertIn('Exit code: 7', smtp.return_value.__enter__.return_value.send_message.call_args.args[0].get_content())
        if os.name != 'nt':
            result, smtp = self.invoke('import os, signal\nos.kill(os.getpid(), signal.SIGTERM)')
            self.assertEqual(result, 128 + signal.SIGTERM)
            self.assertIn('signal', smtp.return_value.__enter__.return_value.send_message.call_args.args[0].get_content().lower())

    def test_missing_or_invalid_config_preserves_result(self):
        for content in (None, '{', '{}', json.dumps({'sender':'a', 'app_password':'', 'recipients':['b']})):
            with self.subTest(content=content):
                if content is None:
                    self.config.unlink(missing_ok=True)
                else:
                    self.config.write_text(content)
                error = io.StringIO()
                with contextlib.redirect_stderr(error):
                    result, smtp = self.invoke('raise SystemExit(4)')
                self.assertEqual(result, 4)
                self.assertRegex(error.getvalue(), r'^\n\[\d{2}:\d{2}:\d{2} portable-monitor ERROR\] Notification failed\.')
                self.assertNotIn('secret', error.getvalue())
                smtp.assert_not_called()

    def test_smtp_errors_and_refusals_are_safe(self):
        self.script.write_text('raise SystemExit(3)')
        for failure in ('connect', 'refused'):
            error = io.StringIO()
            with self.subTest(failure=failure), contextlib.redirect_stderr(error), patch('portable_monitor.emailer.smtplib.SMTP_SSL') as smtp:
                if failure == 'connect':
                    smtp.side_effect = OSError('secret')
                else:
                    smtp.return_value.__enter__.return_value.send_message.return_value = {'to@example.com': (550, b'secret')}
                self.assertEqual(run(str(self.script), config_path=str(self.config)), 3)
            self.assertIn('notification', error.getvalue().lower())
            self.assertNotIn('secret', error.getvalue())

    def test_inherited_streams_and_environment(self):
        self.script.write_text('import os, sys\nprint(os.environ["PORTABLE_TEST_VALUE"])\nprint("target error", file=sys.stderr)\n')
        env = dict(os.environ, PORTABLE_TEST_VALUE='target output')
        env['PYTHONPATH'] = str(Path(__file__).resolve().parents[1] / 'portable')
        command = [sys.executable, '-c', 'from portable_monitor import run; import sys; sys.exit(run(sys.argv[1], config_path=sys.argv[2]))', str(self.script), str(self.root / 'missing')]
        result = subprocess.run(command, text=True, capture_output=True, env=env, cwd=self.root)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout, 'target output\n')
        self.assertIn('target error\n', result.stderr)

    def test_start_failure_still_notifies(self):
        with patch('portable_monitor.runner.subprocess.run', side_effect=OSError('failure')), patch('portable_monitor.emailer.smtplib.SMTP_SSL') as smtp:
            smtp.return_value.__enter__.return_value.send_message.return_value = {}
            self.assertEqual(run(str(self.script), config_path=str(self.config)), 1)
        self.assertIn('Exit code: 1', smtp.return_value.__enter__.return_value.send_message.call_args.args[0].get_content())
