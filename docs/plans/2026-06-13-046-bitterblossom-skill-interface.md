# Context Packet: Bitterblossom Agent Skill Interface

## Goal

Expose Bitterblossom as a first-class agent interface by adding a portable
`skills/bitterblossom/` skill folder that consuming agents can copy or symlink.

## Non-Goals

- Do not add runtime behavior to the Rust spine.
- Do not create harness-specific duplicate skill bridges in this repo.
- Do not commit a brittle absolute symlink into Harness Kit.

## Design

Add a product-owned skill folder:

```text
skills/bitterblossom/
  SKILL.md
  agents/openai.yaml
  references/operator-recipes.md
```

`SKILL.md` owns trigger metadata, operating stance, routing, gotchas, and
completion evidence. The reference file owns concrete command recipes so the
entrypoint stays concise.

The artifact is the folder, not a single markdown file. Harness Kit can later
project it by source entry, bootstrap rule, or explicit relative symlink from
the local sibling checkout. A manual copy is the fallback, not the preferred
path, because it drifts.

## Oracle

```bash
python3 /Users/phaedrus/.codex/skills/.system/skill-creator/scripts/quick_validate.py skills/bitterblossom
cargo test bitterblossom_skill_is_exportable_agent_interface
./scripts/verify.sh
```

Acceptance:

- Skill frontmatter triggers on `bb`, Bitterblossom, event-plane, run/DLQ/task
  inventory, submissions, and review-factory work.
- Skill routes agents to JSON CLI surfaces before dispatch.
- Skill records the verdict-task `submission` payload gotcha found during
  dogfood.
- Repo tests fail if the skill loses core command recipes or install guidance.

## Harness Kit Integration Boundary

Do not symlink into Harness Kit in this change. A symlink from
`/Users/phaedrus/Development/harness-kit/skills/bitterblossom` to this checkout
would be useful locally but fragile as a committed artifact. The next Harness
Kit slice should add a source/projection mechanism for product-owned local
skills, then point it at this folder.
