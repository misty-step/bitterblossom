### Technical Overview: /reports

This directory serves as the centralized repository for End-to-End (E2E) shakedown reports and benchmarking data for the Bitterblossom system. The documentation focuses on the reliability of the dispatch pipeline, the performance of remote execution environments (sprites), and the comparative efficacy of various Large Language Model (LLM) harnesses.

#### Purpose and Architecture
The reports document a distributed agentic architecture designed to automate software engineering tasks. The system utilizes a CLI (`bb`) to orchestrate work across a fleet of remote VMs—named sprites (e.g., `fern`, `thorn`, `bramble`, `sage`)—hosted on Fly.io. The architecture relies on a multi-phase dispatch pipeline:
1.  **Orchestration:** Build and environment validation.
2.  **Fleet Management:** Health Probing and sprite selection.
3.  **Execution:** Dispatching tasks to either the `Claude Code` harness (Anthropic-native) or the `OpenCode` harness (OpenRouter-compatible).
4.  **Verification:** Monitoring agent progress via a watchdog and calculating work deltas (git diffs/commits) upon completion.

#### Key File Roles
*   **`e2e-shakedown-[date].md`**: These files provide high-level "grades" and granular phase results for specific system runs. They categorize findings by severity (P0–P3) and functional area (e.g., Silent Failure, Infrastructure Fragility, Credential Pain). They track the lifecycle of a dispatch from initial build to PR creation.
*   **`model-comparison-[date].md`**: These files detail the interoperability between different LLM providers and the Bitterblossom harnesses. They track success rates for specific tasks across models like `claude-sonnet-4-5`, `glm-5`, and `minimax-m2.5`, documenting cost-per-task, implementation quality, and harness-specific limitations.

#### Important Dependencies
*   **Fly.io API:** Critical for machine state management; the system relies on Fly.io for sprite lifecycle operations.
*   **Anthropic & OpenRouter APIs:** Provide the intelligence layer. The system differentiates between direct Anthropic integration and OpenRouter proxies for model diversity.
*   **GitHub CLI (`gh`):** Utilized by agents on sprites for issue context retrieval and Pull Request management.
*   **Claude Code & OpenCode:** The execution engines that interpret the `ralph.sh` dispatch script and interact with the sprite's filesystem.

#### Technical Gotchas
*   **Stale Fleet State:** A recurring issue where the Fly.io machine status reports sprites as "warm" or "idle" despite them being unreachable via TCP or trapped in stale execution loops.
*   **TCP Timeout Cascades:** Degraded network connectivity can cause a 45-second delay per pipeline step, leading to significant dispatch overhead.
*   **Credential Leakage:** Simultaneous presence of `ANTHROPIC_AUTH_TOKEN` and `ANTHROPIC_API_KEY` can cause Claude Code to prioritize incorrect credentials, resulting in silent authentication failures.
*   **Git Ownership Conflicts:** Divergent user contexts between the `bb` CLI and sprite-level execution often trigger git "dubious ownership" errors, necessitating explicit `safe.directory` configurations.
*   **Silent Git Failures:** Agents occasionally complete code implementations but fail to perform `git push` or PR creation due to missing `GH_TOKEN` environment variables or terminal signal mismatches (e.g., missing `TASK_COMPLETE` markers).
*   **JSON Pollution:** Non-JSON progress text (e.g., token exchange logs) sometimes leaks into stdout during `--json` mode, breaking machine-parseability for downstream tooling.