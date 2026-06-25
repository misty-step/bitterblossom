# Cerberus review commission

This Bitterblossom task runs the external Cerberus review runner. Bitterblossom
owns the event-plane mechanics: trigger filtering, one run per PR head, dispatch,
budget/parking, and ledger receipts. Cerberus owns review judgment,
`ReviewRequest.v1`, `ReviewArtifact.v1`, rendering, and GitHub projection.

The command harness invokes `bitterblossom/scripts/cerberus-review-wrapper.sh`.
The wrapper reads `RUN.json` and `EVENT.json`, derives the repository and PR
number, then calls `cerberus review-pr`.

## Input

Read `RUN.json` first for the actual task name, then read `EVENT.json` in this
directory. `EVENT.json` identifies the PR, either as GitHub webhook payload
(`repository.full_name`, `pull_request.number`) or as the manual shape
`{"repo": "owner/name", "pr": 123}`. If `EVENT.json` is missing or names no PR,
print an error and exit non-zero — do not guess.

Manual payloads may request measurement mode with either `"measurement": true`
or `"comment": false`. Measurement mode still reviews the real PR through the
same Cerberus process, but it must pass `--dry-run` and never post a GitHub
comment. Webhook payloads post only when the task is exactly `review` and no
dry-run override is present.

If `RUN.json.task` is not exactly `review`, force measurement mode regardless of
payload. This keeps accidental wrapper reuse from posting public PR comments.

Keep the wrapper thin. It may translate event shape, select dry-run vs post, and
collect Cerberus artifacts into `REPORT.json`. It must not contain reviewer
policy, severity rules, finding filters, or repo-specific judgment.

## Output

The wrapper writes `REPORT.json` with the Cerberus artifact location, posting
mode, repo, PR number, and available usage/cost telemetry. The command harness
also receives a structured final stdout object so the `bb` ledger can account
for token and cost data when Cerberus reports it.

## Red lines

- One Cerberus projection per normal run. Zero projections per measurement run.
  Never approve, request changes, merge, push, or edit code from this wrapper.
- If `gh` is unauthenticated or the PR is inaccessible, fail loudly with
  the exact error — never fabricate a review.
