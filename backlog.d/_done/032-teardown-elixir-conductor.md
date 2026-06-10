# Tear down the Elixir conductor and persona fleet

Priority: P1
Status: done
Estimate: M

## Goal
The repo contains only the v3 surface: vision, contracts, backlog, the Rust
spine (031), and archived prior art — no live Elixir conductor, no resident
persona sprites, no factory docs presented as current.

## Blocked on
~~Operator ratification of the v3 direction and 031 maturity~~ — both held
as of 2026-06-10: v3 ratified ("hit it" + full-delivery directive), spine
shipped with the sprites substrate passing live QA on lane-1.

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
- [x] No file outside `docs/archive/` describes the builder/fixer/polisher
      factory or the OTP conductor as current — conductor/, sprites/,
      base/, fleet.toml, factory root docs (WORKFLOW/PLAN/PROMPT/QA/
      MEMORY/Makefile/dagger), docs/{CONDUCTOR,CLI-REFERENCE,
      COMPLETION-PROTOCOL,CODEBASE_MAP}.md, docs/architecture/,
      docs/context/ all removed (git history is the record); CLAUDE.md +
      README rewritten for v3
- [x] Persona-name grep returns only self-announcing historical refs:
      project.md prior-art note and dated audits/walkthroughs/plans
- [x] CLAUDE.md Build & Test matches reality: cargo build/fmt/clippy/test
      (= the CI gate)
