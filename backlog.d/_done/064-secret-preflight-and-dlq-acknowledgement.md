# Make secret preflight and DLQ acknowledgement first-class

Priority: P1 | Status: ready | Estimate: M

## Goal

Prevent avoidable BB storm fanout failures and give operators a durable way to
close superseded pre-execute dead letters without replaying them.

## Oracle

- [ ] `bb run` or a new preflight command can report missing declared secrets
      and unspawnable command-harness binaries for one task or a
      submission-storm member set before dispatch creates run rows.
- [ ] `bb dlq list --json` distinguishes open, replayed, and acknowledged
      dead letters with acknowledgement reason and timestamp.
- [ ] Operators can acknowledge a pre-execute DLQ only with an explicit reason;
      replay history remains immutable.
- [ ] `bb status --json` no longer makes superseded acknowledged DLQs look like
      unresolved operator work.
- [ ] Tests cover missing-secret preflight, acknowledgement persistence,
      status grouping, and replay rejection after acknowledgement.
- [ ] `./scripts/verify.sh` passes.

## Notes

Dogfood source: PR #858 final review accidentally launched a submission storm
without `GH_TOKEN`. Five runs dead-lettered before execution, a replacement
submission later passed, and the original DLQs remain open because BB has no
acknowledge/resolve path for superseded pre-execute failures.

Dogfood source: 2026-06-17 PR-reflex-storm local webhook drill used a temporary
command harness with `bin = "/bin/true"` on macOS. The webhook correctly created
the review run, submission, and storm member rows, but every execution failed
pre-execute because `/bin/true` was not present. A preflight should catch this
class before a storm creates multiple doomed runs.

Keep the Rust spine small: this is operator-state plumbing, not workload
judgment. Do not make acknowledgement hide failures by default; every
acknowledgement needs an operator reason.
