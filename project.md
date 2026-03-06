# Project: Bitterblossom

## Vision
Bitterblossom is the conductor for a single-repo software factory: it routes GitHub work to persistent sprites, drives implementation and review, and merges only when governance says the run is truly done.

**North Star:** An always-on remote conductor clears a fully agent-runnable backlog end-to-end with truthful run state, isolated execution, and human-auditable decisions.
**Target User:** An operator or autonomous agent supervising a persistent sprite workforce for one repository.
**Current Focus:** Make the single-repo factory trustworthy enough to run 24/7 without a laptop in the loop.
**Key Differentiators:** Thin transport CLI, run-centric control plane, persistent sprites, GitHub as the work ledger, explicit governance instead of “builder says done.”

## Domain Glossary

| Term | Definition |
|------|-----------|
| Conductor | The always-on control plane in `scripts/conductor.py` that owns intake, leases, routing, review, CI, and merge decisions. |
| Worker Sprite | A persistent remote execution surface used for builder and reviewer runs. |
| Review Council | The independent reviewer set that audits a builder result before merge. |
| Run | One durable execution record with a `run_id`, explicit phase, artifacts, and event history. |
| Lease | The machine-facing claim that one run currently owns one GitHub issue. |
| Profile | The runtime configuration chosen by the router: model, provider, persona, prompt pack, tools, and budget policy. |
| Variant | One parallel implementation path for the same issue under a different profile. |
| Trace Bullet | The narrow proof path for the factory: lease issue, build, review, revise, pass CI, merge, reconcile. |

## Active Focus

- **Milestone:** `Now: Current Sprint` — harden the accepted conductor MVP into a reliable single-repo factory.
- **Key Issues:** #470, #468, #469, #98, #102
- **Theme:** Always-on control plane, isolated execution, truthful observability, and proof of the full trace bullet.

## Quality Bar

- [ ] Every issue the conductor can lease is runnable by sprites without clarification loops.
- [ ] Run state tells the truth: phase, heartbeat, blocking reason, review status, and merge outcome are inspectable.
- [ ] Builder and reviewer execution is isolated per run; stale workspace state cannot leak forward.
- [ ] The full trace bullet can be reproduced on demand against a target repository.

## Patterns to Follow

### Run-Centric State
```python
update_run(conn, run_id, phase="reviewing", pr_number=builder.pr_number, pr_url=builder.pr_url)
record_event(conn, event_log, run_id, "review_complete", {"reviewer": review.reviewer, "verdict": review.verdict})
```

The conductor should expose explicit state transitions instead of inferring truth from ad hoc shell behavior.

### Thin Edge, Rich Control Plane
```python
# cmd/bb stays transport; orchestration lives in scripts/conductor.py
worker = select_worker(runner, args.repo, args.worker, pathlib.Path(args.builder_template))
builder, builder_payload = run_builder(...)
reviews = run_review_round(...)
```

Keep transport logic thin and deterministic. Put workflow judgment and orchestration in the conductor, not in `cmd/bb/`.

## Lessons Learned

| Decision | Outcome | Lesson |
|----------|---------|--------|
| Registry/fleet/proxy-heavy architecture | Rejected and largely deleted | Keep Bitterblossom focused on the conductor path, not general fleet management. |
| Shared worker checkout per run | Causes state leakage and brittle cleanup | Persistent mirrors are fine; execution surfaces must be isolated per run. |
| Local-only orchestration | Good for proving the loop, bad for 24/7 operation | Production orchestration belongs on a remote coordinator runtime. |
| Generic status strings and hidden failure modes | Erode operator trust | Surface explicit run phases, review outcomes, and blocking reasons. |

---
*Last updated: 2026-03-06*
*Updated during: /groom session*
