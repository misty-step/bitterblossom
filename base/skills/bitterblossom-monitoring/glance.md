The `bitterblossom-monitoring` directory defines an operational "skill" within the Bitterblossom ecosystem designed for the lifecycle management and recovery of autonomous agent tasks (sprites). Its primary purpose is to provide a standardized diagnostic interface for identifying and resolving stalled, silent, or blocked task dispatches.

### Architecture and Workflow
The skill operates as a diagnostic layer atop the Bitterblossom (`bb`) and `sprite` CLI tools. It follows a tiered monitoring architecture:
1.  **High-Level Status Polling:** Utilizing `bb status` to track task progression and state.
2.  **Active Watchdog Monitoring:** Using `bb watchdog` to monitor live dispatches, particularly when operating in `--wait` mode.
3.  **State-Based Triage:** A heuristic approach that categorizes sprite health into specific states—`running`, `blocked`, `complete`, `dead`, or `stale`—each triggering a specific recovery or inspection path.
4.  **Low-Level Introspection:** A fallback mechanism using `sprite exec` to bypass abstraction layers and inspect the remote workspace filesystem directly.

### Key File Roles
*   **`SKILL.md`**: Serves as the technical specification and runbook for the monitoring skill. It defines the allowed toolset (Read, Grep, Glob, Bash), configures the environment requirements, and maps specific command-line patterns to diagnostic outcomes. It dictates how to handle specific failure modes, such as inspecting `BLOCKED.md` in the sprite workspace or re-dispatching "dead" tasks.

### Dependencies and Gotchas
*   **Tooling Dependencies:** The skill relies on the `bb` CLI for orchestration and the `sprite` CLI for container-level access. It specifically requires the `FLY_ORG` environment variable to be set for remote execution.
*   **Environment Configuration:** Operations depend on sourcing `.env.bb` to establish the necessary context for the Bitterblossom environment.
*   **Execution Stability:** Direct probes via `sprite exec` are identified as potentially unstable; the architecture necessitates the use of the `timeout` utility to prevent diagnostic commands from hanging indefinitely.
*   **State Sensitivity:** Triage heuristics depend on the presence of specific markers within the sprite's remote environment, such as the `BLOCKED.md` file for stalls or PR URLs for completed tasks.