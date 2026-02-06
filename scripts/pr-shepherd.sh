#!/bin/bash
# pr-shepherd.sh — Monitor all open sprite PRs across the org
# Usage: ./pr-shepherd.sh
# Output: JSON report of PR statuses and needed actions
#
# Designed to run as a cron job every 30 minutes.
# Checks CI status, review status, and age for all open PRs
# authored by kaylee-mistystep (sprite account).

set -euo pipefail

ORG="misty-step"
AUTHOR="kaylee-mistystep"
STATE_FILE="/tmp/pr-shepherd-state.json"

# Get all open PRs by sprite account across the org
echo "[pr-shepherd] Checking open PRs by $AUTHOR in $ORG..." >&2

PRS=$(gh api "search/issues" \
  --method GET \
  -f q="org:$ORG is:pr is:open author:$AUTHOR" \
  -f per_page=50 \
  --jq '.items[] | {
    number: .number,
    title: .title,
    repo: (.repository_url | split("/") | last),
    created_at: .created_at,
    updated_at: .updated_at,
    html_url: .html_url
  }' 2>/dev/null) || { echo "[]"; exit 0; }

if [ -z "$PRS" ]; then
  echo "[]"
  exit 0
fi

# Process each PR
RESULTS="["
FIRST=true

echo "$PRS" | jq -c '.' | while IFS= read -r pr; do
  REPO=$(echo "$pr" | jq -r '.repo')
  NUMBER=$(echo "$pr" | jq -r '.number')
  TITLE=$(echo "$pr" | jq -r '.title')
  CREATED=$(echo "$pr" | jq -r '.created_at')
  UPDATED=$(echo "$pr" | jq -r '.updated_at')
  URL=$(echo "$pr" | jq -r '.html_url')

  # Get CI status
  CI_STATUS="unknown"
  CHECKS=$(gh pr checks "$NUMBER" --repo "$ORG/$REPO" 2>&1) || true
  if echo "$CHECKS" | grep -q "fail"; then
    CI_STATUS="failing"
  elif echo "$CHECKS" | grep -q "pass"; then
    CI_STATUS="passing"
  elif echo "$CHECKS" | grep -q "pending"; then
    CI_STATUS="pending"
  fi

  # Get review status
  REVIEWS=$(gh api "repos/$ORG/$REPO/pulls/$NUMBER/reviews" \
    --jq '[.[] | select(.state != "COMMENTED")] | last | .state // "none"' 2>/dev/null) || REVIEWS="none"

  # Calculate age in hours
  CREATED_EPOCH=$(date -j -f "%Y-%m-%dT%H:%M:%SZ" "$CREATED" "+%s" 2>/dev/null || date -d "$CREATED" "+%s" 2>/dev/null || echo "0")
  NOW_EPOCH=$(date "+%s")
  AGE_HOURS=$(( (NOW_EPOCH - CREATED_EPOCH) / 3600 ))

  # Updated age
  UPDATED_EPOCH=$(date -j -f "%Y-%m-%dT%H:%M:%SZ" "$UPDATED" "+%s" 2>/dev/null || date -d "$UPDATED" "+%s" 2>/dev/null || echo "0")
  STALE_HOURS=$(( (NOW_EPOCH - UPDATED_EPOCH) / 3600 ))

  # Determine action needed
  ACTION="none"
  if [ "$CI_STATUS" = "failing" ]; then
    ACTION="fix_ci"
  elif [ "$REVIEWS" = "CHANGES_REQUESTED" ]; then
    ACTION="address_reviews"
  elif [ "$CI_STATUS" = "passing" ] && [ "$REVIEWS" != "CHANGES_REQUESTED" ]; then
    ACTION="ready_for_final_review"
  elif [ "$STALE_HOURS" -gt 24 ]; then
    ACTION="stale_investigate"
  fi

  # Output as JSON line
  jq -n \
    --arg repo "$REPO" \
    --argjson number "$NUMBER" \
    --arg title "$TITLE" \
    --arg ci "$CI_STATUS" \
    --arg reviews "$REVIEWS" \
    --argjson age "$AGE_HOURS" \
    --argjson stale "$STALE_HOURS" \
    --arg action "$ACTION" \
    --arg url "$URL" \
    '{repo: $repo, pr: $number, title: $title, ci: $ci, reviews: $reviews, age_hours: $age, stale_hours: $stale, action: $action, url: $url}'

done | jq -s '.'
