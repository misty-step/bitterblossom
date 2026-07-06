# Docs sync plane template

This is a credential-free-to-validate starter plane for docs drift monitoring.
Two task families, two authority levels, never the same level doing both jobs
(see `docs/rollout-scorecards.md`):

- **`docs-sync`** (`report-only`) watches a product repo and produces
  report-only sync recommendations when docs, runbooks, or operator
  contracts may need updates. It never edits a file.
- **`docs-sync-pr`** (`PR-only`, bitterblossom-120) consumes an existing,
  actionable `docs-sync` report and opens exactly one bounded pull request
  applying its recommended changes. It never investigates from scratch,
  never merges, never deploys, and is scoped to a narrower repo allowlist
  than the report-only watcher. Manual dispatch only -- no cron or webhook
  trigger is wired for this task, matching `canary-remediate`'s (backlog
  115) PR-only precedent.

Validate from a clean checkout:

```bash
cargo build
./target/debug/bb --config examples/docs-sync-plane check --json
./target/debug/bb --config examples/docs-sync-plane task list --json
```

`bb check` does not require live credentials. To dispatch the report-only
watcher for real, edit the example repo/host values, mint the scoped
OpenRouter key for `docs-watcher`, set `GH_TOKEN`, set `BB_HOOK_DOCS_SYNC`,
then run manually, serve webhooks, or let the cron trigger fire:

```bash
./target/debug/bb --config examples/docs-sync-plane keys mint docs-watcher
./target/debug/bb --config examples/docs-sync-plane serve
./target/debug/bb --config examples/docs-sync-plane run docs-sync --payload @samples/github-push-main.json
```

`samples/github-push-main.json` matches the webhook filters. The expected
report shape is in `samples/REPORT.json` (schema `bb.docs_sync.report.v2`);
production agents may add fields, but should preserve `schema`, `repo`,
`trigger`, `changed_files`, `docs_targets`, `drift_findings`,
`recommended_changes`, `skipped_mutations`, `artifacts`, `cost_usd`, and
`residual_risk`.

The PR-only writer is manual-dispatch only and consumes a prior report's run
id:

```bash
./target/debug/bb --config examples/docs-sync-plane keys mint docs-sync-writer
./target/debug/bb --config examples/docs-sync-plane run docs-sync-pr --payload '{"source_report":"<docs-sync run id>"}'
```

Its report shape is in `samples/REPORT-pr.json` (schema
`bb.docs_sync_pr.report.v1`): `schema`, `repo`, `source_report`,
`duplicate_check`, `pr`, `changed_files`, `forbidden_actions_confirmed`,
`artifacts`, `cost_usd`, and `residual_risk`.

Per `docs/rollout-scorecards.md`'s doctrine, `docs-sync-pr`'s first live
dispatch against a real repo needs explicit operator approval naming the
target repo and token -- the ordinary Authority Ladder rule every level
carries. A dedicated bot/app GitHub identity was originally floated as an
additional prerequisite; the operator ruled that path permanently out of
scope (2026-07-05, `bitterblossom-925`), since provisioning one requires
web-UI actions the operator declined to perform. `GH_TOKEN` here is the
operator's own token, scoped as narrowly as the operator chooses at
dispatch time. This template validates and tests green without live
credentials; live PR-only dispatch is a separate, explicitly approved step.
