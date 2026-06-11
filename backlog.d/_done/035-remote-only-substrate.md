# Make bb remote-only: demote the local substrate to test machinery

Priority: P1 · Status: done · Estimate: M

## Goal
The plane never manages workload processes on the operator's machine —
every production run dispatches to a remote substrate (Fly Sprites
today), while the substrate seam stays clean enough that a second
remote substrate (Cloudflare, Modal, …) is one adapter, not a redesign.

## Oracle
- [x] `bb check` rejects `substrate = "local"` in a plane config (error
      names the remote substrates) unless the plane sets an explicit
      `dev = true` escape hatch in plane.toml
- [x] `plane/tasks/review/task.toml` runs on `substrate = "sprites"` and
      a real review run completes on a sprite (ledger evidence)
- [x] `cargo test` still passes with no network and no tokens — the
      process-exec code survives only as test/dev machinery, not as a
      documented production substrate
- [x] `docs/spine.md` documents the substrate contract (Substrate /
      Session traits, WorkspacePlan, probe semantics, lease identity) as
      the seam a new adapter implements — README no longer advertises
      "local process" as a peer of sprites

## Children
1. Plane-level `dev` flag + `bb check` rejection of local in non-dev
   planes; keep `LocalSubstrate` compiled for tests and dev planes.
2. Move the review task to sprites (needs a baked checkpoint with `gh`
   auth + the workload harness; see 036 for which harness).
3. Substrate-contract section in docs/spine.md; README rewrite of the
   "local or sprite" framing to "remote-first, sprites today".
4. Decision note (in the doc, not code): what a Cloudflare adapter would
   map WorkspacePlan/probe onto — written to validate the seam, with an
   explicit "do not build until a real need" verdict.

## Notes
Operator direction 2026-06-10: "no reason for bitterblossom to manage
processes on this machine — it should always be a layer for dispatching
work to some remote substrate; opinionated about Fly Sprites but
modular." `run_with_timeout` in src/substrate/local.rs is shared by the
sprites relay — it stays; what dies is `local` as a production substrate
name. The whole test suite (e2e_local, budgets, recovery) runs on the
local adapter; that is the cheap no-network harness boundary and the
reason this is a demotion, not a deletion.

## Evidence (2026-06-11)
- `bb check` rejects local substrate sans dev=true (tests/policy.rs);
  plane roots canonicalize (relative paths broke sprite card upload live,
  run a426fe672c9c).
- Review ran live on sprite lane-1: run e2ac11f86f58 success, 729s,
  pi + Kimi K2.6 via OpenRouter; comment posted on PR #843.
- Substrate contract + remote-first framing in docs/spine.md, README.
