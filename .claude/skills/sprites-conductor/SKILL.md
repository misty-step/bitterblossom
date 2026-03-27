---
name: sprites-conductor
description: >-
  Bitterblossom conductor-sprite integration patterns. How the conductor should
  (and should not) interact with the sprites platform. Architecture critique,
  known failure modes, and the correct abstractions. Depends on /sprites skill
  for platform fundamentals.
tags: [bitterblossom, conductor, sprites, orchestration, architecture]
argument-hint: "[topic: architecture | failures | simplification | dispatch]"
---

# Sprites-Conductor Integration

How the Bitterblossom conductor should interact with sprites. This skill
captures hard-won architectural lessons from production incidents.

**Prerequisite:** Read `/sprites` for platform fundamentals first.

## Current Architecture (as of 2026-03-22)

```
Conductor.Sprite        — exec, provision, probe, status, dispatch
Fleet.Reconciler        — boot-time health check + provisioning
Fleet.HealthMonitor     — periodic re-probe of fixer/polisher sprites
Conductor.Polisher      — Fern dispatch with backoff/health tracking
Conductor.Fixer         — Thorn dispatch with backoff/health tracking
Conductor.Orchestrator  — builder probe-before-dispatch + drain logic
```

## The Fundamental Problem

The conductor was designed as if sprites are always-on servers. They are not.
Sprites auto-stop when idle. This is normal, expected, and by design.

The conductor's `Sprite.probe/2` uses a 15s timeout. Cold sprites may take
variable time to wake via exec. The probe interprets any exec failure as
"unreachable." This single wrong classification cascades through the entire
system, spawning ~350 lines of compensating infrastructure.

### The Compensation Stack (what exists because probe is wrong)

| Component | Lines | Compensates for |
|-----------|-------|-----------------|
| `Fleet.HealthMonitor` | ~200 | Detecting cold→warm transitions |
| Polisher backoff + health | ~30 | Cold-start dispatch failures |
| Fixer backoff + health | ~30 | Same, duplicated |
| Orchestrator probe/drain | ~45 | Cold sprites failing probes |
| Config knobs | ~10 | Tuning the compensation |
| **Total** | **~350** | **A probe that should just work** |

## What Should Change

### Principle: `Sprite.exec` is the only interface. It handles lifecycle transparently.

The `sprite` CLI already auto-wakes cold sprites. Measured latency: 0.3-2s
for healthy cold sprites. The conductor should trust this and stop trying to
manage sprite lifecycle.

### Delete or Simplify

1. **`Sprite.probe/2`** — Replace with `exec("true", timeout: 45_000)`.
   No separate "probe" concept. If exec works, the sprite is alive.

2. **`Fleet.HealthMonitor`** — Delete or reduce to startup-only retry.
   Its entire purpose is detecting cold→warm transitions that exec handles.

3. **Polisher/Fixer backoff** — Keep simple retry for real failures,
   remove health state tracking (`:healthy`/`:degraded`/`:unavailable`).
   Cold starts don't cause failures, so backoff rarely triggers.

4. **Orchestrator probe-before-dispatch** — Just dispatch. If the sprite
   is cold, exec wakes it. If genuinely dead, the run fails and the
   existing retry machinery handles it.

5. **`Application.boot_fleet` health gating** — Phase workers should
   always start. Don't gate on sprite health at boot. Exec handles cold.

### Keep

1. **Checkpoint GC** — Agent-created checkpoints accumulate and fill disks.
   This is a real problem unrelated to lifecycle states. GC after each run
   + periodic sweep.

2. **Stuck sprite detection** — Sprites can enter an unrecoverable state
   (`connection closed` after 45s, `last_running_at: null`). Detection
   and auto-recreate is valuable. But this is not a "health monitor" —
   it's error handling in the dispatch path.

3. **Provisioning** — Fresh sprites need codex installed, repo cloned,
   git auth configured. The Reconciler's provisioning logic is correct.

## Three Independent Health Models (current, problematic)

The conductor maintains three unsynchronized health trackers for the same sprites:

| Module | Tracks | State |
|--------|--------|-------|
| Orchestrator | builders | `{healthy, drained, consecutive_failures}` |
| HealthMonitor | fixer/polisher | `%{name => :healthy \| :unhealthy}` |
| Fixer/Polisher | themselves | `:healthy \| :degraded \| :unavailable` |

These never synchronize. A sprite can be "healthy" in one model and
"unavailable" in another. The correct answer: one model, or better, no model
at all — just dispatch and handle errors.

## Correct Dispatch Pattern

```
WRONG (current):
  probe sprite → if healthy, dispatch → if probe fails, backoff → HealthMonitor re-probes

RIGHT:
  dispatch → exec auto-wakes cold sprites → if exec fails, classify error:
    - stuck sprite (connection closed) → recreate and retry
    - disk full → GC checkpoints and retry
    - auth expired → re-provision and retry
    - genuine failure → fail the run, move on
```

The dispatch path IS the health check. A separate probe step adds latency,
complexity, and wrong conclusions.

## Fleet Configuration

```toml
# fleet.toml — sprite names match Fly machine names
[[sprite]]
name = "bb-builder"     # Weaver — implements issues
role = "builder"

[[sprite]]
name = "bb-fixer"       # Thorn — fixes failing CI
role = "fixer"

[[sprite]]
name = "bb-polisher"    # Fern — reviews and polishes PRs
role = "polisher"
```

Sprites are managed via `sprite -o misty-step` CLI. The conductor wraps
this through `Conductor.Sprite.exec/3` → `Conductor.Shell.cmd/3`.

## Failure Mode Reference

| Failure | Root cause | Correct fix |
|---------|-----------|-------------|
| Sprite "unreachable" at boot | Cold sprite + 15s probe timeout | Increase timeout or remove probe |
| Polisher backs off indefinitely | Cold start classified as dispatch failure | Don't classify cold as failure |
| 107 checkpoints fill disk | No GC, agent creates per-task | GC after each run, keep last 5 |
| Sprite stuck (connection closed) | Platform can't boot VM | Destroy and recreate |
| Provisioning bash syntax error | Elixir heredoc trailing newline | Fixed in PR #777 |
| Three health models disagree | Architecture doesn't have single source of truth | Delete HealthMonitor, simplify to dispatch-path errors |

## Relevant Issues

- #778 — Sprite lifecycle hardening (needs rewrite with this understanding)
- #779 — Dashboard operator visibility
- #780 — Muse sprite for reflection
