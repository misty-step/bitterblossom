The `bitterblossom-dispatch` directory defines a specialized skill for orchestrating task execution on Bitterblossom sprites. Its primary purpose is to manage the lifecycle of a coding task—from initial planning and dry-runs to execution and real-time monitoring—by dispatching GitHub issues or raw prompts to remote environments.

### Architecture and Workflow
The skill operates as a wrapper around the `bb` CLI toolchain, specifically leveraging the `bb dispatch` command. It follows a modular architecture where external capabilities are injected into sprites via "Skill Mounts." When a dispatch command is issued, the system mounts specified skill directories into the sprite's filesystem under `./skills/<name>/` and augments the task prompt with instructions to follow the corresponding `SKILL.md` manifests.

The operational workflow is divided into three distinct phases:
1.  **Preflight:** Verification of the environment and sprite availability using `bb status`.
2.  **Planning:** A mandatory dry-run phase (the default behavior) used to validate the execution plan without committing changes.
3.  **Execution:** Active deployment using the `--execute` flag, often combined with `--wait` for synchronous log monitoring.

### Key File Roles
*   **SKILL.md:** Acts as the central manifest and documentation for the skill. It defines the required toolset (`Read`, `Grep`, `Glob`, `Bash`), sets the user-invocable status, and provides the command-line patterns for mounting additional skills and handling failure states.

### Dependencies and Technical Constraints
*   **Infrastructure:** The skill is dependent on Fly.io for sprite hosting, requiring `FLY_APP`, `FLY_API_TOKEN`, and `FLY_ORG` environment variables to be correctly configured in the `.env.bb` file.
*   **Tooling:** Requires the `bb` CLI utility for dispatching, status checks, and watchdog monitoring.
*   **Validation Logic:** The system implements strict validation based on sprite readiness and GitHub labels. While this can be bypassed using `--skip-validation`, doing so ignores safety checks designed to prevent execution on unready targets.
*   **Skill Semantics:** Explicit mounting is required; the sprite does not inherently possess skills unless they are passed via the `--skill` flag during the dispatch call.