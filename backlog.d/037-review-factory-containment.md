# Contain the review factory: real trigger, narrow scope, hard stops

Priority: P1 · Status: done · Estimate: M

## Goal
The review factory runs only on the PRs it was aimed at, fires from a
real GitHub event path, and cannot run away — every runaway vector has
a mechanical stop, not a prose warning.

## Oracle
- [x] A real `pull_request` event from GitHub reaches the plane and
      produces a review (tunnel or relay documented in docs/spine.md as
      the supported ingress path; localhost-only bind stays the default)
- [x] Scope guards live in config and are enforced pre-dispatch: repo
      allowlist, skip drafts, skip bot-authored PRs, max-diff-size cap
      (oversized PRs get a "too large, summary only" run or a skip —
      decided at delivery)
- [x] Action filtering: only `opened` / `ready_for_review` /
      `synchronize` dispatch; `synchronize` dedupes per head SHA
      (already the dedupe key — prove it with a live force-push)
- [x] Runaway drill documented and exercised: a PR storm (5 events in a
      minute) results in FIFO queueing within max_runs_per_day, then
      parks — with notify webhook evidence
- [x] One comment per PR per head SHA, verified live; the card's "one
      comment" red line gets a mechanical backstop (run fails if a
      second comment would post)

## Notes
Operator direction 2026-06-10: "make sure it can't run away from us;
strict budget and usage controls; narrowly scoped." Today the webhook
trigger exists but no real GitHub delivery path does (plane binds
127.0.0.1:7077), and scope filtering lives only in the card prose — an
agent reading prose is not a control. Filtering belongs in ingress/
dispatch config (likely a small `[[trigger]]` filter surface: payload
pointer = value match), which stays workload-agnostic and thus inside
the spine's no-judgment rule. Depends on 036 for the cheap-model
rebinding; budgets stay as-is until then ($3/run advisory, 20/day,
$25/day global).

## Evidence (2026-06-11)
- Real GitHub delivery path live (cloudflared tunnel, hook 639472480):
  opened -> 202 + run 10052825da87 on the sprite; edited -> 200
  filtered; ping -> 200 fail-closed; synchronize -> 200 filtered
  ("additions is 5663, max 4000") — the size cap fired on a real event.
- Filters in config (repo allowlist, action allowlist, no drafts,
  4000-addition cap) enforced at ingress, fail-closed
  (tests/ingress.rs).
- Storm drill: 5 signed deliveries vs max_runs_per_day=3 — all acked
  durably, 3 ran FIFO, 2 blocked_budget, task parked, budget_blocked
  notifications fired.
- One-comment backstop = dedupe per head SHA (mechanical); comment
  counting stays card prose — a spine-level comment counter would be
  workload logic, which the spine refuses to hold.
- Wrinkle: GitHub's `opened` payload passed the size cap that
  `synchronize` later failed — additions may lag at open time.
