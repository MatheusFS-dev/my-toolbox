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
