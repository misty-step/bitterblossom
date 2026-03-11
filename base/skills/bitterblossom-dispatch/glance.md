The `bitterblossom-dispatch` directory defines a specialized skill for orchestrating task execution on Bitterblossom sprites. Its primary purpose is to manage the lifecycle of a coding task—from initial planning and dry-runs to execution and real-time monitoring—by dispatching GitHub issues or raw prompts to remote environments.

### Architecture and Workflow
The skill operates as a wrapper around the `bb` CLI toolchain, specifically leveraging the `bb dispatch` command. The current workflow is deliberately small: verify readiness with `--dry-run`, dispatch a prompt against a prepared sprite workspace, and use `bb logs` / `bb status` for follow-up inspection.

The operational workflow is divided into three distinct phases:
1.  **Preflight:** Verification of the environment and sprite availability using `bb status`.
2.  **Readiness Probe:** A dry-run phase using `bb dispatch ... --dry-run` to validate auth, connectivity, and repo readiness without starting the agent.
3.  **Execution:** Active dispatch of a prompt, followed by `bb logs` or `bb status` to inspect progress and outcomes.

### Key File Roles
*   **SKILL.md:** Acts as the central manifest and documentation for the skill. It defines the required toolset (`Read`, `Grep`, `Glob`, `Bash`), sets the user-invocable status, and provides the command-line patterns for mounting additional skills and handling failure states.

### Dependencies and Technical Constraints
*   **Infrastructure:** The skill relies on prepared sprites. `bb setup <sprite> --repo <owner/repo>` must happen before dispatch.
*   **Tooling:** Requires the `bb` CLI utility for dispatching, status checks, logs, and process recovery.
*   **Auth:** `GITHUB_TOKEN` is required for dispatch. `SPRITE_TOKEN` is preferred for transport auth, with `FLY_API_TOKEN` as a fallback.
*   **Prompt Contract:** The current transport surface accepts a rendered prompt string. There is no separate issue, skill-mount, or execute/wait flag layer in `bb`.
