This directory contains the Architectural Decision Records (ADRs) for Bitterblossom, defining the technical foundation for its agentic "sprite" infrastructure. The documentation outlines a transition from a complex, multi-package Go system to a streamlined architecture centered on Claude Code and the Sprites Go SDK.

### Architecture and Purpose
The architecture follows a "Thin CLI, Thick Skills" philosophy. It bifurcates the system into a deterministic transport layer and an intelligent execution layer:
*   **Transport Layer (Go):** A minimal CLI tool (`bb`) manages non-intelligent operations including connectivity probes, file synchronization, credential isolation, and long-lived streaming of stdout/stderr.
*   **Execution Layer (Claude Code):** Agentic intelligence, task decomposition, and repository analysis are delegated to Claude Code skills and the `ralph-loop` plugin.

### Key File Roles
*   **`001-claude-code-canonical-harness.md`**: Establishes Claude Code as the sole supported harness for sprite dispatch. It details the deprecation of OpenCode due to stability issues and outlines the use of a proxy provider to route requests through OpenRouter to non-Anthropic models.
*   **`002-architecture-minimalism.md`**: Codifies the reduction of the Go codebase from ~42K LOC to <1K LOC by leveraging the `sprites-go` SDK. It defines the specific responsibilities of the `bb` CLI (dispatch, setup, logs, status) versus the intelligence residing in agent skills.
*   **`scripts/ralph.sh`**: Referenced as the "sacred" core iteration loop that invokes the harness, manages signal files, and enforces execution limits.

### Dependencies and Technical Constraints
*   **Harness:** Claude Code pinned to the Sonnet 4.6 runtime with the `ralph-loop` plugin.
*   **SDK:** `github.com/superfly/sprites-go` is required for native filesystem access and streaming command execution on Fly Machines.
*   **Model Routing:** Multi-model support (e.g., Kimi K2.5, GLM 4.7) is achieved via PR #136 (`feat/proxy-provider`), which routes traffic through OpenRouter.
*   **Configuration Gotcha:** Sprite environments must use `ANTHROPIC_BASE_URL` and `ANTHROPIC_AUTH_TOKEN` for proxy routing rather than standard OpenRouter environment variables.
*   **Timeout Limitations:** The Go-based transport is strictly required because the Claude Code Bash tool imposes a 600-second timeout, which is insufficient for the 30+ minute durations typical of "ralph loops."
*   **Execution Mode:** Production dispatch relies on Claude Code’s `--yolo` PTY mode for automated execution.