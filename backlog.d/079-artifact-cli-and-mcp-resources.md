# Make run artifacts first-class for CLI and MCP consumers

Priority: P1 · Status: ready · Estimate: M

## Goal

Let humans and agents inspect run evidence directly through `bb` and MCP instead of spelunking attempt artifact directories.

## Oracle

- [ ] `bb artifacts list <run-id> --json` returns artifact names, relative paths, sizes, content types where knowable, and safety metadata for a run's attempts.
- [ ] `bb artifacts read <run-id> <path>` prints a safe text/JSON artifact such as `REPORT.json`, with binary/oversized artifacts rejected or summarized by contract.
- [ ] `bb artifacts bundle <run-id> --out <path>` is implemented or explicitly deferred with a follow-up if bundling expands scope.
- [ ] MCP exposes artifact resources or tools, e.g. `bb_artifacts_list` and `bb_artifact_read`, backed by the same helper as CLI.
- [ ] The portable skill requires artifact inspection in closeout for any run that claims success.
- [ ] Tests cover successful `REPORT.json` read, missing artifact, path traversal rejection, and multi-attempt behavior.
- [ ] `./scripts/verify.sh` passes.

## Verification System

- Claim: a consuming agent can verify BB output by reading artifacts through public interfaces, not local path archaeology.
- Falsifier: agent must infer attempt directory layout; traversal can escape artifact root; binary output floods stdout; or MCP/CLI disagree.
- Driver: local-plane run producing `REPORT.json`, then CLI and MCP artifact reads.
- Grader: artifact content matches file on disk; unsafe paths fail; missing artifacts produce structured errors.
- Evidence packet: command transcript and run id.
- Cadence: artifact contract test joins the agent-interface gate.

## Notes

Why: run state says a task finished; artifacts prove whether it did useful work. This is especially important for unsupervised report-only flows such as Canary triage and backlog-chewer dry runs.
