#!/usr/bin/env bash
set -euo pipefail

# Regenerate the Bitterblossom Command Center dashboard with live data.
# Writes to dashboard/public/index.html and optionally deploys to Fly.io.
#
# Usage:
#   ./scripts/refresh-dashboard.sh              # Generate only
#   ./scripts/refresh-dashboard.sh --deploy      # Generate + deploy

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
OUT_DIR="$ROOT_DIR/dashboard/public"
SPRITE_CLI="${SPRITE_CLI:-sprite}"
ORG="${FLY_ORG:-misty-step}"

mkdir -p "$OUT_DIR"

NOW=$(date '+%Y-%m-%d %H:%M %Z')

# --- Gather Data ---

# Sprites
SPRITES_JSON=$("$SPRITE_CLI" api -o "$ORG" /sprites 2>/dev/null | python3 -c "
import sys, json
data = json.load(sys.stdin)
sprites = []
for s in data.get('sprites', []):
    sprites.append({'name': s['name'], 'status': s['status'], 'url': s.get('url','')})
print(json.dumps(sprites))
" 2>/dev/null || echo "[]")

# Open PRs across repos
PRS=""
for repo in heartbeat bitterblossom conviction scry gitpulse chrondle volume bibliomnomnom cadence overmind linejam sploot; do
  pr_data=$(gh pr list --repo "misty-step/$repo" --author kaylee-mistystep --json number,title,headRefName,state,statusCheckRollup \
    --jq ".[] | \"$repo|\(.number)|\(.title)|\(.headRefName)|\(.statusCheckRollup // [] | map(.conclusion // \"PENDING\") | join(\",\"))\"" 2>/dev/null || true)
  if [[ -n "$pr_data" ]]; then
    PRS="${PRS}${pr_data}"$'\n'
  fi
done

# Sprite task status
SPRITE_STATUSES=""
for sprite_name in $(echo "$SPRITES_JSON" | python3 -c "import sys,json; [print(s['name']) for s in json.loads(sys.stdin.read())]" 2>/dev/null); do
  task_status=$("$SPRITE_CLI" exec -o "$ORG" -s "$sprite_name" -- bash -c \
    'if [ -f /home/sprite/workspace/task.pid ] && kill -0 $(cat /home/sprite/workspace/task.pid) 2>/dev/null; then
       echo "RUNNING"
     elif [ -f /home/sprite/workspace/TASK_COMPLETE ]; then
       echo "COMPLETE"
     elif [ -f /home/sprite/workspace/BLOCKED.md ]; then
       echo "BLOCKED"
     else
       echo "IDLE"
     fi' 2>/dev/null || echo "UNKNOWN")
  SPRITE_STATUSES="${SPRITE_STATUSES}${sprite_name}|${task_status}"$'\n'
done

# Count stats
NUM_SPRITES=$(echo "$SPRITES_JSON" | python3 -c "import sys,json; print(len(json.loads(sys.stdin.read())))" 2>/dev/null || echo "0")
NUM_PRS=$(echo "$PRS" | grep -c '|' || echo "0")
NUM_RUNNING=$(echo "$SPRITE_STATUSES" | grep -c 'RUNNING' 2>/dev/null || true)
NUM_RUNNING="${NUM_RUNNING:-0}"
NUM_RUNNING=$(echo "$NUM_RUNNING" | tr -d '[:space:]')
NUM_COMPLETE=$(echo "$SPRITE_STATUSES" | grep -c 'COMPLETE' 2>/dev/null || true)
NUM_COMPLETE="${NUM_COMPLETE:-0}"
NUM_COMPLETE=$(echo "$NUM_COMPLETE" | tr -d '[:space:]')

# --- Sprite definitions (for unprovisioned sprites) ---
ALL_SPRITES="bramble|Systems & Data|🌿
willow|Interface & Experience|🌳
thorn|Quality & Security|🌹
fern|Platform & Operations|🍀
moss|Architecture & Evolution|🪨"

# --- Generate HTML ---
cat > "$OUT_DIR/index.html" << 'HTMLEOF'
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>🌿 Bitterblossom Command Center</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, 'SF Pro', system-ui, sans-serif;
    background: #0a0a0f; color: #e0e0e0; min-height: 100vh; padding: 24px;
  }
  .header { display: flex; align-items: center; gap: 16px; margin-bottom: 32px; border-bottom: 1px solid #1a1a2e; padding-bottom: 20px; }
  .header h1 { font-size: 28px; font-weight: 700; background: linear-gradient(135deg, #7c3aed, #06b6d4); -webkit-background-clip: text; -webkit-text-fill-color: transparent; }
  .header .time { margin-left: auto; color: #666; font-size: 14px; font-family: 'SF Mono', monospace; }
  .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(400px, 1fr)); gap: 20px; margin-bottom: 24px; }
  .card { background: #12121e; border: 1px solid #1e1e3a; border-radius: 12px; padding: 20px; }
  .card h2 { font-size: 14px; text-transform: uppercase; letter-spacing: 1.5px; color: #666; margin-bottom: 16px; }
  .full-width { grid-column: 1 / -1; }
  .stats { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; margin-bottom: 24px; }
  .stat { text-align: center; padding: 16px 8px; background: #12121e; border-radius: 12px; border: 1px solid #1e1e3a; }
  .stat .num { font-size: 28px; font-weight: 700; background: linear-gradient(135deg, #7c3aed, #06b6d4); -webkit-background-clip: text; -webkit-text-fill-color: transparent; }
  .stat .label { font-size: 11px; color: #666; text-transform: uppercase; letter-spacing: 1px; margin-top: 4px; }
  .sprite-list { display: flex; flex-direction: column; gap: 10px; }
  .sprite { display: flex; align-items: center; gap: 12px; padding: 12px; background: #0a0a14; border-radius: 8px; border: 1px solid #1a1a2e; }
  .sprite .icon { width: 36px; height: 36px; border-radius: 8px; display: flex; align-items: center; justify-content: center; font-size: 18px; background: #1a2e1a; }
  .sprite .name { font-weight: 600; font-size: 15px; }
  .sprite .pref { color: #666; font-size: 12px; }
  .sprite .right { margin-left: auto; text-align: right; }
  .badge { display: inline-block; padding: 2px 10px; border-radius: 10px; font-size: 11px; font-weight: 600; }
  .b-green { background: #064e3b; color: #34d399; }
  .b-red { background: #4e0619; color: #f87171; }
  .b-blue { background: #1e3a5f; color: #60a5fa; }
  .b-yellow { background: #3a2e1e; color: #fbbf24; }
  .b-gray { background: #1e1e2e; color: #888; }
  .b-purple { background: #2e1a3e; color: #c084fc; }
  .pr-table { width: 100%; border-collapse: collapse; }
  .pr-table th { text-align: left; padding: 8px 12px; font-size: 12px; text-transform: uppercase; letter-spacing: 1px; color: #555; border-bottom: 1px solid #1e1e3a; }
  .pr-table td { padding: 10px 12px; font-size: 13px; border-bottom: 1px solid #111; }
  .pr-num { color: #7c3aed; font-weight: 600; }
  .mono { font-family: 'SF Mono', 'Menlo', monospace; font-size: 11px; }
  .dimmed { opacity: 0.4; }
</style>
</head>
<body>
<div class="header">
  <h1>🌿 Bitterblossom Command Center</h1>
  <div class="time">TIMESTAMP_PLACEHOLDER • Fae Court v1</div>
</div>
<div class="stats">
  <div class="stat"><div class="num">SPRITES_COUNT</div><div class="label">Active Sprites</div></div>
  <div class="stat"><div class="num">PRS_COUNT</div><div class="label">Open PRs</div></div>
  <div class="stat"><div class="num">RUNNING_COUNT</div><div class="label">Tasks Running</div></div>
  <div class="stat"><div class="num">COMPLETE_COUNT</div><div class="label">Tasks Done Today</div></div>
</div>
<div class="grid">
  <div class="card">
    <h2>🌿 Sprite Fleet</h2>
    <div class="sprite-list">
SPRITE_ROWS_PLACEHOLDER
    </div>
  </div>
  <div class="card">
    <h2>📋 Open Pull Requests</h2>
    <table class="pr-table">
      <thead><tr><th>PR</th><th>Repo</th><th>Title</th><th>Branch</th><th>CI</th></tr></thead>
      <tbody>
PR_ROWS_PLACEHOLDER
      </tbody>
    </table>
  </div>
</div>
</body>
</html>
HTMLEOF

# --- Fill in placeholders ---

replace_placeholder() {
  local placeholder="$1"
  local value="$2"

  python3 - "$OUT_DIR/index.html" "$placeholder" "$value" <<'PY'
import sys

path, placeholder, value = sys.argv[1:]
with open(path, encoding="utf-8") as f:
    html = f.read()
html = html.replace(placeholder, value)
with open(path, "w", encoding="utf-8") as f:
    f.write(html)
PY
}

replace_placeholder "TIMESTAMP_PLACEHOLDER" "$NOW"
replace_placeholder "SPRITES_COUNT" "$NUM_SPRITES"
replace_placeholder "PRS_COUNT" "$NUM_PRS"
replace_placeholder "RUNNING_COUNT" "$NUM_RUNNING"
replace_placeholder "COMPLETE_COUNT" "$NUM_COMPLETE"

# Generate sprite rows
SPRITE_HTML=""
while IFS='|' read -r sname spref sicon; do
  [[ -z "$sname" ]] && continue
  # Check if provisioned
  is_live=$(echo "$SPRITES_JSON" | python3 -c "import sys,json; print('yes' if any(s['name']=='$sname' for s in json.loads(sys.stdin.read())) else 'no')" 2>/dev/null)
  # Get task status
  task_st=$(echo "$SPRITE_STATUSES" | grep "^$sname|" | cut -d'|' -f2 | tr -d '[:space:]')

  if [[ "$is_live" == "yes" ]]; then
    case "$task_st" in
      RUNNING) badge='<span class="badge b-blue">RUNNING</span>' ;;
      COMPLETE) badge='<span class="badge b-green">COMPLETE</span>' ;;
      BLOCKED) badge='<span class="badge b-red">BLOCKED</span>' ;;
      *) badge='<span class="badge b-yellow">IDLE</span>' ;;
    esac
    SPRITE_HTML="${SPRITE_HTML}<div class=\"sprite\"><div class=\"icon\">$sicon</div><div><div class=\"name\">$sname</div><div class=\"pref\">$spref</div></div><div class=\"right\">$badge</div></div>"
  else
    SPRITE_HTML="${SPRITE_HTML}<div class=\"sprite dimmed\"><div class=\"icon\">$sicon</div><div><div class=\"name\">$sname</div><div class=\"pref\">$spref</div></div><div class=\"right\"><span class=\"badge b-gray\">NOT PROVISIONED</span></div></div>"
  fi
done <<< "$ALL_SPRITES"

# Use python to do the replacement safely (sed struggles with HTML)
python3 -c "
import sys
html = open('$OUT_DIR/index.html').read()
html = html.replace('SPRITE_ROWS_PLACEHOLDER', '''$SPRITE_HTML''')
open('$OUT_DIR/index.html', 'w').write(html)
"

# Generate PR rows
PR_HTML=""
while IFS='|' read -r repo num title branch checks; do
  [[ -z "$repo" ]] && continue
  # Determine sprite from branch prefix
  sprite=""
  case "$branch" in
    thorn/*) sprite="Thorn" ;;
    bramble/*) sprite="Bramble" ;;
    willow/*) sprite="Willow" ;;
    fern/*) sprite="Fern" ;;
    moss/*) sprite="Moss" ;;
    claw/*) sprite="Claw" ;;
    *) sprite="—" ;;
  esac
  # CI status
  if echo "$checks" | grep -qi "failure"; then
    ci='<span class="badge b-red">✗ Failing</span>'
  elif echo "$checks" | grep -qi "success"; then
    ci='<span class="badge b-green">✓ Passing</span>'
  elif echo "$checks" | grep -qi "pending"; then
    ci='<span class="badge b-yellow">⏳ Pending</span>'
  else
    ci='<span class="badge b-gray">No CI</span>'
  fi
  PR_HTML="${PR_HTML}<tr><td class=\"pr-num\">#$num</td><td class=\"mono\">$repo</td><td>$title</td><td class=\"mono\">$branch</td><td>$ci</td></tr>"
done <<< "$PRS"

python3 -c "
import sys
html = open('$OUT_DIR/index.html').read()
html = html.replace('PR_ROWS_PLACEHOLDER', '''$PR_HTML''')
open('$OUT_DIR/index.html', 'w').write(html)
"

echo "[dashboard] Generated: $OUT_DIR/index.html ($NOW)"
echo "[dashboard] Sprites: $NUM_SPRITES | PRs: $NUM_PRS | Running: $NUM_RUNNING"

# Deploy if --deploy flag
if [[ "${1:-}" == "--deploy" ]]; then
  echo "[dashboard] Deploying to Fly.io..."
  cd "$ROOT_DIR/dashboard"
  fly deploy --org "$ORG" 2>&1
  echo "[dashboard] Deployed!"
fi
