"""Gmail configuration and delivery for portable notifications."""

from email.message import EmailMessage
import json
from pathlib import Path
import smtplib
import ssl


def send_notification(config_path, script, status, exit_code, elapsed):
    """Send one completion message using a local Gmail configuration.

    Args:
        config_path: Path to the JSON configuration.
        script: Name of the target script.
        status: Human-readable target status.
        exit_code: Shell-compatible target exit code.
        elapsed: Elapsed target time in seconds.

    Raises:
        ValueError: The configuration is incomplete or recipients are refused.
        OSError: The configuration or SMTP connection cannot be accessed.
    """
    with open(config_path, encoding="utf-8") as source:
        config = json.load(source)
    if not isinstance(config, dict):
        raise ValueError("Invalid configuration")
    sender = config.get("sender")
    password = config.get("app_password")
    recipients = config.get("recipients")
    if not isinstance(sender, str) or not sender.strip() or not isinstance(password, str) or not password.strip() or not isinstance(recipients, list) or not recipients or any(not isinstance(item, str) or not item.strip() for item in recipients):
        raise ValueError("Incomplete configuration")
    message = EmailMessage()
    message["From"] = sender
    message["To"] = ", ".join(recipients)
    message["Subject"] = "Python script {}: {}".format(status, Path(script).name)
    message.set_content("Script: {}\nStatus: {}\nExit code: {}\nElapsed time: {:.2f} seconds\n".format(Path(script).name, status, exit_code, elapsed))
    with smtplib.SMTP_SSL("smtp.gmail.com", 465, timeout=30, context=ssl.create_default_context()) as smtp:
        smtp.login(sender, password)
        refused = smtp.send_message(message)
        if refused:
            raise ValueError("Recipients refused")
