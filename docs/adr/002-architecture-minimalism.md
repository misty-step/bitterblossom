# ADR-002: Architecture Minimalism — Thin CLI, Thick Skills

- **Status:** Accepted
- **Date:** 2026-02-15
- **Related:** ADR-001 (Claude Code as canonical harness)

## Context

Bitterblossom v1 grew to ~42K LOC across 8 internal packages (`agent`, `dispatch`, `fleet`, `lifecycle`, `provider`, `monitor`, `watchdog`, `contracts`), plus `pkg/fly` and `pkg/events`. The CLI surface had 10+ commands. Most of this complexity existed to work around limitations of running agents via SSH on Fly Machines.

The Sprites Go SDK (`github.com/superfly/sprites-go`) eliminated those limitations: native filesystem access, streaming command execution, token exchange, and first-class Go types. With the SDK, the entire dispatch pipeline collapsed from ~2,500 LOC across 4 packages to a single function.

Three architectures were evaluated:

1. **Pure skills (0 LOC Go):** Replace `bb` entirely with Claude Code skills using the Bash tool. Blocked by: 600-second Bash tool timeout (ralph loops run 30+ minutes), no streaming stdout, no credential isolation, no parallelism.

2. **Hybrid (keep dispatch in Go, skills for intelligence):** The approach we chose. Go handles transport (connectivity probes, file upload, streaming execution, exit codes). Skills handle intelligence (prompt rendering, persona selection, repo analysis).

3. **Full CLI (status quo):** Keep all 42K LOC. Rejected: the SDK makes 90% of it redundant. Fleet composition, watchdog, event systems, transport fallbacks — all solved better by the SDK or by skills.

## Decision

**Keep `bb` as thin deterministic transport (<1k LOC).** Intelligence lives in Claude Code skills and the ralph loop script. Don't add features to `bb` — if logic requires judgment, it belongs in a skill.

The CLI stays small:

| Command | Purpose |
|---------|---------|
| `dispatch` | Probe, sync, upload prompt, run ralph loop |
| `setup` | Configure sprite with configs, persona, ralph script |
| `logs` | Stream agent output from the sprite |
| `status` | Fleet overview or single sprite detail |
| `version` | Print version |

Plus `scripts/ralph.sh`: the iteration loop that invokes the agent harness, checks signal files, enforces time/iteration limits.

## Rationale

1. **Streaming.** Ralph loops run 30+ minutes. The Bash tool caps at 600 seconds. Go can stream stdout/stderr from the sprite SDK indefinitely.

2. **Credential isolation.** `setup` bakes LLM keys into `settings.json` on the sprite once. Dispatch only passes `GITHUB_TOKEN` at runtime. Skills would need to shuttle secrets through environment variables on every invocation.

3. **Deterministic transport.** Connectivity probes, stale signal cleanup, process kill, repo sync — these are mechanical operations that must succeed identically every time. Go gives us typed errors and explicit exit codes. Shell scripts in skills would introduce fragility.

4. **Parallelism.** Fleet status probes all sprites concurrently (3-second timeout each). A skill running sequential Bash commands would take N * 3 seconds instead of 3 seconds.

5. **Exit code semantics.** Dispatch returns 0 (success), 1 (failure), 2 (blocked). Callers (CI, other agents) depend on these. Skills can't control process exit codes.

## Consequences

- **Don't add Go packages.** No `internal/` directory. All logic lives in `cmd/bb/`.
- **Avoid new commands.** If you're tempted to add `bb foo`, write a skill instead.
- **Skills own intelligence.** Prompt rendering, persona selection, task decomposition, PR review — these belong in Claude Code skills, not Go code.
- **Ralph loop is sacred.** The bash script is the core value proposition. Changes require careful review.
- **Test at integration level.** With a <1k LOC CLI and no internal packages, unit tests add less value than e2e dispatch tests against real sprites.
