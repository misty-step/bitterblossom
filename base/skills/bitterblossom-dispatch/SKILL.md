---
name: bitterblossom-dispatch
user-invocable: true
description: "Dispatch a GitHub issue or prompt to a Bitterblossom sprite with explicit skill mounting, safe dry-run planning, and wait-mode monitoring."
allowed-tools:
  - Read
  - Grep
  - Glob
  - Bash
---

# Bitterblossom Dispatch

Run this skill when you want a sprite to execute a coding task through `bb dispatch`.

## Preflight

```bash
source .env.bb
bb status --format text
```

Confirm:
- `FLY_APP`, `FLY_API_TOKEN`, and `FLY_ORG` are set.
- Target sprite exists (or let dispatch provision it).

## Workflow

1. Plan first (dry-run default):

```bash
bb dispatch <sprite> --issue <number> --repo <owner/repo>
```

2. Execute with explicit skill(s):

```bash
bb dispatch <sprite> --issue <number> --repo <owner/repo> \
  --skill base/skills/bitterblossom-dispatch \
  --execute --wait
```

3. For multiple skills, repeat `--skill`:

```bash
bb dispatch <sprite> "Implement feature X" \
  --repo <owner/repo> \
  --skill base/skills/bitterblossom-dispatch \
  --skill base/skills/bitterblossom-monitoring \
  --execute --wait
```

## Skill Mount Semantics

- Each `--skill` path may be a skill directory or `SKILL.md`.
- Bitterblossom mounts the full skill directory under `./skills/<name>/` on sprite.
- Prompt is augmented with required instructions:
  - `Follow the skill at ./skills/<name>/SKILL.md`

## Failure Handling

- Validation blocked by labels/readiness:
  - Use `--skip-validation` only for intentional bypasses.
- If `--wait` shows no progress, run:

```bash
bb status <sprite> --format text
bb watchdog --sprite <sprite>
```

