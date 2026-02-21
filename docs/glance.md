The `/docs` directory serves as the technical repository for Bitterblossom, an agentic infrastructure system designed to manage "Sprites"—persistent, stateful Linux sandboxes hosted on Fly.io. The documentation chronicles a structural pivot from a complex, multi-package Go orchestration layer to a "Thin CLI, Thick Skills" architecture that leverages the `sprites-go` SDK and Claude Code as the primary execution harness.

### Architecture and Purpose
The system is bifurcated into two distinct layers to bypass the execution limits of standard agentic tools:
*   **Transport Layer (Go):** The `bb` CLI manages non-intelligent operations, including connectivity probing, file synchronization, credential isolation, and long-lived streaming. This layer is strictly required because the Claude Code Bash tool imposes a 600-second timeout, whereas Bitterblossom tasks often exceed 30 minutes.
*   **Execution Layer (Claude Code & Ralph Loop):** Agentic intelligence and task decomposition are handled by Claude Code, wrapped in a "sacred" core iteration loop known as `ralph.sh`. This loop manages signal files and enforces execution limits.

### Key File Roles
*   **Architectural Decision Records (ADRs):** `001-claude-code-canonical-harness.md` and `002-architecture-minimalism.md` codify the transition to a minimal Go codebase (<1K LOC) and establish a proxy-based routing system to support non-Anthropic models (e.g., Kimi K2.5) through OpenRouter.
*   **CLI-REFERENCE.md:** Defines the operational surface of the `bb` tool, detailing commands for sprite lifecycle management: `setup` (provisioning), `dispatch` (task execution), `kill` (process recovery), and `status` (fleet reconciliation).
*   **COMPLETION-PROTOCOL.md:** Specifies the inter-process communication (IPC) between the agent and the Go transport layer. It defines a filesystem-based signaling mechanism using specific files (`TASK_COMPLETE`, `BLOCKED.md`) to drive the `ralph` state machine.
*   **Archive/ & Shakedown Reports:** The `archive` directory preserves the legacy design philosophy (Ousterhout × UNIX), while the shakedown reports provide technical post-mortems on websocket stability, deadlock issues in the `sdk-v2` rewrite, and credential chain failures.

### Dependencies and Technical Constraints
*   **Harness & SDK:** The system is pinned to the Claude Code Sonnet 4.6 runtime and requires `github.com/superfly/sprites-go` for native filesystem access and PTY streaming on Fly Machines.
*   **Model Routing:** Multi-model compatibility is achieved via a custom proxy provider. Environments must use `ANTHROPIC_BASE_URL` and `ANTHROPIC_AUTH_TOKEN` to route traffic through OpenRouter, as standard OpenRouter variables are not natively supported by the harness.
*   **Operational Gotchas:** 
    *   **Signal Sensitivity:** The completion protocol is sensitive to file extensions; while `TASK_COMPLETE` is canonical, the system includes fallback logic for `.md` extensions automatically appended by certain LLMs.
    *   **Websocket Deadlocks:** The transport layer is susceptible to deadlocks if `CombinedOutput()` is used improperly over the `sprites-go` websocket layer.
    *   **Execution Mode:** Production dispatch relies on Claude Code’s `--yolo` mode for unattended execution, requiring robust "off-rails" detection within the `ralph` loop.
    *   **Dry-Run Defaults:** To protect infrastructure, all mutating CLI commands default to dry-run mode and require the `--execute` flag.