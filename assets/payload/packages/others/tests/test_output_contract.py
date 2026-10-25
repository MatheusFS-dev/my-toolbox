"""Tests for the shared Python terminal-output contract."""

import io
import os
import unittest
from unittest import mock

from packages.toolbox_output import TerminalOutput


class TerminalOutputTest(unittest.TestCase):
    """Checks deterministic formatting, color policy, and typed prompts."""

    def output(self, color="never"):
        """Create a deterministic formatter and its destination."""
        destination = io.StringIO()
        with mock.patch.dict(os.environ, {}, clear=True):
            output = TerminalOutput(
                destination,
                "mount-drive",
                clock=lambda: "09:32:23",
                color=color,
            )
        return output, destination

    def test_levels_open_once_prefix_each_line_and_reset_colors(self):
        """Prevent missing prefixes, opening gaps, and ANSI color bleed."""
        output, destination = self.output("256")

        output.info("Checking...\nsecond")
        output.prompt("Continue?")
        output.warn("Careful.")
        output.error("Failed.")
        output.success("Completed.")

        self.assertEqual(
            destination.getvalue(),
            "\n[09:32:23 mount-drive INFO] Checking...\n"
            "[09:32:23 mount-drive INFO] second\n"
            "\x1b[36m[09:32:23 mount-drive PROMPT] Continue?\x1b[0m\n"
            "\x1b[38;5;208m[09:32:23 mount-drive WARN] Careful.\x1b[0m\n"
            "\x1b[31m[09:32:23 mount-drive ERROR] Failed.\x1b[0m\n"
            "\x1b[32m[09:32:23 mount-drive SUCCESS] Completed.\x1b[0m\n",
        )

    def test_no_color_environment_disables_explicit_color(self):
        """Honor NO_COLOR even when the terminal otherwise supports color."""
        with mock.patch.dict(os.environ, {"NO_COLOR": "1"}):
            destination = io.StringIO()
            output = TerminalOutput(
                destination,
                "mount-drive",
                clock=lambda: "09:32:23",
                color="256",
            )
            output.warn("Plain.")
        self.assertNotIn("\x1b", destination.getvalue())

    def test_menu_prefixes_only_question(self):
        """Keep numbered options visually attached to one prompt."""
        output, destination = self.output()
        output.menu("Choose a device", ["1. First", "2. Second"])
        self.assertEqual(destination.getvalue(), "\n[09:32:23 mount-drive PROMPT] Choose a device\n  1. First\n  2. Second\n")

    def test_prompts_apply_defaults_and_retry_required_values(self):
        """Make displayed defaults functional and reject empty required text."""
        output, destination = self.output()
        answers = iter(["", "", "value"])

        self.assertTrue(output.ask_yes_no("Continue?", True, lambda: next(answers)))
        self.assertEqual(
            output.ask_required("Name", lambda: next(answers)),
            "value",
        )
        rendered = destination.getvalue()
        self.assertIn("Continue? [Y/n] (default: Y):", rendered)
        self.assertIn("A value is required; enter a value to continue.", rendered)

    def test_external_bytes_are_not_decoded_or_formatted(self):
        """Expose the original binary destination for external processes."""
        destination = io.BytesIO()
        output = TerminalOutput(destination, "tool", clock=lambda: "09:32:23")
        output.external.write(b"child\x00\xff\n")
        self.assertEqual(destination.getvalue(), b"child\x00\xff\n")

    def test_auto_classifies_legacy_messages_without_marking_step_success(self):
        """Migrate existing scripts while keeping final success explicit."""
        output, destination = self.output()

        output.auto("Warning: optional item skipped")
        output.auto("Error: write failed")
        output.auto("Installed one component")

        rendered = destination.getvalue()
        self.assertIn(" WARN] optional item skipped", rendered)
        self.assertIn(" ERROR] write failed", rendered)
        self.assertIn(" INFO] Installed one component", rendered)
        self.assertNotIn(" SUCCESS]", rendered)


if __name__ == "__main__":
    unittest.main()
