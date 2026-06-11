# Context Packet: The submission loop — verdict storm, mechanical gate, bounded rounds

## Goal

Completed agent work gets autonomously quality-assured and landed: a
storm of independent verdict-producing tasks attacks a submitted change,
a mechanical gate decides blocking vs. clear, the implementing agent
loops on the report, and the loop terminates by construction — no human
reads the code, no pull request is required for coordination.

## Non-Goals

- **Push-event reflex ingress** (GitHub App or post-receive hook on a
  self-hosted remote). v1 is dispatch-driven by operator decision
  (AskUserQuestion, 2026-06-11): the implementing agent fires the storm.
  The trigger seam is the plane's existing trigger table; reflex arrives
  as a follow-up ticket once the plane has a durable home.
- **Plane deployment.** The loop runs from a laptop plane + sprite fleet.
- **PR integration.** No comments posted anywhere. A PR may exist for
  external visibility; it is never the channel.
- **git-notes export** of verdicts. The ledger is canonical; a notes
  export step can ride in a marshal card later without spine changes.
- **An agentic-QA storm member** (drives the running app). Needs a
  per-repo runnable-app harness; v2.
- **Multi-repo gate config.** One `[gate]` per plane in v1; the config
  is data, so per-repo gates later are a config-shape change, not a
  schema change. (Flagged by critique; accepted as conscious v1 scope.)
- **Mechanical verification of advisory filing.** The gate stays about
  code state; whether the driver filed advisories to backlog.d is
  audited by the harness-gardener loop, not by `bb gate` (verifying
  workload-specific artifacts would put repo judgment in the spine).

## Constraints (invariants that survive)

- **No workload logic in the spine.** Submissions, verdicts, and gate
  arithmetic are generic data mechanics (like budgets and runs). What a
  reviewer looks for and how findings are phrased — cards only.
- **Termination is never bought by silencing fresh blockers.** The
  round cap + escalation is the only termination mechanism. (First
  draft demoted fresh round-N blockers; adversarial critique showed
  that mechanically clears fatal late discoveries. Redesigned.)
- **Model & auth policy holds**: storm members are reflex-class —
  api-auth agents, cheap OpenRouter models, hermetic.
- **Spine LOC budget ≤ 5k** (currently 4,032; this estimates +700–900).
  If the budget breaks, stop and renegotiate explicitly — do not trim
  tests to fit.
- **Unparseable output is failure** — a verdict task whose result is
  not valid verdict JSON fails the run; never a silent pass.
- **`change` keys and `rev`s are opaque strings** to the spine. v1
  drivers pass branch/ticket as the change key and the SHA as rev; the
  jj migration later swaps in change IDs with zero spine change.

## Repo Anchors

- `src/spec.rs` — config loading + load-time validation; where
  `verdict = "<kind>"` on TaskSpec, `[gate]` on PlaneSpec, and the
  `command` harness's relaxed model requirement land.
- `src/ledger.rs` — table + accessor patterns and CAS transitions
  (`try_transition`); submissions reuse this discipline exactly.
- `src/dispatch.rs` — `attempt_on_host` collecting phase (verdict
  extraction) and `WorkspacePlan` (REPORT.json injection rides next to
  the existing EVENT.json materialization).
- `src/harness.rs` — the adapter seam; `command` is one more arm that
  maps exit status to a verdict, no LLM.
- `src/serve.rs` / `src/main.rs` — read API and CLI verb patterns for
  `bb submit`, `bb gate`, `bb verdicts`.
- `plane/tasks/review/card.md` + `task.toml` — exemplar verdict task
  (its JSON-summary final answer is already verdict-shaped).
- `tests/e2e_local.rs` — stub-harness e2e pattern for loop tests.
- `CLAUDE.md` (Verification) — the live-evidence contract this extends.

## Alternatives

1. **Old mode: PR + GitHub CI + comment storm.** Vendor-locked
   coordination, markdown threads as data model, termination in prose.
   Documented fallback, not built.
2. **Orchestrated fan-out/join in the spine** (run groups, join state
   machines, report assembly). Workflow semantics that grow without
   bound — the Python conductor's death. Rejected.
3. **Pure git-notes blackboard, zero spine change.** Notes-ref write
   contention, sprite push credentials, nothing queryable, policy as
   card prose. Rejected for v1; notes become an export later.
4. **Blackboard via ledger with explicit submission sessions** (chosen,
   revised after critique): verdict tasks write structured verdicts;
   submissions give the gate a locked instance to judge; `bb gate`
   evaluates pure data policy; the driver loops. Coordination through
   the artifact; the spine grows two tables and one evaluator, all
   generic.

## Design

**Submissions (new, critique-driven).** `bb submit open --change <key>
--rev <sha> [--context <text>]` creates a submission row:
`(id, change_key, rev, round, state, prior_report_json, created_at)`.
CAS invariant: at most one submission in a non-terminal state per
change_key (`open` → `clear | blocked | escalated | abandoned`; opening
round N+1 requires round N `blocked`). Round numbering is owned by the
plane: each new submission for a change increments it. `prior_report_json`
is snapshotted by the plane from the previous round's gate report at
open time — **the driver cannot soften, omit, or rewrite prior findings;
reviewers receive the canonical report** (injected as `REPORT.json` in
the workspace, exactly as EVENT.json is materialized today).

**Verdict tasks.** `task.toml` gains `verdict = "<kind>"`. The storm
payload contract is `{"submission": <id>, "repo": "...", "rev": "...",
"context": "..."}`. For verdict tasks, a successful run's parsed result
MUST be JSON:

```json
{"verdict": "pass|blocking|advisory",
 "findings": [{"severity": "blocking|serious|minor",
               "file": "src/x.rs", "line": 42,
               "claim": "...", "evidence": "...",
               "fingerprint": "<copied from REPORT.json when re-raising, else omitted>"}]}
```

Dispatch records a verdict row keyed to the submission; the plane
computes `sha256(kind|file|claim)` when fingerprint is absent.
Unparseable verdict JSON fails the run, raw output preserved.

**The `command` harness (critique-driven).** `harness = "command"`
tasks run `bin` + `args` directly: exit 0 → `pass`, non-zero →
`blocking` with the stderr tail as the finding evidence. No LLM, no
tokens, no model required in the agent spec. The `verify` storm member
uses it — a deterministic gate result is never mediated by an agent's
JSON. (First draft wrapped `verify.sh` in an LLM; critique correctly
called that judgment-smuggling into a required gate member.)

**The gate.** `plane.toml`:

```toml
[gate]
required = ["verify", "correctness", "security", "simplification", "product"]
max_rounds = 3
```

`bb gate --submission <id>` (and `--change <key>` resolving to the
non-terminal submission; `GET /api/gate?...`) evaluates **only this
submission's** verdicts:

- any required kind without a terminal run → `pending`, listing per-kind
  run states. A required kind whose run is terminal-failure with
  mechanical retries exhausted → the submission transitions to
  `escalated` (notify fires once): infrastructure failure is loud,
  never an eternal `pending`. Gate output is therefore stable against
  driver timing — `clear` is only ever emitted over a complete round.
- **Blocking rule, all rounds**: any finding with severity `blocking`
  blocks — fresh or carried, round 1 or round N. Late fatal discoveries
  are never demoted by recency. (`regression: true` from the first
  draft is deleted; it was an unaudited reviewer-controlled override.)
- **Anti-needling, mechanically**: `serious` and `minor` findings never
  block after round 1 (round 1 may treat `serious` as blocking is NOT
  the rule — only `blocking` blocks, every round; severity inflation is
  the residual risk, see below). A fingerprint the driver has
  **rejected** (`bb submit reject <fp> --reason <text>`) cannot block
  again — but rejecting a `blocking`-severity finding only takes effect
  once an `arbiter` verdict (a designated kind, different model family,
  fired by the driver on the disputed finding) independently sustains
  the rejection. Rejections and their reasons appear verbatim in every
  subsequent REPORT.json and in gate output — visible, costly to abuse,
  audited by the gardener loop.
- a submission `blocked` at `round == max_rounds` → `escalated`
  (notify): rounds 1..=max_rounds may run; there is no round
  max_rounds+1. (First draft was off by one.)
- otherwise → `clear`.

Gate output groups findings `blocking / advisory / rejected`, joined
with per-member run states and costs.

**Storm config (pure files).** Six tasks in `plane/tasks/`:
`verify` (command harness, runs `./scripts/verify.sh`), `correctness`,
`security`, `simplification`, `product` (single-pass cards derived from
the existing review card minus the `gh pr comment` step; different
model families across them for decorrelation), and `arbiter` (invoked
only on disputed blocking findings; frontier-leaning cheap model,
receives one finding + the code + the rejection reason, answers
sustain/overrule). Each binds its own sprite host (`bb-polisher`,
`bb-polisher-2`, …) so members parallelize — host leases serialize per
host, so parallelism is a hosting choice, not a spine change.

**The driver (convention, documented in docs/spine.md).** On judging
work complete: push the branch; `bb submit open`; fire the required
storm members as parallel `bb run` background processes with idempotency
keys `storm:<submission>:<kind>`; `bb gate` (safe to call any time);
on `clear` → file advisories to backlog.d, squash-land, push,
`bb submit close --landed`; on `blocked` → fix, push new rev,
`bb submit open` for the next round (plane snapshots the report),
repeat; on `escalated` → stop; the operator is already notified.
Judgment (what to fix, what to reject, what to file) stays with the
agent; arithmetic (what blocks, when rounds end) lives in `bb gate`.

**Why this decomposition**: CI, review, red-team, and product critique
are all the same shape — verdict tasks; the merge gate is a query over
a locked submission; "waiting for CI before the storm" collapses into
wave ordering the driver controls.

## Oracle (Definition of Done)

Commands that must all exit 0:

- `./scripts/verify.sh` — including new tests:
  - submissions: CAS single-non-terminal-per-change; round increments
    plane-side; `prior_report_json` snapshot on open; REPORT.json
    materialized in verdict-task workspaces;
  - verdict parsing: valid JSON → row on the right submission; invalid
    JSON on a verdict task → run failure, raw output preserved;
  - command harness: exit 0 → pass verdict; exit non-zero → blocking
    verdict carrying stderr; no model required in spec;
  - gate rules, each its own test: `pending` with per-kind run states
    on incomplete round; terminal-failure of a required kind →
    `escalated` + notify; fresh `blocking` finding blocks in round 2;
    `serious`/`minor` never block; rejected non-blocking fingerprint
    stays non-blocking; rejected `blocking` fingerprint still blocks
    until an arbiter verdict sustains the rejection; `blocked` at
    `max_rounds` → `escalated` + notify; `clear` only over a complete
    round.
- `bb check` on `plane/` validates the six storm tasks and `[gate]`.
- LOC tripwire ≤ 5000.

Live evidence (harness: `scripts/verify.sh` + CLAUDE.md live-evidence
contract, both updated as part of this work):

- **Full loop on a seeded change in this repo**: branch with ≥2 planted
  flaws → round-1 storm, ≥2 members concurrent on distinct sprites
  (overlapping ledger timestamps) → gate `blocked` naming the plantings
  → fix → round-2 `clear` → squash-landed. Evidence: gate JSON both
  rounds, per-member costs, total loop cost ≤ ~$1.
- **Arbiter drill (live)**: driver rejects a planted blocking finding →
  gate still `blocked` → arbiter run sustains rejection → gate `clear`.
- **Termination drill (stub harness, dev plane)**: blockers persist
  through round 3 → submission `escalated` exactly at `max_rounds`,
  one notify delivery; plus a dead-lettered required member →
  `escalated`, never eternal `pending`.

## Premise Source

Premise Source: sha256:19be5adb56ca265fe00c943a64988b285fe737d96cd0ed34cd36a0089d93e0a3 docs/plans/2026-06-11-039-premise-transcript.md

Voice Transcript Metadata:
- source_kind: voice
- source_hash: sha256:19be5adb56ca265fe00c943a64988b285fe737d96cd0ed34cd36a0089d93e0a3
- transcript_model: unknown
- transcript_confidence: unknown
- audio_duration_seconds: unknown
- redaction_status: sanitized
- redaction_tool: agent-transcript
- created_at: 2026-06-11T04:30:00Z
- residual_risk: Transcript accuracy unverified; dictation compressed to
  load-bearing excerpts; the surrounding brainstorm (PR decomposition,
  blackboard coordination, rung ladder) is paraphrased in-session rather
  than quoted.

## Critique Record

Adversarial fresh-context review by codex/gpt-5.5 (receipt
`.harness-kit/traces/provider-lanes/20260611T161652.535180Z-codex-27408258.txt`),
5 blocking + 8 serious + 3 minor. Accepted and redesigned: fresh-blocker
demotion deleted (termination now rests solely on the round cap),
`regression: true` deleted, explicit submission lifecycle with CAS,
plane-side round ownership and report snapshotting (closes the
driver-withholding channel), round-completeness gating (closes the
partial-round race), required-member failure → escalation (no eternal
pending), `command` harness for verify (no LLM mediating a deterministic
gate), arbiter quorum for rejecting blocking findings, max_rounds
off-by-one fixed. Rejected with reasons: gate config per-repo (v1 has
one repo; config-shape change later), mechanical advisory-filing
verification (workload judgment; gardener audits instead),
product/simplification in the required set (that's operator config —
exactly where workload judgment belongs).

## Risks + Rollout

- **Severity inflation is the residual needling vector**: with only
  `blocking` able to block, a needler inflates severities. Counters:
  the falsifiability bar in cards (a blocking finding must name a
  concrete failure), the arbiter path (driver rejects + arbiter
  sustains at ~$0.01), the round cap, and gardener review of reviewers
  whose blocking findings are repeatedly overruled. Accepted
  consciously: the failure mode is now "operator gets pinged at round
  3" rather than "fatal bug lands" — the right direction per premise.
- **Fingerprint matching is imperfect** (rewording escapes rejection
  stickiness). Fails toward re-blocking → arbiter → bounded by rounds.
  Accepted.
- **Spine growth +700–900 LOC** against 968 headroom. Tightest
  constraint in the packet; the budget conversation triggers before
  trimming anything. Trim candidates if needed, in order: arbiter
  mechanics (degrade to convention), API routes (CLI-only v1).
- **Stop conditions for the builder**: payload/contract needs a field
  not specified here → stop and surface; gate rules need a
  workload-specific branch ("security findings never rejectable") →
  stop, constitutional tension to resolve explicitly; storm cost per
  round exceeds ~$1 → stop and rebind models; submissions CAS
  conflicts with the existing host-lease or run state machines in any
  way → stop and re-shape rather than special-case.
- **Rollback**: additive tables + config; revert the squash commit and
  delete the task dirs. No migrations beyond new SQLite tables.
