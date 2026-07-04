#!/bin/sh
# Thin Bitterblossom command-harness wrapper around Cerberus. It translates the
# bb run payload into `cerberus review-pr`, then emits a bb.command_result.v1
# object so the ledger can capture available Cerberus cost/token telemetry.
set -eu

event_file="${BB_EVENT_FILE:-EVENT.json}"
run_file="${BB_RUN_FILE:-RUN.json}"
out_dir="${CERBERUS_REVIEW_OUT_DIR:-cerberus-review}"
summary_target="${CERBERUS_SUMMARY_TARGET:-status}"
timeout_seconds="${CERBERUS_TIMEOUT_SECONDS:-900}"
harness="${CERBERUS_HARNESS:-container-opencode}"
openrouter_provisioning_env="${CERBERUS_OPENROUTER_PROVISIONING_KEY_ENV:-OPENROUTER_API_KEY}"
openrouter_key_limit_usd="${CERBERUS_OPENROUTER_KEY_LIMIT_USD:-1.25}"
container_binary="${CERBERUS_CONTAINER_BINARY:-}"
container_egress_allow_host="${CERBERUS_CONTAINER_EGRESS_ALLOW_HOST:-openrouter.ai:443}"

validate_env_name() {
  label="$1"
  value="$2"
  stripped=$(printf '%s' "$value" | tr -d 'A-Za-z0-9_')
  case "$value" in
    [A-Za-z_]*) ;;
    *)
      echo "$label must be a valid environment variable name, got: $value" >&2
      exit 64
      ;;
  esac
  if [ -n "$stripped" ]; then
    echo "$label must be a valid environment variable name, got: $value" >&2
    exit 64
  fi
}

env_value_present() {
  name="$1"
  [ -n "$(eval "printf '%s' \"\${${name}:-}\"")" ]
}

if [ "$harness" = "container-opencode" ] && [ -z "$container_binary" ]; then
  if [ -n "${CERBERUS_OPENCODE_BINARY:-}" ]; then
    container_binary="$CERBERUS_OPENCODE_BINARY"
  else
    container_binary="$(command -v opencode 2>/dev/null || true)"
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

# Cerberus refuses ambient `gh` auth for both reads and posting; it requires an
# explicit token source. The default is a bot/app token env declared on the
# review agent, not the operator's personal GH_TOKEN.
#
# gh_token_env names the env var, it is never used as data, but the value
# still reaches an indirect-expansion `eval` below. Validate it as a plain
# shell identifier first so a malformed override can't inject shell syntax
# into this process's environment.
gh_token_env="${CERBERUS_GH_TOKEN_ENV:-CERBERUS_REVIEW_GH_TOKEN}"
validate_env_name "CERBERUS_GH_TOKEN_ENV" "$gh_token_env"
case "$gh_token_env" in
  GH_TOKEN|GITHUB_TOKEN)
    echo "CERBERUS_GH_TOKEN_ENV must name a bot/app token env, not operator env $gh_token_env" >&2
    exit 64
    ;;
esac
if env_value_present "$gh_token_env"; then
  set -- "$@" --gh-token-env "$gh_token_env"
else
  echo "warning: \$${gh_token_env} is unset; review-pr will refuse ambient gh auth" >&2
fi

# Backlog 104: never forward OPENROUTER_API_KEY as raw child env. BB injects
# this run's scoped per-workload-family provider key by name, and Cerberus M1
# uses it only as the explicit provisioning source for a per-review capped key.
if [ -z "${CERBERUS_FIXTURE_OUTPUT:-}" ]; then
  validate_env_name "CERBERUS_OPENROUTER_PROVISIONING_KEY_ENV" "$openrouter_provisioning_env"
  if ! env_value_present "$openrouter_provisioning_env"; then
    echo "$openrouter_provisioning_env is unset; cannot mint a scoped Cerberus review key" >&2
    exit 64
  fi
  set -- "$@" \
    --openrouter-scoped-key \
    --openrouter-provisioning-key-env "$openrouter_provisioning_env" \
    --openrouter-key-limit-usd "$openrouter_key_limit_usd"
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
  if [ "$harness" = "container-opencode" ]; then
    if [ -n "$container_binary" ]; then
      set -- "$@" --container-binary "$container_binary"
    fi
    if [ -n "$container_egress_allow_host" ]; then
      set -- "$@" --container-egress-allow-host "$container_egress_allow_host"
    fi
    if [ -n "${CERBERUS_DOCKER_BINARY:-}" ]; then
      set -- "$@" --docker-binary "$CERBERUS_DOCKER_BINARY"
    fi
    if [ -n "${CERBERUS_CONTAINER_IMAGE:-}" ]; then
      set -- "$@" --container-image "$CERBERUS_CONTAINER_IMAGE"
    fi
    if [ -n "${CERBERUS_CONTAINER_HOST_ROOT:-}" ]; then
      set -- "$@" --container-host-root "$CERBERUS_CONTAINER_HOST_ROOT"
    fi
  fi
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

def numeric(value):
    if value is None or isinstance(value, bool):
        return None
    if isinstance(value, (int, float)):
        return value
    try:
        return float(value)
    except (TypeError, ValueError):
        return None

def add_numeric(total, value):
    value = numeric(value)
    if value is None:
        return total
    return value if total is None else total + value

def normalize_number(value):
    if isinstance(value, float) and value.is_integer():
        return int(value)
    return value

tokens_in = None
tokens_out = None
cost = None
if artifact:
    run = artifact.get("run") or {}
    cost = numeric(run.get("cost_usd"))
    receipt_cost = None
    for receipt_row in artifact.get("receipts") or []:
        usage = receipt_row.get("usage") or {}
        tokens_in = add_numeric(tokens_in, usage.get("prompt_tokens"))
        tokens_out = add_numeric(tokens_out, usage.get("completion_tokens"))
        receipt_cost = add_numeric(receipt_cost, usage.get("cost_usd"))
    if cost is None:
        cost = receipt_cost
tokens_in = normalize_number(tokens_in)
tokens_out = normalize_number(tokens_out)
cost = normalize_number(cost)

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
