"""Multipart Monitor email rendering and SMTP delivery."""

import html
import re
import smtplib
import ssl
from pathlib import Path
from email.message import EmailMessage
from email.utils import make_msgid
from typing import Any, Dict, Iterable, List


SUBJECTS = {
    "test": "{title} — Email test",
    "heartbeat": "{title} — Heartbeat",
    "recovery": "{title} — Recovered",
    "scheduled_restart": "{title} — Scheduled restart",
    "final_failure": "{title} — Failed",
    "completion": "{title} — Completed",
    "possible_leak": "{title} — Possible memory leak",
    "possible_code_error": "{title} — Possible code error",
}

STATUS_LABELS = {
    "test": "Email test",
    "heartbeat": "Running",
    "recovery": "Recovered",
    "scheduled_restart": "Restarted",
    "final_failure": "Failed",
    "completion": "Completed",
    "possible_leak": "Attention needed",
    "possible_code_error": "Possible code error",
}

CONTROL_SEQUENCE = re.compile(r"(?:\x1b\[[0-?]*[ -/]*[@-~])|[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]")


def build_message(kind: str, title: str, recipients: Iterable[str], output_lines: List[str], metrics: Dict[str, Any], graph_paths=None) -> EmailMessage:
    """Build a multipart notification with escaped target output.

    Args:
        kind: Supported notification template identifier.
        title: User-selected title for the monitored run.
        recipients: Destination email addresses.
        output_lines: Recent target output to include.
        metrics: Latest resource metrics or final run details.
        graph_paths: Optional PNG resource plots to embed.

    Returns:
        A plain-text and HTML email message ready for SMTP delivery.

    Raises:
        ValueError: If the notification template identifier is unsupported.
    """
    if kind not in SUBJECTS:
        raise ValueError("unsupported email template: {}".format(kind))
    clean_title = " ".join(CONTROL_SEQUENCE.sub("", title).split()).strip() or "Monitor"
    subject = SUBJECTS[kind].format(title=clean_title)
    status = STATUS_LABELS[kind]
    output_lines = [CONTROL_SEQUENCE.sub("", line) for line in output_lines]
    metric_lines = ["{}: {}".format(_metric_label(key), _metric_value(key, value)) for key, value in sorted(metrics.items())]
    latest_lines = list(output_lines[-10:]) or ["No recent output."]
    body_lines = [
        subject,
        "=" * len(subject),
        "",
        "Run summary",
        "Title: {}".format(clean_title),
        "Status: {}".format(status),
        "Notification: {}".format(kind.replace("_", " ")),
        "",
        "Resource metrics",
    ] + (metric_lines or ["No resource metrics available."]) + ["", "Latest output"] + latest_lines
    plain = "\n".join(body_lines).rstrip() + "\n"
    html_metrics = "".join(
        '<tr><td style="padding:7px 10px;color:#64748b;border-bottom:1px solid #e2e8f0">{}</td>'
        '<td style="padding:7px 10px;text-align:right;font-weight:600;border-bottom:1px solid #e2e8f0">{}</td></tr>'.format(
            html.escape(_metric_label(key)), html.escape(_metric_value(key, value)))
        for key, value in sorted(metrics.items())
    ) or '<tr><td style="padding:10px;color:#64748b">No resource metrics available.</td></tr>'
    html_output = "\n".join(html.escape(line) for line in latest_lines)
    graph_paths = graph_paths or []
    graph_ids = [(path, make_msgid(domain="monitor.local")) for path in graph_paths]
    html_graphs = "".join(
        '<div style="margin:0 0 14px"><div style="margin-bottom:6px;font-size:12px;font-weight:700;color:#475569">{}</div>'
        '<img src="cid:{}" alt="{} graph" style="display:block;width:75%;max-width:480px;height:auto;border:1px solid #e2e8f0;border-radius:8px"></div>'.format(
            html.escape(Path(path).stem.upper()), identifier[1:-1], html.escape(Path(path).stem))
        for path, identifier in graph_ids
    )
    graph_section = ""
    if html_graphs:
        graph_section = '<div style="padding:0 24px 22px"><h3 style="margin:0 0 12px;font-size:15px;color:#334155">Resource graphs</h3>{}</div>'.format(html_graphs)
    html_body = (
        '<html><body style="margin:0;background:#f1f5f9;font-family:Arial,sans-serif;color:#1e293b">'
        '<div style="max-width:680px;margin:24px auto;background:#ffffff;border:1px solid #e2e8f0;border-radius:12px;overflow:hidden">'
        '<div style="padding:22px 24px;background:#0f172a;color:#ffffff">'
        '<div style="font-size:12px;letter-spacing:1.4px;text-transform:uppercase;color:#7dd3fc">Monitor</div>'
        '<h1 style="margin:7px 0 4px;font-size:22px">{}</h1><div style="color:#cbd5e1">{}</div></div>'
        '<div style="padding:20px 24px 8px"><h2 style="margin:0 0 12px;font-size:16px">Run summary</h2>'
        '<table role="presentation" style="width:100%;border-collapse:collapse"><tr><td style="padding:7px 10px;color:#64748b">Title</td>'
        '<td style="padding:7px 10px;text-align:right;font-weight:600">{}</td></tr><tr><td style="padding:7px 10px;color:#64748b">Status</td>'
        '<td style="padding:7px 10px;text-align:right;font-weight:700">{}</td></tr></table></div>'
        '<div style="padding:14px 24px"><h3 style="margin:0 0 10px;font-size:15px;color:#334155">Resource metrics</h3>'
        '<table role="presentation" style="width:100%;border-collapse:collapse;background:#f8fafc;border-radius:8px">{}</table></div>'
        '<div style="padding:8px 24px 20px"><h3 style="margin:0 0 10px;font-size:15px;color:#334155">Latest output</h3>'
        '<pre style="margin:0;padding:14px;overflow:auto;background:#0f172a;color:#e2e8f0;border-radius:8px;font:12px/1.5 monospace">{}</pre></div>'
        '{}<div style="padding:14px 24px;background:#f8fafc;color:#64748b;font-size:12px">Monitor notification · {}</div>'
        '</div></body></html>'
    ).format(
        html.escape(clean_title), html.escape(status), html.escape(clean_title), html.escape(status),
        html_metrics, html_output, graph_section, html.escape(kind.replace("_", " ")),
    )
    message = EmailMessage()
    message["Subject"] = subject
    message["To"] = ", ".join(recipients)
    message.set_content(plain)
    message.add_alternative(html_body, subtype="html")
    html_part = message.get_payload()[1]
    for path, identifier in graph_ids:
        with open(path, "rb") as stream:
            html_part.add_related(stream.read(), maintype="image", subtype="png", cid=identifier, filename=Path(path).name)
    return message


def _metric_label(key: str) -> str:
    labels = {
        "cpu_percent": "CPU usage",
        "attempt": "Attempt",
        "attempt_duration_seconds": "Attempt duration",
        "exit_code": "Exit code",
        "elapsed_seconds": "Elapsed time",
        "gpu_memory_mib": "GPU memory",
        "gpu_memory_total_mib": "Total GPU memory",
        "gpu_percent": "GPU usage",
        "gpu_scope": "GPU scope",
        "ram_mib": "RAM usage",
        "remaining_retries": "Remaining retries",
        "system_ram_total_bytes": "Total system RAM",
    }
    if key in labels:
        return labels[key]
    acronyms = {"cpu": "CPU", "gpu": "GPU", "ram": "RAM", "mib": "MiB", "gb": "GB", "pid": "PID"}
    words = [acronyms.get(word, word) for word in key.split("_")]
    value = " ".join(words)
    return value[:1].upper() + value[1:]


def _metric_value(key: str, value: Any) -> str:
    if value is None:
        return "Unavailable"
    if key in {"cpu_percent", "gpu_percent"}:
        return "{}%".format(value)
    if key in {"ram_mib", "gpu_memory_mib", "gpu_memory_total_mib"}:
        return "{} MiB".format(value)
    if key == "system_ram_total_bytes":
        return "{:.3f} GB".format(float(value) / 1000000000)
    if key == "elapsed_seconds":
        return "{} s".format(value)
    if key == "attempt_duration_seconds":
        return "{} s".format(value)
    return str(value)


def send_message(credentials: Dict[str, Any], message: EmailMessage) -> None:
    """Deliver one message using implicit TLS or STARTTLS credentials."""
    sender = credentials["sender"]
    message["From"] = sender
    context = ssl.create_default_context()
    if credentials["security"] == "tls":
        client = smtplib.SMTP_SSL(credentials["host"], int(credentials["port"]), timeout=30, context=context)
    elif credentials["security"] == "starttls":
        client = smtplib.SMTP(credentials["host"], int(credentials["port"]), timeout=30)
        client.ehlo()
        client.starttls(context=context)
        client.ehlo()
    else:
        raise ValueError("SMTP security must be starttls or tls")
    try:
        client.login(sender, credentials["password"])
        refused = client.send_message(message, from_addr=sender, to_addrs=[address.strip() for address in message["To"].split(",")])
        if refused:
            raise smtplib.SMTPRecipientsRefused(refused)
    finally:
        client.quit()
