# Factory Audit: 2026-03-27 — Post-Refactor Shakedown

## Context
First full audit after the agent-first refactor (#796). Deleted 12K+ LOC of conductor judgment code. Infrastructure is now 5,525 LOC. This audit evaluates: does the remaining code have a right to exist?

## Findings

### F1: Master had compile warnings + 10 test failures after squash merge [P0, FIXED]
**Root cause:** Squash merge of 15 commits flattened rebase conflict resolutions. Dead helpers and auto-close tests survived into master.
**Resolution:** Hotfix PR #798 merged. 299 tests, 0 failures.
**Action:** Consider using merge commits instead of squash for large refactors.

### F2: store.ex is 813 LOC, ~85% dead [P1]
Only 3 of 20+ public functions are called by live code:
- `record_event/3` (health monitor, 3 refs)
- `list_all_events/1` (CLI, 1 ref)
- `list_runs/1` (CLI status, 4 refs — and even this is questionable since runs aren't created anymore)

Dead: `create_run`, `update_run`, `find_run_by_pr`, `complete_run`, `terminate_run`, `heartbeat_run`, `acquire_lease`, `release_lease`, `leased?`, `record_incident`, `list_incidents`, `record_waiver`, `list_waivers`, `set_dispatch_paused`, `dispatch_paused?`, `validate_columns`, `list_active_runs`, `mark_semantic_ready`.

**Estimated savings:** ~650 LOC deleted from store.ex + ~400 LOC from store_test.exs.

### F3: github.ex is 870 LOC, ~75% dead [P1]
19 of ~25 public functions are unused. The CodeHost behaviour (15 callbacks) has zero callers. Agents call `gh` directly from the sprite.

Dead: `checks_green?`, `checks_failed?`, `ci_status`, `merge`, `labeled_prs`, `evaluate_checks`, `evaluate_checks_failed`, `add_label`, `remove_label`, `close_issue`, `close_pr`, `find_open_pr`, `issue_open_prs`, `pr_state`, `pr_review_comments`, `pr_ci_failure_logs`, `create_issue_comment`, `list_eligible`, `eligible_issues`.

**Estimated savings:** ~600 LOC deleted from github.ex + ~700 LOC from github_test.exs.

### F4: config.ex has 8 dead config functions [P2]
Functions for deleted modules: `ci_timeout`, `max_replays`, `builder_retry_max_attempts`, `pr_minimum_age_seconds`, `issue_cooldown_cap_minutes`, `fixer_timeout`, `polisher_timeout`, `max_starts_per_tick`.

**Estimated savings:** ~80 LOC.

### F5: code_host.ex behaviour is entirely dead [P2]
15 callbacks, 0 callers. Can delete the whole module (69 LOC).

### F6: Flaky config_dispatch_env test [P3]
`dispatch_env/0 includes OPENAI_API_KEY when API key fallback is active` fails intermittently due to env pollution from other tests. Pre-existing.

### F7: Launcher persona mapping is fragile [P2]
`launcher.ex` has hardcoded `persona_for_role(:builder) -> :weaver` mapping. If fleet.toml roles change, this silently breaks. Should derive from fleet config or sprites/ directory.

### F8: No agent loop health monitoring [P1]
When an agent loop exits (codex process dies), nothing restarts it. HealthMonitor checks sprite reachability but doesn't verify the codex process is running. Need: detect agent exit → re-launch.

### F9: Multi-repo readiness gap [P2]
Launcher hardcodes a single repo from fleet defaults. For multi-repo, each sprite needs its own repo assignment. fleet.toml supports per-sprite repo but Launcher doesn't use it.

## Scores

| Area | Score | Notes |
|------|-------|-------|
| Agent autonomy | 9/10 | Agents are fully autonomous, pick own work, use skills |
| Infrastructure simplicity | 6/10 | 5.5K LOC but ~1.5K is dead code |
| Test health | 8/10 | 299 pass, 1 flaky (pre-existing) |
| Observability | 5/10 | Event log exists but dashboard is minimal |
| Resilience | 4/10 | No loop restart, no health-of-agent monitoring |
| Multi-repo readiness | 3/10 | Single-repo only in practice |

## Recommended actions (priority order)

1. **Dead code purge** (P1, ~2K LOC deletion): store.ex, github.ex, config.ex, code_host.ex
2. **Agent loop monitoring** (P1): Detect codex exit → re-launch via HealthMonitor
3. **Multi-repo launcher** (P2): Use per-sprite repo from fleet.toml
4. **Persona mapping cleanup** (P2): Derive from sprites/ directory, not hardcoded map
5. **Flaky test fix** (P3): Isolate config_dispatch_env test env
