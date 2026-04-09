---
name: canary-responder
description: "Handle one Canary incident end to end: claim, investigate, fix, review, verify, and annotate."
---

# /canary-responder

Own one Canary incident all the way through.

## Execution Stance

You are the executive orchestrator.

- Keep incident judgment, repo selection, merge/deploy decisions, and recovery
  judgment on the lead model.
- Delegate bounded evidence gathering, implementation, and review to subagents.
- Treat Canary reads as truth and annotations as the visible audit trail.

## Inputs

Required environment:

- `CANARY_ENDPOINT`
- `CANARY_API_KEY`

Required repo artifacts:

- `canary-services.toml`
- working `bitterblossom canary service <service> --json`
- working `bitterblossom canary incidents|report|timeline|annotations|annotate`

## Protocol

### 1. Claim

Pick one active incident that does not already carry a live
`bitterblossom.claimed` annotation.

Prefer:

```bash
cd conductor
mix conductor canary incidents --without-annotation bitterblossom.claimed --json
```

Write a claim annotation immediately with:

- `agent = "tansy"`
- `action = "bitterblossom.claimed"`
- metadata: `run_id`, `service`, `claimed_at`, `sprite`

Then write `bitterblossom.investigating` before any repo mutation.

### 2. Gather Truth

Read the incident from Canary, then pull bounded context:

- incident list/detail via `mix conductor canary incidents --json`
- report for the service via `mix conductor canary report --window 24h --json`
- timeline for the service via `mix conductor canary timeline --service <service> --window 24h --limit 200 --json`
- focused error detail and target-check history when relevant

Webhooks are never enough by themselves. Reconcile from read APIs before acting.

### 3. Resolve the Repo

Resolve the service through the catalog:

```bash
cd conductor
mix conductor canary service <service> --json
```

That result is the only allowed authority for:

- target repo
- target clone URL
- default branch
- test command
- deploy command
- rollback command
- auto-merge / auto-deploy policy
- stabilization window

### 4. Investigate

Run `/investigate` first.

You are looking for:

- reproduction
- root cause
- strategic fix options
- the smallest robust fix, not the fastest patch

When the design is not obvious or the blast radius is high, run:

```text
/research thinktank <focused question>
```

### 5. Repair

Work in the resolved target repo, not in Bitterblossom.

- write or update the narrowest regression test first when practical
- implement the chosen fix
- run the catalog `test_cmd`

If the repo is not already present, clone it into the standard sprite workspace
root under `/home/sprite/workspace/<owner>/<repo>`.

### 6. Review

Run `/code-review` on the actual diff. Thinktank review is mandatory.

If blocking findings appear, fix them and re-run review. Do not merge on a
conditional or stale verdict.

### 7. Merge and Deploy

Only proceed when the catalog explicitly allows it.

- `auto_merge = false` means stop at reviewed, verified, merge-ready state and
  annotate `bitterblossom.escalated`.
- `auto_deploy = false` means do not run deployment, even if the repo merged.

When allowed:

1. squash merge
2. run the catalog deploy command
3. if deploy fails or worsens the incident, run the rollback command and
   annotate `bitterblossom.rollback`

### 8. Recovery Verification

Wait through the catalog stabilization window and re-check Canary.

Only mark `bitterblossom.resolved` when:

- the incident is resolved or materially recovering in Canary
- no fresh contradictory timeline evidence appears during the window

Otherwise annotate `bitterblossom.escalated` with the reason.

## Anti-Patterns

- Guessing the repo from the service name
- Treating webhooks as truth
- Skipping investigation and jumping straight to `/autopilot`
- Auto-merging because the patch “looks small”
- Declaring success before Canary stabilizes
- Editing unrelated repos while the incident target is unresolved
