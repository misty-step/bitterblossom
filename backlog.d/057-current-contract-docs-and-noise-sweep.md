# Sweep stale docs so current Rust-plane contracts are unmistakable

Priority: P2 | Status: ready | Estimate: M

## Goal

Make the documentation set tell one current story: Bitterblossom v3 is the Rust
event plane, while older Go/Python/Elixir/conductor material is clearly archived
or superseded.

## Oracle

- [ ] `docs/spine.md`, README, CLAUDE/AGENTS guidance, and
      `skills/bitterblossom` agree with live CLI help.
- [ ] Historical ADRs and walkthroughs are marked superseded or moved under an
      archive boundary without deleting useful prior art.
- [ ] Terminal transcript duplicates in `docs/walkthroughs/*terminal.txt` are
      either archived, justified, or removed with their canonical markdown
      counterpart preserved.
- [ ] Searches for old operational commands such as `cmd/bb`, stale `go test`
      paths, `--var`, and unsupported `--since` return only historical/archive
      contexts or test fixtures.
- [ ] `./scripts/verify.sh` passes.

## Children

1. [x] Build the stale-doc inventory.
2. Mark old ADRs as superseded by ADR 005 and the spine contract.
3. Archive or remove duplicate terminal walkthrough transcripts.
4. Audit remaining live CLI snippets after 050's parity checks.
5. Extend the stale-command regression check only for gaps not already covered
   by 050.

## Notes

Why: docs and simplification lanes found stale current-looking guidance. This
is not the P0 because 050 first fixed the live command examples and exported
skill recipes it depends on.

Evidence:

- 050 added a live-help parity gate for the known `--payload` and
  `runs export` examples; this sweep should look for remaining current-looking
  stale material outside that covered surface.
- `docs/walkthroughs/` contains paired markdown and `*-terminal.txt`
  transcripts.
- Some older ADRs still describe pre-v3 harness/control-plane assumptions at
  the same visibility as the Rust event-plane direction.

## Slice 2026-07-02

Child 1 landed as `docs/stale-doc-inventory.md`. The inventory records the
exact scan commands, current surfaces already covered by
`tests/cli_contract_docs.rs`, archive-safe historical hits, current-looking
historical ADR/walkthrough files, terminal transcript duplicates, and the
follow-up owner for each remaining child.
