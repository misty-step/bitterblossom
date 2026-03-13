### Technical Overview: Bitterblossom Skills Directory

The `/base/skills` directory now has two layers:

1. **Imported autonomy skills** vendored from `phrazzld/agent-skills` so Bitterblossom can ship a version-pinned, testable skill surface to managed sprites.
2. **Bitterblossom-specific runtime skills** that stay close to the `bb` transport and conductor operating model.

These are repo-local runtime assets, not a second transport API. Repo `WORKFLOW.md` is the contract that tells sprites when to use which phase and skill.

### Imported autonomy skills
- `autopilot` — bounded autonomous execution workflow
- `shape` — turn rough work into a clearer executable spec
- `build` — implementation workflow with verification discipline
- `pr` — PR creation and intent/verification discipline
- `pr-walkthrough` — structured PR inspection and explanation
- `debug` — systematic investigation and root-cause classification
- `pr-fix` — bounded PR remediation for feedback and CI failures
- `pr-polish` — final merge-readiness and cleanup pass

`simplify` and `ux-polish` are still pending as standalone imports or repo-local wrappers.

### Bitterblossom-specific skills
- `bitterblossom-dispatch` — dry-run probing plus prompt dispatch through `bb`
- `bitterblossom-monitoring` — monitoring, status checks, and recovery triage

### Provisioning contract
- `bb setup` copies everything under `base/skills/` onto the sprite under `/home/sprite/.claude/skills/`.
- Imported skills are version-pinned by the Bitterblossom repo state, not by ad hoc sprite drift.
- Imported skills are meant to execute inside the repo `WORKFLOW.md` contract, not replace it with their own repo policy.
- Sprite personas advertise role-specific skill packs so the same imported skill surface can be specialized by worker type.

### Key File Roles
- `SKILL.md`: the manifest and runbook for each skill.
- `glance.md`: a short local summary for fast retrieval.
- `references/`: linked supporting material that should travel with imported skills.
- `.env.bb`: repo-level environment bootstrap used by dispatch and monitoring workflows.

### Dependencies and Technical Constraints
- These skills assume the supported `bb` surface from `docs/CLI-REFERENCE.md`, not historical wrapper commands.
- `SPRITE_TOKEN` is the preferred transport credential; `FLY_API_TOKEN` remains a fallback path.
- More invasive remote introspection should go through `sprite exec` with a timeout rather than expanding the transport boundary.
