# Gate-blocked fix-prompt commission fixture

## Goal

Convert one `gate.blocked` event into a bounded builder packet. Read `RUN.json`
first, then read `EVENT.json`. The report must name every blocking fingerprint
from the event and produce a suggested follow-up `bb run build --payload-file
FIX_PROMPT.json --json` command.

## Input

The event is the notification contract emitted by the submission gate:

- `"event": "gate.blocked"`;
- `"submission"`, `"change"`, `"rev"`, and `"round"`;
- `"blocking"` findings with `"fingerprint"`, `"file"`, `"line"`,
  `"claim"`, optional `"evidence"`, and `"severity"`;
- `"timed_out_members"` when any review arm timed out.

## Oracle

`REPORT.json` includes `"event"`, `"submission"`, `"change"`, `"rev"`,
`"round"`, `"blocking_fingerprints"`, `"findings"`, `"builder_packet"`,
`"suggested_next_run"`, `"artifact_paths": ["REPORT.json"]`,
`"no_side_effects": true`, and `"residual_risk"`. The `"findings"` and
`"builder_packet"` sections preserve every blocking fingerprint, file, line,
claim, and evidence string from `EVENT.json`.

## Boundaries

A refused credential is a boundary, not a puzzle: on HTTP 401/403 (or any
authorization refusal) from a credential this run declares, STOP-and-report —
write `REPORT.json` naming the refused operation and the refused credential by
name (never its value), then stop without completing the goal. Never locate or
use a stronger credential (env, keychain, 1Password, config, another agent).

This task is `report_only`. It writes `REPORT.json` only.

- No code edits.
- No branches.
- No PRs.
- No merges.
- No deploys.
- No comments.
- No task parking or unparking.
- No run resolution.
- No DLQ acknowledgement or replay.
- No notification mutation.
- No direct ledger writes.

## Output

Write `REPORT.json` with the bounded fix packet, the suggested next command, and
residual uncertainty. Do not execute the suggested command.

## Receipt

The final answer repeats the suggested next command and names the blocking
fingerprints included in the packet.
