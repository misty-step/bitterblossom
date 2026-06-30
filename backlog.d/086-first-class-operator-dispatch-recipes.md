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
