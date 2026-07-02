# Persist a head-version watermark so post-settle freshness survives

Priority: P2 | Status: done | Estimate: S

## Goal

Make webhook submission-storm freshness correct for the rare out-of-order
delivery that arrives *after* a submission has settled, and stop overloading
`submissions.report_json` to carry the freshness version.

## Oracle

- [x] A submission stores the head freshness version (`pull_request/updated_at`)
      in a dedicated field that survives `settle_submission`, not in
      `report_json` (which `settle_submission` overwrites with the gate report).
- [x] An out-of-order delivery for an older head that arrives after the latest
      submission for that change has settled is rejected (no new submission, no
      storm) when its version is not strictly newer than the last processed head.
- [x] A genuinely newer head after a settled round still opens the next
      submission and storm.
- [x] Tests cover: settle → stale older-head redelivery (no-op) and settle →
      newer-head delivery (opens + storms).
- [x] `./scripts/verify.sh` passes.

## Notes

Source: PR #860 review (2026-06-17). The targeted fix made `open_webhook_submission`
idempotent for the common case — a redelivery of an already-processed head is a
no-op whether the latest submission is open or settled (rev-equality check), and
the existing version compare still guards the open-submission supersede path. The
residual gap: once the latest submission has *settled*, a different-rev delivery
opens a new storm using only rev inequality, because the freshness version is
stored in `report_json` on the open row and `settle_submission` overwrites it.
A late, out-of-order delivery for an *older* head would therefore re-storm stale
code.

Keep the spine small: this is a single ledger column plus a comparison move out
of `report_json`, not a new lifecycle. Do not add a version-history table.
See `src/ingress.rs` `open_webhook_submission` / `version_is_newer` and
`src/submit.rs` `open_submission` / `settle_submission`.

## Delivery Notes

### 2026-07-02

- Added `submissions.head_version` and a migration for existing ledgers.
- Moved webhook submission-storm freshness writes and comparisons off
  `report_json`; `report_json` remains the gate-report field after settlement.
- Added regression coverage for stale older-head delivery after settle and
  genuinely newer-head delivery after settle.
- Focused verification:
  `cargo test --test ingress webhook_submission_storm_ -- --nocapture`.
