# Design Contract Provenance

This table records the facts behind Bitterblossom's `noir-ledger` visual
contract. It is evidence for future agents, not a second token source.

| Source | Fact | Provenance | Confidence | Use | Evidence / Notes |
|---|---|---|---|---|---|
| `backlog.d/074-adopt-misty-step-comic-ops-aesthetic.md` | Adopt the `noir-ledger` comic-ops flavor for dispatch, run ledger, readiness, and receipt surfaces. | provided | high | keep | Ticket names ledgers, proof strips, caption bands, and hard square panels. |
| `src/operator.html` | The first maintained visual surface is a static operator dashboard served by `bb serve`. | observed | high | keep | The file consumes `/api/status`, `/api/tasks`, `/api/runs`, `/api/dlq`, `/api/leases`, `/api/ingress`, and `/api/export`. |
| `docs/plans/2026-07-01-072-bb-dashboard-design.html` | Dashboard density, rail plus desk structure, ledger grid, compact tables, and neutral palette are already the operator-dashboard direction. | observed | high | keep | This 074 slice carries that shape into the durable design contract and live HTML. |
| `http://serenity.tail5f5eb4.ts.net:8788/bitterblossom-noir-ledger-concept.png` | Noir-ledger reference board is visual direction, not a source to clone. | provided | medium | do-not-copy | Network/Tailscale availability can vary in overnight mode; the ticket gives the direction in text, so the implementation uses local repo evidence. |
| `@misty-step/aesthetic` commit `9bbe0f9` or later | Package adoption is deferred for this Rust-only static HTML surface. | inferred | medium | change | runtime import deferred: the repo has no package manager or frontend build step. If a maintained JS UI is introduced, adopt `@misty-step/aesthetic` at `9bbe0f9` or later instead of copying tokens by hand. |
| `DESIGN.md` | Token roles, square panels, proof strips, and caption bands are the stable BB visual contract. | observed | high | keep | Validate the contract when UI-facing changes touch durable tokens or component grammar. |
