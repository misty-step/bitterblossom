The `e2e-test` skill drives an adversarial end-to-end shakedown of the `bb dispatch` pipeline. It is invocable via `/e2e-test` and is structured as a nine-phase workflow that exercises build, fleet health, issue selection, credential validation, dispatch, monitoring, completion verification, PR quality review, and findings reporting.

### Key Principles

*   **Adversarial mindset**: Assume something is broken. Every phase is scored PASS / FRICTION / FAIL — friction (unnecessary difficulty, confusion, or delay) is a reportable finding, not "working as expected."
*   **Mandatory issue filing**: A shakedown that finds problems but doesn't file GitHub issues is incomplete. The report is a byproduct; filed issues are the deliverable.
*   **Known failure modes**: Watch for stale `TASK_COMPLETE` signals, polling loops, zero-effect oneshot dispatches, proxy health failures, and stdout/JSON pollution.

### Files

*   **`SKILL.md`**: The skill definition. Contains the full nine-phase workflow with exact shell commands, monitoring guidance, and issue-filing instructions.
*   **`references/`**: Reference documents consulted during execution (evaluation rubric, friction taxonomy).
*   **`templates/`**: Output scaffolds for the findings report produced at Phase 9.
