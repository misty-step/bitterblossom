### Technical Overview: /scripts

The `scripts` directory holds the run-centric control plane plus a small set of supporting operational utilities. The canonical operator boundary is:

- `bb` for sprite transport (`setup`, `dispatch`, `status`, `logs`, `kill`)
- `scripts/conductor.py` for issue leasing, review orchestration, CI waits, and merge

#### Core Architecture and Key Roles

**1. Agent Orchestration (The "Ralph" Loop)**
The system implements the Ralph loop for on-sprite execution until a task completes or blocks.
- `ralph.sh`: the harness that invokes Claude Code, enforces iteration limits, and checks completion signals.
- `ralph-prompt-template.md`: the dispatch template rendered by `bb dispatch`.
- `sprite-agent.sh`: an older remote supervisor retained for legacy or ad hoc workflows, not the primary transport path.

**2. Control Plane**
- `conductor.py`: the control plane for GitHub issue intake, builder/reviewer dispatch, reconciliation, CI waiting, and merge.
- `test_conductor.py`: regression coverage for the run lifecycle and governance rules.

**3. Supporting Utilities**
- `dispatch.sh`: a legacy shell dispatch helper. Prefer `bb dispatch` for the supported path.
- `sprite-bootstrap.sh`: idempotent remote bootstrap helper for shell-driven environments.
- `onboard.sh`: local environment bootstrap for operators.

**4. Monitoring and Observability**
A suite of tools provides visibility into the distributed agent fleet.
- `watchdog-v2.sh` / `watchdog.sh`: older monitoring experiments.
- `health-check.sh` / `fleet-status.sh`: deeper shell-based inspection helpers.
- `refresh-dashboard.sh`: static dashboard generator.
- `webhook-receiver.sh`: event collector for posted sprite-agent events.

**5. External Integrations**
- `pr-shepherd.sh`: tracks PR and CI state.
- `sentry-watcher.sh`: polls Sentry for anomalies.

#### Shared Logic and Libraries
- `lib.sh`: shared shell helpers for auth, environment resolution, and remote shell utilities.

#### Dependencies and Key Constraints
- Prefer `SPRITE_TOKEN` for transport auth; `FLY_API_TOKEN` is a fallback token-exchange path.
- `OPENROUTER_API_KEY` is required during `bb setup` so sprite-side settings can be rendered.
- Completion is signaled through `TASK_COMPLETE`, `TASK_COMPLETE.md`, and `BLOCKED.md` in the workspace root.
- New transport behavior should land in `cmd/bb`, not in additional shell wrapper surfaces.
