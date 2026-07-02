# Cerberus review commission fixture

This task runs `cerberus review-pr` through the command harness. The wrapper
reads RUN.json and EVENT.json, derives the repo and PR, and writes `REPORT.json`.

Manual payloads may request measurement mode. Webhook payloads post only when
the task is exactly `review` and no dry-run override is present.

Red lines: no approvals, request-changes reviews, merges, pushes, or source
edits from this wrapper.

