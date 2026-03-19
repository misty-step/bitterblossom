---
name: diagnose-ci
description: Turn failing CI output into a root-cause hypothesis with confidence.
---

# /diagnose-ci

Read the failing checks before proposing any fix.

## Steps

1. Split the CI output by failing check.
2. For each failure, identify the check name, failing test or command, and the concrete error.
3. Classify it: compile error, test assertion failure, lint violation, timeout, flaky infrastructure, or environment issue.
4. Read the relevant code and tests.
5. Trace the failing expectation back to the issue or PR intent when possible.

## Output

Return:

- Primary root-cause hypothesis
- Confidence level
- Evidence
- Any competing hypotheses still worth checking
