# Canary Triage Live Proof Packet

Backlog: bitterblossom-114 (child of epic bitterblossom-094). Proves the
already-deployed, report-only `canary-triage` task's trigger-through-dispatch
path against the production plane, using a replayed real low-severity Canary
incident. **Update 2026-07-05: fully proven end to end.** The first attempt
(below, "Dispatch") was a partial proof per lead ruling, blocked on
bitterblossom-925 (bot identity). The operator then rescoped 925 to the
fully agent-driven path — required-vs-optional secret semantics, `GH_TOKEN`
marked optional on `canary-triager` (bot-identity creation is web-UI-only and
the operator declined it) — and once that shipped, dead letter 24 was
replayed. See "Completion" below for the final run.

## Summary

| Field | Value |
|-------|-------|
| Date | 2026-07-04 |
| Plane | production (`/Users/phaedrus/Development/bitterblossom/plane`) |
| Task | `canary-triage` (`report-only` authority, backlog 080) |
| Trigger | manual (`bb run canary-triage --payload-file <fixture>`) |
| Payload | replayed — `2026-07-04-canary-triage-live-proof-payload.json` in this directory (the `.webhook` sub-object of `tests/fixtures/contracts/canary.low_severity_incident_drill.v1.json`) |
| Incident referenced | `INC-ay76lctwao3z` (real canary-service incident, opened 2026-07-02T20:11:43Z) |
| Run id | `066128e70685` |
| Idempotency key | `bitterblossom-114-live-proof-replay-INC-ay76lctwao3z` |
| Dead letter | `24` (status: `open`) |
| Outcome | `failure` — dispatch-time secret gate refused the harness before execution |

## Why "replayed," not a freshly-live incident

Canary's current open incidents are all `severity: medium` — matching the
established contract from bitterblossom-108/canary-910: incident severity is
derived purely from active correlated signal count and is always one of
`{medium, high}`; there is no `low` tier. "Low-severity" here means the
*originating signal's* severity, which the 108 fixture already captured as a
pinned, provenance-tracked replay of a real canary-service incident. Replaying
it is the acceptance-sanctioned alternative to a freshly-opened low-severity
incident, since no such incident can exist by Canary's own design.

## Preflight (read-only, before dispatch)

```text
$ ./target/debug/bb --config plane preflight canary-triage --json
{
  "tasks_checked": ["canary-triage"],
  "findings": [
    {
      "task": "canary-triage",
      "kind": "missing_secret",
      "detail": "declared secret 'GH_TOKEN' is not set in the environment"
    }
  ]
}
```

`OPENROUTER_API_KEY`, `CANARY_ENDPOINT`, and `CANARY_API_KEY` were already
resolvable in the environment; `GH_TOKEN` was the only missing declared
secret at investigation time.

```text
$ ./target/debug/bb --config plane runs list --task canary-triage --json
[]
```

No `canary-triage` run had ever executed on the production plane before this
proof packet.

## Dispatch

Ran without `GH_TOKEN` set at all — no operator personal identity used,
per lead ruling (the Cerberus reviewer wrapper's refuse-ambient-auth
principle applies with extra force on a maiden production run of a new
pathway, read-only or not).

```text
$ ./target/debug/bb --config plane run canary-triage \
    --payload-file 2026-07-04-canary-triage-live-proof-payload.json \
    --idempotency-key bitterblossom-114-live-proof-replay-INC-ay76lctwao3z \
    --json
```

## Evidence

- **Run id:** `066128e70685`
- **State:** `failure` (`state_reason: dead_letter:24 secret env var 'GH_TOKEN' not set`)
- **Attempts:** 3, all `phase: acquired` (harness never spawned) —
  `cost_usd: null` on every attempt: zero cost, zero external side effects,
  zero Canary/repo mutation.
- **Dead letter:** id `24`, `status: open`, `run_id: 066128e70685`, payload
  matches the fixture verbatim (confirmed via `bb dlq list --json`).

## The finding

BB's own secret-presence gate refuses to spawn the harness at all when a
declared agent secret is unset — it fails *before* the agent process starts,
so the agent's own documented fallback ("if credentials are missing, write a
blocked report naming the exact command and error", `card.md`) never gets a
chance to run. That graceful-degradation text describes what the *agent*
does once it has started and discovers a problem; it does not describe what
happens when the plane's dispatch layer won't start it in the first place.

This is a real required-vs-optional secret semantics gap: `canary-triager`'s
own card explicitly anticipates degrading gracefully on missing credentials,
but the agent config's `secrets = [...]` list has no way to mark one entry
"soft" (best-effort, let the agent discover and report the gap) vs. "hard"
(refuse to dispatch at all). Every declared secret is currently hard-required.
Whether to add that distinction, or to keep `GH_TOKEN` hard-required and
resolve this purely by provisioning a bot identity, is a design call for
whoever picks up bitterblossom-925 or a dedicated follow-up — not decided
here.

## Exact payload/delivery identity

- Payload file: `2026-07-04-canary-triage-live-proof-payload.json` (this
  directory) — the `.webhook` sub-object of
  `tests/fixtures/contracts/canary.low_severity_incident_drill.v1.json`; the
  whole drill-wrapper file is not a valid `EVENT.json` payload, confirmed
  against `tests/canary_contract.rs`'s own extraction convention.
- `subject.id`: `INC-ay76lctwao3z`
- `signal.fingerprint`: `ERR-y4y82bm9zq61`

## Disposition

Per lead ruling, this counts as proof of the trigger→dispatch half of the
pathway: a real replayed low-severity incident, dispatched through the
legitimate `bb run` ingest path (not manual ledger insertion), with correct
payload/idempotency identity and clean dead-letter observability at zero
cost. It does **not** satisfy the epic's full "run end to end" acceptance
item, since the agent itself never executed and no `REPORT.json` exists.

The remaining half — an actual successful run producing `REPORT.json` — is
blocked on a dedicated bot/app `GH_TOKEN` for `canary-triager`, tracked as
bitterblossom-925 (operator-gated: creating that identity is not something
an agent can do for itself). Once that lands, replay dead letter 24
(`bb dlq replay 24 --json`) to give this exact run/payload/idempotency
lineage a genuine completion rather than dispatching an unrelated new run.

## Completion (2026-07-05)

bitterblossom-925 shipped `optional_secrets` (PR #976,
`142ffcead5097735cdde0c341c5b4f40f6af41a8`) — a declared secret that's
unresolvable degrades the run instead of dead-lettering it. `canary-triager`'s
`GH_TOKEN` moved from `secrets` to `optional_secrets` on the production plane
(`plane/agents/canary-triager.toml`); `bb --config plane preflight
canary-triage --json` now reports it as the informational
`missing_optional_secret`, not the blocking `missing_secret`.

Replaying dead letter 24 (`bb --config plane dlq replay 24 --json`) advanced
past the secret gate into workspace prep and surfaced a second, unrelated
finding: `task.toml`'s `workspace.repos` pinned the bitterblossom clone to
`ref = "factory/bitterblossom-lane-20260701"`, a since-merged-and-deleted
feature branch (confirmed absent via `git ls-remote --heads origin
factory/bitterblossom-lane-20260701`, empty). Corrected to `ref = "master"`,
matching the sibling `canary` repo entry — an ordinary stale-config fix in
the operator's own untracked runtime plane, not a security-boundary edit.

Replaying the resulting dead letter 25 (`bb --config plane dlq replay 25
--json`) ran to completion:

| Field | Value |
|-------|-------|
| Run id | `51a0944c862e` (parent `9a3884522dea` → `066128e70685`) |
| State | `success`, exit code `0` |
| Cost | `$0.0073` (43,381 input / 6,119 output tokens) |
| Duration | 119.4s |
| `GH_TOKEN` | never set — no operator identity, no bot identity, none at all |

`REPORT.json` was written as designed:

```json
{
  "status": "blocked",
  "summary": "Canary API at https://canary-obs.fly.dev rejected all requests with HTTP 401 Unauthorized. The CANARY_API_KEY environment variable is present (32 chars) but not accepted by the endpoint. ...",
  "residual_uncertainty": [
    "...",
    "GH_TOKEN is empty — no GitHub read-only inspection was possible for cross-referencing signal fingerprints against repo code"
  ]
}
```

The agent ran to completion and degraded exactly as its card promises: it
noticed `GH_TOKEN`'s absence and named it correctly as one *minor* residual
gap, not the blocker. The actual blocker is that `CANARY_API_KEY` itself is
rejected (401) by the live Canary endpoint — almost certainly the same
credential implicated in canary-916 (P1 rotation, filed 2026-07-04 after an
unrelated grep mistake printed its value into a transcript); this run is
corroborating evidence that the pre-rotation key is now invalid against the
live service, not a new finding of its own.

**Full acceptance now met:** the required-vs-optional mechanism works, the
agent's own graceful-degradation behavior works, and a real `REPORT.json`
exists from a genuine dispatch — end to end, zero GitHub credential of any
kind.

## Residual risk

- The rollout scorecard
  (`docs/rollout-scorecards.md#canary-triage-report-only-backlog-080`) needs
  `>=3` real low-severity incidents before promotion eligibility; this packet
  supplies one full completed run plus the earlier trigger-side dead-letter
  proof.
- `CANARY_API_KEY` on this plane appears stale/rejected by the live Canary
  endpoint (HTTP 401) — worth confirming against canary-916's rotation
  status; canary-triage cannot produce a fully-informed report until it's
  current.
- A live `CANARY_API_KEY` value was accidentally printed into an agent
  transcript while investigating this card's credential availability (an
  unrelated grep-pattern mistake, not used for anything beyond that one
  accidental print). Filed and being rotated as canary-916.
