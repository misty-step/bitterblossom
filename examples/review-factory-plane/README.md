# Review factory plane template

This is a credential-free-to-validate starter plane for a pull-request
review factory. It shows the production shape without carrying any
operator-owned task cards, budgets, repo allowlists, or ledgers from a real
plane.

Validate from a clean checkout:

```bash
cargo build
./target/debug/bb --config examples/review-factory-plane check --json
./target/debug/bb --config examples/review-factory-plane task list --json
```

`bb check` does not require live credentials. To dispatch this template for
real, edit the example org/repo/host values, mint the scoped OpenRouter key
for `reviewer`, set `GH_TOKEN`, set `BB_HOOK_REVIEW_FACTORY`, then run or
serve the task:

```bash
./target/debug/bb --config examples/review-factory-plane keys mint reviewer
./target/debug/bb --config examples/review-factory-plane serve
./target/debug/bb --config examples/review-factory-plane run review --payload @samples/github-pull-request-opened.json
```

The webhook example in `samples/github-pull-request-opened.json` matches the
task filters. The expected agent output shape is shown in
`samples/REPORT.json`; production agents may add fields, but should preserve
the top-level verdict, findings, comment policy, and residual-risk fields so
operators can compare review runs consistently.
