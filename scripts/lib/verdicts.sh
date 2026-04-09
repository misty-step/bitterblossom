#!/usr/bin/env bash

# Git-native verdict refs for local review and landing.

verdict_ref() {
  local branch="${1:?branch required}"
  printf 'refs/verdicts/%s' "$branch"
}

verdict_branch_ref() {
  local branch="${1:?branch required}"
  printf 'refs/heads/%s' "$branch"
}

verdict_branch_head_sha() {
  local branch="${1:?branch required}"
  local ref

  ref="$(verdict_branch_ref "$branch")"
  git show-ref --verify --quiet "$ref" || {
    echo "unknown local branch: $branch" >&2
    return 1
  }

  git rev-parse "$ref"
}

verdict_evidence_dir() {
  local branch="${1:?branch required}"
  local stamp="${2:-$(date +%F)}"
  printf '.evidence/%s/%s' "$branch" "$stamp"
}

verdict_normalize_json() {
  local payload="${1:?payload required}"

  python3 - "$payload" <<'PY'
import json
import sys

payload = json.loads(sys.argv[1])
print(json.dumps(payload, sort_keys=True, separators=(",", ":")))
PY
}

verdict_read() {
  local branch="${1:?branch required}"
  local ref

  ref="$(verdict_ref "$branch")"
  git rev-parse --verify "$ref" >/dev/null 2>&1 || {
    echo "missing verdict ref: $ref" >&2
    return 1
  }

  git cat-file -p "$ref"
}

verdict_write() {
  local branch="${1:?branch required}"
  local payload="${2:?payload required}"
  local normalized blob_sha evidence_dir evidence_path

  normalized="$(verdict_normalize_json "$payload")" || return 1
  blob_sha="$(printf '%s\n' "$normalized" | git hash-object -w --stdin)" || return 1
  git update-ref "$(verdict_ref "$branch")" "$blob_sha" || return 1

  evidence_dir="$(verdict_evidence_dir "$branch")"
  evidence_path="$evidence_dir/verdict.json"
  mkdir -p "$evidence_dir"
  printf '%s\n' "$normalized" > "$evidence_path"
  printf '%s\n' "$evidence_path"
}

verdict_validate() {
  local branch="${1:?branch required}"
  shift
  local allowed_verdicts=("$@")
  local expected_sha payload

  if [[ ${#allowed_verdicts[@]} -eq 0 ]]; then
    allowed_verdicts=("ship")
  fi

  expected_sha="$(verdict_branch_head_sha "$branch")" || return 1

  payload="$(verdict_read "$branch")" || return 1

  python3 - "$branch" "$expected_sha" "$payload" "${allowed_verdicts[@]}" <<'PY'
import json
import sys

expected_branch = sys.argv[1]
expected_sha = sys.argv[2]
payload = json.loads(sys.argv[3])
allowed_verdicts = sys.argv[4:] or ["ship"]

branch = payload.get("branch")
sha = payload.get("sha")
verdict = payload.get("verdict")

if branch != expected_branch:
    raise SystemExit(f"verdict branch mismatch: expected {expected_branch}, got {branch}")

if sha != expected_sha:
    raise SystemExit(f"verdict sha mismatch: expected {expected_sha}, got {sha}")

if verdict not in {"ship", "conditional", "dont-ship"}:
    raise SystemExit(f"invalid verdict: {verdict}")

if verdict not in allowed_verdicts:
    allowed = ", ".join(allowed_verdicts)
    raise SystemExit(
        f"verdict {verdict} is not allowed for this operation; refresh review or require one of: {allowed}"
    )

print(f"validated verdict {verdict} for {branch} @ {sha}")
PY
}
