# Add Repo Boundary Guardrails

Priority: medium
Status: ready
Estimate: M

## Goal
Add the repo-level validation and editing guardrails that make agent work predictable: editor defaults, commit-time feedback, and a clearer repo-wide check surface.

## Oracle
- [ ] `.editorconfig` exists and reflects the repo's actual formatting conventions
- [ ] Pre-commit automation exists for the existing check stack
- [ ] Repo-level validation commands are documented and executable

## Notes
- Derived from the 2026-03-27 agent-readiness audit
- Corresponds to umbrella issue #810

