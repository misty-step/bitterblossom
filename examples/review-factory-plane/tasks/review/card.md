# Pull-request review factory

## Goal

Review the pull request named in `EVENT.json` or the manual payload. Produce a
focused engineering review that helps the maintainer decide whether to merge,
request changes, or route follow-up work.

## Oracle

The review names concrete defects, risks, or missing verification with file and
line evidence when available. If there are no blocking findings, it says that
directly and still records residual risk. The run writes `REPORT.json`.

For any meaningful implementation diff (not docs-only or tiny-config-only),
also apply the Thermo-Nuclear maintainability lens named in this agent's
`skills` (backlog 088): read the lens file from the synced `review-rules`
workspace repo before reviewing (copy in
`vendor/skills/thermo-nuclear-code-quality-review/SKILL.md` of the
Bitterblossom repo, with upstream provenance in the adjacent
`.sync-meta.json` — vendor it into your own `review-rules` repo verbatim,
do not retype it). A genuine structural regression (file-size sprawl past
~1000 lines, spaghetti branching, thin wrappers, canonical-layer leaks) is a
blocking finding; a stylistic nit is advisory, never blocking. Skip the lens
only for a docs-only or tiny-config-only diff, and say so explicitly in the
summary — name the risk tier (`docs-only` or `tiny-config`) so the driver
can record `bb submit waive --change <key> --rev <rev> --kind <kind> --reason
"risk-tier:<tier>"` instead of leaving the gate member perpetually pending.
A waiver applies only to that exact rev — the next rev needs its own.

## Boundaries

A refused credential is a boundary, not a puzzle: on HTTP 401/403 (or any
authorization refusal) from a credential this run declares, STOP-and-report —
write `REPORT.json` naming the refused operation and the refused credential by
name (never its value), then stop without completing the goal. Never locate or
use a stronger credential (env, keychain, 1Password, config, another agent).

Read repository state, pull-request metadata, CI output, and changed files only.
Do not push, merge, approve, request changes, edit source, change labels, or
post public comments unless an operator has explicitly enabled that policy in
the runtime copy of this task.

## Output

Write `REPORT.json` using the shape in
`samples/REPORT.json`: schema, verdict, summary, findings, comment_policy, and
residual_risk. Findings should be ordered by severity and grounded in evidence.

## Receipt

The final answer repeats the verdict, finding count by severity, the repo/PR
reviewed, and the path to `REPORT.json`.
