#!/bin/sh
# Thin Bitterblossom command-harness wrapper around Cerberus. It translates the
# bb run payload into `cerberus review-pr`, then emits a bb.command_result.v1
# object so the ledger can capture available Cerberus cost/token telemetry.
set -eu

event_file="${BB_EVENT_FILE:-EVENT.json}"
run_file="${BB_RUN_FILE:-RUN.json}"
out_dir="${CERBERUS_REVIEW_OUT_DIR:-cerberus-review}"
summary_target="${CERBERUS_SUMMARY_TARGET:-check-run}"
timeout_seconds="${CERBERUS_TIMEOUT_SECONDS:-900}"
harness="${CERBERUS_HARNESS:-}"

if [ -z "$harness" ]; then
  if command -v opencode >/dev/null 2>&1; then
    harness="opencode"
  elif command -v omp >/dev/null 2>&1; then
    harness="omp"
  else
    harness="opencode"
  fi
fi

if [ "$harness" = "omp" ] && [ -n "${HOME:-}" ]; then
  bun_binary="$(command -v bun 2>/dev/null || true)"
  if [ -n "$bun_binary" ]; then
    mkdir -p "$HOME/.local/bin"
    ln -sf "$bun_binary" "$HOME/.local/bin/bun"
  fi
fi

if [ ! -f "$event_file" ]; then
  echo "EVENT.json not found" >&2
  exit 64
fi
if [ ! -f "$run_file" ]; then
  echo "RUN.json not found" >&2
  exit 64
fi

eval "$(
  python3 - "$event_file" "$run_file" <<'PY'
import json
import shlex
import sys

event_path, run_path = sys.argv[1], sys.argv[2]
with open(event_path, "r", encoding="utf-8") as f:
    event = json.load(f)
with open(run_path, "r", encoding="utf-8") as f:
    run = json.load(f)

pr = event.get("pr") or event.get("number") or event.get("pull_request", {}).get("number")
repo = event.get("repo") or event.get("repository", {}).get("full_name")
head_sha = event.get("head_sha") or event.get("pull_request", {}).get("head", {}).get("sha") or ""
measurement = bool(event.get("measurement")) or event.get("comment") is False
task = run.get("task", "")

if not repo or not pr:
    raise SystemExit("EVENT.json must name repo and pr")

values = {
    "BB_REVIEW_REPO": repo,
    "BB_REVIEW_PR": str(pr),
    "BB_REVIEW_HEAD_SHA": head_sha,
    "BB_REVIEW_TASK": task,
    "BB_REVIEW_MEASUREMENT": "1" if measurement else "0",
}
for key, value in values.items():
    print(f"{key}={shlex.quote(value)}")
PY
)"

mode="post"
if [ "$BB_REVIEW_MEASUREMENT" = "1" ] || [ "$BB_REVIEW_TASK" != "review" ] || [ "${CERBERUS_REVIEW_FORCE_DRY_RUN:-0}" = "1" ]; then
  mode="dry-run"
fi

set -- review-pr \
  --number "$BB_REVIEW_PR" \
  --repo "$BB_REVIEW_REPO" \
  --out-dir "$out_dir" \
  --summary-target "$summary_target" \
  --request-id "bb:${BB_REVIEW_REPO}#${BB_REVIEW_PR}:${BB_REVIEW_HEAD_SHA:-manual}" \
  --timeout-seconds "$timeout_seconds" \
  --receipt-bundle "$out_dir/receipt-bundle.json"

if [ -n "${OPENROUTER_API_KEY:-}" ]; then
  set -- "$@" --allow-env OPENROUTER_API_KEY
fi

if [ "$mode" = "dry-run" ]; then
  set -- "$@" --dry-run
else
  set -- "$@" --post
fi

if [ -n "${CERBERUS_FIXTURE_OUTPUT:-}" ]; then
  set -- "$@" --harness fixture --fixture-output "$CERBERUS_FIXTURE_OUTPUT"
else
  set -- "$@" --harness "$harness"
fi

if [ -n "${CERBERUS_MODEL:-}" ]; then
  set -- "$@" --model "$CERBERUS_MODEL"
fi
if [ -n "${CERBERUS_GH_BINARY:-}" ]; then
  set -- "$@" --gh-binary "$CERBERUS_GH_BINARY"
fi
if [ -n "${CERBERUS_OPENCODE_BINARY:-}" ]; then
  set -- "$@" --opencode-binary "$CERBERUS_OPENCODE_BINARY"
elif [ "$harness" = "opencode" ]; then
  opencode_binary="$(command -v opencode 2>/dev/null || true)"
  if [ -n "$opencode_binary" ]; then
    set -- "$@" --opencode-binary "$opencode_binary"
  fi
fi
if [ -n "${CERBERUS_OPENCODE_AGENT:-}" ]; then
  set -- "$@" --opencode-agent "$CERBERUS_OPENCODE_AGENT"
fi
if [ -n "${CERBERUS_OMP_BINARY:-}" ]; then
  set -- "$@" --omp-binary "$CERBERUS_OMP_BINARY"
elif [ "$harness" = "omp" ]; then
  omp_binary="$(command -v omp 2>/dev/null || true)"
  if [ -n "$omp_binary" ]; then
    set -- "$@" --omp-binary "$omp_binary"
  fi
fi

if [ -n "${CERBERUS_BIN:-}" ]; then
  "$CERBERUS_BIN" "$@"
elif [ -f "./cerberus/Cargo.toml" ] && command -v cargo >/dev/null 2>&1; then
  cargo run --locked --manifest-path ./cerberus/Cargo.toml --quiet -- "$@"
elif command -v cerberus >/dev/null 2>&1; then
  cerberus "$@"
elif [ -x "./cerberus/target/debug/cerberus" ]; then
  ./cerberus/target/debug/cerberus "$@"
else
  echo "cerberus binary not found; set CERBERUS_BIN or include ./cerberus with cargo" >&2
  exit 127
fi

python3 - "$BB_REVIEW_REPO" "$BB_REVIEW_PR" "$mode" "$out_dir" <<'PY'
import json
import os
import sys

repo, pr, mode, out_dir = sys.argv[1:5]

def load_json(path):
    try:
        with open(path, "r", encoding="utf-8") as f:
            return json.load(f)
    except FileNotFoundError:
        return None

artifact_path = os.path.join(out_dir, "artifact.json")
receipt_path = os.path.join(out_dir, "receipt-bundle.json")
review_path = os.path.join(out_dir, "review.md")
post_plan_path = os.path.join(out_dir, "post-plan.json")
post_result_path = os.path.join(out_dir, "post-result.json")

artifact = load_json(artifact_path)
receipt = load_json(receipt_path)
post_result = load_json(post_result_path)

tokens_in = None
tokens_out = None
cost = None
if artifact:
    run = artifact.get("run") or {}
    raw_cost = run.get("cost_usd")
    if raw_cost is not None:
        try:
            cost = float(raw_cost)
        except (TypeError, ValueError):
            pass
    for receipt_row in artifact.get("receipts") or []:
        usage = receipt_row.get("usage") or {}
        tokens_in = tokens_in or usage.get("prompt_tokens")
        tokens_out = tokens_out or usage.get("completion_tokens")
        if cost is None and usage.get("cost_usd") is not None:
            cost = usage.get("cost_usd")

review_markdown = None
try:
    with open(review_path, "r", encoding="utf-8") as f:
        review_markdown = f.read()
except FileNotFoundError:
    pass

report = {
    "schema_version": "bb.cerberus_review_report.v1",
    "repo": repo,
    "pr": int(pr),
    "mode": mode,
    "comment_posted": mode == "post",
    "cerberus": {
        "out_dir": out_dir,
        "artifact_path": artifact_path if os.path.exists(artifact_path) else None,
        "receipt_bundle_path": receipt_path if os.path.exists(receipt_path) else None,
        "review_markdown_path": review_path if os.path.exists(review_path) else None,
        "post_plan_path": post_plan_path if os.path.exists(post_plan_path) else None,
        "post_result_path": post_result_path if os.path.exists(post_result_path) else None,
    },
    "artifact": artifact,
    "receipt_bundle": receipt,
    "post_result": post_result,
    "review_markdown": review_markdown,
    "usage": {
        "tokens_in": tokens_in,
        "tokens_out": tokens_out,
        "cost_usd": cost,
    },
    "artifact_paths": ["REPORT.json"],
}
with open("REPORT.json", "w", encoding="utf-8") as f:
    json.dump(report, f, indent=2, sort_keys=True)
    f.write("\n")

print(json.dumps({
    "schema_version": "bb.command_result.v1",
    "result": f"cerberus review {mode} complete for {repo}#{pr}",
    "tokens_in": tokens_in,
    "tokens_out": tokens_out,
    "cost_usd": cost,
}, sort_keys=True))
PY
