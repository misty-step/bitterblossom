# Adopt Cerberus M1/M2 hardening (scoped keys, container isolation) in the review wrapper

Priority: P2 | Status: ready | Estimate: M

## Goal

`cerberus-review-wrapper.sh` should use Cerberus's scoped-key and
container-isolation flags once it is reviewing PRs whose diffs are not fully
trusted, instead of forwarding the plane's real long-lived
`OPENROUTER_API_KEY` via `--allow-env`.

## Context

Cerberus's own README is explicit about this: "A plain `--allow-env
OPENROUTER_API_KEY` review gives the substrate (which has shell and webfetch
access) your real, long-lived OpenRouter key and an unrestricted network — a
prompt-injected PR can exfiltrate both." Cerberus ships the fix already and
per the fleet assessment (`~/.factory-lanes/assess/cerberus.md`, 2026-07-02)
both are "live-verified, production-enabled": M1 (`--openrouter-scoped-key`,
per-review capped/revocable keys) and M2 (`--harness container-opencode`,
Docker-isolated substrate with CONNECT-only egress).

`scripts/cerberus-review-wrapper.sh` (the one BB actually dispatches) does
neither today — it forwards the raw key and runs on the plain `opencode`/`omp`
substrate. The `review` task's whitelist is currently misty-step-org-only
(trusted, first-party diffs), so this is not an active incident, but it's the
known gap standing between "advisory review of our own repos" and "safe to
point at externally-contributed PRs."

## Oracle

- [ ] Wrapper mints a scoped OpenRouter key per review via
      `--openrouter-scoped-key` instead of forwarding `OPENROUTER_API_KEY`
      wholesale, using the M1 provisioning path cerberus already documents.
- [ ] Wrapper runs the harness through `--harness container-opencode` (or the
      current M2 flag name) so diff exploration happens in the isolated
      substrate.
- [ ] A real review still succeeds end-to-end with both flags on (measurement
      mode against a real PR is sufficient proof, no new comment required).
- [ ] Cost/latency delta from the extra isolation is measured once and noted
      here or in `docs/model-evals/review/`.

## Non-goals

Do not change cerberus itself — this is a BB-side wrapper/wiring change
consuming flags cerberus already ships. Do not widen the reviewed-repo
whitelist beyond misty-step/phrazzld until this lands if any target repo
might carry untrusted external contributions.
