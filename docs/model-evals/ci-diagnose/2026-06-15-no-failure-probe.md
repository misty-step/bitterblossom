# 2026-06-15 CI Diagnose No-Failure Probe

## Scenario

Manual dogfood run against the latest successful GitHub Actions run on
`misty-step/bitterblossom`:

- Commit: `0512d3fb4f58aaa498d75f393624fc1fe03785a8`
- GitHub Actions run: `27582831343`
- Workflow: `ci`
- Probe type: no-failure handling, not failed-log diagnosis

## Accepted Candidate Runs

| Task | Model | Run | Cost | Duration | Result |
|---|---|---:|---:|---:|---|
| `ci-diagnose` | `deepseek/deepseek-v4-flash` | `3f6045ef22a0` | `$0.0035532752` | 37.3s | `no_failure` |
| `ci-diagnose-kimi` | `moonshotai/kimi-k2.7-code` | `911332e55df7` | `$0.02310014` | 86.0s | `no_failure` |
| `ci-diagnose-glm` | `z-ai/glm-5.1` | `e7f66cdada5b` | `$0.012604312` | 46.4s | `no_failure` |

These runs were produced after the plane began materializing `RUN.json` and the
CI card began requiring agents to report `task` from that file. The Kimi and GLM
reports correctly identify as `ci-diagnose-kimi` and `ci-diagnose-glm`.

## Accepted Evaluator Run

| Task | Model | Run | Cost | Duration | Result |
|---|---|---:|---:|---:|---|
| `model-eval` | `openai/gpt-5.5` | `204a6653e0c5` | `$0.152528` | 63.4s | Kimi K2.7 Code won the no-failure probe. |

## Winner

For this no-failure slice, `ci-diagnose-kimi` on
`moonshotai/kimi-k2.7-code` produced the best report. It was the most expensive
candidate, but it gave the clearest operator-facing proof: commit-scoped run
discovery, direct run-id metadata, empty failed-log evidence, and the best
residual-risk framing for stale or already-resolved triggers.

Do not promote Kimi as the default root-cause diagnoser from this alone. This
record only evaluates no-failure handling.

## Reference Context

For future no-failure `ci-diagnose` probes, the strongest report should:

- Name the target repo, commit, run id, and workflow.
- Show exact replayable `gh` commands with concrete repo, commit, and run id.
- Prove the referenced run concluded `success`.
- Include direct run-id metadata, job/step-level success, or an explicit
  zero-failed-runs query.
- State that `gh run view --log-failed` produced no failed output.
- Set `suggested_next_run` to `null`.
- Keep residual risk limited to stale payloads, already-resolved failures,
  wrong run, wrong workflow, wrong commit, or reruns outside the supplied
  payload.
- Preserve candidate task names from `RUN.json`; do not collapse variants back
  to the base flow name.

DeepSeek Flash remains the cheapest adequate baseline for no-failure probes.
GLM 5.1 gave a strong cost-to-evidence ratio and useful job-step corroboration.
Kimi K2.7 Code won this record because its evidence and residual-risk framing
were materially better.

## Superseded Discovery Runs

Earlier runs in this same session found and fixed prompt/plane bugs:

| Run | Finding |
|---|---|
| `75ac8f56a5b0` | Evaluator penalized report-local `cost_usd: null` even though the plane owns attempt cost. |
| `b02969880b0c` | Evaluator score scale was implicit. |
| `173c7a607d7a` | Evaluator treated correct variant task names as a contract mismatch. |

Candidate runs `4fb823bee887`, `95d742268437`, and `9c71b7c2629f` predated
`RUN.json`; Kimi/GLM reports self-identified as the base task and are superseded.

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
- Bug found and fixed: variant tasks had no generic run metadata, so symlinked
  cards could not preserve actual task names. `RUN.json` now materializes task,
  run, agent, model, and substrate metadata.
- Bug found and fixed: the evaluator card incorrectly treated variant task
  names as a contract mismatch instead of checking `report.task` against the
  candidate task.

## Residual Risk

This was not a failed-CI diagnosis. The next model-evaluation record for this
flow should use a real failed run and score root-cause quality, reproduction
quality, and builder handoff quality.
