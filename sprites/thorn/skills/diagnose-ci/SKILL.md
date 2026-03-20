---
name: diagnose-ci
description: Turn failing CI output into a root-cause hypothesis with confidence.
---

# /diagnose-ci

Read the failing checks before proposing any fix.

## Steps

1. Split the CI output by failing check.
2. For each failure, identify the check name, failing test or command, error message, and useful stack trace.
3. Classify it: compile error, test assertion failure, lint violation, timeout, flaky infrastructure, or environment issue.
4. For assertion failures, read the test code and the production code it exercises.
5. Trace the failing expectation back to the issue or PR intent when possible.
6. Separate probable root cause from secondary fallout so you do not fix the symptom instead of the defect.

## Output

Return:

- Primary root-cause hypothesis
- Confidence level
- Evidence from CI output, code, tests, and issue or PR intent
- Any competing hypotheses still worth checking
