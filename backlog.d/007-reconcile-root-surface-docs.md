# Reconcile Root Docs With Actual Runtime Surface

Priority: high
Status: ready
Estimate: S

## Goal
Align the root README and adjacent docs with the actual Elixir conductor + Codex runtime so agents are not trained on stale `bb`, `cmd/bb`, or `make build` instructions.

## Oracle
- [ ] `README.md` no longer references removed or unsupported surfaces
- [ ] Root docs agree with `docs/CONDUCTOR.md` about setup and runtime flow
- [ ] Legacy Go CLI references are either removed or explicitly scoped to issue #703

## Notes
- Derived from the 2026-03-27 agent-readiness audit
- Corresponds to umbrella issue #810
- Related existing issue: #703

