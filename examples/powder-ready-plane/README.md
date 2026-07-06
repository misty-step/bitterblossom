# Powder ready-ticket dispatch plane template

Starter plane for bitterblossom-931 pilot (a): a Powder card moving to
`ready` auto-dispatches a roster-briefed builder agent, end to end, with a
full ledger trace. Production-shaped, all repos/hosts/secrets are examples
for a runtime copy.

Validate from a clean checkout:

```bash
cargo build
./target/debug/bb --config examples/powder-ready-plane check --json
./target/debug/bb --config examples/powder-ready-plane task list --json
```

`bb check` does not require live credentials. To dispatch this template for
real:

1. Register a Powder event subscription pointed at this plane's public
   ingress URL, filtered to `moved-to-ready`:
   `mcp__powder__create_event_subscription` (or the Powder HTTP API) with
   `url = "https://<your-plane-host>/hooks/powder-ready"` and
   `event_filter = ["moved-to-ready"]`. Powder returns a one-time signing
   secret — store it as `BB_HOOK_POWDER_READY`.
2. Set `OPENROUTER_API_KEY`, `POWDER_API_BASE_URL`, `POWDER_API_KEY` (a
   scoped key the dispatched agent uses to claim/comment/complete the card
   via its own powder MCP — see `tasks/dispatch-ready/card.md`).
3. `./target/debug/bb --config examples/powder-ready-plane serve`

## Why this shape

- Powder signs webhook deliveries with a single `X-Signature-256` header
  over the raw body (`sha256=hex(hmac(secret, body))`) and sends no
  delivery-id header (see `crates/powder-server/src/main.rs` in the powder
  repo). That is bb's plain-body HMAC fallback path
  (`src/ingress.rs::verify_delivery_hmac`), already generic — no new
  ingress code was needed. Dedupe therefore keys on the envelope's own
  `event_id` (`dedupe_key = "json:/event_id"`), not a header.
- The trigger filters on `schema_version` (contract pin), `event_type`
  (`moved-to-ready` only — other lifecycle events like `comment-added` are
  filtered, ack'd 200, no run), and `card.repo` (scope to the repos this
  plane instance is meant to dogfood; broadening the scope is a one-line
  config edit, not a code change — bitterblossom-931's "easy to change"
  acceptance bar).
- The agent is hand-authored (not `[roster]`-materialized) so its
  `policy.trigger_bindings` can correctly declare `["manual", "webhook"]`;
  `render_bb_agent` in vendor/roster always emits `trigger_bindings =
  ["manual"]`, which would be stale metadata on a webhook-bound task. The
  task instead carries `[roster_brief]` so the roster `builder` role's full
  commission, skills, and permissions are prepended to the card at dispatch
  — same provenance, correct declared bindings. Model/harness mirror the
  existing `canary-triager` reflex agent (`pi` + `deepseek/deepseek-v4-flash`,
  `auth = "api"`), the only auth class bb permits on webhook/cron triggers.

`samples/powder-moved-to-ready.json` matches the pinned
`powder.card_event.v1` shape used by Bitterblossom's contract tests
(`tests/powder_ready_contract.rs`, `tests/ingress.rs`).
