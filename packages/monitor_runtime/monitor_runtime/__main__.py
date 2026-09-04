"""JSON-lines entry point for the private Monitor supervisor runtime."""

import json
import os
import sys
from pathlib import Path

from .config import load_config, validate_credentials
from .emailer import build_message, send_message
from .protocol import PROTOCOL_VERSION, ProtocolError, emit, validate_request
from .supervisor import run_queue


def _sink(event):
    sys.stdout.write(json.dumps(event, ensure_ascii=False, separators=(",", ":")) + "\n")
    sys.stdout.flush()


def _load_json(path):
    with Path(path).open("r", encoding="utf-8") as stream:
        return json.load(stream)


def main():
    """Read one request from stdin, emit events, and return its process status."""
    try:
        line = sys.stdin.readline()
        if not line:
            raise ProtocolError("missing protocol request")
        request = validate_request(json.loads(line))
        emit("handshake", runtime_version="1.0.0", pid=os.getpid())
        config = load_config(Path(request.get("config_path", Path.home() / ".monitor" / "config.json")))
        if request["type"] == "run":
            credentials_path = request.get("credentials_path", str(Path.home() / ".monitor" / "credentials.json"))

            def notify(kind, title, output_lines, metrics, graph_paths=None):
                try:
                    credentials = validate_credentials(_load_json(credentials_path))
                    message = build_message(kind, title, config["recipients"], output_lines, metrics, graph_paths)
                    send_message(credentials, message)
                    emit("email_result", kind=kind, success=True, recipients=config["recipients"])
                    return True
                except Exception as error:
                    emit("email_result", kind=kind, success=False, error=str(error))
                    return False

            titles = request.get("titles") or [Path(script).name for script in request["scripts"]]
            return run_queue(request["scripts"], request["interpreter"], config, _sink, notify, titles)
        credentials_path = request.get("credentials_path", str(Path.home() / ".monitor" / "credentials.json"))
        credentials = validate_credentials(_load_json(credentials_path))
        message = build_message("test", "Monitor", config["recipients"], ["Email delivery is configured."], {})
        send_message(credentials, message)
        emit("email_result", kind="test", success=True, recipients=config["recipients"])
        return 0
    except KeyboardInterrupt:
        return 130
    except Exception as error:
        try:
            emit("final_outcome", outcome="error", error=str(error), exit_code=1)
        except Exception:
            pass
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
