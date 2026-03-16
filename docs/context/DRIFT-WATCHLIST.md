# Drift Watchlist

These are the places most likely to drift away from the current conductor-first architecture.

## Confirmed Stale or High-Risk Areas

### Skill docs that still imply old `bb` surfaces

- [`base/skills/bitterblossom-dispatch/SKILL.md`](../../base/skills/bitterblossom-dispatch/SKILL.md)
- [`base/skills/bitterblossom-dispatch/glance.md`](../../base/skills/bitterblossom-dispatch/glance.md)
- [`base/skills/bitterblossom-monitoring/SKILL.md`](../../base/skills/bitterblossom-monitoring/SKILL.md)
- [`base/skills/bitterblossom-monitoring/glance.md`](../../base/skills/bitterblossom-monitoring/glance.md)

Risk:

- imply flags or flows that are no longer authoritative
- easy for agents to over-trust because they look purpose-built

### Glance docs that describe older architecture shapes

- [`glance.md`](../../glance.md)
- [`docs/glance.md`](../glance.md)
- [`cmd/bb/glance.md`](../../cmd/bb/glance.md)
- [`scripts/glance.md`](../../scripts/glance.md)
- [`base/glance.md`](../../base/glance.md)
- [`reports/glance.md`](../../reports/glance.md)

Risk:

- tend to preserve older orchestration and wrapper-script mental models
- useful for historical context, risky for exact implementation truth

### Historical wrapper surfaces in `scripts/`

Examples:

- [`scripts/provision.sh`](../../scripts/provision.sh)
- [`scripts/teardown.sh`](../../scripts/teardown.sh)
- [`scripts/status.sh`](../../scripts/status.sh)
- deleted wrapper entrypoints preserved only in historical docs and old reports
- [`scripts/test_legacy_wrappers.sh`](../../scripts/test_legacy_wrappers.sh)

Risk:

- wrappers can make old workflows look more current than they are
- they are not the center of gravity for the present conductor architecture

### README and general repo framing docs

- [`README.md`](../../README.md)
- [`QA.md`](../../QA.md)

Risk:

- easy to read first
- may lag behind exact command surface, runtime defaults, or current control-plane boundaries

## Semantic Drift Watchpoints

These are not necessarily wrong in every file, but they drift easily.

- claiming routing is already fully semantic or LLM-driven
- claiming compositions are the authoritative scheduler input today
- claiming execution is already fully isolated per-run worktrees
- treating `base/skills/*` as the source of truth for current CLI behavior
- reviving older wrapper-heavy architecture language instead of the current conductor-first split
- forgetting that GitHub is the human-facing ledger while `.bb/conductor.db` + `.bb/events.jsonl` are the machine-facing truth

## Refresh Triggers

Refresh `docs/CODEBASE_MAP.md` and `docs/context/*` when any of these change:

- `bb` subcommands, flags, or operator semantics
- conductor run phases, blocking reasons, or lease logic
- builder/reviewer artifact paths or schemas
- completion protocol signal files
- worker selection / readiness / repair flow
- actual move to worktree isolation
- repository-registry state machine semantics (`active` / `paused` / `draining`) or `show-repos` / `set-repo-state` surfaces

## Quick Audit Commands

Use these when you suspect drift:

```bash
rg -n "watchdog|provision|teardown|Fly Machines|--issue|--skill|--execute|fleet|proxy|registry" README.md QA.md base/skills docs scripts
rg -n "conductor.db|events.jsonl|run_id|blocking_reason|heartbeat_age_seconds" docs project.md scripts/conductor.py
```

## Rule Of Thumb

If a file makes a strong claim about current behavior and it is not code, an ADR, [`docs/CONDUCTOR.md`](../CONDUCTOR.md), [`docs/CLI-REFERENCE.md`](../CLI-REFERENCE.md), [`docs/CODEBASE_MAP.md`](../CODEBASE_MAP.md), [`docs/context/*`](./), or [`docs/architecture/*`](../architecture/), verify it before trusting it.
