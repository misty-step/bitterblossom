# Powder chewer local-drill card (stub variant)

This is the DEV DRILL twin of `plane/tasks/powder-chew/card.md` (the live
task). It exists to prove the pull-model shape end to end on a laptop
before the live cron task ships: real Powder reads, fake execution.

This card is never actually read by a model -- the bound agent's harness is
`command` (`powder-chew-dev-stub.sh`), a deterministic script, not an LLM.
It is kept here anyway so the task directory shape matches every other
Bitterblossom task (`card.md` + `task.toml` pair) and so a future promotion
of this drill to a real stub-model variant has somewhere to start from.

## What the stub actually does

1. Calls the real `powder list-ready --limit 20` (read-only) using the real
   `POWDER_API_BASE_URL`/`POWDER_API_KEY` resolved from the operator's own
   environment.
2. Applies the same repo-allowlist prefix pre-filter (`sploot`) the
   production selection oracle's step 2 uses.
3. Writes `REPORT.json` naming what it *would* select, plus a sample of the
   raw ready queue for evidence.
4. Stops. It never calls `claim`, never checks out a repo, never opens a
   shell beyond the one Powder call.

## Boundaries

A refused credential is a boundary, not a puzzle: on HTTP 401/403 (or any
authorization refusal) from a credential this run declares, STOP-and-report —
write `REPORT.json` naming the refused operation and the refused credential by
name (never its value), then stop without completing the goal. Never locate or
use a stronger credential (env, keychain, 1Password, config, another agent).

- Never claims a real card.
- Never touches any repo.
- Never merges, pushes, or opens a PR.
- Safe to run repeatedly against the live Powder board.
