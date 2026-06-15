# 2026-06-15 CI Diagnose No-Failure Probe

## Scenario

Manual dogfood run against the latest successful GitHub Actions run on
`misty-step/bitterblossom`:

- Commit: `0512d3fb4f58aaa498d75f393624fc1fe03785a8`
- GitHub Actions run: `27582831343`
- Workflow: `ci`
- Probe type: no-failure handling, not failed-log diagnosis

## Candidate Runs

| Task | Model | Run | Cost | Duration | Result |
|---|---|---:|---:|---:|---|
| `ci-diagnose` | `deepseek/deepseek-v4-flash` | `4fb823bee887` | `$0.0033439162` | 58.8s | `no_failure` |
| `ci-diagnose-kimi` | `moonshotai/kimi-k2.7-code` | `95d742268437` | `$0.01707657` | 55.6s | `no_failure` |
| `ci-diagnose-glm` | `z-ai/glm-5.1` | `9c71b7c2629f` | `$0.011620616` | 40.8s | `no_failure` |

## Evaluator Runs

| Task | Model | Run | Cost | Duration | Result |
|---|---|---:|---:|---:|---|
| `model-eval` | `openai/gpt-5.5` | `75ac8f56a5b0` | `$0.13401` | 52.4s | Prompt bug found: evaluator penalized report-local `cost_usd: null`. |
| `model-eval` | `openai/gpt-5.5` | `b02969880b0c` | `$0.120755` | 48.0s | Clean evaluation after card fix. |

## Winner

Keep `ci-diagnose` on `deepseek/deepseek-v4-flash` for this no-failure slice.
It produced the most replayable proof at the lowest cost: exact `gh run list`,
`gh run view --json jobs`, and `gh run view --log-failed` evidence, clean
`suggested_next_run: null`, and the best cost/latency fit.

## Reference Context

For future no-failure `ci-diagnose` probes, the strongest report should:

- Name the target repo, commit, run id, and workflow.
- Show exact replayable `gh` commands with concrete repo, commit, and run id.
- Prove the referenced run concluded `success`.
- Include job/step-level success or an explicit zero-failed-runs query.
- State that `gh run view --log-failed` produced no failed output.
- Set `suggested_next_run` to `null`.
- Keep residual risk limited to wrong run, wrong workflow, wrong commit, or a
  later rerun outside the supplied payload.

`z-ai/glm-5.1` contributed a useful pattern: explicitly query for failed runs on
the commit. Its report was weaker because evidence source strings used
placeholders such as `<rev>` instead of concrete commands. `Kimi K2.7 Code`
was valid but more expensive and returned a less clean `suggested_next_run`
object with `command: null`.

## Dogfood Notes

- Good: `bb` produced clear run ids, artifact directories, costs, token counts,
  durations, models, and trace ids for each candidate and evaluator run.
- Good: Immutable candidate receipts let the evaluator card be fixed and rerun
  without rerunning the three candidate agents.
- Friction: `bb run --json` is silent while waiting. For multi-agent cohorts and
  higher-cost evaluator runs, operators need live attempt state, model, elapsed
  time, and cost-so-far signals.
- Friction: `bb task list --json` returned a top-level array, and entries with
  nullable fields made obvious `jq` filters fail. The shape is usable but not
  discoverable enough for operators.
- Bug found and fixed: the evaluator card did not state that candidate-level
  `cost_usd` is the source of truth when report-local `cost_usd` is null.
- Bug found and fixed: the evaluator card did not specify the numeric score
  scale.
- Bug found and fixed: the CI diagnose card did not explicitly require
  `suggested_next_run: null` for no-failure reports or prohibit placeholder
  evidence commands.

## Residual Risk

This was not a failed-CI diagnosis. The next model-evaluation record for this
flow should use a real failed run and score root-cause quality, reproduction
quality, and builder handoff quality.
