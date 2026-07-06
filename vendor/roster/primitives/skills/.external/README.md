# Vendored external skills

Skills under this directory are copied bodies of externally-owned skills,
not roster-authored content. Roster does not re-pin or re-sync them
independently; harness-kit's `registry.yaml` remains the canonical
provenance ledger for these vendors.

## misty-powder

- Source: `misty-step/powder` (external repo, not this vault or harness-kit)
- Pinned commit: `a20d4ecefcb6a16d595966177b48c47a87dfffc8`
- Copied byte-identical from `harness-kit/skills/.external/misty-powder/`
  on 2026-07-05 as part of the roster-916 primitives migration (phase 1 of
  roster-005). `.sync-meta.json` inside `misty-powder/` is the vendor's own
  fetch record and is preserved unmodified.
- harness-kit's `registry.yaml` entry for `misty-step/powder` is the
  authoritative pin; this copy tracks that pin as of the date above and does
  not independently re-sync. Re-copy from harness-kit (or from upstream at
  the new pinned sha) if harness-kit's pin advances.
