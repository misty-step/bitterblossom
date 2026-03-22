---
name: verify-invariants
description: Verify the fix preserved the PR's scope, tests, and load-bearing gates.
---

# /verify-invariants

Run after the fix, before pushing.

## Checks

1. Re-run the failing check locally.
2. Re-run nearby tests that were already passing.
3. Confirm no test was deleted or rewritten to match broken behavior without proof from the acceptance criteria.
4. Confirm no security, authorization, guard, gate, or policy code was removed or weakened.
5. Confirm the diff only fixes the failing PR and does not add unrelated scope.

## Output

Return:

- `PASS` or `FAIL`
- The commands run
- Any invariant risk that still needs human review
