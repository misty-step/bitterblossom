# Give the plane a durable home and real reflex ingress across repos

Priority: P1 · Status: shipped · Estimate: L

## Goal

The plane runs detached (not on the operator's laptop) and reflex work
fires from real events across a chosen subset of repos — the review
factory reviews PRs on the projects the operator picks, continuously,
with no tunnel ceremony.

## Oracle

- [x] `bb serve` runs on a durable host (Fly machine, dedicated sprite,
      or any always-on box — the host is config, not architecture) and
      survives operator-laptop shutdown; `/api/tasks` reachable
      (token-gated) from the laptop
- [x] A `pull_request` event from a real repo produces a posted review
      with no manually-started tunnel in the path (GitHub App ingress,
      or post-receive hook on a self-hosted bare remote — decided at
      shape time; see Notes)
- [x] The reviewed-repo subset is operator-editable in one place
      (App installation list, or per-repo hook config) and the plane's
      existing trigger filters enforce it fail-closed
- [x] Plane secrets (OPENROUTER_API_KEY, hook secrets, App key) live on
      the durable host, never in the repo; `bb check` passes there
- [x] Recovery survives a host restart: `bb recover` classifies
      inherited runs on boot (existing behavior, exercised on the new
      host)

## Notes

**Why:** the practical half of the Ona-arc research (2026-06-11) — every
shipped competitor (Ona, Codex cloud, Devin, Jules) is "background"
because the control plane doesn't live on a laptop. This is also the
named precondition for 039's reflex follow-up ("once the plane has a
durable home") and for rung 3 of the coordination ladder (self-hosted
bare remote + post-receive = forge-optional events; GitHub demoted to
refs + mirror). Shape decision deferred to /shape: GitHub App (works for
GitHub-resident repos, org-level webhook, installation tokens, bot
identity) vs self-hosted remote + post-receive (zero GitHub in the
critical path, four-line hook, but front-loads remote migration).
Likely answer is both, App first. Live evidence precedent: 2026-06-10
tunnel-based delivery on PR #844 proved the ingress path end to end.

## Evidence (2026-06-12)

- Shape chosen: durable `bb serve` host + per-repo GitHub webhook first.
  This keeps the plane workload-agnostic, reuses the existing signed
  webhook/dedupe/filter path, and leaves a future GitHub App as a hook
  provider swap instead of a spine change.
- Superseded Sprite attempt: `bb-plane-live` proved no-tunnel webhook ingress
  once, including review run `0bb68f1845cc`, but was not acceptable as the
  durable host because `*.sprites.app` timed out from the operator laptop and
  later review-worker transports failed with `harness exit 1: Error:
  connection closed`.
- Durable host: Fly app `bitterblossom-plane` in org `misty-step`, one
  always-on `dfw` machine, encrypted volume `bb_plane_data` mounted at
  `/app/plane/.bb`, public URL `https://bitterblossom-plane.fly.dev`.
- The Fly image installs `bb` plus Sprite CLI `v0.0.1-rc44`; the running
  container passes `sprite list -o misty-step` and `bb --config plane check`.
- Laptop read API evidence: `curl https://bitterblossom-plane.fly.dev/health`
  returned `{"pending":0,"running":0,"oldest_pending":null}`;
  unauthenticated `/api/tasks` returned `401`; bearer-token `/api/tasks`
  returned `200`.
- Host state evidence: `flyctl status --app bitterblossom-plane` shows one
  started machine; `flyctl volumes list --app bitterblossom-plane` shows
  encrypted volume `bb_plane_data` attached to it.
- Secrets: `BB_API_TOKEN`, `BB_HOOK_REVIEW`, `OPENROUTER_API_KEY`, `GH_TOKEN`,
  and `SPRITE_TOKEN` are Fly secrets; no secret values are stored in git.
- Recovery/boot path on the Fly host: `bb --config plane recover` returned
  `no inherited running runs`.
- GitHub hook delivery `3825289016337498000` for PR #845 head
  `a5aafd93a8c07f3498302d65100c0f197d4294dd` hit
  `https://bitterblossom-plane.fly.dev/hooks/review` and returned `202`.
- Fly ledger run `6c24c4fac8b8` was created from that delivery with
  idempotency key `wh:review:a5aafd93a8c07f3498302d65100c0f197d4294dd`;
  it ended `success` after one attempt, cost `$0.10219904`, 64,154 input
  tokens, 17,048 output tokens, and 11 turns.
- External effect: PR comment `4694205667` was posted by the review factory
  at `2026-06-12T18:39:45Z`, with an approve-leaning verdict and no
  blocking, serious, or minor issues.
