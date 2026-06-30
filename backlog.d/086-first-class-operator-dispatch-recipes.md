# Make BB operator dispatch recipes first-class and hard to misuse

Priority: P1 · Status: ready · Estimate: L

## Goal

Replace fragile ad-hoc shell wrappers for builder/storm dispatch with BB-owned operator recipes or CLI commands that validate payloads, secrets, duplicate work, and watcher receipts before paid work begins.

## Problem / Dogfood Evidence

The 077/079 dogfood loops repeatedly failed for non-semantic reasons:

- malformed shell wrapper around `GH_TOKEN` produced `syntax error near unexpected token ')'`;
- missing `GH_TOKEN` produced dead-lettered storm lanes;
- missing `repo` in storm payload made verification try `https://github.com/.git/`;
- different idempotency keys created duplicate semantic work for backlog 079;
- killed local wrappers left BB runs needing recovery/classification;
- `submit open --json` returned top-level `{id,...}` while the ad-hoc wrapper expected `{submission:{id}}`, proving the recipe needs a tested receipt schema instead of wrapper-side guesses;
- `op-agent` supplied some secrets but not `GH_TOKEN`, so the wrapper had to know a second auth preflight/export rule;
- the operator had to manually assemble payload JSON, `op-agent`, GitHub auth, storm member dispatch, and watcher logic.

These are not model-intelligence problems. They are BB operator UX and product-contract gaps.

## Oracle

- [ ] A builder dispatch recipe/command can run a groomed backlog item with one safe command or checked-in script.
- [ ] A submit-and-storm recipe/command can open a submission and dispatch required lanes with one safe command or checked-in script.
- [ ] Payload JSON is accepted via `--payload-file` and validated before ledger mutation.
- [ ] Required fields (`repo`, `change`, `rev`, `backlog`, `base_ref`, etc.) are validated with source-specific errors.
- [ ] Required secrets are preflighted before paid remote execution; missing secrets fail before storm fanout.
- [ ] Duplicate active work is refused unless `--force` is explicit and recorded.
- [ ] The command returns a receipt: run/submission id, idempotency key, branch/change, watcher command or status URL, and safe next command.
- [ ] `bb submit list --json` exposes recent submission/gate state so supervisors can find active or stale review work without direct SQLite queries.
- [ ] Secrets and prompts travel on stdin/env-safe paths, not process-table-visible argv where avoidable.
- [ ] `./scripts/verify.sh` passes.

## Verification System

- Claim: an operator or Hermes cron supervisor can dispatch BB work without inventing a one-off shell wrapper and without creating duplicate paid semantic work.
- Falsifier: the happy path still requires manual JSON quoting; missing `repo` reaches verifier; missing `GH_TOKEN` consumes storm runs; duplicate active backlog/build work is accepted silently; or receipt lacks the safe next inspection command.
- Driver: local/dev-plane fixture for payload validation and duplicate refusal; one live low-cost builder or storm dry run under `plane/` with explicit receipt.
- Grader: command/script exits before mutation on invalid payload/secret; one active duplicate is blocked; receipt is machine-readable and enough for a Hermes cron supervisor to continue.
- Evidence packet: command transcript, receipt JSON, duplicate-refusal transcript, preflight failure transcript, and one live run/submission id.
- Cadence: every time a new operator recipe is added or a dispatch prompt changes.

## Suggested Shape

Start with repo-owned scripts if that is faster, then promote to `bb` commands once the contract stabilizes:

```text
scripts/bb-dispatch-build.sh --backlog 079 --base-ref bb/build/agent-friendly-local-contracts --branch-slug artifact-cli-first-slice --payload-file packet.json
scripts/bb-submit-storm.sh --repo misty-step/bitterblossom --change <key> --rev <sha> --payload-file storm.json
```

or CLI-native:

```text
bb operator build --payload-file packet.json --json
bb operator storm --payload-file storm.json --json
```

The contract matters more than the spelling.

## Promotion Metrics

- After 5 successful dogfood dispatches with no shell-wrapper edits, promote script contract into CLI if still script-owned.
- After 0 duplicate active-run incidents across 5 dogfood cycles, allow the Hermes supervisor to use the recipe at Level 2 authority.
- If any auth/payload field failure reaches paid execution, demote the recipe back to manual-only until fixed and storm-tested.

## Notes

This is the BB-side complement to backlog 085. Hermes can schedule a loop, but BB must give that loop safe, typed operator actions.

2026-06-30 tick evidence: after local verify and Thermo approval for `e2ccd32`, the hand-built storm wrapper opened submission `e0584b6875b3` but dispatched payload `{"submission":"e0584b6875b3"}` without `repo`. The verify lane cloned `https://github.com/.git/` and the security lane refused to produce a verdict because `EVENT.json` lacked `repo`, escalating the gate before simplification/product ran. This polluted submission should be abandoned; the next clean storm must use a typed recipe/payload that includes `repo":"misty-step/bitterblossom"` and refuses to dispatch if required fields are missing.

2026-06-30 cron tick evidence: the Level 3 dogfood loop reached the clean storm point for rev `e9531a3` (local verify green, Cursor Thermo PASS, branch pushed) but `bb preflight --storm --json` failed before fanout because the fresh Hermes cron environment lacked `OPENROUTER_API_KEY`; `GH_TOKEN` was recoverable from `gh auth token`. The recipe/supervisor contract needs a first-class secret bootstrap/preflight receipt so cron fails before opening or dispatching a storm, without each tick hand-assembling shell exports.

2026-06-30 repeated-blocker evidence: the next scheduled tick bootstrapped `GH_TOKEN` again but still had no `OPENROUTER_API_KEY` after sourcing the operator `.env`, so it stopped before opening a new submission or dispatching paid storm lanes. Hold this exact-rev storm until either the Hermes cron profile receives the required secret through an approved path or backlog 086 ships a BB-owned recipe/preflight that can prove the secret contract before mutation.

2026-06-30 follow-up slice: the cron loop also had to query `plane/.bb/plane.db` directly because the shell-level `bb submit list --json` recipe was absent from the CLI even though the ledger already had a typed `list_submissions` shape. Add `bb submit list --json` as the smallest BB-owned discovery primitive before larger submit-and-storm recipes.
