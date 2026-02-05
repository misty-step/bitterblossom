# Design Decisions & Open Questions

## 1. Permission Hierarchy: Kaylee vs Sprites

### Problem
We want sprites to have FEWER permissions than Kaylee (OpenClaw):
- **Sprites should:** Create branches, push to branches, open PRs, run CI
- **Sprites should NOT:** Merge PRs, delete repos, admin actions
- **Kaylee should:** All of the above, plus merge PRs, manage repos, admin

But if sprites use Kaylee's GitHub account, they inherit all of Kaylee's permissions.

### Options

#### A. Hook-Based Enforcement (Current Approach)
Sprites share Kaylee's `kaylee-mistystep` account but the destructive-command-guard blocks `gh pr merge`, direct pushes to main, etc.

**Pros:** Simple, already works, no extra accounts needed.
**Cons:** Enforcement is client-side only. A determined (or buggy) Claude Code instance could work around hooks. Not a real security boundary.

**Verdict:** Good enough for v1. Sprites are our own agents — we're protecting against accidents, not adversaries.

#### B. Separate Bot Account
Create a `misty-step-sprites` GitHub account with limited org permissions.
- Sprites authenticate with this account (write access to branches only)
- Kaylee keeps `kaylee-mistystep` with full admin
- Branch protection rules enforce PRs for main

**Pros:** Real permission boundary. Clean audit trail (sprite commits vs Kaylee commits).
**Cons:** Second GitHub account to manage. Need to provision tokens on each sprite.

**Verdict:** Better for v2 when we have more sprites and want real isolation.

#### C. GitHub App
Create a Misty Step GitHub App with precisely scoped permissions.
- App gets: contents (write), pull-requests (write), issues (write)
- App does NOT get: admin, merge, delete
- Each sprite authenticates via app installation token

**Pros:** Most precise permission control. Best practice.
**Cons:** Complex setup. Tokens need periodic refresh.

**Verdict:** Ideal for v3 / production. Overkill for experimentation phase.

### Decision (v1)
Use **Option A: Hook-based enforcement**. Sprites share Kaylee's GitHub identity. The destructive-command-guard blocks dangerous operations. This is a trust boundary based on hooks, not GitHub permissions.

We accept the risk that hooks are client-side enforcement. Our sprites aren't adversaries — we're guarding against Claude Code doing something dumb on autopilot.

**Migrate to Option B when:**
- We have 5+ sprites running simultaneously
- We want clean audit trails
- We're running sprites on tasks from external contributors

## 2. Self-Modifying Config with Version Control

### Problem
Each sprite should:
1. Know its own identity (Moss knows it's Moss)
2. Have its own CLAUDE.md that it can modify
3. Version control those modifications
4. Restart with updated config

### Design
Each sprite gets the bitterblossom repo cloned into its workspace:

```
/home/sprite/workspace/
├── bitterblossom/           # Cloned repo
│   ├── base/                # Shared config (read from, don't modify)
│   ├── sprites/moss.md      # Its own persona (can propose changes)
│   └── ...
├── CLAUDE.md                # Live config (started from base, sprite evolves it)
├── PERSONA.md               # Symlink or copy from sprites/<name>.md
├── MEMORY.md                # Per-sprite learnings
└── <work repos>/            # Cloned repos for actual tasks
```

**Config evolution flow:**
1. Sprite starts with base CLAUDE.md + persona overlay
2. After each leg of work, sprite reviews and may update CLAUDE.md
3. Sprite commits CLAUDE.md and MEMORY.md changes to bitterblossom repo on its own branch
4. Kaylee reviews sprite config changes, cherry-picks good ones into base

**Restart pattern:**
After updating CLAUDE.md, the Ralph loop naturally restarts Claude Code with the new config (since it's read fresh each iteration).

### Version control
Each sprite pushes to its own branch in bitterblossom:
- `sprite/bramble` — Bramble's config changes
- `sprite/moss` — Moss's config changes
- etc.

Kaylee can review these branches and merge good changes back to main/base.

## 3. Model Configuration

### Current Setup
All sprites use Kimi K2.5 via Moonshot's Anthropic-compatible API:
```json
{
  "ANTHROPIC_BASE_URL": "https://api.moonshot.ai/anthropic",
  "ANTHROPIC_MODEL": "kimi-k2.5",
  "ANTHROPIC_AUTH_TOKEN": "<moonshot-key>"
}
```

This is set in `base/settings.json` and passed as env vars to Claude Code.
The `model: inherit` in sprite definitions means "use whatever's in settings.json" — i.e., the base model config.

### Future
We may want different models for different sprites or tasks:
- Cheap model for routine work
- Expensive model for complex architecture
- Different providers for experimentation

This can be handled by overriding env vars per-sprite in the composition YAML, then applying them during provisioning.

## 4. Ralph Loop Design

### Core Pattern
```bash
while :; do cat PROMPT.md | claude -p --permission-mode bypassPermissions ; done
```

### Enhancements for Bitterblossom
1. **Completion signals:** Sprite creates TASK_COMPLETE file → loop stops
2. **Blocked signals:** Sprite creates BLOCKED.md → loop stops, Kaylee notified
3. **Iteration logging:** Each loop iteration logged with timestamp
4. **Config refresh:** Each iteration reads fresh CLAUDE.md (self-evolution works naturally)
5. **Checkpoint between iterations:** Consider auto-checkpointing every N iterations

### Open Questions
- How long should we let a Ralph loop run before checking in?
- Should there be a max iteration count (safety valve)?
- How do we handle the case where a sprite keeps looping without making progress?
- Do we need a heartbeat mechanism for sprite health?

## 5. Communication: Kaylee ↔ Sprites

### Current
Kaylee dispatches tasks via `sprite exec` and reads output.

### Needed
- Check on running Ralph loops (tail logs)
- Send additional instructions mid-task
- Read sprite MEMORY.md to understand what they've learned
- Review sprite CLAUDE.md changes
- Get notified when TASK_COMPLETE or BLOCKED

### Approach
1. **Log tailing:** `sprite exec -s <name> -- tail -f workspace/ralph.log`
2. **Mid-task instructions:** Write to a `NOTES.md` file that the prompt tells the sprite to check
3. **Status checks:** dispatch.sh --status already reads TASK_COMPLETE, BLOCKED, logs, MEMORY
4. **Notifications:** Kaylee runs a periodic check (cron or heartbeat) on all active sprites
