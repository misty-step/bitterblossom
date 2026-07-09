#!/bin/sh
# Drill stub for bitterblossom-959 powder-chew: consumes the harness's
# stdin, polls the REAL Powder list-ready endpoint read-only, exercises the
# same repo-allowlist prefix filter the production card.md's selection
# oracle uses, and writes a REPORT.json -- never claims, never touches a
# repo, never opens more than the one read-only Powder call.
#
# This is the FAKE half of "real Powder, fake execution": the model/agent
# loop is replaced by this deterministic script, but POWDER_API_BASE_URL and
# POWDER_API_KEY are real -- resolved from the operator's own environment at
# dispatch time (declared `secrets` on agents/powder-chewer-stub.toml), so
# list-ready reads the live board, not a fixture.
set -eu
cat >/dev/null || true

ALLOWLIST="sploot"
NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)

RAW=$(powder list-ready --limit 20 2>&1) || RAW=""
SELECTED_ID=""
SELECTED_TITLE=""

# id convention: <repo>-<n>; a cheap prefix pre-filter, exactly like the
# production card's selection-oracle step 2 (a real get-card confirmation is
# skipped here on purpose -- the drill proves the read+filter path and never
# claims anything).
CANDIDATE_LINE=$(printf '%s\n' "$RAW" | awk -F'\t' -v allow="$ALLOWLIST" '
  { n=split(allow, reps, " ");
    for (i=1;i<=n;i++) if (index($1, reps[i] "-") == 1) { print; exit } }
')

if [ -n "$CANDIDATE_LINE" ]; then
  SELECTED_ID=$(printf '%s' "$CANDIDATE_LINE" | awk -F'\t' '{print $1}')
  SELECTED_TITLE=$(printf '%s' "$CANDIDATE_LINE" | awk -F'\t' '{print $3}')
fi

python3 - "$SELECTED_ID" "$SELECTED_TITLE" "$NOW" "$RAW" <<'PY' > REPORT.json
import json, sys
selected_id, selected_title, now, raw = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
report = {
    "schema_version": 1,
    "mode": "dev_drill",
    "repo_allowlist": ["sploot"],
    "would_select": ({"id": selected_id, "title": selected_title} if selected_id else None),
    "list_ready_raw_sample": raw.splitlines()[:20],
    "claimed": False,
    "note": (
        "DRILL STUB: read-only against the live Powder board; never claims, "
        "never opens a repo. Exercises the same repo-allowlist prefix filter "
        "the production card.md selection oracle (step 2) uses."
    ),
    "generated_at": now,
    "artifact_paths": ["REPORT.json"],
}
print(json.dumps(report, indent=2))
PY

echo "powder-chew dev drill: wrote REPORT.json (would_select=${SELECTED_ID:-none})"
