# Make subscription-auth builder readiness visible before dispatch

Priority: P1 | Status: done | Estimate: S

## Goal

Stop `bb run build` from being the first place an operator learns that the
remote subscription-auth harness is unusable on the sprite.

## Oracle

- [x] A preflight/readiness path for manual subscription-auth tasks reports the
      concrete auth state for the configured harness on the configured
      substrate host before the operator spends a build run.
- [x] The output names the task, host, harness binary, model, and exact
      remediation when Codex/Claude auth is expired or refresh fails.
- [x] `bb status --json` or the new readiness command makes the failed state
      machine-readable enough for agents to stop before dispatch.
- [x] A failing readiness check does not create a normal authoring run row; if
      it records evidence, it is clearly classified as readiness/preflight, not
      a failed implementation attempt.
- [x] Tests cover a stubbed subscription-auth refresh failure and preserve the
      existing dispatch behavior for healthy tasks.
- [x] `./scripts/verify.sh` passes.

## Notes

Dogfood source: 2026-06-18 `bb-dogfood` on backlog 070. Preflight showed the
latest `build` run had already failed with Codex `refresh_token_reused`, but
there was no current readiness proof. Running:

```bash
GH_TOKEN=$(gh auth token) ./target/debug/bb --config plane run build \
  --payload '{"repo":"misty-step/bitterblossom","backlog":"backlog.d/070-gate-blocked-fix-prompt-reflex.md","branch_slug":"070-fix-prompt-reflex","dry_run":false}' \
  --json
```

created run `380ca26ed25b`, then failed in 17.6s with the same expired Codex
subscription auth on the sprite. Artifact stderr:
`plane/.bb/runs/380ca26ed25b/attempt-1/stderr.txt`.

Related but distinct from backlog 064: 064 covers declared secrets, command
availability, and DLQ acknowledgement. This ticket covers subscription-auth
harness readiness for manual builder tasks, where the credential lives on the
remote harness substrate rather than in the plane's declared secret set.

Delivered 2026-07-02: `bb preflight <task> --json` now emits classified
subscription-auth readiness findings for manual-only Codex/Claude agents before
ledger ingest. The readiness probe is operator/substrate supplied through
`BB_PREFLIGHT_SUBSCRIPTION_AUTH_PROBE_<HARNESS>` or
`BB_PREFLIGHT_SUBSCRIPTION_AUTH_PROBE`; the plane passes task, host, substrate,
harness, bin, and model to the probe and records non-zero output as
`subscription_auth_unready` with remediation. No-probe subscription tasks fail
visible as `subscription_auth_unverified` instead of silently discovering auth
state through an implementation run. Verification includes a stubbed
`refresh_token_reused` probe failure that leaves the run ledger empty plus a
local subscription-harness dispatch test proving preflight does not interpose on
normal `bb run` behavior. Overnight guardrail: no live sprite dispatch was run;
the current checked-in `build` task is API-auth OMP/GLM, so this closes the
Codex/Claude subscription regression surface rather than changing current
builder routing.
