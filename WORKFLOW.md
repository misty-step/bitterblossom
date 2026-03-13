---
version: 1
control_surface: repo-owned
factory:
  default_workspace_model: one-work-item-one-workspace
  core_phases:
    - shape
    - build
    - review
    - fix
    - merge
    - recover
  workers:
    shape: moss
    build: bramble
    review: thorn
    fix: willow
    merge: fern
    recover: foxglove
  required_skills:
    - shape
    - build
    - pr
    - pr-walkthrough
    - debug
    - pr-fix
    - pr-polish
    - autopilot
  merge_policy:
    separate_semantic_readiness_from_mechanical_checks: true
    known_false_reds_require_incident_reference: true
    merge_method: squash
---

# Bitterblossom Workflow Contract

This file is the primary agent-facing runtime contract for Bitterblossom.

If another document disagrees with this file about execution flow, phases, artifacts, or merge policy, prefer this file unless a more specific task instruction overrides it.

## Primary Principles

1. **One work item, one durable workspace.**
   - Every issue lane, PR-fix lane, review lane, and recovery lane gets its own workspace by default.
   - Do not reuse a dirty workspace for unrelated work.

2. **Small deterministic kernel, agent-forward semantics.**
   - The kernel owns leasing, workspace lifecycle, retries, scheduling, merge execution, and audit state.
   - Agents own shaping, implementation, review interpretation, remediation, and follow-up issue generation through imported skills.

3. **Phase-specialized workers.**
   - Shape work before building.
   - Build work before reviewing.
   - Review and fix before merge.
   - Recovery is a first-class phase, not an ad hoc operator patch.

4. **Truth over convenience.**
   - Keep semantic readiness, policy mergeability, and mechanical GitHub check state distinct.
   - Never pretend a red check is a semantic blocker if it is a known false-red.
   - Never pretend a green check means the PR is semantically good if review evidence disagrees.

## Planning and Workpad Expectations

- Non-trivial work starts with a written plan or workpad.
- Shape output should leave behind a durable planning artifact such as:
  - an issue update,
  - a repo doc under `docs/plans/`, or
  - a workspace `PLAN.md` when the work is still local to one lane.
- That plan should capture:
  - the problem statement,
  - acceptance criteria,
  - the next bounded implementation slice,
  - open questions or risks.
- Build, fix, and recover lanes should update that workpad when the plan changes materially instead of silently drifting.
- Completion notes should record what changed, what was verified, and what remains deferred.

## Phase Contract

### 1. Shape

**Goal:** Turn a raw issue or prompt into a buildable, reviewable work item.

**Default worker:** `moss`

**Required skill(s):** `shape`, optionally `autopilot`

**Inputs:**
- GitHub issue or operator request
- `project.md`
- repo architecture/docs/context
- related code and tests

**Outputs:**
- clarified problem statement
- intent contract
- acceptance criteria
- durable plan or workpad reference
- confidence / uncertainty note
- updated issue context if needed

**Stop conditions:**
- unresolved product ambiguity remains too high
- issue is already satisfied or should not route into fresh work

### 2. Build

**Goal:** Implement the shaped work with bounded scope and verification.

**Default worker:** `bramble`

**Required skill(s):** `build`, `pr`

**Outputs:**
- code change in isolated workspace
- tests and verification evidence
- PR-ready branch state

**Rules:**
- prefer TDD for non-trivial fixes
- keep changes narrow
- do not invent a second feature while implementing the first

### 3. Review

**Goal:** Interpret review surfaces semantically, not mechanically.

**Default worker:** `thorn`

**Required skill(s):** `pr-walkthrough`, `debug`

**Outputs:**
- semantic finding ledger:
  - active merge-blocking findings
  - non-blocking suggestions
  - duplicates
  - already addressed findings
  - defer-to-follow-up candidates

**Rules:**
- unresolved threads are not automatically merge-blocking
- duplicate chatter is not new work
- maintainability nits are not correctness failures by default

### 4. Fix

**Goal:** Resolve active blocking findings and polish merge-ready work.

**Default worker:** `willow`

**Required skill(s):** `pr-fix`, `pr-polish`

**Outputs:**
- addressed blocking findings
- narrower diff where possible
- refreshed verification
- documented follow-up issues for non-blockers worth keeping

### 5. Merge

**Goal:** Merge when policy says merge, not only when GitHub UI happens to be clean.

**Default worker:** `fern`

**Required skill(s):** `pr`, `autopilot`

**Outputs:**
- squash merge via CLI
- run reconciliation
- durable merge audit trail

**Rules:**
- distinguish:
  - semantic readiness
  - policy mergeability
  - mechanical mergeability
- known false-reds on trusted surfaces may be waived if:
  - semantic review is clean
  - repo-side checks are clean
  - upstream incident is linked
  - waiver is recorded in run artifacts or PR comments

### 6. Recover

**Goal:** Handle replay, retriable failures, and external incidents truthfully.

**Default worker:** `foxglove`

**Required skill(s):** `debug`, optionally `autopilot`

**Outputs:**
- classified incident or failure type
- replay decision, waiver decision, or escalation
- updated run state with explicit reason

**Failure classes:**
- transient infra
- auth/config
- semantic code failure
- flaky check
- known false-red trusted surface
- human-policy block
- unknown

## PR Policy

Every implementation or remediation lane should converge on exactly one of these outcomes:
- merge
- close as superseded or already satisfied
- defer with follow-up issue
- block with explicit reason

Do not leave ambiguous “open but maybe done” lanes behind.

## False-Red Policy

A trusted external check may be treated as non-blocking when all are true:
- failure matches a known incident signature
- upstream issue exists and is open
- semantic review is clean
- repo-side checks are green
- no active merge-blocking findings remain

If policy allows override, merge via CLI and record the waiver rationale.

## Operator Notes

- `README.md` explains the product and provisioning surface.
- `docs/CONDUCTOR.md` explains the kernel/operator runtime.
- `sprites/*.md` define worker specialization and imported skill packs.
- This file defines the workflow contract that ties those pieces together.