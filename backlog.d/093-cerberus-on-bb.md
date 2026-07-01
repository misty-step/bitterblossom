# Epic: Cerberus-on-BB advisory PR review workload

Priority: P1 | Status: ready | Estimate: XL

## Goal

Run Cerberus as a BB workload on non-draft pull request opens and pushes, with a
repo whitelist, advisory artifacts/comments, and no merge-blocking authority.

## Oracle

- [ ] `review` or a successor Cerberus task triggers on non-draft PR open and
      synchronize events for a reviewed repo whitelist.
- [ ] The task uses operator GitHub auth or a declared GitHub credential name and
      never persists token values.
- [ ] The run produces an artifact with the reviewed diff, reviewer receipt,
      findings, costs, model/team, and residual uncertainty.
- [ ] GitHub output is advisory: comment/check/artifact only; it never blocks or
      merges.
- [ ] Draft PRs, bot noise, oversized diffs, and non-whitelisted repos are
      rejected before paid execution with visible ingress evidence.
- [ ] At least three real PR events produce useful advisory output with no
      duplicate comments or storm fanout.
- [ ] `./scripts/verify.sh` passes.

## Children

- [ ] Align BB task/card with Cerberus current CLI and review dimensions.
- [ ] Non-draft PR trigger, whitelist, bot, and size filters.
- [ ] Advisory GitHub posting contract and artifact receipt.
- [ ] Duplicate suppression across PR pushes.
- [ ] Live dogfood on Bitterblossom and two other factory repos.

## Notes

The operator decision is explicit: Cerberus is advisory for now. Blocking gates
wait for Crucible-measured consistency; BB's job is durable execution, cost,
receipt, and visibility.
