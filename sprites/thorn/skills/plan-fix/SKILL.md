---
name: plan-fix
description: Define the minimum fix and the invariants that must survive it.
---

# /plan-fix

Do not edit code until this plan is written down.

## Plan Format

1. Root cause
2. What the failing check expects
3. Why that expectation is correct according to the linked issue or PR intent
4. The minimum code change that should satisfy the check
5. Invariants to preserve:
   - security and policy gates
   - existing passing tests
   - PR scope and acceptance criteria

## Self-Approval

Answer this before coding:

`Does this fix satisfy the failing check without undermining the PR's design intent?`
