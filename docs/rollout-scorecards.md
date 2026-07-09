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
`GH_TOKEN` narrowly scoped to the allowlisted repo(s) was the original
concern bitterblossom-925 opened to address. **Correction (2026-07-05,
recorded here during bitterblossom-122's evidence closeout): the operator
ruled that path dead.** A dedicated bot/app identity requires web-UI actions
(GitHub App manifest flow, org access-control grants) the operator declined
to perform ("if that's going to require me, then let's not"), so
bitterblossom-925 was rescoped to the fully agent-driven path instead:
`GH_TOKEN` is a `required` secret where a PR genuinely cannot be opened
without it (this task), and an `optional_secret` where it is merely
read-only context (`canary-triage`) that can degrade gracefully without it.
There is no bot-identity prerequisite left to satisfy. The remaining gate
before this task's first live dispatch against a real repo is the ordinary
Authority Ladder rule every level carries: explicit operator approval,
naming which repo and which token (today, that means the operator's own
`GH_TOKEN`, scoped as narrowly as the operator chooses at dispatch time) --
not a missing mechanism.

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
Bot identity / token provisioning: SUPERSEDED 2026-07-05 -- bot/app identity provisioning is permanently out of scope per operator ruling (bitterblossom-925 comment log); canary-remediator declares GH_TOKEN as a required secret (a PR cannot open without it), the operator's own token, scoped as narrowly as the operator chooses at dispatch time
Token rotation:         follows whatever rotation policy the operator sets for their own token; no shared bot identity exists or is planned
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

### powder-chew (dry-run, shipping) — bitterblossom-959

The pull-model counterpart to the push-model `dispatch-ready-builder`
(`examples/powder-ready-plane`): instead of a Powder webhook dispatching the
plane, `powder-chew` polls `powder list-ready` itself on a cron tick and acts
on one eligible card. Scoped to an explicit repo allowlist (currently `sploot`
only) declared in both `card.md` prose and `task.toml`.

**Ships tonight at `dry-run`, not `PR-only` (operator ruling 2026-07-09).**
The operator has not yet ruled on sploot branch protection or token scoping,
and will not grant an unattended agent push authority on a repo whose default
branch (`master`) accepts direct pushes (its protection is
`required_status_checks` + `enforce_admins` only -- no "require a pull
request" rule -- so a repo-scoped `GH_TOKEN` could push straight to it;
status checks gate merges, not pushes). So the shipping variant:

- polls `powder list-ready`, selects ONE eligible card by the same oracle
  `backlog-chewer-dry-run` uses (ready + unclaimed + concrete acceptance +
  S/M-sized + no destructive ask), and writes a full execution PLAN to
  `REPORT.json` (acceptance checklist, gate command, proposed branch name,
  expected changed paths, ordered implementation steps, best-effort proposed
  diff, stop conditions);
- **claims nothing, pushes nothing, opens no PR, mutates no Powder card,
  touches no git remote.** `[rollout] authority = "dry-run"`;
- carries **no `GH_TOKEN` in the agent's `secrets` list at all** -- the
  no-push boundary is a capability control, not a prompt: an agent that
  cannot authenticate to GitHub physically cannot push to `master`.

The `PR-only` variant (which claims the card, works it against the target
repo's gate, and opens one PR) is preserved verbatim in the repo, inert,
ready to promote once branch protection and token scoping land:
`plane/agents/powder-chewer-pr-only.toml.disabled`,
`plane/tasks/powder-chew/task.pr-only.toml.disabled`, and
`plane/tasks/powder-chew/card.pr-only.md.disabled` (bb's loaders read only
`agents/*.toml` and exactly `tasks/<name>/task.toml`, so the `.disabled`
siblings never load). Its scorecard block and the go-live hardening analysis
that produced its pre-push-hook belt are retained below as the promotion
target.

```text
Task family:            powder-chew
Current authority:      dry-run
Allowed actions:        poll Powder ready queue (allowlisted repos only, read-only), select one eligible card, write an execution plan to REPORT.json
Forbidden actions:      claim, any Powder mutation, branch, push, PR, merge, deploy, repo clone/edit, any git remote operation, planning more than one card per run
Evidence metrics:       every REPORT.json names exactly one selected card (or a clean no-op) with a full plan; 0 claims; 0 Powder mutations; 0 git operations; 0 repos touched; no-op runs produce a clean REPORT.json with zero side effects
Promotion trigger:      dry-run scorecard green across the cron cadence PLUS an operator ruling on sploot branch protection + token scoping makes PR-only (below) eligible
Rollback / hold trigger: any claim, Powder mutation, or git operation attempt (should be impossible -- no GH_TOKEN, read-only card use); or a plan naming more than one card
Budget / cost cap:      12 runs/day, $0.50/run cap from task.toml; provider_spend_cap_usd 5.0 as a belt-and-suspenders child-key ceiling (bb keys mint powder-chewer), not sized per-run
Duplicate-suppression key: n/a at dry-run (nothing is claimed or opened); becomes relevant only at PR-only (see below)
Required artifacts:     REPORT.json
Actor identity / token provisioning: a dedicated Powder API key (`bb-powder-chewer`, agent scope) minted via `powder key-create` against the deployed instance's own database, used READ-ONLY at this authority (bitterblossom-905 pre-check: the same key successfully drove a throwaway card through ready -> claimed -> running -> done -> abandoned end to end, 2026-07-09, proving it CAN drive lifecycle when promoted); NO GH_TOKEN at dry-run, by design
Rollback / stop conditions: any side effect at all is an immediate hold -- the agent has no write path, so a side effect would indicate a config regression to root-cause before re-enabling
Operator approval needed for next level: yes (plus the branch-protection ruling)
```

#### PR-only (promotion target, held) — bitterblossom-959

Promote to this only after the operator rules on branch protection and token
scoping. Reuses the already-proven push-model builder pattern
(`dispatch-ready-builder`, live since bitterblossom-931) unchanged except for
the trigger direction; Powder's own claim/status/work-log audit trail (one
claim, one PR link, one comment, one work-log entry per attempt) is the
evidence surface. The selection oracle is the same shape as the dry-run
variant, plus a duplicate-PR-pressure check.

Powder's `CardStatus` enum (`backlog`, `ready`, `claimed`, `running`,
`awaiting_input`, `blocked`, `done`, `shipped`, `abandoned` -- verified
against `powder-core::model::CardStatus` 2026-07-09) has no distinct "in
review" state. `running` plus a linked PR plus a comment is the mapped
in-flight-under-review state the card uses; `complete-card` is never called
by this task, since merge (and therefore real completion) is out of its
authority.

```text
Task family:            powder-chew
Current authority:      PR-only
Allowed actions:        poll Powder ready queue (allowlisted repos only), claim one eligible card, work it with the target repo's own gate, open exactly one pull request, link/comment/work-log the card, move status to running
Forbidden actions:      merges, force-push, deploy, repo-settings/secret/branch-protection edits, direct push to a repo's default branch, complete-card, claiming a second card in the same run, touching any repo outside the allowlist, grinding past two failed gate attempts on the same card
Evidence metrics:       every PR traces to exactly one claimed Powder card; 0 merges; 0 force-pushes; 0 repos touched outside the allowlist; at most one active claim/PR per card; no-op runs (no eligible card) produce a clean REPORT.json with zero side effects
Promotion trigger:      scorecard green across >=2 weeks of cron runs against sploot makes guarded-land (merge behind CI/gate/review) eligible for this repo family + operator approval
Rollback / hold trigger: any merge/force-push/settings-mutation attempt, any repo touched outside the allowlist, a second concurrently-claimed card, or grinding past the two-attempt gate-failure limit
Budget / cost cap:      12 runs/day, $1.00/run cap from task.toml; provider_spend_cap_usd 5.0 as a belt-and-suspenders child-key ceiling (bb keys mint powder-chewer), not sized per-run
Duplicate-suppression key: Powder's own claim (only one live claim per card) plus an agent-checked `gh pr list --search "<card-id> in:body" --state open` before claiming -- no BB-mechanism idempotency key today, since the trigger is a cron tick, not a webhook delivery with a natural dedupe id
Required artifacts:     REPORT.json
Actor identity / token provisioning: a dedicated Powder API key (`bb-powder-chewer`, agent scope) minted via `powder key-create` against the deployed instance's own database (bitterblossom-905 pre-check: an agent-scope key successfully drove a throwaway card through ready -> claimed -> running -> done -> abandoned end to end, 2026-07-09); GH_TOKEN is a required secret (a PR cannot open without it), the operator's own token scoped as narrowly as the operator chooses at dispatch time -- same superseded-bot-identity posture as canary-remediate
Rollback / stop conditions: any single forbidden action above is an immediate hold -- revert this task to manual-only (drop the cron trigger) until root-caused
Operator approval needed for next level: yes
```

**Go-live hardening pass (2026-07-09, fresh-context critic review before tonight's ship):**

- **PR-only was prompt-only, now has a mechanical belt.** Checked sploot's live
  GitHub branch protection on `master`: `required_status_checks` +
  `enforce_admins`, no "require a pull request before merging" rule -- status
  checks gate merges, not pushes, so a repo-scoped `GH_TOKEN` genuinely could
  push straight to `master`. `task.toml`'s `pre_command` now installs a
  client-side `pre-push` git hook in the same workspace clone the agent
  works in, refusing any push whose remote ref is `refs/heads/master` (exit
  1, no side effect); verified locally against both a refused `master` push
  and an allowed feature-branch push before shipping. `card.md` forbids
  `--no-verify`, `gh pr merge`, and enabling auto-merge explicitly. The
  GitHub-side "require a pull request" rule is still the stronger,
  server-side fix and remains the operator's call -- this hook is the belt
  behind that ask being open, not a replacement for it.
- **Claimed TTL is 3000s (50min), deliberately longer than the 45min
  wall-clock kill**, not equal to it. `side_effect_policy = "kill"` SIGKILLs
  an overrun run before its own `park-don't-grind` cleanup can execute, so a
  timed-out run always strands its claim. Verified against
  `powder-core::model::Card::is_ready_at` (2026-07-09): a `claimed`/`running`
  card's freshness is re-evaluated on every `list-ready` call using the
  claim's own `expires_at` -- once the TTL passes, the card reappears in
  `list-ready` automatically, no re-claim attempt or background sweep
  required. Residual risk: the card is invisible to `list-ready` (and to a
  human reading the board) for that bounded window (worst case ~5 minutes
  after a kill), not indefinitely.
- **cost_usd non-null, verified against the exact shipped harness+model.**
  `src/budget.rs`'s per-run/day cost ceilings only trigger when
  `cost_usd` parses as `Some` (a harness that reports no usage silently
  bypasses both the per-run cap and the daily ceiling). Ran one real
  dispatch of the `powder-chewer` agent (harness `pi`, model
  `deepseek/deepseek-v4-flash`) against the live `misty-step/lane-1` sprite
  substrate with a trivial, no-Powder-access card
  (`plane/tasks/powder-chewer-cost-smoke`, deleted after the smoke) and
  confirmed via `bb runs show <id> --json` that `cost_usd` is a real
  non-null number for this harness+model pair -- see the run id and value
  recorded in this task's implementation report.
- **Sprite substrate liveness, proven with a real dispatched run, not just a
  raw `sprite exec` ping.** The same smoke run above executed through bb's
  full lease/prepare/execute/collect/release pipeline against
  `misty-step/lane-1` and returned `state: "success"` -- this is the same
  host every currently-shipped task in this plane already uses
  (`canary-triage`, `ci-audit-dogfood`, `self-drill`), reducing (not
  eliminating) the bitterblossom-941 dead-sprite risk for the specific host
  this task depends on.
- **No repo-scoped daily cost ceiling exists in `bb` today.** `src/budget.rs`
  (103 lines total) enforces exactly two live spend controls: a per-task
  `max_runs_per_day` and the plane-global `[budget].max_cost_per_day_usd`
  (`ledger.cost_today()`, summed across every task in the plane). A
  `[[workload_repo]].budget_caps` field exists, but it only clamps what
  budget values a *repo-owned* `.bb/task.toml` is allowed to request at
  config-load time -- it is not a live per-repo spend tracker, and it only
  applies to the separate `[[workload_repo]]` declaration shape (repo-owned
  tasks), not a plane-owned task like this one. Building a real per-repo
  daily ceiling would be a `src/budget.rs` change (new ledger query scoped
  by repo, not by task) -- out of scope for this workload-as-config
  delivery; a Rust mechanism gap, not a config gap, so no config key was
  added that would silently do nothing. The practical mitigation available
  today: this task's allowlist is one repo (`sploot`), so its own
  `max_runs_per_day` (12) × `max_cost_per_run_usd` ($1.00) already bounds
  sploot's worst-case exposure from this task to $12/day -- a de facto,
  single-repo cap for as long as the allowlist stays at one repo. The
  moment a second repo is added to the allowlist, this stops being a
  meaningful per-repo bound (both repos would share the same task-level
  ceiling) and a real `src/budget.rs` per-repo ceiling becomes a legitimate,
  separately-shaped backlog card, not a config toggle to fake now.

### docs-sync (report-only) — backlog 120

Extends the `examples/docs-sync-plane` starter into a production-shaped
workload for the `document` skill: watches an explicitly allowlisted repo
(`workspace.repos`) on PR-merge (webhook, filtered to the allowlisted repo
and `refs/heads/main`, bot-sender exclusions, delivery-id dedupe) or a weekly
cron, and writes a structured drift report. This level never edits a file;
`docs-sync-pr` below is the separate PR-only task that acts on its reports.

```text
Task family:            docs-sync
Current authority:      report-only
Allowed actions:        inspect the changed repo/ref named in EVENT.json or the manual payload, identify docs/runbook/operator-contract drift, write REPORT.json naming source repo, trigger, changed files, stale docs targets, recommended patch, artifacts, cost, and residual risk
Forbidden actions:      edit files, push branches, open PRs, create issues, change labels, post comments
Evidence metrics:       useful, reviewer-actionable reports across the allowlisted repo(s); 0 mutations attempted; dedupe holds; spend within cap
Promotion trigger:      scorecard green makes docs-sync-pr (PR-only) eligible for operator approval on that repo family
Rollback / hold trigger: any report that claims a mutation happened, or a dedupe storm
Budget / cost cap:      12 runs/day, $0.50/run (examples/docs-sync-plane/tasks/docs-sync/task.toml budget block)
Duplicate-suppression key: webhook delivery id (header:X-GitHub-Delivery), ledger-enforced at ingress -- same mechanism class as canary-triage's incident-fingerprint dedupe
Required artifacts:     REPORT.json (schema bb.docs_sync.report.v2)
Operator approval needed for next level: yes
```

### docs-sync-pr (PR-only) — backlog 120

The PR-only companion to `docs-sync`, following `canary-remediate`'s
(backlog 115) exact precedent: a separate task and agent (never the same
authority level doing both jobs) that consumes an existing, actionable
`docs-sync` report and opens one bounded docs PR -- it never investigates
drift from scratch. Manual-dispatch only: no cron or webhook trigger is
wired in `task.toml`, and `docs-sync-writer`'s own `policy.trigger_bindings`
declares only `manual`. Scoped to a narrower repo allowlist than the
report-only watcher (one repo, not two) -- BB itself only provisions that
one repo into the workspace at dispatch time, but the sprite harness still
runs in an unrestricted remote shell with `GH_TOKEN` exported as an env var,
so the real reach boundary is whatever repos that token can access.
**Correction (2026-07-05, recorded during bitterblossom-122's evidence
closeout):** the original canary-remediate scorecard cited a bot-identity
prerequisite here; the operator has since ruled that path permanently out
of scope (web-UI-only provisioning, declined). There is no bot-identity
gate before this task's first live dispatch -- the remaining gate is the
ordinary Authority Ladder rule, explicit operator approval naming which
repo and which token.

```text
Task family:            docs-sync-pr
Current authority:      PR-only
Allowed actions:        read a prior docs-sync REPORT.json, clone/checkout only the allowlisted repo, create one branch, edit only the files that report's recommended_changes named, open exactly one pull request, write REPORT.json describing the PR
Forbidden actions:      merges, deploys, a second active PR for the same source report, editing any file outside the source report's recommended_changes, touching any repo outside the allowlist, investigating drift from scratch
Evidence metrics:       every PR traces to a prior actionable docs-sync report; 0 merges; 0 deploys; 0 files touched outside recommended_changes; 0 repos touched outside the allowlist; at most one active PR per source report
Promotion trigger:      scorecard green + operator approval makes guarded-land (merge behind CI/gate/review/allowlist) eligible for this repo family
Rollback / hold trigger: any merge/deploy attempt, any PR against a non-allowlisted repo, any file edited outside recommended_changes, or a duplicate-PR storm
Budget / cost cap:      3 runs/day, $0.75/run (examples/docs-sync-plane/tasks/docs-sync-pr/task.toml budget block)
Duplicate-suppression key: agent-verified only, not plane-enforced -- the card instructs checking for an existing open PR against the allowlisted repo before branching (gh pr list --repo <repo> --state open --search "docs-sync"); same as canary-remediate, because no webhook trigger exists at this authority level to key a ledger-level dedupe off
Required artifacts:     REPORT.json (schema bb.docs_sync_pr.report.v1)
Bot identity / token provisioning: SUPERSEDED 2026-07-05 -- bot/app identity provisioning is permanently out of scope per operator ruling (bitterblossom-925 comment log); docs-sync-writer declares GH_TOKEN as a required secret (a PR cannot open without it), the operator's own token, scoped as narrowly as the operator chooses at dispatch time
Rollback / stop conditions: any single forbidden action (merge, deploy, out-of-scope file edit, out-of-allowlist repo touch) is an immediate hold -- revert this task to report-only-equivalent (no dispatch) until root-caused
Operator approval needed for next level: yes
```

### ci-audit (report-only) — backlog 121

The proactive counterpart to the existing `ci-diagnose` reflex (public-plane
fixture): `ci-diagnose` reacts to one already-failed CI signal named in an
incoming webhook; `ci-audit` inspects an explicitly allowlisted repo's own
gates, tests, and lints on a daily cron or manual dispatch, whether or not
anything just failed, and writes a report-only hardening recommendation. A
manual dispatch payload must name one repo already in `workspace.repos`; a
payload naming any other repo is refused before the audit runs and no
`REPORT.json` is written for the refused call.

```text
Task family:            ci-audit
Current authority:      report-only
Allowed actions:        inspect one allowlisted repo's CI configuration (workflow files, test runners, lint configs), write REPORT.json naming current gates, missing/weak gates, proposed checks, risk, cost, and exact reproduction commands
Forbidden actions:      edit files, push branches, open PRs, weaken an existing gate, merge, deploy, post comments
Evidence metrics:       useful, reviewer-actionable reports across the allowlisted repo(s); 0 mutations attempted; every reproduction command actually reproduces the named evidence; spend within cap
Promotion trigger:      scorecard green makes ci-audit-pr (PR-only) eligible for operator approval on that repo family
Rollback / hold trigger: any report that claims a mutation happened, or proposes weakening an existing gate
Budget / cost cap:      4 runs/day, $0.50/run (examples/ci-audit-plane/tasks/ci-audit/task.toml budget block)
Duplicate-suppression key: n/a at this level -- no webhook trigger exists (proactive audit, not event-reactive); the daily cron and the task's max_runs_per_day cap bound worst-case fanout, same role bb's cron catch-up bound (backlog 083) plays elsewhere
Required artifacts:     REPORT.json (schema bb.ci_audit.report.v1)
Operator approval needed for next level: yes
```

### ci-audit-pr (PR-only) — backlog 121

The PR-only companion to `ci-audit`, following `canary-remediate`'s
(backlog 115) and `docs-sync-pr`'s (backlog 120) exact precedent: a separate
task and agent (never the same authority level doing both jobs) that
consumes an existing, actionable `ci-audit` report and opens one bounded
CI-hardening PR -- it never audits from scratch. Manual-dispatch only: no
cron or webhook trigger is wired in `task.toml`, and `ci-hardener`'s own
`policy.trigger_bindings` declares only `manual`. Scoped to a narrower repo
allowlist than the report-only auditor (one repo, not two). This task
family's one absolute red line, beyond the general PR-only ladder rules: it
must never weaken, loosen, skip, or remove an existing gate, test, or lint --
`gates_weakened` in its report must always be an empty array.
**Correction (2026-07-05, recorded during bitterblossom-122's evidence
closeout):** the docs-sync-pr scorecard this section modeled itself on
cited a bot-identity prerequisite; the operator has since ruled that path
permanently out of scope (web-UI-only provisioning, declined). There is no
bot-identity gate before this task's first live dispatch -- the remaining
gate is the ordinary Authority Ladder rule, explicit operator approval
naming which repo and which token.

```text
Task family:            ci-audit-pr
Current authority:      PR-only
Allowed actions:        read a prior ci-audit REPORT.json, clone/checkout only the allowlisted repo, create one branch, add or strengthen only the checks that report's proposed_checks named, open exactly one pull request, write REPORT.json describing the PR
Forbidden actions:      merges, deploys, weakening/loosening/skipping/removing any existing gate/test/lint, a second active PR for the same source report, editing any file outside the hardening, touching any repo outside the allowlist, auditing from scratch
Evidence metrics:       every PR traces to a prior actionable ci-audit report; 0 merges; 0 deploys; 0 gates weakened (gates_weakened always empty); 0 files touched outside the hardening; 0 repos touched outside the allowlist; at most one active PR per source report
Promotion trigger:      scorecard green + operator approval makes guarded-land (merge behind CI/gate/review/allowlist) eligible for this repo family
Rollback / hold trigger: any merge/deploy attempt, any gate weakened, any PR against a non-allowlisted repo, or a duplicate-PR storm
Budget / cost cap:      3 runs/day, $0.75/run (examples/ci-audit-plane/tasks/ci-audit-pr/task.toml budget block)
Duplicate-suppression key: agent-verified only, not plane-enforced -- the card instructs checking for an existing open PR against the allowlisted repo before branching (gh pr list --repo <repo> --state open --search "ci-audit"); same as canary-remediate and docs-sync-pr, because no webhook trigger exists at this authority level to key a ledger-level dedupe off
Required artifacts:     REPORT.json (schema bb.ci_audit_pr.report.v1)
Bot identity / token provisioning: SUPERSEDED 2026-07-05 -- bot/app identity provisioning is permanently out of scope per operator ruling (bitterblossom-925 comment log); ci-hardener declares GH_TOKEN as a required secret (a PR cannot open without it), the operator's own token, scoped as narrowly as the operator chooses at dispatch time
Rollback / stop conditions: any single forbidden action (merge, deploy, gate weakening, out-of-scope file edit, out-of-allowlist repo touch) is an immediate hold -- revert this task to report-only-equivalent (no dispatch) until root-caused
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
