"""Monitor configuration defaults, validation, and atomic persistence."""

import json
import math
import os
import re
import tempfile
from pathlib import Path
from typing import Any, Dict, Iterable


SCHEMA_VERSION = 1
EMAIL_PATTERN = re.compile(r"^[^\s@]+@[^\s@]+\.[^\s@]+$")


class ConfigError(ValueError):
    """Raised when persisted Monitor configuration is invalid."""


def default_config() -> Dict[str, Any]:
    """Return a new configuration containing Monitor's approved defaults."""
    return {
        "schema_version": SCHEMA_VERSION,
        "recipients": [],
        "notifications": {
            "heartbeat": False,
            "recovery": True,
            "scheduled_restart": True,
            "final_failure": True,
            "completion": True,
            "possible_leak": True,
            "possible_code_error": True,
        },
        "heartbeat_interval_minutes": 60,
        "restart": {
            "crash_retries": 10,
            "base_delay_seconds": 3,
            "backoff_multiplier": 1.2,
            "max_delay_seconds": 30,
            "rapid_crash_seconds": 60,
            "scheduled_interval_minutes": 0,
            "memory_aware": False,
            "memory_limit_gb": 1.0,
        },
        "leak_detection": {
            "enabled": True,
            "warmup_seconds": 300,
            "window_seconds": 300,
            "minimum_growth_mib": 100,
            "minimum_slope_mib_per_minute": 5,
        },
        "sampling_interval_seconds": 1,
        "reports_enabled": True,
        "gui_viewer": False,
    }


def validate_recipients(recipients: Iterable[str]) -> list:
    """Validate and normalize an iterable of recipient email addresses."""
    result = []
    for recipient in recipients:
        normalized = recipient.strip()
        if not EMAIL_PATTERN.fullmatch(normalized):
            raise ConfigError("invalid recipient email address: {}".format(recipient))
        if normalized not in result:
            result.append(normalized)
    if not result:
        raise ConfigError("at least one recipient is required")
    return result


def validate_credentials(credentials: Dict[str, Any]) -> Dict[str, Any]:
    """Validate SMTP credentials while retaining the password only in memory."""
    if not isinstance(credentials, dict):
        raise ConfigError("credentials must be a JSON object")
    required = ("host", "port", "security", "sender", "password")
    if any(not credentials.get(key) for key in required):
        raise ConfigError("SMTP credentials are incomplete")
    if credentials["security"] not in ("starttls", "tls"):
        raise ConfigError("SMTP security must be starttls or tls")
    try:
        port = int(credentials["port"])
    except (TypeError, ValueError):
        raise ConfigError("SMTP port must be an integer")
    if port < 1 or port > 65535:
        raise ConfigError("SMTP port is outside the valid range")
    sender = credentials["sender"].strip()
    if not EMAIL_PATTERN.fullmatch(sender):
        raise ConfigError("invalid SMTP sender address")
    return {"host": credentials["host"].strip(), "port": port, "security": credentials["security"], "sender": sender, "password": credentials["password"]}


def validate_config(config: Dict[str, Any]) -> Dict[str, Any]:
    """Validate persisted configuration and merge missing current defaults."""
    if not isinstance(config, dict):
        raise ConfigError("configuration must be a JSON object")
    schema = config.get("schema_version")
    if isinstance(schema, int) and schema > SCHEMA_VERSION:
        raise ConfigError("configuration uses a newer schema version")
    if schema != SCHEMA_VERSION:
        raise ConfigError("unsupported configuration schema version")
    merged = default_config()
    configured_restart = config.get("restart", {})
    for key, value in config.items():
        if isinstance(value, dict) and isinstance(merged.get(key), dict):
            merged[key].update(value)
        else:
            merged[key] = value
    restart = merged["restart"]
    if "memory_limit_gb" not in configured_restart and "memory_limit_mib" in configured_restart:
        try:
            restart["memory_limit_gb"] = float(configured_restart["memory_limit_mib"]) * 1048576 / 1000000000
        except (TypeError, ValueError):
            raise ConfigError("memory restart limit must be numeric")
    restart.pop("memory_limit_mib", None)
    if merged["recipients"]:
        merged["recipients"] = validate_recipients(merged["recipients"])
    if merged["sampling_interval_seconds"] <= 0:
        raise ConfigError("sampling interval must be positive")
    if merged["heartbeat_interval_minutes"] <= 0:
        raise ConfigError("heartbeat interval must be positive")
    if restart["crash_retries"] < 0 or restart["base_delay_seconds"] < 0:
        raise ConfigError("restart values cannot be negative")
    try:
        rapid_crash_seconds = float(restart.get("rapid_crash_seconds", 0))
    except (TypeError, ValueError):
        raise ConfigError("rapid-crash threshold must be numeric seconds")
    if not math.isfinite(rapid_crash_seconds) or rapid_crash_seconds <= 0:
        raise ConfigError("rapid-crash threshold must be positive seconds")
    restart["rapid_crash_seconds"] = rapid_crash_seconds
    if restart.get("scheduled_interval_minutes", 0) < 0:
        raise ConfigError("scheduled restart interval cannot be negative")
    if restart.get("memory_aware") and restart.get("scheduled_interval_minutes", 0) > 0:
        raise ConfigError("memory-aware and time-scheduled restarts cannot both be enabled")
    try:
        memory_limit_gb = float(restart.get("memory_limit_gb", 0))
    except (TypeError, ValueError):
        raise ConfigError("memory restart limit must be numeric")
    if restart.get("memory_aware") and (not math.isfinite(memory_limit_gb) or memory_limit_gb <= 0):
        raise ConfigError("memory restart limit must be positive")
    restart["memory_limit_gb"] = memory_limit_gb
    if merged["leak_detection"].get("warmup_seconds", 0) <= 0:
        raise ConfigError("memory-leak warm-up interval must be positive")
    return merged


def load_config(path: Path) -> Dict[str, Any]:
    """Load and validate a UTF-8 JSON configuration file."""
    try:
        with Path(path).open("r", encoding="utf-8") as stream:
            return validate_config(json.load(stream))
    except ConfigError:
        raise
    except (OSError, ValueError) as error:
        raise ConfigError("cannot load configuration: {}".format(error))


def save_json_atomic(path: Path, value: Dict[str, Any]) -> None:
    """Atomically save JSON with current-user-only permissions."""
    destination = Path(path)
    destination.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    os.chmod(str(destination.parent), 0o700)
    descriptor, temporary = tempfile.mkstemp(prefix=".{}-".format(destination.name), suffix=".tmp", dir=str(destination.parent))
    try:
        os.fchmod(descriptor, 0o600)
        with os.fdopen(descriptor, "w", encoding="utf-8") as stream:
            descriptor = -1
            json.dump(value, stream, indent=2, sort_keys=True)
            stream.write("\n")
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, str(destination))
        os.chmod(str(destination), 0o600)
    finally:
        if descriptor >= 0:
            os.close(descriptor)
        try:
            os.unlink(temporary)
        except FileNotFoundError:
            pass
