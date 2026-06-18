# Calibrate the OMP/GLM builder cost contract

Priority: P1 | Status: ready | Estimate: M

## Goal

Keep the default `build` task useful after moving it to OMP / GLM 5.2: a
successful builder should not routinely strand the primary build lane behind an
operator unpark step.

## Oracle

- [ ] Run the same shaped Bitterblossom build packet through `build`,
      `build-glm`, and `build-kimi`, using dry-run for comparators unless a live
      branch-producing run is explicitly needed for the final candidate.
- [ ] Record a dated result under `docs/model-evals/build/` with run ids,
      branch/report evidence, cost, tokens, duration, and evaluator judgment for
      each candidate.
- [ ] Decide one concrete default: keep OMP / GLM 5.2 and raise
      `max_cost_per_run_usd`, switch `build` to a cheaper API-auth lane, or split
      cheap dry-run planning from expensive live authoring with distinct task
      names.
- [ ] After the decision, `bb status --json` shows the default `build` task is
      not immediately parked by expected successful runs.
- [ ] Preserve the project boundary: no subscription-auth builder is required
      for the default path, and the spine stays mechanism-only.
- [ ] `./scripts/verify.sh` passes.

## Notes

Dogfood source: 2026-06-18 backlog 074 delivery with `bb-dogfood`.

The new default worked mechanically: run `d19d71f1eeae` used
`bb-builder-rust@v2` through `omp` / `z-ai/glm-5.2`, produced branch
`bb/build/074-artifact-contract`, wrote `REPORT.json`, and the submission gate
cleared. But the successful run cost `$3.207397`, so `bb status --json`
re-parked `build` for `run cost $3.2074 > max_cost_per_run_usd $2.00`.

The prior OMP/GLM dry-run `b33bbe05b5d9` also exceeded the same cap at
`$2.46364376`. That is enough evidence that the cap/default pairing is wrong,
but not enough evidence to choose a replacement without a same-packet
comparison.
