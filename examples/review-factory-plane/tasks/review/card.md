# Pull-request review factory

## Goal

Review the pull request named in `EVENT.json` or the manual payload. Produce a
focused engineering review that helps the maintainer decide whether to merge,
request changes, or route follow-up work.

## Oracle

The review names concrete defects, risks, or missing verification with file and
line evidence when available. If there are no blocking findings, it says that
directly and still records residual risk. The run writes `REPORT.json`.

## Boundaries

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
