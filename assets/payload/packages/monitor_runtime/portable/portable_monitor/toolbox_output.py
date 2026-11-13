"""Shared terminal output and prompt helpers for Python toolbox tools."""

from __future__ import print_function

import datetime
try:
    import builtins
except ImportError:
    import __builtin__ as builtins
import os
import re
import sys


class TerminalOutput(object):
    """Format human-facing toolbox messages while preserving external bytes."""

    def __init__(self, stream, tool, clock=None, color=None):
        """Initialize one tool's output lifecycle.

        Args:
            stream: Text stream for toolbox-owned messages, or a binary stream
                when only ``external`` is used.
            tool: Stable command name displayed in each prefix.
            clock: Optional callable returning ``HH:MM:SS`` for deterministic
                tests. Defaults to the current local time.
            color: Optional ``never``, ``basic``, or ``256`` override.
        """
        self.stream = stream
        self.tool = tool
        self.clock = clock or self._local_time
        self.color = self._detect_color(stream) if color is None else color
        if "NO_COLOR" in os.environ:
            self.color = "never"
        self.started = False

    @property
    def external(self):
        """Return the original destination for byte-for-byte child output."""
        return self.stream

    def info(self, message):
        """Write an uncolored informational message."""
        self._write("INFO", message, None)

    def prompt(self, message):
        """Write a cyan prompt message."""
        self._write("PROMPT", message, "36")

    def menu(self, question, options, color_options=True):
        """Write one labeled question followed by unprefixed choices.

        Args:
            question: Menu question shown as a prompt.
            options: Display lines for the available choices.
            color_options: Apply prompt color to the choice lines.
        """
        self.prompt(question)
        for option in options:
            rendered = "  " + str(option)
            if self.color != "never" and color_options:
                rendered = "\x1b[36m" + rendered + "\x1b[0m"
            self.stream.write(rendered + "\n")
        self.stream.flush()

    def warn(self, message):
        """Write an orange warning, falling back to basic yellow."""
        color = "38;5;208" if self.color == "256" else "33"
        self._write("WARN", message, color)

    def error(self, message):
        """Write a red actionable error message."""
        self._write("ERROR", message, "31")

    def success(self, message):
        """Write the tool's single green success summary."""
        self._write("SUCCESS", message, "32")

    def auto(self, message):
        """Classify one legacy human message during incremental migration.

        Args:
            message: Human-facing text without machine-readable data.
        """
        text = str(message)
        lowered = text.lower()
        if lowered.startswith(("error: ", "failed: ")):
            self.error(text.split(":", 1)[1].strip())
        elif lowered.startswith(("warning: ", "warn: ")):
            self.warn(text.split(":", 1)[1].strip())
        elif "invalid" in lowered or "required" in lowered or lowered.startswith(
            ("enter ", "please ", "skipping ", "skip ")
        ):
            self.warn(text)
        else:
            self.info(text)

    def ask_yes_no(self, question, default, input_function=None):
        """Ask a yes/no question until a valid answer is entered.

        Args:
            question: Prompt without the default suffix.
            default: Boolean returned for an empty answer.
            input_function: Callable returning one input line.

        Returns:
            bool: Parsed answer or the displayed default.
        """
        reader = input_function or input
        default_name = "Y" if default else "n"
        marker = "Y/n" if default else "y/N"
        while True:
            self.prompt("{0} [{1}] (default: {2}):".format(question, marker, default_name))
            answer = reader().strip().lower()
            if not answer:
                return default
            if answer in ("y", "yes"):
                return True
            if answer in ("n", "no"):
                return False
            self.warn(
                "Enter yes or no, or press Enter for {0}.".format(default_name)
            )

    def ask_required(self, question, input_function=None):
        """Ask for non-empty text and explain empty-input retries.

        Args:
            question: Prompt title without punctuation.
            input_function: Callable returning one input line.

        Returns:
            str: Non-empty stripped input.
        """
        reader = input_function or input
        while True:
            self.prompt("{0}:".format(question))
            answer = reader().strip()
            if answer:
                return answer
            self.warn("A value is required; enter a value to continue.")

    def _write(self, level, message, color):
        if not self.started:
            self.stream.write("\n")
            self.started = True
        for text in str(message).rstrip("\n").split("\n"):
            rendered = "[{0} {1} {2}] {3}".format(
                self.clock(), self.tool, level, text
            )
            if self.color != "never" and color:
                rendered = "\x1b[{0}m{1}\x1b[0m".format(color, rendered)
            self.stream.write(rendered + "\n")
        self.stream.flush()

    @staticmethod
    def _local_time():
        return datetime.datetime.now().strftime("%H:%M:%S")

    @staticmethod
    def _detect_color(stream):
        if "NO_COLOR" in os.environ:
            return "never"
        if not hasattr(stream, "isatty") or not stream.isatty():
            return "never"
        if os.environ.get("TERM", "").lower() == "dumb":
            return "never"
        if "256color" in os.environ.get("TERM", "").lower() or os.environ.get(
            "COLORTERM"
        ):
            return "256"
        return "basic"


def terminal_output(tool, stream=None):
    """Create a formatter for one command using stdout by default.

    Args:
        tool: Stable command name.
        stream: Optional destination stream.

    Returns:
        TerminalOutput: Configured output lifecycle.
    """
    return TerminalOutput(stream or sys.stdout, tool)


def legacy_print(tool, stdout=None, stderr=None):
    """Return a print-compatible formatter for a legacy Python entrypoint.

    Args:
        tool: Stable command name.
        stdout: Optional standard-output replacement.
        stderr: Optional standard-error replacement.

    Returns:
        callable: A ``print``-compatible function. Non-terminal file writes
        continue through the original built-in unchanged.
    """
    standard_output = stdout or sys.stdout
    standard_error = stderr or sys.stderr
    outputs = {
        standard_output: TerminalOutput(standard_output, tool),
        standard_error: TerminalOutput(standard_error, tool),
    }

    def write(*values, **keywords):
        destination = keywords.get("file", standard_output)
        if destination not in outputs or keywords.get("end", "\n") != "\n":
            return builtins.print(*values, **keywords)
        separator = keywords.get("sep", " ")
        outputs[destination].auto(separator.join(str(value) for value in values))
        return None

    write.output = outputs[standard_output]
    return write


def legacy_input(tool, stream=None, output=None):
    """Return an input-compatible prompt formatter for a legacy entrypoint.

    Args:
        tool: Stable command name.
        stream: Optional prompt destination.

    Returns:
        callable: An ``input``-compatible reader with explicit defaults.
    """
    prompt_output = output or TerminalOutput(stream or sys.stdout, tool)

    def read(prompt=""):
        text = prompt.strip()
        default_match = re.search(r"\[(Y/n|y/N)\]", text)
        if default_match:
            default_name = "Y" if default_match.group(1) == "Y/n" else "n"
            text = text.rstrip(": ") + " (default: " + default_name + "):"
        text = re.sub(r"\(default:\s*([^)]*)\)", r"[default: \1]", text)
        if not default_match:
            text = text.rstrip(": ")
        prompt_output.prompt(text)
        return builtins.input()

    return read
