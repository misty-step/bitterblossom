### Technical Overview: Bitterblossom Skills Directory

The `/base/skills` directory serves as the centralized repository for modular capability definitions and policy enforcement within the Bitterblossom autonomous agent framework. It functions as a library of "Skill Mounts"—specialized instruction sets and tool configurations that are injected into remote execution environments ("sprites") to orchestrate complex tasks, maintain architectural standards, and manage external integrations.

### Architecture and Workflow
The directory follows a manifest-driven architecture where each subdirectory represents a discrete domain of expertise. The core operational model relies on injecting these directories into a sprite’s filesystem under `./skills/<name>/` during task dispatch. This allows the system to augment agent behavior without bloating the core runtime.

The workflow across these skills generally follows a tiered execution pattern:
1.  **Validation & Preflight:** Checking environment variables (via `.env.bb`) and infrastructure readiness using the `bb` CLI.
2.  **Manifest-Guided Execution:** Sprites consume the `SKILL.md` files within these directories to understand their tool constraints (e.g., `Bash`, `Grep`, `Read`) and operational boundaries.
3.  **State-Aware Monitoring:** Skills like `bitterblossom-monitoring` provide a diagnostic layer to triage "stale" or "blocked" tasks by inspecting the remote workspace.
4.  **Policy Enforcement:** Non-invocable skills (e.g., `naming-conventions`, `git-mastery`) act as automated quality gates, enforcing linear git histories, semantic naming, and behavioral testing standards.

### Key File Roles
*   **SKILL.md:** The primary orchestrator and manifest for every subdirectory. It contains YAML metadata defining the skill's identity, whether it is `user-invocable`, and the `allowed-tools` (typically restricted to `Read`, `Grep`, `Glob`, and `Bash`). It also serves as the technical runbook for the agent.
*   **BLOCKED.md:** A state-tracking file (referenced in monitoring logic) used by agents to signal execution stalls, triggering specific recovery paths within the monitoring skill.
*   **Configuration Files (.env.bb):** While often located at the root, these files are critical to the skills directory as they provide the necessary credentials for Fly.io and GitHub integrations used by the dispatch and monitoring modules.

### Dependencies and Technical Constraints
*   **Orchestration Tooling:** The entire directory is tightly coupled with the `bb` (Bitterblossom) and `sprite` CLI utilities. The `bitterblossom-dispatch` skill specifically acts as a wrapper for `bb dispatch`.
*   **Infrastructure Dependency:** Remote execution and monitoring are dependent on Fly.io; missing `FLY_APP`, `FLY_API_TOKEN`, or `FLY_ORG` variables will result in preflight failures.
*   **Explicit Mounting Requirement:** Skills are not inherent to the agent's environment. They must be explicitly passed via the `--skill` flag during the dispatch process to be available in the sprite's filesystem.
*   **Strict Environment Formatting:** The `external-integration` logic treats trailing whitespace in environment files as fatal errors, reflecting a "fail-loudly" philosophy.
*   **Tooling Limitations:** Skills are architecturally restricted to basic Unix utilities. More complex operations (like direct filesystem introspection) require the use of the `timeout` utility to prevent diagnostic hangs during remote execution.