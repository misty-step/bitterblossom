# Model evaluation commission fixture

Input JSON must include `"candidates"` with at least three candidates that are
materially different, a `"scorecard"`, `"winner"`, `"reference_context"`, and
`"residual_risk"`.

For blocked reports include `blocked_reason`, `winner: null`, and an empty
scorecard. Scores are an integer from 1 to 5. Use the candidate object's
`cost_usd` field as the source of truth. when present, matches between the
candidate task and report task should be checked.
