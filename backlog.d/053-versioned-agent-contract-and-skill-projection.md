# Make the agent-facing bb contract versioned and schema-backed

Priority: P2 | Status: ready | Estimate: L

## Goal

Make Bitterblossom safe for consuming applications and agents by versioning
CLI/API JSON contracts, testing the portable skill against those contracts, and
choosing a durable Harness Kit projection path.

## Oracle

- [ ] Versioned schemas or fixtures exist for key agent surfaces:
      `task list --json`, `runs list --json`, `runs show --json`,
      `dlq list --json`, `gate --json`, and `/api/*` mirrors.
- [ ] Tests validate required fields and types, not only string presence.
- [ ] `skills/bitterblossom/` recipes are checked against live help and schema
      fixtures.
- [ ] A source-of-truth projection decision is documented: symlink, bootstrap
      source entry, or other no-drift path into Harness Kit.
- [ ] Lane-card/task-card quality requirements are validated or explicitly
      deferred with a shaped follow-up.

## Children

1. Inventory current JSON outputs and API mirrors.
2. Add schema/fixture tests for the smallest stable set.
3. Strengthen `tests/skill_artifacts.rs` beyond string contains.
4. Decide and document the Harness Kit projection path for the skill folder.
5. Add a task-card contract check for Goal, Oracle, Boundaries, Output, and
   Receipt, or file the exact follow-up.

## Notes

Why: the agent-readiness lane found the portable skill baseline is real, but
the guarantees are shallow.

Evidence:

- `skills/bitterblossom/SKILL.md` and `references/operator-recipes.md` provide
  a usable first-class skill folder.
- `tests/skill_artifacts.rs:11-29` checks string presence, not command/schema
  compatibility.
- `tests/task_cli.rs:60-67` checks only a few `task list` JSON fields.

Demoted P1 → P2 (groom 2026-06-17): this is enabling-infrastructure — it
protects the agent-facing contract from *future* breakage but adds no
throughput — and it is the heaviest LOC item (~+120-250 in `src/`), budget-
blocked behind the 069 extraction. Reprioritize up once 069 frees headroom and a
contract-breakage actually bites.
