# CI audit plane template

This is a credential-free-to-validate starter plane for proactive CI-gate
auditing. It is deliberately distinct from `ci-diagnose` (see
`tests/fixtures/public-plane/tasks/ci-diagnose/`): `ci-diagnose` reacts to one
already-failed CI signal named in an incoming webhook; this plane's
`ci-audit` proactively inspects a repo's own gates, tests, and lints on a
daily cron or manual dispatch, whether or not anything just failed, looking
for what is missing or weak.

Two task families, two authority levels, never the same level doing both
jobs (see `docs/rollout-scorecards.md`):

- **`ci-audit`** (`report-only`) audits one explicitly allowlisted repo's
  own CI and writes a report-only hardening recommendation. It never edits a
  file.
- **`ci-audit-pr`** (`PR-only`, bitterblossom-121) consumes an existing,
  actionable `ci-audit` report and opens exactly one bounded CI-hardening
  pull request. It never audits from scratch, never merges, never deploys,
  and -- its one absolute red line -- never weakens, loosens, skips, or
  removes an existing gate. Manual dispatch only, matching
  `canary-remediate`'s (backlog 115) and `docs-sync-pr`'s (backlog 120)
  PR-only precedent.

Validate from a clean checkout:

```bash
cargo build
./target/debug/bb --config examples/ci-audit-plane check --json
./target/debug/bb --config examples/ci-audit-plane task list --json
```

`bb check` does not require live credentials. To dispatch the auditor for
real, edit the example repo/host values, mint the scoped OpenRouter key for
`ci-auditor`, set `GH_TOKEN`, then run manually against one allowlisted repo
or let the daily cron audit every allowlisted repo in turn:

```bash
./target/debug/bb --config examples/ci-audit-plane keys mint ci-auditor
./target/debug/bb --config examples/ci-audit-plane serve
./target/debug/bb --config examples/ci-audit-plane run ci-audit --payload @samples/manual-audit-payload.json
```

`samples/manual-audit-payload.json` names the repo to audit; it must match a
repo in the task's `workspace.repos` allowlist or the payload is refused
before the audit runs. The expected report shape is in `samples/REPORT.json`
(schema `bb.ci_audit.report.v1`): `schema`, `repo`, `trigger`,
`current_gates`, `missing_or_weak_gates`, `proposed_checks`, `risk`,
`reproduction_commands`, `artifacts`, `cost_usd`, and `residual_risk`.

The PR-only hardener is manual-dispatch only and consumes a prior report's
run id:

```bash
./target/debug/bb --config examples/ci-audit-plane keys mint ci-hardener
./target/debug/bb --config examples/ci-audit-plane run ci-audit-pr --payload '{"source_report":"<ci-audit run id>"}'
```

Its report shape is in `samples/REPORT-pr.json` (schema
`bb.ci_audit_pr.report.v1`): `schema`, `repo`, `source_report`,
`duplicate_check`, `pr`, `gates_added`, `gates_weakened` (must always be an
empty array), `artifacts`, `cost_usd`, and `residual_risk`.

Per `docs/rollout-scorecards.md`'s doctrine, `ci-audit-pr`'s first live
dispatch against a real repo needs explicit operator approval naming the
target repo and token -- the ordinary Authority Ladder rule every level
carries. A dedicated bot/app GitHub identity was originally floated as an
additional prerequisite; the operator ruled that path permanently out of
scope (2026-07-05, `bitterblossom-925`), since provisioning one requires
web-UI actions the operator declined to perform. `GH_TOKEN` here is the
operator's own token, scoped as narrowly as the operator chooses at
dispatch time. This template validates and tests green without live
credentials; live PR-only dispatch is a separate, explicitly approved step.
