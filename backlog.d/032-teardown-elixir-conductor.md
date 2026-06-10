# Tear down the Elixir conductor and persona fleet

Priority: P1
Status: blocked
Estimate: M

## Goal
The repo contains only the v3 surface: vision, contracts, backlog, the Rust
spine (031), and archived prior art — no live Elixir conductor, no resident
persona sprites, no factory docs presented as current.

## Blocked on
Operator ratification of the v3 direction (project.md 2026-06-10) and 031
reaching a state where nothing still depends on the conductor.

## Scope
- Move `conductor/` (Elixir), `sprites/` persona definitions, `fleet.toml`,
  and the factory-era root docs (WORKFLOW.md, PLAN.md, PROMPT.md, QA.md,
  CONDUCTOR.md, CLI-REFERENCE.md, COMPLETION-PROTOCOL.md) to `docs/archive/`
  or delete where git history is record enough — operator chooses per item.
- Rewrite CLAUDE.md/README.md to describe v3 only; kill the sprite-name
  roster and factory operating model sections.
- Preserve earned knowledge explicitly: the conductor's sprite-lifecycle
  lessons already live in harness-kit `skills/sprites/references/
  provisioning.md`; Tansy's mission survives as workload spec #2.
- Update auto-memory (MEMORY.md fleet/factory entries) to match.

## Oracle
- [ ] No file outside `docs/archive/` describes the builder/fixer/polisher
      factory or the OTP conductor as current
- [ ] `git grep -li "weaver\|thorn\|fern" -- ':!docs/archive' ':!backlog.d/_done'`
      returns only historical references that announce themselves as such
- [ ] CLAUDE.md Build & Test section matches what actually builds
