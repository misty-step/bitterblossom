# Make the agent-facing bb contract versioned and schema-backed

Priority: P0 | Status: done | Estimate: L

## Goal

Make Bitterblossom safe for consuming applications and agents by versioning
CLI/API JSON contracts, testing the portable skill against those contracts, and
choosing a durable Harness Kit projection path for the agent-friendly layer v1
(epic 076).

## Oracle

- [x] Versioned schemas or fixtures exist for key agent surfaces:
      `task list --json`, `runs list --json`, `runs show --json`,
      `dlq list --json`, `gate --json`, and `/api/*` mirrors.
- [x] Tests validate required fields and types, not only string presence.
- [x] `skills/bitterblossom/` recipes are checked against live help and schema
      fixtures.
- [x] A source-of-truth projection decision is documented: symlink, bootstrap
      source entry, or other no-drift path into Harness Kit.
- [x] Lane-card/task-card quality requirements are validated or explicitly
      deferred with a shaped follow-up.

## Verification System

- Claim: the agent-facing BB contract is stable enough for skills, MCP, HTTP consumers, and scripts to depend on without prose-only interpretation.
- Falsifier: a documented skill recipe or MCP tool emits/parses a shape that differs from CLI/API output; a JSON field used by an agent changes without fixture failure; or a stale help example survives the gate.
- Driver: fixture tests for each key JSON surface plus live-help/doc parity tests and a local-plane smoke once backlog 077 lands.
- Grader: required fields and types match fixtures; unknown additive fields are tolerated only where the compatibility contract says so; removed/renamed fields fail.
- Evidence packet: fixture diffs, focused test transcript, and one local-plane command transcript linked from the implementing PR.
- Cadence: every CLI/API/MCP/skill change touching agent-facing surfaces.

## Children

1. [x] Inventory current JSON outputs and API mirrors.
2. [x] Add schema/fixture tests for the smallest stable set.
3. [x] Strengthen `tests/skill_artifacts.rs` beyond string contains.
4. [x] Decide and document the Harness Kit projection path for the skill folder.
5. [x] Add a task-card contract check for Goal, Oracle, Boundaries, Output, and
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

Re-promoted P2 → P0 (groom 2026-06-29): the operator explicitly prioritized
the agent-friendly layer — packaged skill, agent CLI, MCP, and unsupervised
workflow authoring — as the product surface. LOC cost remains a risk, but the
first implementation should minimize spine growth by extracting shared read-view
helpers and using fixtures rather than adding workflow judgment. This ticket now
serves epic 076.

## Delivery Notes

### 2026-07-02 read-surface contract fixture slice

- Added `tests/fixtures/contracts/bb.agent_read_surfaces.v1.schema.json`, a
  durable required-path/type contract for `task list`, `runs list`,
  `runs show`, `dlq list`, `gate --json`, and their `/api/*` mirrors.
- Strengthened `tests/agent_contract_fixtures.rs` so the gate loads that
  fixture and validates live CLI output plus served HTTP API responses.
- The fixture deliberately tolerates additive fields but fails removed,
  renamed, or type-changed required paths, giving consuming agents a concrete
  contract file to diff.

### 2026-07-02 skill projection decision slice

- Added ADR-006: `skills/bitterblossom/` in this repo is the single source of
  truth for the portable agent interface.
- Accepted projection paths are source-entry bootstrap, symlink, or automated
  whole-folder replacement. Manual copied skill folders are explicitly not an
  accepted editing surface.
- Strengthened `tests/skill_artifacts.rs` to gate the ADR, source path, and
  duplicate-alias invariant while keeping `.agents/skills/bb-dogfood/` as the
  repo-local dogfood interface.

### 2026-07-02 public task-card contract slice

- Added `tests/task_card_contract.rs`, requiring public-plane task cards to
  carry `Goal`, `Oracle`, `Boundaries`, `Output`, and `Receipt` sections and
  to name `REPORT.json`.
- Reshaped the public fixture cards under `tests/fixtures/public-plane/tasks/`
  to the contract without touching private runtime plane data.
