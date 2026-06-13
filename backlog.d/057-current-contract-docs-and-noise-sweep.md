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

1. Build the stale-doc inventory.
2. Mark old ADRs as superseded by ADR 005 and the spine contract.
3. Archive or remove duplicate terminal walkthrough transcripts.
4. Reconcile live CLI snippets after 050.
5. Add a cheap stale-command regression check if 050 has not already covered
   it.

## Notes

Why: docs and simplification lanes found stale current-looking guidance. This
is not the P0 because 050 must first fix the live contract it will document.

Evidence:

- `docs/spine.md:355-360` has stale CLI snippets.
- `docs/walkthroughs/` contains paired markdown and `*-terminal.txt`
  transcripts.
- Some older ADRs still describe pre-v3 harness/control-plane assumptions at
  the same visibility as the Rust event-plane direction.
