# Canary Triage Live Proof Packet

Backlog: bitterblossom-114 (child of epic bitterblossom-094). Proves the
already-deployed, report-only `canary-triage` task's trigger-through-dispatch
path against the production plane, using a replayed real low-severity Canary
incident. This packet is a **partial** proof by lead ruling — see
"Disposition" below — with the remaining half tracked as bitterblossom-925.

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

## Residual risk

- This is the first-ever execution attempt of this pathway; the rollout
  scorecard (`docs/rollout-scorecards.md#canary-triage-report-only-backlog-080`)
  needs `>=3` real low-severity incidents before promotion eligibility — this
  packet supplies the trigger-side half of one, pending bitterblossom-925.
- A live `CANARY_API_KEY` value was accidentally printed into an agent
  transcript while investigating this card's credential availability (an
  unrelated grep-pattern mistake, not used for anything beyond that one
  accidental print). Filed and being rotated as canary-916; not blocking
  for this packet, noted here for completeness.
