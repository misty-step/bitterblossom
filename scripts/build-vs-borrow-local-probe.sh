#!/bin/sh
set -eu

cd "$(dirname "$0")/.."

include_timeout=0
report_path=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --include-timeout)
      include_timeout=1
      shift
      ;;
    --report)
      [ "$#" -ge 2 ] || { echo "--report requires a path" >&2; exit 2; }
      report_path=$2
      shift 2
      ;;
    *)
      echo "usage: $0 [--include-timeout] [--report PATH]" >&2
      exit 2
      ;;
  esac
done

BB_BIN=${BB_BIN:-./target/debug/bb}
if [ ! -x "$BB_BIN" ]; then
  cargo build --quiet --bin bb
fi

tmp=$(mktemp -d "${TMPDIR:-/tmp}/bb-build-vs-borrow-probe.XXXXXX")
cleanup() {
  rm -rf "$tmp"
}
trap cleanup EXIT INT TERM

mkdir -p "$tmp/agents" "$tmp/tasks/probe"

cat > "$tmp/probe-agent.sh" <<'SH'
#!/bin/sh
set -eu

mode=$(python3 - <<'PY'
import json
import pathlib

event = pathlib.Path("EVENT.json")
if event.exists():
    print(json.loads(event.read_text()).get("mode", "happy"))
else:
    print("happy")
PY
)

: "${BB_PROBE_SECRET:?BB_PROBE_SECRET missing}"
argv=$(ps -ww -p $$ -o args= || true)
if printf '%s' "$argv" | grep -F "$BB_PROBE_SECRET" >/dev/null 2>&1; then
  echo "secret leaked through argv" >&2
  exit 41
fi

echo "progress:prepared"
echo "progress:executing" >&2

case "$mode" in
  happy)
    mkdir -p nested
    printf 'nested artifact body\n' > nested/NOTE.txt
    python3 - <<'PY'
import json
import os
import pathlib

report = {
    "schema_version": "bb.coding_harness_probe.v1",
    "status": "pass",
    "mode": "happy",
    "secret_present": bool(os.environ.get("BB_PROBE_SECRET")),
    "secret_leaked_to_argv": False,
    "artifact_paths": ["REPORT.json", "nested/NOTE.txt"],
}
pathlib.Path("REPORT.json").write_text(json.dumps(report, sort_keys=True) + "\n")
PY
    printf '{"schema_version":"bb.command_result.v1","result":"probe happy","tokens_in":0,"tokens_out":0,"turns":1,"cost_usd":0.0}\n'
    ;;
  missing_artifact)
    printf '{"schema_version":"bb.command_result.v1","result":"probe missing artifact","tokens_in":0,"tokens_out":0,"turns":1,"cost_usd":0.0}\n'
    ;;
  timeout)
    sleep 90
    ;;
  *)
    echo "unknown mode: $mode" >&2
    exit 42
    ;;
esac
SH
chmod +x "$tmp/probe-agent.sh"

cat > "$tmp/plane.toml" <<'TOML'
dev = true
db_path = ".bb/plane.db"

[budget]
max_cost_per_day_usd = 1.0
TOML

cat > "$tmp/agents/probe.toml" <<TOML
version = 1
harness = "command"
model = ""
bin = "sh"
args = ["$tmp/probe-agent.sh"]
secrets = ["BB_PROBE_SECRET"]
TOML

cat > "$tmp/tasks/probe/task.toml" <<'TOML'
agent = "probe"
substrate = "local"
required_artifacts = ["REPORT.json"]

[budget]
timeout_minutes = 1
max_runs_per_day = 10
max_cost_per_run_usd = 0.01

[[trigger]]
kind = "manual"
TOML

cat > "$tmp/tasks/probe/card.md" <<'MD'
# Build-vs-borrow local substrate probe

Run the local command harness probe. The executable receives mode through
EVENT.json and a declared secret through BB_PROBE_SECRET.
MD

secret="bb-probe-secret-$$"
export BB_PROBE_SECRET=$secret

"$BB_BIN" --config "$tmp" check --json >/dev/null
"$BB_BIN" --config "$tmp" preflight probe --json >/dev/null

results="$tmp/results.jsonl"
: > "$results"

run_mode() {
  mode=$1
  out="$tmp/$mode.stdout.json"
  err="$tmp/$mode.stderr.txt"
  set +e
  "$BB_BIN" --config "$tmp" run probe \
    --idempotency-key "probe:$mode" \
    --payload "{\"mode\":\"$mode\"}" \
    --json >"$out" 2>"$err"
  code=$?
  set -e
  python3 - "$tmp" "$mode" "$code" "$out" "$err" <<'PY' >> "$results"
import json
import os
import pathlib
import sqlite3
import sys

root = pathlib.Path(sys.argv[1])
mode = sys.argv[2]
exit_code = int(sys.argv[3])
stdout_path = pathlib.Path(sys.argv[4])
stderr_path = pathlib.Path(sys.argv[5])
secret = os.environ["BB_PROBE_SECRET"]

conn = sqlite3.connect(root / ".bb" / "plane.db")
conn.row_factory = sqlite3.Row
run = conn.execute(
    "select id, state, state_reason, cost_usd, duration_ms from runs where idempotency_key = ?",
    (f"probe:{mode}",),
).fetchone()
if run is None:
    raise SystemExit(f"missing run row for {mode}")
attempt = conn.execute(
    """select phase, outcome, error, exit_code, tokens_in, tokens_out, turns, cost_usd, artifact_dir
       from attempts where run_id = ? order by n desc limit 1""",
    (run["id"],),
).fetchone()
events = [
    dict(row)
    for row in conn.execute(
        "select kind, data from run_events where run_id = ? order by id",
        (run["id"],),
    )
]
artifact_dir = pathlib.Path(attempt["artifact_dir"]) if attempt and attempt["artifact_dir"] else None
artifact_paths = []
artifact_text = ""
if artifact_dir and artifact_dir.exists():
    for path in sorted(p for p in artifact_dir.rglob("*") if p.is_file()):
        rel = path.relative_to(artifact_dir).as_posix()
        artifact_paths.append(rel)
        if path.stat().st_size <= 200000:
            try:
                artifact_text += path.read_text(errors="replace")
            except OSError:
                pass
stdout_text = stdout_path.read_text(errors="replace")
stderr_text = stderr_path.read_text(errors="replace")

print(json.dumps({
    "mode": mode,
    "bb_exit_code": exit_code,
    "run": dict(run),
    "attempt": dict(attempt) if attempt else None,
    "events": events,
    "artifact_paths": artifact_paths,
    "secret_leaked": secret in (stdout_text + stderr_text + artifact_text),
    "stdout_bytes": len(stdout_text),
    "stderr_bytes": len(stderr_text),
}, sort_keys=True))
PY
}

run_mode happy
run_mode missing_artifact
if [ "$include_timeout" = 1 ]; then
  run_mode timeout
fi

python3 - "$results" "$include_timeout" "$report_path" <<'PY'
import json
import pathlib
import sys

results_path = pathlib.Path(sys.argv[1])
include_timeout = sys.argv[2] == "1"
report_path = pathlib.Path(sys.argv[3]) if sys.argv[3] else None
rows = [json.loads(line) for line in results_path.read_text().splitlines() if line.strip()]
by_mode = {row["mode"]: row for row in rows}

checks = []

happy = by_mode["happy"]
checks.append({
    "name": "happy run succeeds",
    "pass": happy["run"]["state"] == "success" and happy["attempt"]["outcome"] == "success",
    "evidence": {"run_id": happy["run"]["id"], "artifacts": happy["artifact_paths"]},
})
checks.append({
    "name": "declared secret is not in captured logs or artifacts",
    "pass": not any(row["secret_leaked"] for row in rows),
    "evidence": {row["mode"]: row["secret_leaked"] for row in rows},
})

missing = by_mode["missing_artifact"]
missing_error = (missing["attempt"] or {}).get("error") or ""
checks.append({
    "name": "missing REPORT.json fails the run",
    "pass": missing["run"]["state"] == "failure" and "missing required artifact" in missing_error,
    "evidence": {"run_id": missing["run"]["id"], "error": missing_error},
})

if include_timeout:
    timeout = by_mode["timeout"]
    timeout_error = (timeout["attempt"] or {}).get("error") or ""
    checks.append({
        "name": "timeout kills executing harness",
        "pass": timeout["run"]["state"] == "failure" and "timeout" in timeout_error,
        "evidence": {"run_id": timeout["run"]["id"], "error": timeout_error},
    })

report = {
    "schema_version": "bb.build_vs_borrow.local_probe_report.v1",
    "candidate": "bitterblossom-local-substrate",
    "workload": "bb-coding-harness-probe.v1",
    "timeout_probe_included": include_timeout,
    "checks": checks,
    "runs": rows,
    "status": "pass" if all(check["pass"] for check in checks) else "fail",
}

body = json.dumps(report, indent=2, sort_keys=True) + "\n"
if report_path:
    report_path.write_text(body)
print(body, end="")
if report["status"] != "pass":
    raise SystemExit(1)
PY
