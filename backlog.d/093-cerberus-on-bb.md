# Epic: Cerberus-on-BB advisory PR review workload

Priority: P1 | Status: live (hardening follow-ups open) | Estimate: XL

## Goal

Run Cerberus as a BB workload on non-draft pull request opens and pushes, with a
repo whitelist, advisory artifacts/comments, and no merge-blocking authority.

## Oracle

- [x] `review` triggers on non-draft PR open/synchronize for a reviewed repo
      whitelist — webhook route `review` (misty-step/bitterblossom, pre-existing)
      plus a new route `review-fleet` (misty-step/powder, misty-step/weave)
      added 2026-07-02, both live-verified against real PR events.
- [x] The task uses a declared GitHub credential name (`GH_TOKEN` via
      `--gh-token-env`) and never persists token values — fixed 2026-07-02
      (bb#936); the wrapper previously never forwarded the token at all, so
      100% of dispatches failed before Cerberus could run.
- [~] The run produces an artifact with the reviewed diff, reviewer receipt,
      findings, model/team, and residual uncertainty — confirmed present.
      **Gap:** `cost_usd`/token usage is not populating (`"Cost:" unknown` in
      every posted comment, `cost_usd: null` in the ledger) even on
      successful runs; worth a follow-up to wire the wrapper's usage
      aggregation through to what `review-pr` actually emits.
- [x] GitHub output is advisory: comment-only, verified across three real
      reviews — no check/gate/merge action taken.
- [ ] Draft PRs, bot noise, oversized diffs, and non-whitelisted repos are
      rejected before paid execution with visible ingress evidence — filters
      are in place (see task.toml) but not freshly re-verified this pass.
- [x] At least three real PR events produce useful advisory output with no
      duplicate comments or storm fanout — misty-step/bitterblossom#842
      (PASS), #936 (FAIL, real finding — see bb#937), misty-step/powder#31
      (PASS). One near-miss: unparking the task briefly re-queued a 59-deep
      historical backlog before it was caught and retired — see backlog 102.
- [x] `./scripts/verify.sh` passes.

## Children

- [x] Align BB task/card with Cerberus current CLI and review dimensions.
- [x] Non-draft PR trigger, whitelist, bot, and size filters.
- [x] Advisory GitHub posting contract and artifact receipt.
- [ ] Duplicate suppression across PR pushes — dedupe_key is wired
      (PR URL + head SHA) but not exercised by a real push-after-open event
      this pass.
- [~] Live dogfood on Bitterblossom and two other factory repos —
      bitterblossom and powder proven live 2026-07-02; weave has the
      `review-fleet` webhook wired (verified `ping` delivery) but no real PR
      exercised it yet.

## Follow-ups filed 2026-07-02

- Backlog 102: bulk `task unpark` re-queues the whole historical
  blocked-budget backlog, not just the run that tripped the cap — real
  incident this pass, mild blast radius but a real footgun for a higher-
  authority task.
- Backlog 103: reviews post as the operator's personal GitHub identity
  (`phrazzld`), not a bot/app identity.
- Backlog 104: adopt Cerberus's M1 scoped-key / M2 container-isolation
  flags in the wrapper before pointing this at externally-contributed PRs.

## Notes

The operator decision is explicit: Cerberus is advisory for now. Blocking gates
wait for Crucible-measured consistency; BB's job is durable execution, cost,
receipt, and visibility.
