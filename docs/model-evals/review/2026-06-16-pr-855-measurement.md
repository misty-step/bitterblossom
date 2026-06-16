# Review PR 855 Measurement

Date: 2026-06-16

Objective: compare the first-class `review` candidate lanes on PR #855 in
measurement mode after adding model-evaluation lanes for every agentic flow.

Payload:

```json
{"repo":"misty-step/bitterblossom","pr":855,"measurement":true}
```

Candidate runs:

| Task | Model | Run | Cost | Duration | Result |
|---|---|---|---:|---:|---|
| `review` | `moonshotai/kimi-k2.6:minimal` | `a55669478968` | `$0.21864445` | `305127ms` | Approve-leaning, no findings, no comment posted. |
| `review-deepseek` | `deepseek/deepseek-v4-pro` | `6cbb863c82cf` | `$0.039999729` | `134257ms` | Approve-leaning, no findings, no comment posted. |
| `review-glm` | `z-ai/glm-5.1` | `e1541c1e738b` | `$0.038415748` | `130821ms` | Approve-leaning, no findings, no comment posted. |

Evaluator:

| Task | Model | Run | Cost | Duration |
|---|---|---|---:|---:|
| `model-eval` | `openai/gpt-5.5` | `32fc41854a79` | `$0.122747` | `53399ms` |

Accepted conclusion:

`review-glm` is the preferred candidate for this review measurement probe. It
was the lowest-cost and fastest lane, stayed in measurement mode, and gave the
most PR-specific clean-review rationale. `review-deepseek` was close and also
useful. The default Kimi lane satisfied the output contract but produced almost
no falsifiable evidence while costing much more.

Reference context for future review-model runs:

- Do not promote solely from this one clean-PR measurement. Use at least one PR
  with known seeded or historical findings before changing the production
  default.
- Prefer review candidates that include exact commands, file paths, line
  references, or raw diff/log evidence. All three reports summarized evidence
  more than ideal.
- Measurement-mode side-effect protection worked: all three candidate reports
  had `comment_posted = false`.

Dogfood notes:

- `bb run --json` remained silent during long-running lanes. Ledger reads showed
  progress, but the foreground operator experience still needs heartbeats.
- Parallel host distribution worked: DeepSeek and GLM completed while the base
  Kimi run was still executing.
- Cost spread was large enough to matter: the base Kimi run cost more than five
  times either variant on this probe.
