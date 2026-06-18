# Make subscription-auth builder readiness visible before dispatch

Priority: P1 | Status: ready | Estimate: S

## Goal

Stop `bb run build` from being the first place an operator learns that the
remote subscription-auth harness is unusable on the sprite.

## Oracle

- [ ] A preflight/readiness path for manual subscription-auth tasks reports the
      concrete auth state for the configured harness on the configured
      substrate host before the operator spends a build run.
- [ ] The output names the task, host, harness binary, model, and exact
      remediation when Codex/Claude auth is expired or refresh fails.
- [ ] `bb status --json` or the new readiness command makes the failed state
      machine-readable enough for agents to stop before dispatch.
- [ ] A failing readiness check does not create a normal authoring run row; if
      it records evidence, it is clearly classified as readiness/preflight, not
      a failed implementation attempt.
- [ ] Tests cover a stubbed subscription-auth refresh failure and preserve the
      existing dispatch behavior for healthy tasks.
- [ ] `./scripts/verify.sh` passes.

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
