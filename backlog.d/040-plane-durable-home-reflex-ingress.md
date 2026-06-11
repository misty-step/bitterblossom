# Give the plane a durable home and real reflex ingress across repos

Priority: P1 · Status: pending · Estimate: L

## Goal

The plane runs detached (not on the operator's laptop) and reflex work
fires from real events across a chosen subset of repos — the review
factory reviews PRs on the projects the operator picks, continuously,
with no tunnel ceremony.

## Oracle

- [ ] `bb serve` runs on a durable host (Fly machine or dedicated
      sprite) and survives operator-laptop shutdown; `/api/tasks`
      reachable (token-gated) from the laptop
- [ ] A `pull_request` event from a real repo produces a posted review
      with no manually-started tunnel in the path (GitHub App ingress,
      or post-receive hook on a self-hosted bare remote — decided at
      shape time; see Notes)
- [ ] The reviewed-repo subset is operator-editable in one place
      (App installation list, or per-repo hook config) and the plane's
      existing trigger filters enforce it fail-closed
- [ ] Plane secrets (OPENROUTER_API_KEY, hook secrets, App key) live on
      the durable host, never in the repo; `bb check` passes there
- [ ] Recovery survives a host restart: `bb recover` classifies
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
