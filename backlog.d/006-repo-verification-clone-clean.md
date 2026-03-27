# Make Repo Verification Clone-Clean

Priority: high
Status: ready
Estimate: S

## Goal
Make the supported repo verification command work from a fresh checkout without requiring a hidden manual `cd conductor && mix deps.get` step first.

## Oracle
- [ ] `make test` succeeds from a fresh checkout without a separate manual dependency bootstrap step
- [ ] The conductor dependency bootstrap is encoded in the supported command path, not tribal knowledge
- [ ] Root docs point to one supported verification command

## Notes
- Derived from the 2026-03-27 agent-readiness audit
- Corresponds to umbrella issue #810

