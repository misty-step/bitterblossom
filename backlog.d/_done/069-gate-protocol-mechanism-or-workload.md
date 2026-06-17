# Resolved: the submission/gate protocol is a plane primitive (mechanism)

Priority: P1 | Status: done | Estimate: M

## Resolution (2026-06-17)

The gate protocol in `src/submit.rs` is **mechanism, not workload** — a generic
multi-member, multi-round, fingerprint-stable, arbiter-overridable gate any
gating workload (security audit, docs compliance) could reuse unchanged. There
is **no ~300-line extraction**; the groom's earlier estimate was wrong.
Extracting the thin review-shaped skin would be a net loss. The cap reflects the
outcome with a conscious re-baseline **up** (5100 → 6000), justified by this
audit — not a drop.

## Evidence

Two independent reads converged — the lead, and a fresh-context adversarial
critic explicitly tasked to argue the workload side as hard as the code allows:

- **Already config-driven.** The things that *would* be workload judgment if
  hardcoded are all `GateSpec` config resolved through opaque `verdict` kinds:
  membership (`GateSpec.required`, `spec.rs:56`), arbiter (`gate.arbiter`,
  `spec.rs:59`), round cap (`gate.max_rounds`, `spec.rs:57`). A required kind
  must be declared by exactly one task's `verdict = "..."` (`spec.rs:482-499`);
  adding/removing a reviewer is a TOML + card edit — zero `src/` lines.
- **The review-specific skin is ~3 lines** — the closed vocabularies
  `SUBMISSION_STATES`/`VERDICTS`/`SEVERITIES` (`submit.rs:7,9,10`). These are
  generic gate language (pass/blocking/advisory, open/clear/blocked/escalated),
  not code-review lore. `grep -ni 'review|pr|diff|lint|security'` over
  `submit.rs` returns zero hits.
- **The rest is generic mechanism** — the submission state machine
  (`open`/`settle_submission`), verdict storage (`record_verdict`/`verdicts`),
  fingerprint dedup (`fingerprint`/`known_fingerprints`/`enforce_fingerprints`),
  rejection tracking, the arbiter-sustain query, and the `evaluate` decision
  ladder (`submit.rs:411-539`: infra_failure → pending → clear →
  blocked/escalated). Swap `gate.required` to `["sast","dep-audit","secrets"]`
  and it is a security gate with no Rust change.
- **Keeping the vocab in Rust is correct.** `parse_verdict` enforcing the fixed
  vocabulary is the spine's boundary contract against malformed agent output: a
  hard, fast, mechanical check a thin spine *should* own. Config-izing it would
  grow `src/` (against the invariant) to buy nothing concrete.

## Consequence

069 does **not** free LOC headroom. 064 (+~70) and 053 (+~120-250) need a
conscious cap re-baseline — now justified, because the lean audit `verify.sh`
demands is done. The cap is reframed from a tight per-feature budget into a
bloat tripwire (CLAUDE.md, `scripts/verify.sh`) and raised to 6000 so it rings
only on genuine bloat; the per-module breakdown is the finer signal.

## Watch-item (from the critic)

`dispatch.rs:286-328` switches only on `harness == "command"` and the *presence*
of `task.spec.verdict` — both generic. If a future commit branches it on a
*specific* verdict kind (`if kind == "security"`), that is workload judgment
crossing into the spine. Hold that line.
