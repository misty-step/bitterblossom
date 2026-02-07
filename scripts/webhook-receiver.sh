#!/usr/bin/env bash
set -euo pipefail

# webhook-receiver.sh — Minimal sprite event collector
#
# Usage:
#   ./scripts/webhook-receiver.sh
#   ./scripts/webhook-receiver.sh --port 18790

usage() {
    local exit_code="${1:-0}"
    cat <<'EOF'
Usage: webhook-receiver.sh [--port <port>] [--event-log <path>] [--alert-log <path>]

Defaults:
  port:      18790
  event-log: /var/data/sprite-events.jsonl
  alert-log: /tmp/sprite-alerts.jsonl
EOF
    exit "$exit_code"
}

PORT="${PORT:-18790}"
EVENT_LOG="${EVENT_LOG:-/var/data/sprite-events.jsonl}"
ALERT_LOG="${ALERT_LOG:-/tmp/sprite-alerts.jsonl}"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --port) PORT="${2:-}"; shift 2 ;;
        --event-log) EVENT_LOG="${2:-}"; shift 2 ;;
        --alert-log) ALERT_LOG="${2:-}"; shift 2 ;;
        --help|-h) usage 0 ;;
        *) echo "ERROR: Unknown argument: $1" >&2; usage 1 ;;
    esac
done

export PORT EVENT_LOG ALERT_LOG
exec python3 - <<'PY'
import json
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PORT = int(os.environ["PORT"])
EVENT_LOG = os.environ["EVENT_LOG"]
ALERT_LOG = os.environ["ALERT_LOG"]
HIGH_PRIORITY = {"task_complete", "task_blocked", "task_failed"}

os.makedirs(os.path.dirname(EVENT_LOG), exist_ok=True)
os.makedirs(os.path.dirname(ALERT_LOG), exist_ok=True)

class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(length)
        try:
            payload = json.loads(body.decode("utf-8"))
            if not isinstance(payload, dict):
                raise ValueError("payload must be a JSON object")
        except Exception:
            self.send_response(400)
            self.end_headers()
            self.wfile.write(b'{"ok":false,"error":"invalid json"}')
            return

        line = json.dumps(payload, separators=(",", ":"))
        with open(EVENT_LOG, "a", encoding="utf-8") as f:
            f.write(line + "\n")
        if payload.get("event") in HIGH_PRIORITY:
            with open(ALERT_LOG, "a", encoding="utf-8") as f:
                f.write(line + "\n")

        self.send_response(200)
        self.end_headers()
        self.wfile.write(b'{"ok":true}')

    def log_message(self, fmt, *args):
        return

print(f"webhook receiver listening on :{PORT}")
ThreadingHTTPServer(("0.0.0.0", PORT), Handler).serve_forever()
PY
