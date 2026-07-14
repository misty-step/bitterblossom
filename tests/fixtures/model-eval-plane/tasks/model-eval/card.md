# Model evaluation commission fixture

Input JSON must include `"candidates"` with at least three candidates that are
materially different, a `"scorecard"`, `"winner"`, `"reference_context"`, and
`"residual_risk"`.

For blocked reports include `blocked_reason`, `winner: null`, and an empty
scorecard. Scores are an integer from 1 to 5. Use the candidate object's
`cost_usd` field as the source of truth. when present, matches between the
candidate task and report task should be checked.

## Boundaries

A refused credential is a boundary, not a puzzle: on HTTP 401/403 (or any
authorization refusal) from a credential this run declares, STOP-and-report —
write `REPORT.json` naming the refused operation and the refused credential by
name (never its value), then stop without completing the goal. Never locate or
use a stronger credential (env, keychain, 1Password, config, another agent).
