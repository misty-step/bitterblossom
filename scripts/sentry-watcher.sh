#!/usr/bin/env bash
# sentry-watcher.sh — Polls Sentry API for new unresolved issues across all Misty Step projects
# Compares against last check state, outputs JSON anomaly report.
# Designed to run as a cron job.
#
# Usage: ./sentry-watcher.sh [--quiet] [--state-file /path/to/state.json]
# Output: JSON report to stdout (pipe to file or consume in cron)
#
# Requires: curl, jq, SENTRY_MASTER_TOKEN in ~/.secrets

set -euo pipefail

# ─── Config ───────────────────────────────────────────────────────────────────
STATE_FILE="${SENTRY_STATE_FILE:-/tmp/sentry-state.json}"
ORG="misty-step"
API_BASE="https://sentry.io/api/0"
QUIET=false

# Parse args
while [[ $# -gt 0 ]]; do
  case "$1" in
    --quiet) QUIET=true; shift ;;
    --state-file) STATE_FILE="$2"; shift 2 ;;
    *) echo "Unknown arg: $1" >&2; exit 1 ;;
  esac
done

# ─── Load Token ───────────────────────────────────────────────────────────────
if [[ -z "${SENTRY_MASTER_TOKEN:-}" ]]; then
  if [[ -f "$HOME/.secrets" ]]; then
    source "$HOME/.secrets"
  fi
fi

if [[ -z "${SENTRY_MASTER_TOKEN:-}" ]]; then
  echo '{"error": "SENTRY_MASTER_TOKEN not set. Source ~/.secrets or export it."}' >&2
  exit 1
fi

AUTH_HEADER="Authorization: Bearer $SENTRY_MASTER_TOKEN"

# ─── Helper Functions ─────────────────────────────────────────────────────────
api_get() {
  local endpoint="$1"
  curl -sf -H "$AUTH_HEADER" "${API_BASE}${endpoint}" 2>/dev/null || echo "[]"
}

log() {
  if [[ "$QUIET" != "true" ]]; then
    echo "[sentry-watcher] $*" >&2
  fi
}

now_iso() {
  date -u +"%Y-%m-%dT%H:%M:%SZ"
}

now_epoch() {
  date +%s
}

# ─── Load Previous State ─────────────────────────────────────────────────────
if [[ -f "$STATE_FILE" ]]; then
  PREV_STATE=$(cat "$STATE_FILE")
  PREV_TIMESTAMP=$(echo "$PREV_STATE" | jq -r '.timestamp // "1970-01-01T00:00:00Z"')
  PREV_ISSUES=$(echo "$PREV_STATE" | jq -r '.known_issue_ids // []')
  log "Loaded previous state from $STATE_FILE (last check: $PREV_TIMESTAMP)"
else
  PREV_STATE="{}"
  PREV_TIMESTAMP="1970-01-01T00:00:00Z"
  PREV_ISSUES="[]"
  log "No previous state found. First run — all issues will be reported as new."
fi

# ─── Fetch All Projects ──────────────────────────────────────────────────────
log "Fetching projects for org: $ORG"
PROJECTS_JSON=$(api_get "/organizations/$ORG/projects/")
PROJECT_SLUGS=$(echo "$PROJECTS_JSON" | jq -r '.[].slug')
PROJECT_COUNT=$(echo "$PROJECTS_JSON" | jq 'length')
log "Found $PROJECT_COUNT projects"

# ─── Scan All Projects ───────────────────────────────────────────────────────
ALL_ISSUES="[]"
NEW_ISSUES="[]"
RESOLVED_ISSUES="[]"
PROJECT_SUMMARIES="[]"
ANOMALIES="[]"
CURRENT_KNOWN_IDS="[]"

for slug in $PROJECT_SLUGS; do
  log "Scanning: $slug"

  # Fetch unresolved issues (up to 100)
  ISSUES=$(api_get "/projects/$ORG/$slug/issues/?query=is:unresolved&limit=100")
  ISSUE_COUNT=$(echo "$ISSUES" | jq 'length')

  # Fetch 24h event stats
  SINCE_24H=$(($(now_epoch) - 86400))
  STATS=$(api_get "/projects/$ORG/$slug/stats/?stat=received&resolution=1h&since=$SINCE_24H")
  EVENTS_24H=$(echo "$STATS" | jq '[.[][][1] // .[].[-1] // 0] | add // 0' 2>/dev/null || echo "$STATS" | jq '[.[] | .[1]] | add // 0' 2>/dev/null || echo "0")

  # Extract issue IDs and details
  ISSUE_IDS=$(echo "$ISSUES" | jq '[.[].id]')
  CURRENT_KNOWN_IDS=$(echo "$CURRENT_KNOWN_IDS $ISSUE_IDS" | jq -s 'add | unique')

  # Find NEW issues (not in previous state)
  PROJECT_NEW=$(echo "$ISSUES" | jq --argjson prev "$PREV_ISSUES" '
    [.[] | select(.id as $id | ($prev | index($id)) == null) |
      {id, title, level, count: (.count|tonumber), firstSeen, lastSeen, isUnhandled, priority, project: "'"$slug"'"}
    ]')
  NEW_COUNT=$(echo "$PROJECT_NEW" | jq 'length')

  # Check for high-frequency issues (>10 events)
  HIGH_FREQ=$(echo "$ISSUES" | jq '[.[] | select((.count|tonumber) > 10) | {id, title, count: (.count|tonumber), project: "'"$slug"'"}]')
  HIGH_FREQ_COUNT=$(echo "$HIGH_FREQ" | jq 'length')

  # Check for fatal/unhandled issues
  UNHANDLED=$(echo "$ISSUES" | jq '[.[] | select(.isUnhandled == true) | {id, title, level, count: (.count|tonumber), project: "'"$slug"'"}]')
  UNHANDLED_COUNT=$(echo "$UNHANDLED" | jq 'length')

  # Build anomalies for this project
  if [[ "$NEW_COUNT" -gt 0 ]]; then
    ANOMALY=$(jq -n --arg slug "$slug" --arg type "new_issues" --argjson count "$NEW_COUNT" --argjson issues "$PROJECT_NEW" \
      '{project: $slug, type: $type, count: $count, issues: $issues}')
    ANOMALIES=$(echo "$ANOMALIES" | jq --argjson a "[$ANOMALY]" '. + $a')
  fi

  if [[ "$HIGH_FREQ_COUNT" -gt 0 ]]; then
    ANOMALY=$(jq -n --arg slug "$slug" --arg type "high_frequency" --argjson count "$HIGH_FREQ_COUNT" --argjson issues "$HIGH_FREQ" \
      '{project: $slug, type: $type, count: $count, issues: $issues}')
    ANOMALIES=$(echo "$ANOMALIES" | jq --argjson a "[$ANOMALY]" '. + $a')
  fi

  if [[ "$UNHANDLED_COUNT" -gt 0 ]]; then
    ANOMALY=$(jq -n --arg slug "$slug" --arg type "unhandled_exceptions" --argjson count "$UNHANDLED_COUNT" --argjson issues "$UNHANDLED" \
      '{project: $slug, type: $type, count: $count, issues: $issues}')
    ANOMALIES=$(echo "$ANOMALIES" | jq --argjson a "[$ANOMALY]" '. + $a')
  fi

  # Project summary
  SUMMARY=$(jq -n \
    --arg slug "$slug" \
    --argjson total "$ISSUE_COUNT" \
    --argjson new "$NEW_COUNT" \
    --argjson events "$EVENTS_24H" \
    --argjson unhandled "$UNHANDLED_COUNT" \
    --argjson highFreq "$HIGH_FREQ_COUNT" \
    '{project: $slug, unresolved_issues: $total, new_since_last_check: $new, events_24h: $events, unhandled: $unhandled, high_frequency: $highFreq}')
  PROJECT_SUMMARIES=$(echo "$PROJECT_SUMMARIES" | jq --argjson s "[$SUMMARY]" '. + $s')

  # Merge into all issues
  ALL_ISSUES=$(echo "$ALL_ISSUES $PROJECT_NEW" | jq -s 'add')
done

# ─── Find Resolved Issues (were in prev state, no longer unresolved) ─────────
RESOLVED_ISSUES=$(echo "$PREV_ISSUES" | jq --argjson current "$CURRENT_KNOWN_IDS" \
  '[.[] | select(. as $id | ($current | index($id)) == null)]')
RESOLVED_COUNT=$(echo "$RESOLVED_ISSUES" | jq 'length')

if [[ "$RESOLVED_COUNT" -gt 0 ]]; then
  ANOMALY=$(jq -n --arg type "resolved_since_last_check" --argjson count "$RESOLVED_COUNT" --argjson ids "$RESOLVED_ISSUES" \
    '{type: $type, count: $count, issue_ids: $ids}')
  ANOMALIES=$(echo "$ANOMALIES" | jq --argjson a "[$ANOMALY]" '. + $a')
fi

# ─── Save New State ──────────────────────────────────────────────────────────
NEW_STATE=$(jq -n \
  --arg ts "$(now_iso)" \
  --argjson ids "$CURRENT_KNOWN_IDS" \
  '{timestamp: $ts, known_issue_ids: $ids}')
echo "$NEW_STATE" > "$STATE_FILE"
log "State saved to $STATE_FILE"

# ─── Generate Report ─────────────────────────────────────────────────────────
TOTAL_UNRESOLVED=$(echo "$PROJECT_SUMMARIES" | jq '[.[].unresolved_issues] | add // 0')
TOTAL_NEW=$(echo "$PROJECT_SUMMARIES" | jq '[.[].new_since_last_check] | add // 0')
TOTAL_EVENTS=$(echo "$PROJECT_SUMMARIES" | jq '[.[].events_24h] | add // 0')
ANOMALY_COUNT=$(echo "$ANOMALIES" | jq 'length')

REPORT=$(jq -n \
  --arg ts "$(now_iso)" \
  --arg prevTs "$PREV_TIMESTAMP" \
  --argjson projectCount "$PROJECT_COUNT" \
  --argjson totalUnresolved "$TOTAL_UNRESOLVED" \
  --argjson totalNew "$TOTAL_NEW" \
  --argjson totalEvents "$TOTAL_EVENTS" \
  --argjson anomalyCount "$ANOMALY_COUNT" \
  --argjson projects "$PROJECT_SUMMARIES" \
  --argjson anomalies "$ANOMALIES" \
  '{
    report: {
      timestamp: $ts,
      previous_check: $prevTs,
      org: "misty-step",
      projects_scanned: $projectCount,
      total_unresolved_issues: $totalUnresolved,
      new_issues_since_last_check: $totalNew,
      total_events_24h: $totalEvents,
      anomaly_count: $anomalyCount
    },
    projects: $projects,
    anomalies: $anomalies
  }')

echo "$REPORT"

# ─── Summary Log ─────────────────────────────────────────────────────────────
log "Report complete: $TOTAL_UNRESOLVED unresolved, $TOTAL_NEW new, $ANOMALY_COUNT anomalies"
