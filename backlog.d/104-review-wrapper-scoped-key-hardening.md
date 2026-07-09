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

## Prior Status (2026-07-02)

An adjacent governance/observability slice first gave Cerberus its own
persistent OpenRouter key, but that was not the M1/M2 hardening described here:
it still left the review substrate on raw `--allow-env OPENROUTER_API_KEY` and
plain `opencode`.

That bridge is superseded by the implementation note below. The review task now
uses BB's policy-bound provider key injection plus Cerberus per-review key
minting and container isolation.

## Implementation Note

The wrapper now consumes Cerberus M1/M2 by default: it passes
`--openrouter-scoped-key` with an explicit provisioning env name, refuses to run
without that env, never forwards `--allow-env OPENROUTER_API_KEY`, and selects
`--harness container-opencode` with model-only egress (`openrouter.ai:443`).

The BB-side `cerberus-reviewer` agent is policy-bound so dispatch injects the
stored, spend-capped per-workload-family OpenRouter key as `OPENROUTER_API_KEY`.
Cerberus uses that only to mint/revoke a per-review child key for the contained
review substrate.

## Update (2026-07-09, bitterblossom-942)

The "real review still succeeds end-to-end with both flags on" oracle item
was never actually true in production: every prior review run failed earlier
(GH-token, then an unrelated OpenRouter provisioning-key type mismatch fixed
under bitterblossom-942), so `--harness container-opencode` was never
exercised live. Once those earlier blockers were fixed, dispatch reached the
container-opencode step and failed immediately: **the `lane-1` sprite host has
neither `docker` nor `opencode` on `PATH`** (`sprite exec -- which docker
opencode` both fail). M2 Docker isolation cannot function on this substrate
as configured.

Interim (applied live, not a permanent fix): `cerberus-reviewer.toml` now
declares `CERBERUS_HARNESS` as a secrets-passthrough name, set to `omp` in the
live app spec -- the wrapper's already-tested, already-supported non-container
harness path (`cerberus_wrapper_can_override_to_omp_for_trusted_compatibility`
in `tests/cerberus_wrapper.rs`). `omp` and `bun` are present on `lane-1`;
`opencode` and `docker` are not. This is not a security regression versus the
documented pre-M2 baseline (`trusted, first-party diffs` whitelist, "not an
active incident") -- it makes today's already-unhardened reality (container
mode was silently broken, not silently secure) explicit and working, rather
than accidentally broken.

Remaining oracle for this card: provision a Docker-capable substrate (or
confirm sprites cannot nest containers and choose a different host) so M2
isolation can actually run, then flip `CERBERUS_HARNESS` back off (removing
the override reverts to the wrapper's `container-opencode` default).

**`omp` fallback is also currently broken, not just `container-opencode`.**
Two live runs against `omp` (`5d35f46063fa`, `54507e408872`) both failed
deterministically in ~7-10s -- too fast for an actual model call -- with
`Error: read emitted review artifact /tmp/cerberus-*/packet/review-artifact.json
... No such file or directory`. This points at `omp` itself failing before or
during the model call (likely a missing `omp`-specific auth/config on
`lane-1` that the per-exec secret injection doesn't cover), not at anything
in Bitterblossom's dispatch/wrapper layer -- both runs got cleanly past the
OpenRouter provisioning-key step with no related warnings or errors. This is
a **third, separate, still-open gap**: neither harness path (`container-opencode`:
missing docker/opencode; `omp`: fails before writing the review artifact)
currently produces a working end-to-end review on `lane-1`. Diagnosing the
`omp` failure needs Cerberus-side investigation (its own repo) into what
`omp` requires at runtime that isn't present in this sprite's per-exec
environment -- out of scope for a Bitterblossom-side patch.

## Update (2026-07-09, bitterblossom-942/940 restoration): root cause found, it IS a BB-side fix

Reproduced the exact `omp` failure locally with the same CLI flags the
wrapper uses (`cerberus review-pr --harness omp --openrouter-scoped-key
--openrouter-provisioning-key-env ... --dry-run`), against the same live PR
(#984). The failure has nothing to do with sprite-host auth/config -- it is
that `cerberus-review-wrapper.sh` never passes `--model`, because
`CERBERUS_MODEL` is not declared as a secrets-passthrough name anywhere, so
`omp` resolves its own bare default. Observed two flavors of the same root
cause depending on which bare model `omp` picked:

- A fuzzy-matched non-OpenRouter provider ("kilo"), which has no credentials
  in Cerberus's scrubbed child env: `error: No API key found for kilo.`
- An OpenRouter model whose default `max_tokens` (65536) the $1.25-capped
  scoped key can't afford: `402 ... You requested up to 65536 tokens, but
  can only afford 10000.`

Either way, `omp` exits non-zero before writing `review-artifact.json`,
which is exactly the "no such file" error dispatch surfaces -- the true
error (visible only in Cerberus's own `--receipt-bundle`/transcript, which
the wrapper does not forward into BB's ledger) never reaches `bb runs show`.

Fix (confirmed live end-to-end against PR #984 in `--dry-run`, cost within
the existing $1.25 cap): explicitly pin an `openrouter/`-prefixed model.
`--model openrouter/z-ai/glm-5.2` produces a complete, valid review verdict
and post-plan. Applied live: `cerberus-reviewer.toml` now declares
`CERBERUS_MODEL` as a secrets-passthrough name (matching the `CERBERUS_HARNESS`
pattern), set to `openrouter/z-ai/glm-5.2` in the live app spec.

This closes the "omp fallback is also broken" gap above; it does not touch
the separate, still-open `container-opencode` docker/opencode gap.
