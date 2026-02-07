#!/usr/bin/env bash
set -euo pipefail

# run-task.sh — Standardized sprite task execution with completion signaling
#
# Usage: run-task.sh <sprite-name> <repo> <issue-number> [persona-role]
#
# This script:
# 1. Fills the dispatch template with the given issue
# 2. Runs Claude in the background
# 3. Monitors for completion
# 4. Signals Kaylee via OpenClaw wake event when done
#
# Environment:
#   OPENCLAW_GATEWAY_URL   Gateway URL for wake signal (optional)
#   OPENCLAW_GATEWAY_TOKEN Gateway token for wake signal (optional)
#   SPRITE_CLI             Path to sprite CLI (default: sprite)

SPRITE_CLI="${SPRITE_CLI:-sprite}"
SPRITE_NAME="${1:?Usage: run-task.sh <sprite> <repo> <issue-number> [persona-role]}"
REPO="${2:?Missing repo name}"
ISSUE_NUMBER="${3:?Missing issue number}"
PERSONA_ROLE="${4:-sprite}"

LOG_FILE="${SPRITE_NAME}-${REPO}-${ISSUE_NUMBER}.log"

echo "[dispatch] Sprite: ${SPRITE_NAME} | Repo: ${REPO} | Issue: #${ISSUE_NUMBER}"

# Generate the dispatch prompt from template
PROMPT=$(cat <<ENDPROMPT
You are ${SPRITE_NAME}, a ${PERSONA_ROLE} sprite in the Fae Court.

Read your persona at /home/sprite/workspace/PERSONA.md for your working philosophy and approach.

## Your Assignment

GitHub issue #${ISSUE_NUMBER} in misty-step/${REPO}:

\`\`\`
gh issue view ${ISSUE_NUMBER} --repo misty-step/${REPO}
\`\`\`

## Execution Protocol

1. **Read the issue** — all specs, context, and acceptance criteria are defined there. The issue is your single source of truth.
2. **Clone the repo** if not already present: \`git clone https://github.com/misty-step/${REPO}.git\`
3. **Create a branch** with a descriptive name based on the issue.
4. **Implement the solution** — follow engineering and architecture best practices per CLAUDE.md.
5. **Write tests** for your changes — edge cases, error paths, not just happy paths.
6. **Open a PR** referencing the issue with a clear description of what you did and why.

## Quality Standards

- Every merged PR should make the codebase easier to work on and understand
- Document non-obvious decisions
- Test edge cases and error handling
- Clean, atomic commits with clear messages
- If blocked, document what you tried in a comment on the issue
ENDPROMPT
)

# Upload prompt and dispatch
echo "[dispatch] Uploading prompt..."
echo "${PROMPT}" | ${SPRITE_CLI} exec -s "${SPRITE_NAME}" -- bash -c 'cat > /home/sprite/workspace/TASK.md'

echo "[dispatch] Cloning repo and starting agent..."
${SPRITE_CLI} exec -s "${SPRITE_NAME}" -- bash -c "
  cd /home/sprite/workspace
  git clone https://github.com/misty-step/${REPO}.git ${REPO} 2>/dev/null || (cd ${REPO} && git pull --ff-only 2>/dev/null || true)
  cd ${REPO}
  cat /home/sprite/workspace/TASK.md | claude -p --permission-mode bypassPermissions > /home/sprite/workspace/${LOG_FILE} 2>&1 &
  AGENT_PID=\$!
  echo \"\${AGENT_PID}\" > /home/sprite/workspace/AGENT_PID
  echo \"[dispatch] Agent started PID=\${AGENT_PID}\"

  # Write status file
  cat > /home/sprite/workspace/STATUS.json << STATUSEOF
{\"sprite\":\"${SPRITE_NAME}\",\"repo\":\"${REPO}\",\"issue\":${ISSUE_NUMBER},\"status\":\"running\",\"pid\":\${AGENT_PID},\"started\":\"\$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}
STATUSEOF
" 2>&1

echo "[dispatch] ${SPRITE_NAME} dispatched to ${REPO}#${ISSUE_NUMBER}"
echo "[dispatch] Log: /home/sprite/workspace/${LOG_FILE}"
echo "[dispatch] Status: /home/sprite/workspace/STATUS.json"
