# Add Repo Boundary Guardrails

Priority: medium
Status: ready
Estimate: M

## Goal
Add the repo-level validation and editing guardrails that make agent work predictable: editor defaults, commit-time feedback, and a clearer repo-wide check surface.

## Sequence
- [ ] Create `.editorconfig` at repo root: indent_style=space, indent_size=2 for Elixir/YAML/JSON/MD, charset=utf-8, trim_trailing_whitespace=true
- [ ] Add pre-commit hook: `mix format --check-formatted` for conductor/, shell lint for scripts/
- [ ] Wire pre-commit via `lefthook.yml` or `.githooks/pre-commit` (check which is already in use)
- [ ] Document the check surface in CLAUDE.md: what runs on commit, what runs in CI, what runs manually
- [ ] Verify: commit with unformatted Elixir code is rejected by the hook

## Oracle
- [ ] `.editorconfig` exists and reflects the repo's actual formatting conventions
- [ ] Pre-commit automation exists for the existing check stack
- [ ] Repo-level validation commands are documented and executable

## Notes
- Derived from the 2026-03-27 agent-readiness audit
- Corresponds to umbrella issue #810
