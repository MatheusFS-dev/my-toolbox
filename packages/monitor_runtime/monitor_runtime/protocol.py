"""Versioned JSON-lines protocol shared by Monitor's frontend and runtime."""

import json
import sys
from typing import Any, Dict


PROTOCOL_VERSION = 1
REQUEST_TYPES = {"run", "test_email"}
EVENT_TYPES = {
    "handshake", "run_created", "attempt_started", "attempt_ended",
    "target_output", "resource_sample", "lifecycle", "restart_decision",
    "email_result", "final_outcome",
}
SECRET_KEYS = {"password", "credentials", "smtp_password", "secret"}


class ProtocolError(ValueError):
    """Raised for malformed or unsupported protocol messages."""


def validate_request(message: Dict[str, Any]) -> Dict[str, Any]:
    """Validate one frontend request without accepting inline secrets."""
    if not isinstance(message, dict):
        raise ProtocolError("request must be a JSON object")
    if message.get("protocol_version") != PROTOCOL_VERSION:
        raise ProtocolError("unsupported protocol_version")
    if _contains_secret_key(message):
        raise ProtocolError("secret values are not accepted in protocol requests")
    request_type = message.get("type")
    if request_type not in REQUEST_TYPES:
        raise ProtocolError("unsupported request type")
    if request_type == "run":
        scripts = message.get("scripts")
        if not isinstance(scripts, list) or not scripts or not all(isinstance(item, str) and item for item in scripts):
            raise ProtocolError("run request requires scripts")
        interpreter = message.get("interpreter")
        if not isinstance(interpreter, str) or not interpreter:
            raise ProtocolError("run request requires interpreter")
        titles = message.get("titles")
        if titles is not None and (
                not isinstance(titles, list)
                or len(titles) != len(scripts)
                or not all(isinstance(item, str) and item.strip() for item in titles)):
            raise ProtocolError("run request titles must match scripts")
    return message


def _contains_secret_key(value: Any) -> bool:
    if isinstance(value, dict):
        return any(str(key).lower() in SECRET_KEYS or _contains_secret_key(item) for key, item in value.items())
    if isinstance(value, list):
        return any(_contains_secret_key(item) for item in value)
    return False


def emit(event_type: str, **values: Any) -> None:
    """Write one validated event as a flushed JSON line to stdout."""
    if event_type not in EVENT_TYPES:
        raise ProtocolError("unsupported event type")
    event = {"protocol_version": PROTOCOL_VERSION, "type": event_type}
    event.update(values)
    sys.stdout.write(json.dumps(event, ensure_ascii=False, separators=(",", ":")) + "\n")
    sys.stdout.flush()
