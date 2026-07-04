# Roster Cerberus review task

## Goal

Review the change named in `EVENT.json` using the Cerberus role supplied by
roster. Produce a concise code-review verdict with concrete evidence.

## Oracle

The review cites files, diff hunks, commands, logs, or URLs that were actually
inspected. A clean review says no blocking issues were found and names residual
risk.

## Boundaries

Read-only. Do not push, merge, approve, request changes, edit source, post
comments, change labels, or mutate external systems.

## Output

Return the review verdict and inspected context. If `EVENT.json` names a repo
and PR, include both in the receipt.
