# 034 Tokenomics Evidence

Backlog item:
[034-review-factory-tokenomics](/backlog.d/_done/034-review-factory-tokenomics.md)
Date: 2026-06-12

## Gate

Command:

```bash
./scripts/verify.sh
```

Result: green. The run completed fmt, clippy, tests, both `bb check`
config validations, and the spine LOC budget.

Relevant tail:

```text
==> spine LOC budget (<= 5000; the Python conductor died of bloat)
    src LOC: 4999
==> verify: all gates green
```

## Median Ledger Evidence

Command:

```bash
./target/debug/bb --config plane runs list --task review --json |
  jq '[.[] | select(.state=="success" and .agent_version==3 and
  (.idempotency_key|startswith("tokenomics:"))) |
  {id,cost_usd,duration_ms,idempotency_key,agent_version,created_at}] |
  sort_by(.created_at)'
```

Rows:

```json
[
  {"id":"33431118212e","cost_usd":0.00512825,"duration_ms":24345,"idempotency_key":"tokenomics:837-v3:1781303292","agent_version":3,"created_at":"2026-06-12T22:28:14.536269Z"},
  {"id":"9cdae182de5f","cost_usd":0.06847158,"duration_ms":278227,"idempotency_key":"tokenomics:843-v3:1781303327","agent_version":3,"created_at":"2026-06-12T22:28:47.569629Z"},
  {"id":"e76266b267e5","cost_usd":0.0103701,"duration_ms":24596,"idempotency_key":"tokenomics:820-v3:1781303653","agent_version":3,"created_at":"2026-06-12T22:34:13.825279Z"},
  {"id":"acbf344bad13","cost_usd":0.00544146,"duration_ms":15662,"idempotency_key":"tokenomics:817-v3:1781303678","agent_version":3,"created_at":"2026-06-12T22:34:38.920538Z"},
  {"id":"3544fdf7b9d6","cost_usd":0.00796505,"duration_ms":21877,"idempotency_key":"tokenomics:806-v3:1781303694","agent_version":3,"created_at":"2026-06-12T22:34:54.965805Z"},
  {"id":"0b5b63509248","cost_usd":0.0073430200000000004,"duration_ms":33601,"idempotency_key":"tokenomics:777-v3:1781303716","agent_version":3,"created_at":"2026-06-12T22:35:17.301512Z"},
  {"id":"6c43fabfc391","cost_usd":0.00570033,"duration_ms":17438,"idempotency_key":"tokenomics:808-v3:1781303750","agent_version":3,"created_at":"2026-06-12T22:35:51.378773Z"},
  {"id":"b2893a429398","cost_usd":0.011014469999999998,"duration_ms":29151,"idempotency_key":"tokenomics:812-v3:1781303768","agent_version":3,"created_at":"2026-06-12T22:36:09.536792Z"},
  {"id":"23e95e715777","cost_usd":0.055751989999999994,"duration_ms":95515,"idempotency_key":"tokenomics:819-v3:1781303798","agent_version":3,"created_at":"2026-06-12T22:36:39.377305Z"},
  {"id":"873a6e4de084","cost_usd":0.05119053,"duration_ms":234765,"idempotency_key":"tokenomics:823-v3:1781303894","agent_version":3,"created_at":"2026-06-12T22:38:15.089901Z"}
]
```

Median command:

```bash
./target/debug/bb --config plane runs list --task review --json |
  jq '[.[] | select(.state=="success" and .agent_version==3 and
  (.idempotency_key|startswith("tokenomics:"))) | .cost_usd] | sort |
  {count:length, costs:., median: ((.[4] + .[5]) / 2), max: .[-1],
  min: .[0]}'
```

Result:

```json
{
  "count": 10,
  "costs": [0.00512825, 0.00544146, 0.00570033, 0.0073430200000000004, 0.00796505, 0.0103701, 0.011014469999999998, 0.05119053, 0.055751989999999994, 0.06847158],
  "median": 0.009167575,
  "max": 0.06847158,
  "min": 0.00512825
}
```

## Trivial Diff

PR #837 changed 2 lines. Run `33431118212e` cost `$0.00512825`
and took `24345ms`; result contained `"comment_posted": false`.

## Seeded Quality

PR #843 was the seeded-flaw PR. Run `9cdae182de5f` cost
`$0.06847158` and took `278227ms`.

Result artifact:

```json
{
  "verdict": "blocking",
  "blocking": 1,
  "serious": 1,
  "minor": 0,
  "comment_posted": false,
  "measurement": true,
  "findings": [
    {
      "severity": "blocking",
      "file": "tools/export-metrics.py",
      "line": 36,
      "claim": "SQL query is assembled with string interpolation, passing user input directly into the statement.",
      "evidence": "WHERE created_at >= '%s' and AND task = '%s' use % formatting with since and task values without parameterization."
    },
    {
      "severity": "serious",
      "file": "tools/prune-runs.sh",
      "line": 7,
      "claim": "rm -rf uses an unquoted variable, so directories with spaces or glob characters will be word-split.",
      "evidence": "rm -rf $dir receives the output of find without quotes, causing bash word splitting on pathnames."
    }
  ]
}
```

PR #843 comment count stayed at 6 before and after the measurement run.

## Comment-Mode Smoke

Initial normal-mode v3 smoke on PR #837 posted a comment but failed run
parsing because the model put final JSON in hidden reasoning. The card now
requires visible final JSON because the harness parses visible assistant
text only.

Second normal-mode smoke on PR #820 succeeded but revealed that direct
`--body "<review>"` loses or executes shell metacharacters in review
markdown. The card now requires writing `REVIEW.md` and using
`gh pr comment --body-file REVIEW.md`.

Final normal-mode smoke:

- PR #817 comment count: `2` before, `3` after.
- Run `01b13f55d7dd`: success, cost `$0.01167536`, duration `32634ms`.
- Last PR comment: `https://github.com/misty-step/bitterblossom/pull/817#issuecomment-4696096959`
- `result.md`:

```json
{"verdict":"approve-leaning","blocking":0,"serious":0,"minor":0,"comment_posted":true,"measurement":false,"findings":[],"review_markdown":"✅ approve-leaning\n\nno findings — sensible workflow guidance added to agent documentation.\n\nreviewed by bitterblossom review factory\n"}
```
