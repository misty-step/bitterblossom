This directory serves as the central repository for End-to-End (E2E) Shakedown Reports, which document the functional validation of the `bitterblossom` system, its CLI (`bb`), and its agentic execution loops. These reports track the performance and stability of specific branches—most notably the `sdk-v2` rewrite—against real-world sprites (remote execution environments) like `fern`.

### Architecture and Key File Roles
The directory consists of Markdown-based reports named by date and test type (e.g., `YYYY-MM-DD-e2e-shakedown.md`). Each file adheres to a standardized technical schema designed to audit the lifecycle of an autonomous agent task:

*   **Summary Metadata:** Tracks the target branch, specific hardware/sprite identifier, the GitHub issue being addressed, and an overall grade (A-F).
*   **Phase Results:** A structured matrix evaluating the eight stages of the dispatch pipeline: Build, Fleet Health, Issue Selection, Credential Validation, Dispatch, Monitor, Completion, and PR Quality.
*   **Findings:** Granular technical post-mortems of failures, categorized by severity (P0–P3) and functional area (e.g., Silent Failures, Credential Pain, or Infrastructure Fragility).
*   **Timeline and Raw Output:** High-resolution logs including UTC timestamps for every transition in the `ralph` execution loop and stack traces for terminal failures like deadlocks.

### Technical Components
The reports document the interaction between several core architectural components:
*   **`bb` CLI:** The primary entry point for managing sprites and dispatching tasks.
*   **`ralph` Loop:** The orchestration script or harness that manages agent iterations, signal detection (e.g., `TASK_COMPLETE`), and "off-rails" detection logic.
*   **`sprites-go` SDK:** The Go-based communication layer utilizing websockets for command execution and real-time output streaming from remote sprites.
*   **Fly.io Infrastructure:** The underlying platform providing the compute resources and the `FLY_API_TOKEN` authentication mechanism.

### Key Dependencies and Gotchas
*   **Websocket Deadlocks:** The system is sensitive to how `sprites-go` handles `CombinedOutput()` vs. `Output()`. Reports indicate that improper channel handling in the websocket layer can lead to total process deadlocks.
*   **Streaming Buffering:** Real-time visibility into agent progress is dependent on the binary message framing of the websocket. Issues like "zero-byte streaming" occur if line-buffering is not explicitly forced on the remote side.
*   **Credential Chain:** Successful dispatch requires a valid `FLY_API_TOKEN` to be exchanged for a sprite-specific token. A common failure mode involves stale environment variables in `.env.bb` producing misleading "unauthorized" errors.
*   **Timeout Hierarchy:** There is a complex relationship between the CLI `--timeout` flag and the internal `ITER_TIMEOUT_SEC` variable; the system may fail to honor short timeouts if the iteration-level grace periods are hardcoded to higher values.
*   **Workspace State:** The `bb status` command may return misleading data on multi-workspace sprites, as it often defaults to the first directory alphabetically rather than the active task workspace.