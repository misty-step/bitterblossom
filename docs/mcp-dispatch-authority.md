# MCP Dispatch Authority

Date: 2026-07-05
Backlog: bitterblossom-116

## Decision

`bb mcp serve` may expose exactly one mutating tool, `bb_dispatch`, and only
when the operator sets `BB_MCP_ENABLE_DISPATCH=1` on that server process. With
the env var unset -- the default -- the server is unconditionally read-only:
`bb_dispatch` is absent from `tools/list` and `tools/call` refuses it with the
same JSON-RPC `-32602` shape any other unknown tool name gets.

When enabled, `bb_dispatch` does exactly one thing: it builds the same
`bb.dispatch_job.v1` payload the CLI `bb dispatch` command builds (the two
share one function, `dispatch::build_dispatch_job_payload`, so they cannot
drift) and enqueues it through `Ledger::ingest` -- the identical ingress door
every webhook, cron, and CLI trigger already uses. It does not open any new
capability the plane did not already have.

## Why

Backlog 078 shipped MCP read-only-only, with graduation criteria for a future
writable surface: real dogfood use, zero shape drift, bounded outputs, and
backlog 083's unattended-loop guardrails landing first. Those guardrails
(global pause/resume, ingress body caps, bounded cron catch-up, notification
outbox, stale-run detection, reserved-spend accounting) shipped and closed
into the fails-visibly epic (089) on 2026-07-02. `bb_dispatch` is the first
tool to draw on that safety margin, and it draws on it narrowly: one bounded,
ad hoc, opt-in action, not an open write surface.

## Authority Boundary

`bb_dispatch` may:

- read a local repo path and an inline prompt, validate them, and enqueue one
  `bb.dispatch_job.v1` run against the plane's already-configured default
  dispatch task (`BB_DISPATCH_TASK`, else a manual `dispatch` task, else a
  manual `build` task, else the plane's single unambiguous manual task --
  the exact same selection `bb dispatch` uses);
- return the run id and follow-up `bb logs`/`bb runs show`/`bb artifacts
  list` commands so the caller inspects everything through the normal,
  already-audited read surfaces;
- refuse a repeat dispatch of the same `(repo, label, branch_slug, base_ref)`
  tuple by deriving a deterministic idempotency key and relying on
  `Ledger::ingest`'s existing `(task, idempotency_key)` uniqueness --
  returning the original run id with `duplicate: true` instead of fanning
  out a second run -- unless the caller passes `force: true`.

`bb_dispatch` may not, and nothing in its implementation reaches far enough
to:

- run synchronously, execute a workload itself, or touch a substrate; it only
  ever calls `Ledger::ingest` and returns. A running `bb serve` (local or
  deployed) drains the pending run exactly as it does for every other
  trigger kind.
- merge a pull request, push to a protected branch, or deploy anything. The
  dispatched task's own agent/harness owns all repo mutation, git operations,
  and PR creation, precisely as it already does for `bb dispatch` and
  `bb run build` today -- MCP does not add or shortcut any of that.
- resolve runs, acknowledge dead letters, park/unpark tasks, cancel runs,
  mutate the submission gate, or touch provider keys. Those remain CLI/API
  only, exactly as documented in `skills/bitterblossom/SKILL.md`'s MCP
  section.
- validate a `model` string against a catalog, enforce an allowlist of
  repos, or add any authority check the CLI `bb dispatch` command does not
  already enforce. Matching CLI parity is the bar; this tool does not
  introduce a stricter or looser boundary than the CLI already has.

## Red Lines

- No mutating tool other than `bb_dispatch` may exist in the MCP surface
  without its own backlog item and its own authority-boundary doc.
- No change may make the read-only tool table conditional on an env var; the
  ten read-only tools are always registered, unconditionally.
- No change may let `bb_dispatch` bypass `dispatch::build_dispatch_job_payload`'s
  validation (repo must exist and be a directory; prompt must be within
  `DISPATCH_BRIEF_MAX_BYTES`) -- nothing may reach `Ledger::ingest` with an
  unvalidated payload.
- No change may make `force: true` the default, or make duplicate refusal
  silent (the tool must always report `duplicate: true`/`false` in its
  response).

## Revisit Criteria

A second mutating MCP tool (e.g. `bb runs cancel`, `bb dlq replay`, a
submission/gate mutation) requires its own backlog item, its own authority
doc following this shape, and evidence that `bb_dispatch` has been in real
use long enough to show the pattern (opt-in flag, shared payload builder,
idempotency-key dedup, no new capability beyond the existing ingress door) is
safe to repeat.
