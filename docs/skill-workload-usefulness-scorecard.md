# Skill Workload Usefulness Scorecard

Date: 2026-07-05, updated 2026-07-06 with five live dispatch attempts
Backlog: 122 (closes the docs-sync/CI-auditor epic opened by 120/121)

Tracks whether the two skill-backed workloads this epic shipped --
`docs-sync`/`docs-sync-pr` (backlog 120, the `document` skill) and
`ci-audit`/`ci-audit-pr` (backlog 121, the `ci` skill) -- actually produce
useful output, at what cost, on what model, and with what accepted/rejected
disposition. This is the evidence ledger `docs/rollout-scorecards.md`'s
promotion doctrine requires before either PR-only companion is considered
for the next authority level.

## Status: mechanism complete, one-week operational proof outstanding

This card's full acceptance -- "within one week of enabling docs-sync/
CI-auditor flows, at least two repos receive useful docs-sync PRs and at
least one repo receives a useful CI-audit PR or report with accepted next
action" -- is an **operational** milestone that requires real elapsed time
against real target repos, not something a single session can manufacture
honestly. What this card actually delivers today:

- This scorecard mechanism: the table below, ready to be filled in as real
  dispatches land.
- The model-choice evidence boundary as an enforced, checked policy (see
  below), not a punt.
- **Five real, live report-only dispatch attempts** against a real repo
  (`misty-step/bitterblossom`, self-audit) on a real Fly Sprite
  (`misty-step/bb-plane`), with real OpenRouter cost -- proving the full
  pipeline end to end (ingest, sprite acquisition, live `pi`+deepseek
  execution, cost accounting, safety-cap enforcement, daily rate-limiting),
  not a fabricated entry. **No attempt produced a completed `REPORT.json`
  within this session** -- see the table and "What This Proved" below for
  exactly why, honestly, rather than papering over it.
- Every genuine blocker named explicitly, with the exact next action to
  close it (see "What's Actually Left" below).

**What is NOT claimed:** two repos receiving useful docs-sync PRs, or a
CI-audit PR/report with a recorded *accepted* outcome from a reviewer other
than the agent itself, or even one *completed* `REPORT.json`. Those require
operator-selected target repos and, for the PR-only paths, real elapsed
dispatch history -- neither of which this session can produce truthfully in
one sitting. The five dispatch attempts below are proof the mechanism is
real and live, not proof the workload has yet produced accepted output.

## Model Choice Evidence Boundary

All four new/extended agents (`docs-watcher`, `docs-sync-writer`,
`ci-auditor`, `ci-hardener`) use `deepseek/deepseek-v4-flash` -- the
established fleet default (14 of 26 configured agent models across every
`examples/*/agents/*.toml` and `tests/fixtures/*/agents/*.toml` in this repo
use it; no other single model comes close). This is BYOK/default: the
operator's own OpenRouter key, the cheapest model already trusted for
comparable report-only/PR-only work elsewhere in the fleet (`canary-triager`,
`self-drill-runner`, `branch-pruner` all use it too).

**No model promotion is proposed or needed here.** Per this repo's existing
`docs/model-evals/README.md` convention (predates this epic, already
enforced by `scripts/verify.sh`'s model-catalog fixture check): "Promote a
new default only after the result is backed by receipts and the repo gate
still passes." Any future change away from `deepseek/deepseek-v4-flash` for
these four agents needs either a `bb run model-eval` receipt under
`docs/model-evals/` (the bitterblossom-native mechanism already shipped) or
a Crucible-recorded benchmark run (the fleet-wide dedicated eval app) --
whichever exists first -- cited by run id/report path in this file before
the change lands. Until then, this scorecard records BYOK/default as the
current, unchanged, and correct choice.

The five live dispatches above are consistent with this: all five ran
`deepseek/deepseek-v4-flash` via a `bb keys mint`-scoped OpenRouter key (not
the operator's bare `OPENROUTER_API_KEY`), matching the shipped
`agents/ci-auditor.toml` policy exactly. No model swap was needed or tried.

## Correction Folded Into This Closeout

While assembling this scorecard, found that `docs/rollout-scorecards.md`'s
`canary-remediate`, `docs-sync-pr`, and `ci-audit-pr` sections all cited a
"dedicated bot/app identity" as a hard prerequisite before PR-only first
dispatch. The operator has since ruled that path permanently out of scope
(`bitterblossom-925` comment log, 2026-07-05: GitHub App provisioning
requires web-UI actions the operator declined -- "if that's going to require
me, then let's not"). Corrected all three scorecard sections plus both new
plane READMEs in this same commit rather than leave stale doctrine standing;
see the diff for the exact corrected language. The practical effect: PR-only
first dispatch for `docs-sync-pr`/`ci-audit-pr` is no longer blocked on an
unbuildable prerequisite -- it needs only the ordinary Authority Ladder rule
every level already carries, explicit operator approval naming a repo and a
token.

## Scorecard

All five rows are one continuous evidence-gathering session (2026-07-06)
against a throwaway plane copy (`examples/ci-audit-plane`, not committed,
built at `/private/tmp/bb-ci-audit-dogfood`) with `workspace.repos` and the
manual payload repointed at the real `misty-step/bitterblossom` repo, on the
real `misty-step/bb-plane` Fly Sprite, using a `bb keys mint ci-auditor`
scoped OpenRouter key (spend cap $0.50, revoked after use;
`limit_remaining_usd: 0.49284167` at revocation, i.e. real usage
$0.00715833 across all five attempts). GH auth was the operator's own
`gh auth token` (no bot identity exists; see the correction above).

| Run id | Attempt | Trigger | Cost | Duration | Outcome | What it proved |
|---|---|---|---|---|---|---|
| `425311345d1b` | 1 | manual | $0 (never reached OpenRouter) | 5.8s | `failure`: `harness exit 127: sh: 1: pi not found` | Real infra gap, not a plane bug: the `bb-plane` sprite had no `pi` CLI on `PATH`. Fixed live: `npm install -g @earendil-works/pi-coding-agent` on the sprite, symlinked into `/.sprite/bin` (already on `PATH`). |
| `16fa4dfc4073` | 2 | manual | $0.0048 | 72.3s | `failure`: `output_bytes_cap kill: observed 24195 > cap 24000` | The dispatch/sprite/harness/cost-accounting pipeline works end to end for real. The shipped template's `output_bytes_cap = 24000` (tuned for the toy `example-org/*` repos) is too tight for a genuine audit of a repo bitterblossom's own size. |
| `64f2f30e3027` | 3 | manual | $0.0028 | 72.5s | `failure`: agent refused per its own `card.md`, `REPORT.json` not written | Confirms empirically what the docs already claimed: the `workspace.repos` allowlist is enforced by the agent reading `card.md` prose, not by any BB-mechanism check on the payload -- I had updated `task.toml`'s allowlist but not `card.md`'s, and the agent correctly refused the mismatch rather than silently proceeding. |
| `ede0e192bcb1` | 4 (cap doubled to 48000) | manual | $0.0278 | 256.2s | `failure`: `output_bytes_cap kill: observed 55287 > cap 48000` | A real, deeper audit pass (256s of live investigation) still overran a doubled cap. `output_bytes_cap` tuning for "audit a repo of your own complexity" needs materially more headroom than the shipped default, not just 2x. |
| `f0cadca6b962` | 5 (cap raised to 96000) | manual | n/a | n/a | `blocked_budget`: `4 runs today >= max_runs_per_day 4` | The task's own `max_runs_per_day = 4` cap correctly refused a fifth same-day dispatch -- a second real BB safety mechanism (daily rate limiting) proven live, independent of the output-cap enforcement above. |

No row above has a PR link (report-only, no PR authority) or an artifact
path (no attempt reached a written `REPORT.json`). Add one row per future
real dispatch as `docs-sync`/`docs-sync-pr`/`ci-audit`/`ci-audit-pr` actually
run against operator-selected repos, once picked. Every future row must
name: run id, repo, trigger, model/provider/key path, cost, the exact
`bb artifacts list <id> --json`/`bb artifacts read <id> <path> --json`
commands used to verify, the PR link when one exists, the accepted/rejected
outcome (and who reviewed it), and residual risk.

## What's Actually Left

To close the full bitterblossom-122 acceptance for real:

1. **Operator picks target repos.** `docs-sync`/`docs-sync-pr` and
   `ci-audit`/`ci-audit-pr` currently ship with placeholder `example-org/*`
   allowlists (`examples/docs-sync-plane`, `examples/ci-audit-plane`).
   Someone with the authority to say "audit/watch repo X for real" needs to
   pick at least two real repos for docs-sync and at least one for
   ci-audit, and those allowlists need updating in a real operator instance
   plane (not the checked-in example templates, which stay generic). This
   session's throwaway self-audit of `misty-step/bitterblossom` proved the
   mechanism but is not a substitute for that operator decision.
2. **Tune `output_bytes_cap` for real-repo audits before the first
   production dispatch.** Both live attempts against bitterblossom's actual
   CI surface overran the cap (24000, then 48000 doubled) -- a real repo's
   workflow/test/lint surface produces materially more investigative output
   than the toy `example-org/product-api` template. Size the cap from a
   completed run's actual byte count once the operator names a real target
   repo, not by guessing.
3. **Let report-only run for real, repeatedly, across those repos, and let
   at least one attempt actually complete a `REPORT.json`.** Cron or
   repeated manual dispatch, accumulating scorecard rows here. None of this
   session's five attempts reached that bar.
4. **Operator approves at least one PR-only dispatch per family** (no bot-
   identity blocker remains per the correction above -- just the ordinary
   "explicit approval naming repo and token" rule).
5. **A human reviews each resulting PR/report and records accepted or
   rejected** in the scorecard table -- "the agent said it was useful" does
   not count; per `docs/rollout-scorecards.md`'s doctrine, "'it has been
   working' is not evidence."
6. **Elapsed time**: the acceptance's "within one week" window starts when
   step 3 begins in earnest, not when this card closes.

## Friction Filed

- `output_bytes_cap` undersized for real-repo audits (see the scorecard
  table, attempts 2 and 4): the builder role's Powder MCP surface has no
  card-creation tool, so this is recorded as a comment on bitterblossom-122
  (not a fabricated new card id) naming the exact gap for whoever grooms the
  backlog next -- a genuine product tuning issue that will recur for every
  repo more complex than the shipped `example-org/*` template, not unique
  to this dogfood session.
- The stale bot-identity doctrine found above was corrected in place rather
  than filed as a separate friction card, since it was a direct, mechanical
  fix to existing prose this card's own scope already touches.
- None new beyond what earlier cards in this campaign already filed
  (bitterblossom-926, the `tests/serve.rs` port-race flake).
