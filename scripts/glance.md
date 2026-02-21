### Technical Overview: /scripts

The `scripts` directory serves as the orchestration and management layer for **Bitterblossom**, a system designed to deploy and supervise autonomous AI agents (referred to as "sprites") running Claude Code. The architecture follows a hub-and-spoke model where local management scripts dispatch tasks to remote execution environments (typically Fly.io machines), which then run persistent agent loops.

#### Core Architecture and Key Roles

**1. Agent Orchestration (The "Ralph" Loop)**
The system implements the "Ralph" pattern—an autonomous execution loop that persists until a task is completed or blocked.
*   **`sprite-agent.sh`**: The primary remote supervisor. It manages the agent's lifecycle, emits structured JSONL events, handles heartbeats, performs periodic git auto-pushes, and monitors for error loops or token exhaustion.
*   **`ralph.sh`**: A lower-level execution harness that wraps the `claude` CLI, providing iteration safety caps and log trimming.
*   **`ralph-prompt-template.md`**: The standardized instruction set for agents, defining a three-phase workflow (Implementation, PR/CI, and Review Response) and strictly forbidding rebases.

**2. Fleet Management and Dispatch**
These scripts handle the lifecycle of sprites from local environments.
*   **`dispatch.sh`**: The entry point for assigning tasks. It supports "one-shot" prompts or persistent "Ralph" loops and handles repository cloning and prompt injection.
*   **`provision.sh`, `sync.sh`, `teardown.sh`, `status.sh`**: Shell wrappers for the `bb` Go binary (resolved via `lib_bb.sh`). They manage the infrastructure lifecycle based on YAML composition files.
*   **`sprite-bootstrap.sh`**: An idempotent setup script that prepares the remote environment, installing dependencies like `ripgrep`, `tmux`, and the `sprite-agent` binary.

**3. Monitoring and Observability**
A suite of tools provides visibility into the distributed agent fleet.
*   **`watchdog-v2.sh` / `watchdog.sh`**: Automated monitors that detect dead Claude processes or "stuck" agents (no commits/branch activity) and trigger redispatch or alerts.
*   **`health-check.sh` / `fleet-status.sh`**: Provide deep inspection of sprite state, calculating "staleness" based on file modification epochs and git activity.
*   **`refresh-dashboard.sh`**: A generator that aggregates sprite status and GitHub PR data into a static HTML dashboard.
*   **`webhook-receiver.sh`**: A Python-based micro-service that collects and logs events POSTed by remote `sprite-agents`.

**4. External Integrations**
*   **`pr-shepherd.sh`**: Monitors GitHub for PRs authored by sprites, tracking CI status and review requests.
*   **`sentry-watcher.sh`**: Polls the Sentry API to detect anomalies or fatal exceptions across the organization's projects.

#### Shared Logic and Libraries
*   **`lib.sh`**: The central library providing shell utilities for GitHub authentication resolution, sprite-to-environment variable mapping, and provider-specific configuration (Moonshot vs. OpenRouter).
*   **`onboard.sh`**: A utility to bootstrap local developer environments by inferring Fly.io and GitHub credentials.

#### Dependencies and Key Constraints
*   **CLI Dependencies**: Requires `sprite` (Fly.io agent CLI), `fly`, `gh` (GitHub CLI), and `yq` (mikefarah/yq) for composition parsing.
*   **Environment Variables**: Operation depends heavily on `ANTHROPIC_AUTH_TOKEN` (for Moonshot/Anthropic) or `BB_OPENROUTER_API_KEY` (for OpenRouter).
*   **PTY Requirement**: `sprite-agent.sh` prefers the `script` command to provide a PTY-backed execution environment, ensuring near-real-time log flushing.
*   **Git Policy**: The system explicitly forbids rebasing due to repository policy hooks; agents are instructed to use `--force-with-lease` only on their own feature branches.
*   **State Management**: Sprite state is signaled via specific extensionless files in the workspace (e.g., `TASK_COMPLETE`, `BLOCKED.md`), which the supervisors use to terminate loops.