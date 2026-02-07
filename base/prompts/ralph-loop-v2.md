# Ralph Loop v2 — Self-Healing PR Workflow

## Overview
Sprites don't just open PRs — they **shepherd them to merge-ready**.
The loop: work → PR → watch CI → fix failures → address reviews → signal done.

## Prompt Template

Use this as the outer wrapper for any sprite dispatch. Replace `{{TASK_PROMPT}}` with the actual task.

```markdown
# Mission: {{TASK_NAME}}

{{TASK_PROMPT}}

---

## Self-Healing PR Protocol

After completing your task and opening a PR, you MUST enter the review loop:

### Step 1: Push and open PR
- Create feature branch FIRST (e.g., `{{SPRITE_NAME}}/{{BRANCH_NAME}}`)
- Do the work, commit incrementally
- Push and open PR against `master` (or `main`)
- Note the PR number

### Step 2: Wait for CI
- Run: `sleep 120` (give CI 2 minutes to start)
- Check CI status:
  ```bash
  gh pr checks {{PR_NUMBER}} 2>&1
  ```
- If CI is still running, wait another 60s and check again
- Max wait: 10 minutes

### Step 3: Handle CI failures
If any check fails:
1. Get the failure details:
   ```bash
   gh run view {{RUN_ID}} --log-failed 2>&1 | grep "error:" | head -30
   ```
2. Read the error, understand the root cause
3. Fix the issue — commit with message: `fix: address CI failure — {{brief description}}`
4. Push — CI re-runs automatically
5. Go back to Step 2

### Step 4: Handle review comments
After CI passes (or while waiting), check for review comments:
```bash
gh api repos/{{OWNER}}/{{REPO}}/pulls/{{PR_NUMBER}}/comments \
  --jq '.[] | "[\(.user.login)] \(.path):\(.line) — \(.body[:200])"'
```

For each comment:
- **Critical/High**: Must fix. Commit with `fix: address {{reviewer}} feedback — {{brief}}`
- **Medium**: Fix if straightforward, note if complex
- **Low/Nitpick**: Fix if trivial, skip if not

Push fixes and go back to Step 2.

### Step 5: Check merge conflicts
After CI passes, check if the PR is actually mergeable:
```bash
gh pr view {{PR_NUMBER}} --json mergeable --jq '.mergeable'
```

If the result is `CONFLICTING`:
1. Rebase on the latest master:
   ```bash
   git fetch origin master
   git rebase origin/master
   ```
2. If there are conflicts, resolve them:
   - Edit conflicting files
   - `git add <resolved files>`
   - `git rebase --continue`
3. Force push (safe after rebase):
   ```bash
   git push origin {{BRANCH}} --force-with-lease
   ```
4. Go back to Step 2 (CI will re-run after force push)

**A PR with merge conflicts is NOT done.** The loop does not stop until the PR is `MERGEABLE`.

### Step 6: Completion
When CI passes AND no unaddressed critical/high comments AND no merge conflicts:
- Print: `TASK_COMPLETE: PR #{{PR_NUMBER}} is merge-ready`
- Print: `SUMMARY: {{one-line description of what was done}}`

### Step 7: Stuck detection
If you've attempted fixes 3+ times and CI still fails, or you can't resolve a review comment:
- Print: `BLOCKED: {{description of what's blocking}}`
- Print: `ATTEMPTED: {{list of what you tried}}`
- Do NOT keep looping — signal and stop

## Git Config
```bash
git config user.name "kaylee-mistystep"
git config user.email "kaylee@mistystep.io"
```
```

## Usage

When dispatching a sprite:

```bash
# 1. Write the task-specific prompt
cat > /tmp/task-prompt.md << 'EOF'
Your specific task here...
EOF

# 2. Combine with Ralph Loop v2 template
# (The dispatch script should auto-append the self-healing protocol)

# 3. Upload and dispatch
sprite -o misty-step -s $SPRITE exec \
  --file /tmp/task-prompt.md:/home/sprite/workspace/$REPO/PROMPT.md \
  -- echo "uploaded"

sprite -o misty-step -s $SPRITE exec -- bash -c \
  "cd /home/sprite/workspace/$REPO && cat PROMPT.md | claude -p --permission-mode bypassPermissions"
```

## Future: Full Ralph Loop

For multi-task autonomous work, the outer loop becomes:

```bash
while true; do
  cat PROMPT.md | claude -p --permission-mode bypassPermissions
  EXIT_CODE=$?
  
  # Check if sprite signaled completion or blockage
  # If TASK_COMPLETE → update PROMPT.md with next task (or exit)
  # If BLOCKED → exit loop, signal coordinator
  # If crashed → log and retry (max 3)
done
```

## Key Principles

1. **Sprites own their PRs** — they don't fire-and-forget
2. **CI is the source of truth** — if it doesn't build, it's not done
3. **Review comments are mandatory** — critical/high MUST be addressed
4. **Fail fast, fail loud** — 3 attempts max, then BLOCKED signal
5. **Force-push only after rebase** — rebase to resolve conflicts, force-push-with-lease to update
6. **Merge conflicts = not done** — a PR with conflicts is NOT merge-ready, keep looping
7. **Mergeable is the exit condition** — CI green + reviews addressed + no conflicts = done
