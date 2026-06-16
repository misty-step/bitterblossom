# Watch model catalogs and promote agent configs safely

Priority: P1 | Status: ready | Estimate: M

## Goal

Keep Bitterblossom's agent roster current as OpenRouter and peer providers ship
new models, without treating a launch announcement as enough evidence to change
production reflex defaults.

## Oracle

- [ ] A catalog check records model id, context length, pricing, tool/structured
      output support when exposed, release date, and provider availability for
      all configured model families.
- [ ] The check compares live provider catalog data against checked-in
      `plane/agents/*.toml` and `docs/model-evals/README.md`.
- [ ] New runnable models create a reviewable recommendation artifact or
      backlog item instead of silently changing configs.
- [ ] Promotion requires at least one `bb` smoke run for the affected task class
      and a model-eval record before changing a default.
- [ ] `./scripts/verify.sh` passes.

## Candidate Approaches

1. **Manual daily/weekly catalog sweep.** Cheap and flexible, but depends on
   operator memory and will miss same-day launches.
2. **Scheduled `model-catalog-watch` reflex task.** Fits Bitterblossom's event
   plane, leaves durable receipts, and can file recommendations; adds token/API
   cost and needs dedupe so it does not spam the backlog.
3. **CI guard over checked-in model ids.** Prevents stale or nonexistent model
   ids from landing; good for safety, but it only runs when code changes.
4. **Provider-webhook/RSS ingestion.** Fastest response if providers expose good
   feeds; brittle when release pages and API catalogs disagree.

## Notes

This ticket was filed after `z-ai/glm-5.2` appeared in the OpenRouter API
catalog on June 16, 2026. The immediate GLM 5.2 adoption happened in PR #857;
this ticket owns making future provider changes routine.
