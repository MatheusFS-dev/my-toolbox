import os
import unittest
from unittest.mock import patch

from monitor_runtime.viewer import open_log_viewer


class ViewerTests(unittest.TestCase):
    def test_headless_environment_warns_and_continues(self):
        with patch.dict(os.environ, {}, clear=True):
            process, warning = open_log_viewer("/tmp/output.log")
        self.assertIsNone(process)
        self.assertIn("display", warning.lower())

    @patch("monitor_runtime.viewer.subprocess.Popen")
    @patch("monitor_runtime.viewer.shutil.which", return_value="/usr/bin/xterm")
    def test_viewer_tail_exits_with_monitor_runtime(self, _which, popen):
        with patch.dict(os.environ, {"DISPLAY": ":0"}, clear=True):
            open_log_viewer("/tmp/output.log")
        command = popen.call_args.args[0]
        self.assertIn("--pid={}".format(os.getpid()), command)
