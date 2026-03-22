PR #785 is pushed and repo-owned checks are green, but the external Cerberus review jobs are still pending after repeated waits:

- review / Cerberus · Architecture
- review / Cerberus · Correctness
- review / Cerberus · Security
- review / Cerberus · Testing

Resolved review-thread feedback has already been addressed in commit `f9f9103`:

- `RunServer` now fails closed when issue re-validation cannot confirm state.
- merged-PR issue auto-close now scans local `Closes #N` references in one pass and ignores cross-repo references.

Current branch: `factory/766-1774194417`
Current PR: `#785`
