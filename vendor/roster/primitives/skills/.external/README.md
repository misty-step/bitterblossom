# Vendored external skills

Skills under this directory are copied bodies of externally-owned skills,
not roster-authored content.

**`registry.yaml` in this directory is the canonical provenance ledger**
(migrated from harness-kit 2026-07-07, roster-926; harness-kit's copy is
archived history). Every vendored dir carries a `.sync-meta.json` fetch
record pinning it to upstream content (commit SHA). To advance a pin:
fetch upstream at the new SHA, copy the body, update `.sync-meta.json`
and the `registry.yaml` entry together. Pins are content-hash-anchored,
never branch-floating — see `docs/research/roster-926-import-design-brief.md`
for the convention and its prior art.

## misty-powder

- Source: `misty-step/powder` (external repo)
- Pinned commit: `a20d4ecefcb6a16d595966177b48c47a87dfffc8`
- Copied byte-identical from harness-kit on 2026-07-05 (roster-916).

## mattpocock-writing-great-skills

- Source: `mattpocock/skills` at `skills/productivity/writing-great-skills`
- Pinned commit: `16a2a5cd00b4416f673f4ff38c7971a04dd708e7` (MIT)
- Vendored 2026-07-07 (roster-926 phase A); also the judgment layer behind
  `primitives/shared/references/skill-authoring-standard.md`.
