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

## Status (2026-07-02)

Adjacent, narrower slice landed first: cerberus now has its own persistent,
attributable OpenRouter key (governance/observability, not the M1 per-review
scoped-key minting this ticket describes) — PR #940. `CERBERUS_OPENROUTER_API_KEY`
is a Fly secret on `bitterblossom-plane`, and `cerberus-review-wrapper.sh`
prefers it over the shared `OPENROUTER_API_KEY`, falling back to the shared
key when unset. Verified live: the wrapper resolves the real 1Password-stored
key (`OPENROUTER_API_KEY__cerberus`, OpenRouter name `app:cerberus`, $1500
cap) correctly ahead of a decoy shared key.

**Not yet done, blocking full effect:** the review task's agent config on the
production plane (`plane/agents/cerberus-reviewer.toml`, excised from git
tracking, lives only on the `bitterblossom-plane` Fly volume) still declares
`secrets = ["GH_TOKEN", "OPENROUTER_API_KEY"]` — `CERBERUS_OPENROUTER_API_KEY`
needs to be added to that list for the value to actually reach the review
sprite at dispatch time. Editing that file requires operator SSH per
`docs/lifecycle-orchestrator-authority.md`; not done here because that
authority boundary is explicit and this session didn't have (or seek) the
override. Until that line lands, review runs still resolve `OPENROUTER_API_KEY`
from the shared ambient env, not the dedicated key.

This ticket's full scope (M1 `--openrouter-scoped-key` per-review minting +
M2 `--harness container-opencode` isolation) is unchanged and still open.
