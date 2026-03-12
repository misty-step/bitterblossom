### Technical Overview: Bitterblossom Skills Directory

The `/base/skills` directory stores Bitterblossom-specific runbooks and guardrails for work done through the current transport surface. These skills are repo-local instructions, not a second transport API.

### Architecture and Workflow
Each subdirectory is a small domain-specific guide. The Bitterblossom-specific skills focus on:
1.  **Dispatch readiness:** using `bb status` and `bb dispatch ... --dry-run`.
2.  **Runtime inspection:** using `bb logs`, `bb status`, and direct workspace checks when needed.
3.  **Recovery:** using `bb kill` to clear stale Ralph or Claude processes.
4.  **Policy guidance:** keeping repo-specific conventions close to the work.

### Key File Roles
- `SKILL.md`: the manifest and runbook for each skill.
- `glance.md`: a short local summary for fast retrieval.
- `.env.bb`: repo-level environment bootstrap used by dispatch and monitoring workflows.

### Dependencies and Technical Constraints
- These skills assume the supported `bb` surface from `docs/CLI-REFERENCE.md`, not historical wrapper commands.
- `SPRITE_TOKEN` is the preferred transport credential; `FLY_API_TOKEN` remains a fallback path.
- More invasive remote introspection should go through `sprite exec` with a timeout rather than expanding the transport boundary.
