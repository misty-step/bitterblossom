---
version: 2
control_surface: repo-owned
factory:
  default_workspace_model: one-work-item-one-workspace
  core_phases:
    - shape
    - build
    - review
    - fix
    - land
    - recover
    - reflect
  workers:
    shape: weaver
    build: weaver
    review: fern
    fix: thorn
    land: fern
    recover: tansy
    reflect: muse
  required_skills:
    - shape
    - autopilot
    - code-review
    - debug
    - settle
  landing_policy:
    verification_surface: dagger
    verdict_ref_required: true
    landing_method: squash
---

# Bitterblossom Workflow Contract

This file is the primary agent-facing runtime contract for Bitterblossom.

If another document disagrees with this file about execution flow, artifacts, or
landing policy, prefer this file unless a more specific task instruction
overrides it.

## Primary Principles

1. **One work item, one durable workspace.**
   - Every backlog lane, incident lane, review lane, and recovery lane gets its
     own workspace by default.
   - Do not reuse a dirty workspace for unrelated work.

2. **Local-first truth surfaces.**
   - The repo is the source of truth for work, evidence, and landing state.
   - Treat hosted remotes as transport only. Publishing is not the same thing as
     landing.

3. **Phase-specialized workers.**
   - Shape before build.
   - Build before review.
   - Review and fix before land.
   - Recovery and reflection are first-class phases, not operator cleanup.

4. **Truth over convenience.**
   - Keep semantic readiness, verdict state, and mechanical Dagger results
     distinct.
   - Never treat a passing Dagger run as proof the change is semantically good
     if review evidence disagrees.
   - Never treat a favorable review as enough to land if verification is stale.

## Planning And Workpad Expectations

- Non-trivial work starts with a written plan or workpad.
- Shape output should leave behind a durable planning artifact such as:
  - a backlog item update,
  - a repo doc under `docs/plans/`, or
  - a workspace `PLAN.md` when the work is still local to one lane.
- That artifact should capture:
  - the problem statement,
  - acceptance criteria,
  - the next bounded implementation slice,
  - open questions or risks.
- Build, fix, and recover lanes should update that artifact when the plan changes
  materially instead of silently drifting.
- Completion notes should record what changed, what was verified, and what
  remains deferred.

## Phase Contract

### 1. Shape

**Goal:** Turn a raw prompt, backlog item, or incident into a buildable local
lane.

**Default worker:** `weaver`

**Required skill(s):** `shape`, optionally `autopilot`

**Inputs:**
- `backlog.d/` item, operator request, or Canary incident
- `project.md`
- repo architecture and current code
- related tests and evidence

**Outputs:**
- clarified problem statement
- acceptance criteria
- durable plan or workpad reference
- next bounded implementation slice
- confidence and uncertainty notes

**Stop conditions:**
- unresolved product ambiguity remains too high
- the work is already satisfied, superseded, or should not route into a fresh
  lane

### 2. Build

**Goal:** Implement the shaped work with bounded scope and verification.

**Default worker:** `weaver`

**Required skill(s):** `autopilot`

**Outputs:**
- code change in an isolated branch
- tests and verification evidence
- a branch ready for local review

**Rules:**
- prefer TDD for non-trivial fixes
- keep changes narrow
- do not invent a second feature while implementing the first

### 3. Review

**Goal:** Review semantically and leave a durable local verdict.

**Default worker:** `fern`

**Required skill(s):** `code-review`

**Outputs:**
- a finding ledger:
  - active land-blocking findings
  - non-blocking suggestions
  - duplicates
  - already addressed findings
  - defer-to-follow-up candidates
- a verdict ref for the branch
- evidence under `.evidence/`

**Rules:**
- local review findings matter more than transport metadata
- duplicate chatter is not new work
- maintainability nits are not correctness failures by default

### 4. Fix

**Goal:** Resolve active blocking findings and restore land-readiness.

**Default worker:** `thorn`

**Required skill(s):** `settle`, `debug`

**Outputs:**
- addressed blocking findings
- refreshed verification evidence
- updated verdict when branch state changed materially
- documented follow-up work for worthwhile non-blockers

### 5. Land

**Goal:** Squash-land locally when the branch is semantically ready and Dagger is
fresh.

**Default worker:** `fern`

**Required skill(s):** `settle`

**Outputs:**
- local squash landing onto the default branch
- refreshed evidence bundle
- optional publish step when policy requires a remote update

**Rules:**
- landing requires a valid verdict ref and fresh Dagger evidence
- publish only after local landing is complete
- use repo-native landing surfaces such as `scripts/land.sh`
- trusted CI runners that execute the Dagger wrapper must set
  `BB_ALLOW_PRIVILEGED_DAGGER_IN_CI=1`

### 6. Recover

**Goal:** Handle retriable failures, regressions, and production incidents
truthfully.

**Default worker:** `tansy`

**Required skill(s):** `debug`, optionally `autopilot`

**Outputs:**
- classified failure or incident
- replay, rollback, waiver, or escalation decision
- updated run state with explicit reason

**Failure classes:**
- transient infra
- auth or config
- semantic code failure
- verification failure
- deploy regression
- human-policy block
- unknown

### 7. Reflect

**Goal:** Turn completed lanes into harness and backlog improvements.

**Default worker:** `muse`

**Required skill(s):** `reflect`

**Outputs:**
- retro note
- backlog updates
- codification targets for repeated failures or wasted work

## Landing Policy

Every implementation or remediation lane should converge on exactly one of
these outcomes:
- land locally
- defer with a follow-up item
- close as superseded or already satisfied
- block with an explicit reason

Do not leave ambiguous "maybe done" branches behind.

Verdict refs live under `refs/verdicts/<branch>`. Evidence bundles live under
`.evidence/<branch>/<date>/`.

Landing requires a fresh `ship` verdict for the exact branch tip. Conditional,
`dont-ship`, or stale verdicts block `scripts/land.sh`.

## Operator Notes

- [README.md](README.md) explains the product and provisioning surface.
- [docs/CONDUCTOR.md](docs/CONDUCTOR.md) explains the kernel and operator
  runtime.
- `sprites/*.md` define worker specialization and imported skill packs.
- This file defines the workflow contract that ties those pieces together.
