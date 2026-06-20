# Build Builder Cost Calibration

Date: 2026-06-20

## Decision

Keep `build` on `omp` / `z-ai/glm-5.2` for now and raise
`plane/tasks/build/task.toml` `max_cost_per_run_usd` from `$2.00` to `$4.00`.

The cap was stale for live authoring: backlog 074 produced a useful branch and
`REPORT.json` but cost `$3.207397`, so the default build lane parked after a
successful run. `$4.00` preserves runaway protection while giving the observed
live authoring path about 25% headroom.

Do **not** promote `build-glm` to the default yet. It was the cheapest dry-run
candidate, but its report says it committed locally during `dry_run`, which is
a contract concern for the default authoring lane. Treat Pi/GLM as promising,
not production-default evidence.

## Candidate Runs

All candidates used the same payload:

```json
{
  "repo": "misty-step/bitterblossom",
  "backlog": "backlog.d/075-calibrate-omp-glm-builder-cost.md",
  "branch_slug": "075-builder-cost-calibration",
  "dry_run": true
}
```

| Task | Harness / model | Run | State | Cost | Tokens in/out | Duration | Report |
|---|---|---:|---|---:|---:|---:|---|
| `build` | `omp` / `z-ai/glm-5.2` | `f6f2d75b2c3a` | success | `$0.51970540` | `165187 / 14125` | `393804ms` | `plane/.bb/runs/f6f2d75b2c3a/attempt-1/REPORT.json` |
| `build-glm` | `pi` / `z-ai/glm-5.2` | `87421671ebd5` | success | `$0.20079511` | `52138 / 9651` | `317722ms` | `plane/.bb/runs/87421671ebd5/attempt-1/REPORT.json` |
| `build-kimi` | `pi` / `moonshotai/kimi-k2.7-code` | `5bccae0c1d4a` | success | `$0.52021343` | `107868 / 11475` | `314427ms` | `plane/.bb/runs/5bccae0c1d4a/attempt-1/REPORT.json` |

The current live OpenRouter catalog check reported:

- `z-ai/glm-5.2`: context `1048576`, prompt `$0.0000012`, completion
  `$0.0000041`.
- `moonshotai/kimi-k2.7-code`: context `262144`, prompt `$0.000000612`,
  completion `$0.000003069`.

## Evaluator

`model-eval` run `a6d019b66cda` completed successfully with cost `$0.164629`,
`7903 / 4051` tokens, and duration `53726ms`.

Evaluator winner:

- `build-glm` produced the strongest dry-run report at the lowest measured
  spend.

Accepted conclusion:

- Use the `build-glm` result as pressure to keep evaluating Pi/GLM.
- Do not switch the default yet because dry-run contract compliance matters
  more than the cheaper report in a branch-producing lane.
- Apply the shared, low-risk recommendation now: keep OMP/GLM and set the
  `build` cap to `$4.00`.

## Prior Receipts

- Live authoring run `d19d71f1eeae` used `bb-builder-rust@v2` through `omp` /
  `z-ai/glm-5.2`, produced branch `bb/build/074-artifact-contract`, wrote
  `REPORT.json`, passed the submission gate, and cost `$3.207397`
  (`288427 / 18850` tokens, 57 turns).
- Earlier OMP/GLM dry-run `b33bbe05b5d9` cost `$2.46364376` and exposed the
  missing artifact-contract problem later fixed in backlog 074.

## Verification Expectations

- `bb status --json` should show `build.parked = null` after an expected
  successful run at or below the observed `$3.21` live authoring cost.
- `./scripts/verify.sh` must continue to pass.
- The spine remains mechanism-only; this is a task budget calibration, not a
  Rust dispatch change.

## Residual Risk

- The comparison was dry-run for all three current candidates. It does not
  prove Pi/GLM or Kimi should author live branches by default.
- `$4.00` is calibrated from one successful live authoring run and two OMP/GLM
  dry-runs. A larger build packet can still park, which is the desired runaway
  behavior.
- `build-glm` and `build-kimi` keep their `$1.50` caps until live comparator
  evidence justifies changing them.
- The builder dry-run contract should be tightened later if candidate lanes
  keep reporting local commits during `dry_run`.
