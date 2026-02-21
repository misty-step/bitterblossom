This directory contains project-local Claude Code skills for Bitterblossom. Skills are invocable via `/skill-name` in Claude Code sessions and are loaded automatically by the `ralph-loop` plugin during sprite dispatch.

### Skills

*   **`e2e-test/`**: The end-to-end dispatch shakedown skill. Exercises the full `bb dispatch` pipeline against a real sprite and issue, acting as adversarial QA. Documents every friction point — not just outright failures — and mandates filing GitHub issues for any P0-P2 findings.

### Structure Convention

Each skill directory contains a `SKILL.md` (the invocable skill definition), a `references/` directory (read-only context the skill consults during execution), and a `templates/` directory (output scaffolds for reports or artifacts produced by the skill).
