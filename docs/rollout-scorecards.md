# Rollout Scorecards And Promotion Gates

Date: 2026-07-02
Backlog: 084

The single reusable contract for shipping a Bitterblossom task family at less
than full authority — read-only, report-only, dry-run, or PR-only — and for
deciding, from evidence, whether to promote, hold, or roll back that authority.

Before 084 every low-authority task invented its own promotion section in its
own backlog ticket, in a slightly different shape (080 "Report-Only Graduation
Metrics", 081 "Promotion Gates By Authority Level", 082 "Dry-Run → PR-Only
Graduation Metrics"). This doc is the source of truth those tickets point at so
the shape stops drifting. A new autonomous task family fills the template here
(or references it) rather than reinventing the fields.

## Why This Exists

The plane owns mechanism, not workload judgment. "How much authority does this
workload have, and what earns it more" is exactly the kind of governance
decision that must be explicit, evidenced, and operator-gated — never promoted
by inertia because a report-only task "seems to work." An autonomous task that
gains write authority from vibes instead of a green scorecard is the failure
this contract prevents.

The rule is simple and load-bearing: **product judgment stays in grooming.**
Bitterblossom consumes shaped, ready work and reports when work is not ready. It
does not decide, on its own, that it has earned more authority.

## Authority Ladder

Every autonomous task family sits at exactly one authority level. Levels are
ordered; you do not skip a level, and each promotion requires the prior level's
scorecard to be green.

| Level | The task may | The task must never |
|---|---|---|
| `read-only` | Read status, config, runs, gates, dead letters, artifacts over CLI/MCP/API. | Any mutation, dispatch, replay, resolve, park/unpark, submit, or credential provisioning. |
| `report-only` | Investigate and write a `REPORT.json` artifact with evidence, hypotheses, and recommended next commands. | Edit code, open branches/PRs, merge, deploy, park/unpark, resolve runs, acknowledge DLQs, post user-visible notifications. |
| `dry-run` | Select ready work and write a plan artifact naming the ticket, verifier, budget, branch name, expected paths, and stop conditions. | Create a branch, edit code, open a PR, merge, or deploy. |
| `PR-only` | Run the deliver/TDD/review workflow and open a reviewed PR. | Merge, deploy, or open more than one active BB-authored PR per repo/task family. |
| `guarded-land` | Merge only behind deterministic CI/gate, fresh-context review, repo allowlist, and an explicit operator policy gate. | Merge on any weaker evidence, or land outside the allowlist. |
| `rollback-own-change` | Revert only the agent's own last known change, and only after the same incident signature or a declared sanity check still fails. | Revert unrelated code, or continue after a no-progress halt. |

081's level names (observe/recommend → branch/PR → guarded land → rollback) map
onto this ladder: `observe/recommend` is `read-only`+`report-only`, `branch/PR`
is `PR-only`, and the remaining two are identical.

## The Scorecard Template

Each autonomous task family carries a compact scorecard. Copy this block into
the task family's backlog ticket (or reference this doc from it) and keep it
current:

```text
Task family:
Current authority:      read-only | report-only | dry-run | PR-only | guarded-land | rollback-own-change
Allowed actions:
Forbidden actions:
Evidence metrics:       what is counted, and the target (e.g. "N useful reports, 0 dangerous")
Promotion trigger:      the measured condition that makes the NEXT level eligible for operator approval
Rollback / hold trigger: the measured condition that demotes or freezes this level
Budget / cost cap:      per-run cost cap and daily run cap
Duplicate-suppression key: the idempotency/dedupe key that prevents storm fanout
Required artifacts:     the artifact handles that carry the evidence (e.g. REPORT.json)
Operator approval needed for next level: yes  (always yes — see Doctrine)
```

## Doctrine

These rules are non-negotiable and apply to every task family:

- **No automatic promotion.** Green metrics only make the next-level backlog
  ticket *eligible* for explicit operator approval. Metrics never flip authority
  by themselves.
- **A promotion ticket cannot be marked ready without evidence.** The higher
  authority ticket must cite the lower-authority mode's evidence packet — run
  ids, artifact handles, gate/review receipts, and cost. A promotion ticket with
  "later add autonomy" language and no cited evidence is not ready.
- **Agents refuse autonomy expansion from vibes.** An agent operating a task
  family must refuse to recommend or take a higher authority action unless the
  scorecard for that level is green *and* an operator has approved the
  promotion. "It has been working" is not evidence; the scorecard is.
- **Merge, unpark, production mutation, and broad rollout stay operator
  authority** until a scorecard plus an explicit operator decision says
  otherwise, consistent with the VISION "no hidden authority escalation" refusal.
- **Failures halt; they do not plow forward.** A level that hits its
  rollback/hold trigger, a no-progress signal, or a failed sanity check stops
  and reports rather than continuing.

## Where Authority Is Visible Today

Authority currently lives in three places, all product-generic:

- **The task config** (`tasks/<name>/task.toml`) may declare `[rollout]`
  `authority` plus `scorecard`. This is read-only visibility metadata, not an
  authority grant or promotion switch. `bb status --json`, `GET /api/status`,
  `bb task list --json`, `/api/tasks`, `bb check --json`, and MCP
  `bb_status`/`bb_tasks` expose it.
- **The task card** (`tasks/<name>/card.md`) states the allowed and forbidden
  actions in prose the agent reads.
- **The run artifact** records the enforced posture for the attempt. Dry-run
  and report-only reports carry it structurally, e.g. the backlog-chewer
  dry-run report's `authority` object (`current`, `no_side_effects`,
  `forbidden_actions`) and canary triage's non-mutation constraints.

`[rollout]` closes the status visibility gap. It intentionally does not decide
whether metrics are green; that remains grooming/operator judgment backed by
the scorecard evidence packet.

## Current Task Family Scorecards

These are the shipped low-authority task families and their scorecards. Each
task family's backlog ticket holds the authoritative, evolving copy; this table
is the fleet-wide index.

### canary-triage (report-only) — backlog 080

```text
Task family:            canary-triage
Current authority:      report-only
Allowed actions:        investigate incident, write REPORT.json with evidence, hypotheses, likely owner files/services, recommended next bb commands, residual uncertainty
Forbidden actions:      code edits, branches, PRs, merges, deploys, remediation claims, incident annotation/ack/resolution, park/unpark, run resolution, user-visible notifications
Evidence metrics:       >=5 replayed fixture incidents + >=3 real low-severity incidents produce useful reports; >=80% reviewer-actionable; 0 dangerously wrong; dedupe holds; spend within cap
Promotion trigger:      scorecard green makes backlog 081 level 2 (branch/PR) eligible for operator approval
Rollback / hold trigger: any dangerously-wrong report, dedupe storm, or spend over cap
Budget / cost cap:      per-run and daily caps from the task's budget block
Duplicate-suppression key: incident fingerprint within the dedupe window
Required artifacts:     REPORT.json
Operator approval needed for next level: yes
```

### canary-remediate (PR-only) — backlog 115

The next authority step after `canary-triage` proves report-only investigation
is trustworthy. A separate task and agent from `canary-triage` (never the
same authority level doing both jobs) that consumes an existing, `actionable`
`canary-triage` report and opens one bounded fix PR — it never investigates
an incident from scratch. Manual-dispatch only at this level: no webhook
trigger is wired in `task.toml`, and `canary-remediator`'s own
`policy.trigger_bindings` only declares `manual`. Neither is a mechanical
guarantee the plane enforces cross-field today — `bb check` does not verify
a task's declared triggers against its agent's `trigger_bindings`, so this is
a config convention this task family follows, not a load-time gate (matching
`canary-triage`'s own precedent: card-level authority is prose the agent
follows, same as "never mutate Canary" there). Scoped to an explicit repo
allowlist declared in the task's `workspace.repos` (currently one repo) —
BB itself only provisions/checks out that one repo into the workspace at
dispatch time, but the sprite harness still runs in an unrestricted remote
shell with `GH_TOKEN` exported as an env var; the real boundary on what the
agent can *reach* is whatever repos that token has access to. Keeping
`GH_TOKEN` narrowly scoped to the allowlisted repo(s) is exactly why
bitterblossom-925's bot-identity provisioning is a hard prerequisite before
this task's first live dispatch, not an optional hardening step.

```text
Task family:            canary-remediate
Current authority:      PR-only
Allowed actions:        read prior canary-triage REPORT.json, clone/checkout only the allowlisted repo(s), create one branch, make a minimal targeted fix, open exactly one pull request, write REPORT.json describing the PR
Forbidden actions:      merges, deploys, incident annotation/ack/resolution, park/unpark, run resolution, a second active PR for the same incident fingerprint, touching any repo outside the allowlist, investigating from scratch
Evidence metrics:       every PR traces to a prior actionable canary-triage report; 0 merges; 0 deploys; 0 incident mutations; 0 repos touched outside the allowlist; at most one active PR per incident fingerprint
Promotion trigger:      scorecard green + operator approval makes guarded-land (merge behind CI/gate/review/allowlist) eligible for this repo family
Rollback / hold trigger: any merge/deploy/incident-mutation attempt, any PR against a non-allowlisted repo, or a duplicate-PR storm
Budget / cost cap:      per-run and daily caps from the task's budget block (3 runs/day, $1.25/run at this authority level)
Duplicate-suppression key: agent-verified only, not plane-enforced -- the card instructs checking for an existing open PR against the allowlisted repo before branching; unlike canary-triage's ledger-level idempotency key (dedupe by delivery id at ingress), there is no BB-mechanism backing this today because no webhook trigger exists at this authority level to key off
Required artifacts:     REPORT.json
Bot identity / token provisioning: canary-remediator declares GH_TOKEN like canary-triager; per bitterblossom-925, provisioning a dedicated bot/app identity scoped to the allowlisted repo(s) (not the operator's personal token, and not a token with broader reach than the allowlist) is an operator-gated prerequisite before this task's first live dispatch, same as canary-triage's
Token rotation:         follows whatever rotation policy bitterblossom-925 establishes for the shared bot identity across canary-triager/canary-remediator
Rollback / stop conditions: any single forbidden action (merge, deploy, incident mutation, out-of-allowlist repo touch) is an immediate hold — revert this task to report-only-equivalent (no dispatch) until root-caused
Operator approval needed for next level: yes
```

### backlog-chewer-dry-run (dry-run) — backlog 082

```text
Task family:            backlog-chewer-dry-run
Current authority:      dry-run
Allowed actions:        scan whitelisted repos, select only ready tickets, shape vague tickets, write a plan artifact naming ticket/verifier/budget/branch/expected paths/stop conditions
Forbidden actions:      branch, pr, merge, deploy, code_edit (report authority object forbids all five)
Evidence metrics:       >=20 dry-run selections; >=90% judged genuinely ready; 0 dangerous/blocked tickets selected; vague tickets shaped not coded; every plan names verifier/acceptance/budget/stop/branch/paths
Promotion trigger:      per-repo dry-run scorecard green makes PR-only eligible for that repo family + operator approval
Rollback / hold trigger: any dangerous/blocked ticket selected, or a plan missing required fields
Budget / cost cap:      low dry-run per-run cap; daily run cap from the task budget block
Duplicate-suppression key: max one active BB-authored PR per repo/task family (enforced before PR-only)
Required artifacts:     REPORT.json (plan artifact)
Operator approval needed for next level: yes
```

### fix-prompt-generator (report-only) — backlog 062

```text
Task family:            fix-prompt-generator
Current authority:      report-only
Allowed actions:        read a signed gate.blocked event, write a bounded builder packet naming every blocking fingerprint/file/line/claim/evidence and a suggested next run
Forbidden actions:      editing code, resolving runs, parking tasks, merging, or any action fan-out beyond the report
Evidence metrics:       every blocking fingerprint/file/line/claim/evidence survives into the report and builder packet; no mutation authority in config/card
Promotion trigger:      none defined; this reflex is designed to stay report-only (a builder packet is consumed by a separate dispatch)
Rollback / hold trigger: any report that drops a blocking fingerprint or asserts a mutation
Budget / cost cap:      per-run cap from the task budget block
Duplicate-suppression key: gate.blocked event dedupe on /hooks/fix-prompt
Required artifacts:     REPORT.json (builder packet)
Operator approval needed for next level: yes
```

### artifact / MCP read surfaces (read-only) — backlog 078, 079

```text
Task family:            artifact and MCP read surfaces
Current authority:      read-only
Allowed actions:        CLI/MCP/API reads of status, config, runs, gates, dead letters, artifacts (bb_status, bb_runs_list, bb_artifacts_list, bb_artifact_read, ...)
Forbidden actions:      any tools/call mutation, dispatch, replay, resolve, park/unpark, submit, merge, or credential provisioning over MCP
Evidence metrics:       concrete usage demand before any output/publication surface grows (e.g. artifact bundle/export, backlog 101, waits for demand)
Promotion trigger:      demonstrated read usage justifies a deliberate export surface, gated by its own ticket
Rollback / hold trigger: any mutating MCP tool request must be rejected (JSON-RPC -32602)
Budget / cost cap:      n/a (read-only)
Duplicate-suppression key: n/a
Required artifacts:     n/a (reads existing artifacts)
Operator approval needed for next level: yes
```

## Adding A New Autonomous Task Family

1. Pick the lowest authority level that produces useful evidence — usually
   `read-only` or `report-only`.
2. Fill the scorecard template in the task family's backlog ticket and add a row
   to the fleet index above.
3. Ship the task/card/agent so the card's forbidden-actions prose matches the
   scorecard, `[rollout]` points to that scorecard, and the report artifact
   carries the enforced posture.
4. Do not write a promotion ticket until the current level's evidence metrics
   are green and cited.
