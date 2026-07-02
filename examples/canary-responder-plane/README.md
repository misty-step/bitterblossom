# Canary incident responder plane template

This is a credential-free-to-validate starter plane for report-only incident
response from Canary-style wake-up events. It is production-shaped, but all
orgs, services, repos, hosts, secrets, and budgets are examples for a runtime
copy.

Validate from a clean checkout:

```bash
cargo build
./target/debug/bb --config examples/canary-responder-plane check --json
./target/debug/bb --config examples/canary-responder-plane task list --json
```

`bb check` does not require live credentials. To dispatch this template for
real, edit the example service/repo/host values, mint the scoped OpenRouter key
for `incident-responder`, set `CANARY_ENDPOINT`, `CANARY_API_KEY`, and
`BB_HOOK_CANARY_INCIDENT`, then serve the plane:

```bash
./target/debug/bb --config examples/canary-responder-plane keys mint incident-responder
./target/debug/bb --config examples/canary-responder-plane serve
```

`samples/canary-incident-opened.json` matches the webhook filters and the
pinned `canary.incident_event.v1` shape used by Bitterblossom tests. The
expected report shape is in `samples/REPORT.json`; production agents may add
fields, but should preserve the incident identity, evidence, hypotheses,
recommended actions, and residual uncertainty fields.
