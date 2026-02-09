# Task: Create Fly.io Sprites Documentation & OpenClaw Skill

## Context
Misty Step uses Fly.io **Sprites** (NOT Machines). These are completely different products.
- Sprites CLI: `sprite` at `~/.local/bin/sprite`
- Sprites API: `api.sprites.dev`
- Machines CLI: `flyctl` (DON'T USE)
- Machines API: `api.machines.dev` (DON'T USE)

## What to Do

### 1. Research Sprites Documentation
- Search the web for "Fly.io Sprites" documentation, API reference, pricing
- Read https://fly.io/docs/sprites/ or wherever the docs live
- Understand: creation, exec, checkpoints, restore, destroy, API endpoints, billing model
- Find any best practices, limitations, gotchas

### 2. Create an OpenClaw Skill
Create a skill at `~/.openclaw/workspace/skills/fly-sprites/SKILL.md` with:
- What Sprites are and how they differ from Machines
- Full CLI reference (`sprite create/list/exec/use/checkpoint/restore/destroy`)
- API reference (`api.sprites.dev` endpoints)
- Best practices for AI coding agents on sprites
- Common gotchas and troubleshooting
- Example workflows (create → exec → checkpoint → destroy)
- Billing/pricing notes

### 3. Update Bitterblossom Documentation
Update `~/bitterblossom/docs/SPRITES.md` (create if needed) with:
- What Sprites are (for the BB context)
- How BB uses Sprites (the `bb agent` supervisor)
- The Sprites vs Machines distinction (table format)
- How to provision, configure, and tear down sprites for BB
- How to configure agents on sprites (Kimi K2.5 via Moonshot API, Codex, etc.)
- Monitoring and log access
- Full example of dispatching a task to a sprite

### 4. Git Commit
- Commit and push all changes to appropriate repos
- BB changes go to a branch and open a PR
- OpenClaw workspace changes commit directly

## Quality Bar
- Clear, concise, practical
- Include actual commands, not just descriptions
- Comparison tables between Sprites and Machines
- Real examples that someone could copy-paste and run
