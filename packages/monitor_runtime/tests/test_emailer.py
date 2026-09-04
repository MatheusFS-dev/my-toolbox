import unittest
from pathlib import Path
from tempfile import TemporaryDirectory

from monitor_runtime.emailer import build_message


class EmailTests(unittest.TestCase):
    def test_message_is_multipart_with_escaped_html_and_no_disclaimer(self):
        message = build_message(
            "completion",
            "Experiment A",
            ["a@example.com", "b@example.com"],
            ["<finished>", "plain & safe"],
            {"elapsed_seconds": 2, "cpu_percent": 1.5, "ram_mib": 12},
        )
        self.assertEqual(message["To"], "a@example.com, b@example.com")
        self.assertEqual(message["Subject"], "Experiment A — Completed")
        alternatives = list(message.iter_parts())
        self.assertEqual([part.get_content_type() for part in alternatives], ["text/plain", "text/html"])
        self.assertIn("&lt;finished&gt;", alternatives[1].get_content())
        self.assertIn("Run summary", alternatives[1].get_content())
        self.assertIn("Resource metrics", alternatives[1].get_content())
        self.assertIn("Latest output", alternatives[1].get_content())
        self.assertIn("max-width:680px", alternatives[1].get_content())
        self.assertNotIn("disclaimer", message.as_string().lower())

    def test_completion_can_embed_png_graphs(self):
        with TemporaryDirectory() as directory:
            graph = Path(directory) / "cpu.png"
            graph.write_bytes(b"fake-png")
            message = build_message("completion", "job.py", ["a@example.com"], [], {}, [str(graph)])
            attachments = [part for part in message.walk() if part.get_content_type() == "image/png"]
            self.assertEqual(len(attachments), 1)
            self.assertEqual(attachments[0].get_filename(), "cpu.png")
            html_part = next(part for part in message.walk() if part.get_content_type() == "text/html")
            self.assertIn("width:75%;max-width:480px", html_part.get_content())

    def test_title_is_safe_for_the_email_subject(self):
        """Keep user-entered titles on one header-safe line."""
        try:
            message = build_message("completion", "First\r\nSecond", ["a@example.com"], [], {})
        except ValueError as error:
            self.fail("title was not made header-safe: {}".format(error))
        self.assertEqual(message["Subject"], "First Second — Completed")

    def test_possible_code_error_email_identifies_the_rapid_failed_attempt(self):
        message = build_message(
            "possible_code_error",
            "Broken training",
            ["a@example.com"],
            ["Traceback", "NameError: missing_name"],
            {"attempt": 1, "attempt_duration_seconds": 0.42, "exit_code": 1, "remaining_retries": 9, "script": "/work/broken.py"},
        )
        self.assertEqual(message["Subject"], "Broken training — Possible code error")
        plain = message.get_body(preferencelist=("plain",)).get_content()
        html = message.get_body(preferencelist=("html",)).get_content()
        self.assertIn("Possible code error", plain)
        self.assertIn("Attempt duration: 0.42 s", plain)
        self.assertIn("Exit code: 1", plain)
        self.assertIn("Script: /work/broken.py", plain)
        self.assertIn("NameError: missing_name", html)

    def test_gpu_acronym_is_uppercase_in_plain_and_html_metrics(self):
        """Keep GPU capitalization in every rendered email body."""
        message = build_message(
            "heartbeat",
            "Training",
            ["a@example.com"],
            [],
            {"gpu_scope": "system-wide", "gpu_percent": 42},
        )
        plain = message.get_body(preferencelist=("plain",)).get_content()
        rendered_html = message.get_body(preferencelist=("html",)).get_content()
        self.assertIn("GPU scope", plain)
        self.assertIn("GPU scope", rendered_html)
        self.assertNotIn("Gpu", plain)
        self.assertNotIn("Gpu", rendered_html)
