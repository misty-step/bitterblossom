# 2026-06-16 CI Diagnose Real-Failure Evaluation

## Scenario

Manual dogfood run against a real failed GitHub Actions run on
`misty-step/bitterblossom`:

- Commit: `2b7e1b2b2b9a9694bfcbfff1950681d10c4e9be4`
- GitHub Actions run: `24208282343`
- Workflow: `ci`
- Failed job: `Hook Tests`
- Failed command:
  `pip install pytest==8.4.1 && python3 -m pytest -q base/hooks/ scripts/test_runtime_contract.py`
- Probe type: failed-log root-cause diagnosis and builder-packet quality

The selected run is intentionally older than the current branch because the
latest visible Bitterblossom CI runs were green on June 16, 2026. It is still a
real GitHub Actions failure with replayable logs.

## Accepted Candidate Runs

| Task | Model | Run | Cost | Duration | Result |
|---|---|---:|---:|---:|---|
| `ci-diagnose` | `deepseek/deepseek-v4-flash` | `701b4ce14c6b` | `$0.0340725056` | 249.3s | `actionable` |
| `ci-diagnose-kimi` | `moonshotai/kimi-k2.7-code` | `789da2d6c5ff` | `$0.03160564` | 190.7s | `actionable` |
| `ci-diagnose-glm` | `z-ai/glm-5.1` | `2fadec48f213` | `$0.039297356` | 190.9s | `actionable` |

## Accepted Evaluator Run

| Task | Model | Run | Cost | Duration | Result |
|---|---|---:|---:|---:|---|
| `model-eval` | `openai/gpt-5.5` | `5a964da02de5` | `$0.157223` | 55.6s | GLM 5.1 won the real-failure diagnosis probe. |

## Winner

For this real-failure slice, `ci-diagnose-glm` on `z-ai/glm-5.1` produced the
best report. The evaluator scored it highest because it separated the two
privileged-Dagger guard failures from the distinct Colima shim log assertion,
included the exact failed-test assertions, and produced the most bounded builder
packet.

The evaluator's scorecard:

| Task | Evidence | Task Fit | Actionability | Cost/Latency | Contract |
|---|---:|---:|---:|---:|---:|
| `ci-diagnose` | 4 | 4 | 4 | 4 | 5 |
| `ci-diagnose-kimi` | 4 | 4 | 4 | 4 | 5 |
| `ci-diagnose-glm` | 5 | 5 | 5 | 4 | 5 |

DeepSeek Flash had useful source inspection and good cost, but it overstated
that the privileged-engine guard caused all three failures even though its own
evidence showed the Colima shim test exited successfully with missing output.
Kimi K2.7 Code gave a clean, bounded report and was the cheapest successful
candidate in this cohort, but it did not inspect source or address the Colima
shim assertion as directly.

## Reference Context

For future real-failure `ci-diagnose` probes, the strongest report should:

- Name the target repo, commit, workflow, GitHub Actions run id, and failed job.
- Quote or paraphrase the first meaningful failed log line.
- Separate deterministic test failures from infrastructure or stale-branch
  failures.
- Recommend at most one builder-ready follow-up run, with concrete repo, base
  ref, idempotency key, and payload.
- Preserve candidate task names from `RUN.json`; do not collapse variants back
  to the base flow name.
- Keep artifact paths limited to plane-collected artifacts.
- Distinguish related failures that share a command from failures with different
  immediate causes; do not flatten them into one root cause when logs say
  otherwise.
- Prefer test-scoped or packet-scoped remediation over broad workflow-level
  privilege changes unless the runner trust boundary is explicit.

For run `24208282343`, future agents should know that the best diagnosis says:
set `BB_ALLOW_PRIVILEGED_DAGGER_IN_CI=1` only in the appropriate
runtime-contract test environments or trusted runner scope, and separately
restore or update the expected `using Colima docker shim` behavior.

## Dogfood Notes

- Good: The three candidate reports were directly comparable because
  `RUN.json` preserved task names for `ci-diagnose`, `ci-diagnose-kimi`, and
  `ci-diagnose-glm`.
- Good: The evaluator payload could use the plane ledger as source of truth for
  run ids, costs, durations, and candidate report JSON.
- Good: The real-failure run exposed a materially different winner from the
  no-failure probe. Kimi won no-failure handling, while GLM won root-cause
  diagnosis on this failure.
- Friction: `bb run --json` remains silent until completion, so the operator
  still needs a second `bb status --json` poll to see long-running cohorts.
- Friction: The baseline DeepSeek run spent 300k input tokens, far more than
  Kimi or GLM, because it inspected additional source via GitHub API. That can
  improve evidence, but evaluators should watch for unnecessary context pulls.
- Friction: The current CI-diagnose card lets agents inspect source when logs
  need it, but it does not require line citations when source is inspected. The
  evaluator treated that as a weakness for GLM's otherwise best report.

## Residual Risk

- The selected failure is a real GitHub Actions failure, but it is from April 9,
  2026 because all recent visible Bitterblossom CI runs were green on June 16,
  2026.
- The evaluator did not replay GitHub commands itself; it scored the three
  candidate reports and their embedded evidence.
- The intended security policy for `BB_ALLOW_PRIVILEGED_DAGGER_IN_CI` remains a
  product decision for any future builder lane.
- The Colima shim output may have been an intentional behavior change, not a
  regression; the builder packet should verify current desired behavior before
  patching.
