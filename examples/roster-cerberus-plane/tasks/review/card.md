# Roster Cerberus review task

## Goal

Review the change named in `EVENT.json` using the Cerberus role supplied by
roster. Produce a concise code-review verdict with concrete evidence.

## Oracle

The review cites files, diff hunks, commands, logs, or URLs that were actually
inspected. A clean review says no blocking issues were found and names residual
risk.

The Cerberus role brief above (`[roster_brief]`) already names the
Thermo-Nuclear maintainability lens as a required skill for meaningful
implementation diffs (backlog 088) — this task card does not need to repeat
its content. Structural findings from that lens are severity `blocking`;
stylistic nits are advisory. A docs-only or tiny-config-only diff may skip
it if the receipt names the risk tier explicitly.

## Boundaries

A refused credential is a boundary, not a puzzle: on HTTP 401/403 (or any
authorization refusal) from a credential this run declares, STOP-and-report —
write `REPORT.json` naming the refused operation and the refused credential by
name (never its value), then stop without completing the goal. Never locate or
use a stronger credential (env, keychain, 1Password, config, another agent).

Read-only. Do not push, merge, approve, request changes, edit source, post
comments, change labels, or mutate external systems.

## Output

Return the review verdict and inspected context. If `EVENT.json` names a repo
and PR, include both in the receipt.
