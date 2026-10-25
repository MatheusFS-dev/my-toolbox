"""Native terminal output contract for the stress-gpu application."""
import datetime
import os
import re
import sys


class TerminalOutput:
    """Render human-facing stress-gpu messages and prompts."""

    def __init__(self, destination=None, clock=None, color=None):
        """Initialize a terminal output destination.

        Args:
            destination (TextIO | None): Message destination, defaulting to stdout.
            clock (Callable[[], str] | None): Injectable local-time clock.
            color (bool | None): Explicit color mode, or automatic detection.
        """
        self.destination = destination or sys.stdout
        self.clock = clock or (lambda: datetime.datetime.now().strftime("%H:%M:%S"))
        self.color = self._detect_color() if color is None else color
        self.started = False

    def _detect_color(self):
        return (
            "NO_COLOR" not in os.environ
            and os.environ.get("TERM") != "dumb"
            and hasattr(self.destination, "isatty")
            and self.destination.isatty()
        )

    def _line(self, level, message, color_code=""):
        if not self.started:
            self.destination.write("\n")
            self.started = True
        for part in str(message).splitlines() or [""]:
            rendered = "[{0} stress-gpu {1}] {2}".format(self.clock(), level, part)
            if self.color and color_code:
                rendered = "\033[{0}m{1}\033[0m".format(color_code, rendered)
            self.destination.write(rendered + "\n")
        self.destination.flush()

    def info(self, message):
        """Write an informational message."""
        self._line("INFO", message)

    def prompt(self, message):
        """Write a prompt message."""
        self._line("PROMPT", message, "36")

    def menu(self, question, options):
        """Write one labeled question followed by unprefixed choices.

        Args:
            question (str): Menu question shown as a prompt.
            options (list[str]): Display lines for the choices.
        """
        self.prompt(question)
        for option in options:
            rendered = "  " + str(option)
            if self.color:
                rendered = "\033[36m" + rendered + "\033[0m"
            self.destination.write(rendered + "\n")
        self.destination.flush()

    def warn(self, message):
        """Write a warning message."""
        color = "38;5;208" if "256color" in os.environ.get("TERM", "") else "33"
        self._line("WARN", message, color)

    def error(self, message):
        """Write an error message."""
        self._line("ERROR", message, "31")

    def success(self, message):
        """Write the final successful outcome."""
        self._line("SUCCESS", message, "32")

    def ask(self, message, input_fn=input):
        """Display a contract prompt and collect one answer.

        Args:
            message (str): Prompt text including any visible default.
            input_fn (Callable[[str], str]): Input function.

        Returns:
            str: Unmodified answer from the input function.
        """
        text = message.rstrip()
        default_match = re.search(r"\[(Y/n|y/N)\]", text)
        if default_match:
            default_name = "Y" if default_match.group(1) == "Y/n" else "n"
            text = text.rstrip(": ") + " (default: " + default_name + "):"
        else:
            text = text.rstrip(": ")
        self.prompt(text)
        return input_fn("")


class ContractWriter:
    """Adapt file-style status writes to contract messages."""

    def __init__(self, output):
        """Store the terminal output renderer.

        Args:
            output (TerminalOutput): Renderer receiving informational writes.
        """
        self.output = output

    def write(self, message):
        """Write non-empty lines as informational messages."""
        message = message.strip("\n")
        if message:
            self.output.info(message)

