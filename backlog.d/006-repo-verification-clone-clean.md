# Make Repo Verification Clone-Clean

Priority: high
Status: ready
Estimate: S

## Goal
Make the supported repo verification command work from a fresh checkout without requiring a hidden manual `cd conductor && mix deps.get` step first.

## Sequence
- [ ] Update `Makefile` test target to run `mix deps.get` before `mix test` if `conductor/deps/` is missing
- [ ] Verify `make test` succeeds from a fresh `git clone` with no prior setup
- [ ] Update CLAUDE.md "Build & Test" section to point to `make test` as the single verification command
- [ ] Remove or redirect any docs that reference `cd conductor && mix deps.get && mix compile && mix test` as separate manual steps

## Oracle
- [ ] `make test` succeeds from a fresh checkout without a separate manual dependency bootstrap step
- [ ] The conductor dependency bootstrap is encoded in the supported command path, not tribal knowledge
- [ ] Root docs point to one supported verification command

## Notes
- Derived from the 2026-03-27 agent-readiness audit
- Corresponds to umbrella issue #810
