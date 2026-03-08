# ADR-004: Bounded Review Governance

- **Status:** Proposed
- **Date:** 2026-03-07
- **Related:** [ADR-003](003-remote-conductor-control-plane.md) (Remote Conductor Control Plane), [issue #102](https://github.com/misty-step/bitterblossom/issues/102), [issue #498](https://github.com/misty-step/bitterblossom/issues/498)

## Context

Bitterblossom currently merges through a semantic gate, not through GitHub's raw "all conversations resolved" setting.

That is intentional. Recent review cycles on [PR #495](https://github.com/misty-step/bitterblossom/pull/495) exposed the core tradeoff:

- multiple bot reviewers can emit fresh conversations after each fix
- late low-severity comments can reopen a PR that is already locally correct
- duplicate findings across review surfaces create bookkeeping churn
- GitHub conversation count is a poor proxy for actual merge risk

Turning on branch protection's `conversation_resolution` requirement immediately would harden the wrong contract. It would reward endless bot churn and create a scrupulosity loop where "more comments exist" is treated as "quality is lower," even when the comments are duplicates, nits, or late restatements.

At the same time, doing nothing is also weak. The repo still needs strong guarantees that real blocking findings are addressed before merge.

The problem is not "should reviews matter?" The problem is "what is the bounded merge contract when review surfaces are noisy, asynchronous, and partially redundant?"

## Decision

**Bitterblossom will use bounded review governance, with `merge-gate` as the merge contract and GitHub conversations as evidence inputs, not the source of truth.**

The merge contract will be:

1. **Bounded review set**
   - Only configured review surfaces for the active revision count.
   - Reviews are grouped into explicit waves.

2. **Ledgered findings**
   - Each actionable finding is normalized into a conductor-owned record with:
     - reviewer
     - wave
     - fingerprint
     - severity
     - decision
     - status

3. **Semantic blocking**
   - Only findings classified as blocking by policy can stop merge.
   - Duplicate, stylistic, or explicitly deferred findings do not keep the PR open forever.

4. **Quiet-window convergence**
   - Merge requires:
     - zero active blocking findings
     - required checks green
     - configured review surfaces settled or timed out
     - no new blocking findings during the quiet window

5. **GitHub conversations remain operator-visible**
   - Threads are still replied to and resolved for transparency.
   - But raw thread count is not the merge invariant.

6. **Do not enable raw GitHub conversation-resolution branch protection yet**
   - Keep `merge-gate` as the required branch-protection status.
   - Revisit branch protection only after bounded review governance proves convergence in practice.

## Why

### 1. GitHub thread mechanics are too shallow

Thread existence tells us that someone commented. It does not tell us:

- whether the finding is novel
- whether it is blocking
- whether it is duplicate of another reviewer
- whether it is already addressed in a later commit
- whether it arrived after the review window should have closed

That means GitHub thread count is too noisy to be the merge contract.

### 2. Merge needs semantic finality

A factory cannot be allowed to loop forever because bots keep restating the same class of observation in new words. The control plane must decide when enough review has happened.

### 3. Bounded governance is stronger than permissive drift

This ADR does not weaken review. It makes the contract stricter and more explicit:

- real blocking findings still block
- duplicate and non-blocking findings stop causing unbounded churn
- the decision path becomes auditable

## Design

### Review Wave

A review wave is the set of configured reviewers asked to evaluate a specific commit (or revision window).

Each wave has:

- `wave_id`
- target commit SHA
- reviewer set
- opened time
- settled time
- quiet-window deadline

Only the latest relevant wave for the active revision can block merge.

### Finding Ledger

Each finding becomes a conductor-owned record:

- `finding_id`
- `wave_id`
- `reviewer`
- `source_comment_id`
- `fingerprint`
- `classification`
- `severity`
- `decision`
- `status`

Where:

- `classification` is one of `bug | risk | style | question`
- `severity` is one of `critical | high | medium | low`
- `decision` is one of `fix_now | defer | reject | noise`
- `status` is one of `open | addressed | deferred | rejected | duplicate`

The conductor merges based on this ledger, not based on raw comment counts.

### Blocking Policy

Default policy:

- `critical`, `high` => blocking unless explicitly rejected with rationale
- `medium` => blocking only if marked `fix_now`
- `low` => non-blocking by default
- `style` => non-blocking by default

This policy lives in conductor-owned code and tests, not in reviewer prose.

### Duplicate Handling

The ledger must support duplicate suppression across reviewers and waves.

Examples:

- Gemini and Greptile both flag raw title injection
- CodeRabbit restates a Greptile test nit after the fix commit
- a late bot comment restates an already-addressed concern on a new line number

These should collapse to one semantic finding, not three independent blockers.

### Settling Rule

The conductor may mark review as settled when all of these are true:

- required CI checks are green
- no blocking findings remain open
- configured trusted review surfaces are terminal or timed out
- no new blocking findings arrive during the quiet window
- wave budget has not been exceeded

If a new non-blocking or duplicate finding appears during settling, the conductor records it without reopening the full review cycle.

### Late Findings

Late findings are triaged, not blindly promoted.

Rules:

- late duplicate => mark duplicate, do not reopen
- late low-severity nit => record, do not reopen
- late high-severity novel finding => reopen or block

This preserves safety without punishing every late bot cycle.

## Non-Goals

- Requiring human approvals for every Bitterblossom PR
- Eliminating bot reviewers
- Encoding semantic reviewer triage as brittle regex-only heuristics
- Treating every comment as equally important

## Consequences

### Positive

- Merge behavior becomes convergent instead of potentially unbounded
- Review policy becomes explicit and testable
- Operators can explain why a PR merged or blocked
- Bot reviewer noise stops dominating merge latency

### Negative

- The conductor now owns more semantic governance state
- Duplicate detection and severity normalization need careful design
- A bad policy could under-block or over-block if poorly tuned

## Implementation Sketch

1. Add a review-ledger model to conductor state.
2. Fingerprint review findings so duplicates can collapse across surfaces.
3. Make `merge-gate` read ledger state, not raw thread counts.
4. Preserve GitHub replies/resolution behavior for transparency.
5. Add end-to-end tests proving convergence with:
   - duplicate findings
   - late-arriving nits
   - late-arriving real blockers
   - quiet-window completion
6. Only revisit branch protection after those tests pass reliably.

## Verification

Success means Bitterblossom can prove all of the following in automated tests:

- a true blocking finding prevents merge
- duplicate findings from multiple reviewers do not create infinite churn
- low-severity late comments do not reopen an otherwise merge-ready PR
- a genuinely novel late high-severity finding still blocks merge
- the review loop converges within bounded waves and quiet windows

Issue #102 should carry the end-to-end proof for these scenarios.

## Follow-Up

- issue #102: prove the full single-repo factory trace bullet end-to-end
- issue #498: decide whether branch protection should require conversation resolution after bounded governance exists
