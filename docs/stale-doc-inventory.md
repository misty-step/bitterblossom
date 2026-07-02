# Stale Documentation Inventory

Date: 2026-07-02

This inventory supports backlog 057. Its job is to separate current operator
contract docs from historical material before we move or supersede anything.

## Scan

Commands used:

```bash
rg -n "cmd/bb|go test|--var|--since|conductor|Elixir|Python|Go CLI|terminal\\.txt" \
  README.md CLAUDE.md AGENTS.md docs skills .github scripts tests backlog.d \
  -g '*.md' -g '*.txt' -g '*.sh' -g '*.rs' -g '*.yml'

for p in 'cmd/bb' 'go test' '--var' '--since'; do
  rg -l --fixed-strings -- "$p" README.md CLAUDE.md AGENTS.md docs skills .github scripts tests backlog.d
done
```

`go test` has false positives inside `cargo test`; future regression checks
should use a word-boundary pattern for the Go command rather than a fixed
substring.

## Current-Surface Status

These current surfaces already agree with live Rust-plane CLI help through
`tests/cli_contract_docs.rs`:

- `README.md`
- `docs/spine.md`
- `skills/bitterblossom/SKILL.md`
- `skills/bitterblossom/references/operator-recipes.md`
- `.agents/skills/bb-dogfood/SKILL.md`
- `docs/operations/README.md`
- `scripts/production-ops-drill.sh`

The existing gate covers the known stale `--var` and unsupported `--since`
forms on current docs/skills, plus positive live CLI examples for `bb run`,
`bb task list`, `bb runs export`, and `bb gate`.

## Historical And Archive-Safe Hits

These contexts can mention old commands because they are already under an
archive or backlog-history boundary:

- `docs/archive/**`
- `backlog.d/_done/**`
- dated dogfood/evidence records in `docs/plans/**`
- dated audit reports in `docs/audit-reports/**` and `docs/audits/**`
- tests that assert stale commands are absent from current docs

Do not delete these just to make search output clean; preserve prior art unless
a later slice moves a current-looking document behind an archive boundary.

## Historical But Still Too Current-Looking

These files live outside `docs/archive/` and still mention Go/Python/Elixir or
conductor-era contracts. Historical ADRs now carry explicit supersession
banners pointing to ADR 005 and `docs/spine.md`; `docs/walkthroughs/README.md`
marks walkthrough prose as historical evidence, not current operator guidance:

- `docs/adr/001-claude-code-canonical-harness.md`
- `docs/adr/002-architecture-minimalism.md`
- `docs/adr/003-conductor-control-plane.md`
- `docs/adr/004-bounded-review-governance.md`
- `docs/adr/004-elixir-conductor-architecture.md`
- `docs/walkthroughs/*.md`

The current ADR exception is `docs/adr/005-rust-event-plane.md`, which is the
current superseding record and intentionally names prior systems as negative
evidence.

## Terminal Transcript Duplicates

Terminal transcript companions are historical evidence, not current operator
instructions. They now live under
`docs/archive/walkthrough-terminal-transcripts/`:

- `docs/archive/walkthrough-terminal-transcripts/codex-simplify-bb-sprite-transport-terminal.txt`
- `docs/archive/walkthrough-terminal-transcripts/codex-simplify-bb-workspace-contract-terminal.txt`
- `docs/archive/walkthrough-terminal-transcripts/codex-simplify-governance-session-terminal.txt`
- `docs/archive/walkthrough-terminal-transcripts/issue-505-qa-intake-terminal.txt`
- `docs/archive/walkthrough-terminal-transcripts/issue-529-trusted-thread-metadata-terminal.txt`

`tests/cli_contract_docs.rs` guards that no `*-terminal.txt` files remain in
the live walkthrough directory.

## Follow-Up Slices

- Child 2: explicit superseded banners on historical ADRs landed.
- Child 3: walkthrough terminal transcript duplicates archived.
- Child 4: live CLI snippet re-audit landed in `tests/cli_contract_docs.rs`.
- Child 5: path-aware stale-command regression landed in
  `tests/cli_contract_docs.rs`, avoiding archive, backlog-history, walkthrough,
  ADR-history, and test-fixture contexts.
