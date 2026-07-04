# Hygiene reflex plane template

This is a production-shaped starter plane for repository hygiene. It defines two
report-first workloads:

- `branch-prune`: lists remote branches fully merged into the default branch and
  reports what would be deleted.
- `dependabot-triage`: lists open Dependabot PRs and reports conservative
  merge-on-green candidates.

Validate from a clean checkout:

```bash
cargo build
./target/debug/bb --config examples/hygiene-plane check
./target/debug/bb --config examples/hygiene-plane task list --json
```

`bb check` does not require live credentials. Real dispatch requires `GH_TOKEN`
as a declared secret and a payload that names the configured repos:

```bash
GH_TOKEN=$(gh auth token) ./target/debug/bb --config examples/hygiene-plane run branch-prune --payload-file samples/branch-prune-event.json --json
GH_TOKEN=$(gh auth token) ./target/debug/bb --config examples/hygiene-plane run dependabot-triage --payload-file samples/dependabot-triage-event.json --json
```

Both workloads default to report-only and write `REPORT.json`. Deletion and
merge modes are intentionally inert unless the payload mode, per-repo config
flag, and named environment graduation flag are all set.
