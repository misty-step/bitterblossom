# Docs sync plane template

This is a credential-free-to-validate starter plane for docs drift monitoring.
It watches a product repo and produces report-only recommendations when docs,
runbooks, or operator contracts may need updates.

Validate from a clean checkout:

```bash
cargo build
./target/debug/bb --config examples/docs-sync-plane check --json
./target/debug/bb --config examples/docs-sync-plane task list --json
```

`bb check` does not require live credentials. To dispatch this template for
real, edit the example repo/host values, mint the scoped OpenRouter key for
`docs-watcher`, set `GH_TOKEN`, set `BB_HOOK_DOCS_SYNC`, then run manually,
serve webhooks, or let the cron trigger fire:

```bash
./target/debug/bb --config examples/docs-sync-plane keys mint docs-watcher
./target/debug/bb --config examples/docs-sync-plane serve
./target/debug/bb --config examples/docs-sync-plane run docs-sync --payload @samples/github-push-main.json
```

`samples/github-push-main.json` matches the webhook filters. The expected
report shape is in `samples/REPORT.json`; production agents may add fields,
but should preserve the repo, source revision, drift findings, recommended
changes, skipped mutations, and residual risk fields.
