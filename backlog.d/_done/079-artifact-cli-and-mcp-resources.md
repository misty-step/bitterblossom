# Make run artifacts first-class for CLI and MCP consumers

Priority: P1 · Status: done · Estimate: M

## Goal

Let humans and agents inspect run evidence directly through `bb` and MCP instead of spelunking attempt artifact directories.

## Oracle

- [x] `bb artifacts list <run-id> --json` returns artifact names, relative paths, sizes, content types where knowable, and safety metadata for a run's attempts.
- [x] `bb artifacts read <run-id> <path>` prints a safe text/JSON artifact such as `REPORT.json`, with binary/oversized artifacts rejected or summarized by contract.
- [x] `bb artifacts bundle <run-id> --out <path>` is implemented or explicitly deferred with a follow-up if bundling expands scope.
- [x] MCP exposes artifact resources or tools, e.g. `bb_artifacts_list` and `bb_artifact_read`, backed by the same helper as CLI.
- [x] The portable skill requires artifact inspection in closeout for any run that claims success.
- [x] Tests cover successful `REPORT.json` read, missing artifact, path traversal rejection, and multi-attempt behavior.
- [x] `./scripts/verify.sh` passes.

## Verification System

- Claim: a consuming agent can verify BB output by reading artifacts through public interfaces, not local path archaeology.
- Falsifier: agent must infer attempt directory layout; traversal can escape artifact root; binary output floods stdout; or MCP/CLI disagree.
- Driver: local-plane run producing `REPORT.json`, then CLI and MCP artifact reads.
- Grader: artifact content matches file on disk; unsafe paths fail; missing artifacts produce structured errors.
- Evidence packet: command transcript and run id.
- Cadence: artifact contract test joins the agent-interface gate.

## Graduation Metrics / Trigger Conditions

This first slice is read-only artifact inspection. Do not add artifact mutation, deletion, redaction rewriting, or automatic publication. Ship with metrics that tell us when to expand capability:

- agents can close out at least 10 recent BB runs using `bb artifacts list/read` without direct `plane/.bb/runs/...` path spelunking;
- gate-blocking evidence includes a public artifact handle/path for the full stdout/stderr or report, not just a truncated excerpt;
- artifact reads reject traversal and oversized/binary output in tests and in one real run drill;
- closeout receipts include artifact command transcripts for builder/storm/verifier runs;
- a dogfood note records every time artifact access still required shell/file tools.

Promotion trigger: read-only MCP artifact inspection shipped in the 2026-07-02
agent-friendly layer slice because backlog 078 had only this gap remaining and
the adapter is bounded by the same path/size/binary helpers as CLI. Artifact
bundle/export automation remains deferred until the usage metrics above are met
and a concrete export consumer needs it.

## Notes

Why: run state says a task finished; artifacts prove whether it did useful work. This is especially important for unsupervised report-only flows such as Canary triage and backlog-chewer dry runs.

2026-06-30 dogfood review note: the first CLI slice now emits structured JSON error envelopes for invalid paths, missing runs, and IO/stat failures, but maps `anyhow` messages to envelope kinds at the CLI boundary. Before expanding this into MCP or bundle, prefer typed artifact errors in `src/artifacts.rs` so CLI/API/MCP cannot drift on error classification. Also decide whether nested artifact listing remains deferred or becomes part of the MCP resource contract.

2026-06-30 storm advisory: `looks_binary` can falsely mark a large UTF-8 text artifact as binary if the 8 KiB sniff boundary cuts through a multibyte character. This affects `artifacts list` metadata for files above `READ_LIMIT`, not `artifacts read`; fix or test the boundary before widening artifact resources.

2026-06-30 dogfood follow-up: local branch `bb-agent-friendly-layer-v1` now treats an incomplete UTF-8 codepoint at the fixed sniff boundary as text, while still marking complete invalid UTF-8 and NUL bytes as binary; `cargo test --locked --test artifacts_cli` covers the oversized split-boundary case.

2026-06-30 Thermo review follow-up: Cursor Thermo-Nuclear review caught that the sniff-boundary fix had reused one helper for both full artifact reads and partial oversized sniffing, which could make a small file ending in an incomplete UTF-8 byte report `io_error` instead of `binary`. Commit `eca7e24` split full-buffer and sniff-buffer classifiers and added `artifacts_read_incomplete_utf8_tail_is_binary_not_io_error`.

### 2026-07-02 artifact MCP slice

- Added MCP read-only tools `bb_artifacts_list` and `bb_artifact_read`, backed
  directly by `artifacts::list` and `artifacts::read` so the adapter cannot
  drift from the CLI helper.
- Extended `tests/mcp_cli.rs` to seed a `REPORT.json`, compare MCP artifact
  list/read output against `bb artifacts ... --json`, and assert traversal
  paths are rejected through the MCP tool boundary.
- Preserved the existing CLI artifact tests for `REPORT.json`, missing
  artifacts, traversal rejection, binary/oversized summaries, and multi-attempt
  newest-first behavior.
- Updated `docs/spine.md` and `skills/bitterblossom/SKILL.md` so consuming
  agents prefer MCP artifact inspection and fall back to CLI JSON.
- Explicitly deferred archive/bundle export to backlog 101. This slice is
  inspection only: no artifact mutation, deletion, redaction rewrite, or
  publication surface.
- LOC tripwire: adding artifact tools is Rust spine mechanism (read-only MCP
  adapter over existing artifact helpers, no workload judgment). The cap moves
  narrowly from 9650 to 9700 for measured source LOC around 9666.
