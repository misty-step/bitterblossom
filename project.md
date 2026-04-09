# Project: Bitterblossom

## Direction Lock

**Current direction lock (2026-04-08):** Bitterblossom is temporarily focused
on one job only: `Tansy` listens to Canary, investigates live incidents,
repairs the correct repo, and verifies recovery. Backlog-clearing factory lanes
remain in the codebase, but they are not the active product priority.

## Vision
Bitterblossom is the conductor for a single-repo software factory: it routes repo-owned work to persistent sprites, drives implementation and review, and lands only when governance says the run is truly done.

**North Star:** An always-on remote conductor clears a fully agent-runnable backlog end-to-end with truthful run state, isolated execution, and human-auditable decisions.
**Target User:** An operator or autonomous agent supervising a persistent sprite workforce for one repository.
**Current Focus:** Make the single-repo factory trustworthy enough to run 24/7 without a laptop in the loop.
**Key Differentiators:** Thin transport CLI, run-centric control plane, persistent sprites, repo-owned work and evidence, explicit governance instead of “Weaver says done.”

## Design Philosophy

Bitterblossom is a **cybernetic governor** for software production. The conductor doesn't write code — it designs and operates the feedback control loop that closes at the architectural level.

**The cybernetics pattern:** Stop turning the valve. Steer. Each time this pattern appears in history (Watt's governor, Kubernetes, agent harnesses), it's because someone built a sensor and actuator powerful enough to close the loop at a new layer. LLMs are the first sensor+actuator that can operate at the level of architectural judgment — not just “does it compile?” but “does this change fit the system?”

**The calibration problem:** The hard work isn't getting the basic loop running (CI, tests, dispatch). It's encoding system-specific knowledge: what “good” means for this codebase, which patterns the architecture rewards, which it avoids. If you don't externalize this judgment, agents make the same mistakes on the hundredth run as the first. CLAUDE.md, project.md, WORKFLOW.md, and the retro loop are the calibration surface.

**The drift trap:** Without codified architectural constraints, agents amplify drift at machine speed. You can't use agents to clean up the mess if the agents don't know what clean looks like. The retro loop's architectural guard (“symptom or root cause?”) is the anti-drift sensor.

**The adaptive harness:** The conductor doesn't just govern itself — it governs arbitrary repos. For each target, it must detect the harness (tests, CI, docs, conventions), build a calibration profile, and adapt its feedback loop. A repo with no tests gets harness investment before feature work. A repo with strong CI gets backlog clearing at speed. The conductor's value scales with its ability to calibrate to unfamiliar systems.

## Domain Glossary

| Term | Definition |
|------|-----------|
| Conductor | The always-on Elixir/OTP control plane in `conductor/` that owns intake, leases, routing, governance, and merge decisions. |
| Worker Sprite | A persistent remote execution surface used by named sprites such as Weaver, Thorn, Fern, and Muse. |
| Review Council | The independent reviewer set that audits a builder result before merge. |
| Run | One durable execution record with a `run_id`, explicit phase, artifacts, and event history. |
| Lease | The machine-facing claim that one run currently owns one repo-local work item or incident lane. |
| Profile | The runtime configuration chosen by the router: model, provider, persona, prompt pack, tools, and budget policy. |
| Variant | One parallel implementation path for the same issue under a different profile. |
| Trace Bullet | The narrow proof path for the factory: claim work, build, review, revise, pass Dagger, land, reconcile. |

## Active Focus

- **Milestone:** `Tansy v1` — one always-on Canary responder sprite with an explicit service catalog and recovery verification loop.
- **Key Issues:** role wiring for `responder`, typed `canary-services.toml`, Tansy persona/skill path, and a safe path to opt-in merge/deploy.
- **Theme:** Truthful incident response over generic factory throughput. Canary is the work queue; incidents come before backlog.

## Architecture Artifacts

- [Architecture Overview](docs/architecture/README.md)
- [Architecture Glance](docs/architecture/glance.md)
- [Conductor Drill Down](docs/architecture/conductor.md)
- [bb CLI Transport Drill Down](docs/architecture/bb-cli.md)
- [Codebase Map](docs/CODEBASE_MAP.md)
- [Context Index](docs/context/INDEX.md)
- [Routing Guide](docs/context/ROUTING.md)
- [Drift Watchlist](docs/context/DRIFT-WATCHLIST.md)

## Quality Bar

- [ ] Every work item the conductor can claim is runnable by sprites without clarification loops.
- [ ] Autopilot-ready work items carry a durable local spec and intent contract, and routing surfaces explicit reasons when a lane is not ready.
- [ ] Run state tells the truth: phase, heartbeat, blocking reason, review status, and merge outcome are inspectable.
- [ ] Builder and reviewer execution is isolated per run; stale workspace state cannot leak forward.
- [ ] The full trace bullet can be reproduced on demand against a target repository.

## Patterns to Follow

### Run-Centric State
```elixir
Store.update_run(run_id, %{phase: "governing", pr_number: pr_number, pr_url: pr_url})
Store.record_event(run_id, "governance_complete", %{verdict: verdict, reason: reason})
```

The conductor exposes explicit state transitions via `RunServer` `handle_continue` chains instead of inferring truth from ad hoc shell behavior.

### Thin Edge, Rich Control Plane
```elixir
# Orchestrator dispatches to RunServer GenServers
{:ok, pid} = DynamicSupervisor.start_child(Conductor.RunSupervisor, {RunServer, opts})
# RunServer owns the full lifecycle: lease -> workspace -> dispatch -> govern -> merge
```

Keep transport logic thin and deterministic. Workflow judgment lives in the Elixir conductor, not in `cmd/bb/`.

## Lessons Learned

| Decision | Outcome | Lesson |
|----------|---------|--------|
| Registry/fleet/proxy-heavy architecture | Rejected and largely deleted | Keep Bitterblossom focused on the conductor path, not general fleet management. |
| Shared worker checkout per run | Causes state leakage and brittle cleanup | Persistent mirrors are fine; execution surfaces must be isolated per run. |
| Local-only orchestration | Good for proving the loop, bad for 24/7 operation | Production orchestration belongs on a remote coordinator runtime. |
| Generic status strings and hidden failure modes | Erode operator trust | Surface explicit run phases, review outcomes, and blocking reasons. |
| Python conductor (20K LOC in 7 days) | Replaced by 1,649 LOC Elixir | Architecture critique before implementation prevents complexity spirals. OTP is the natural fit for agent orchestration. |

---
*Last updated: 2026-03-14*
*Updated during: /groom session*
