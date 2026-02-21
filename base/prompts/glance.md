This directory functions as a historical archive for superseded Large Language Model (LLM) prompt templates and workflow protocols within the `bitterblossom` project. Its primary purpose is to preserve the legacy logic and state-machine patterns that governed the behavior of autonomous agents, or "Sprites," prior to the adoption of more robust automation scripts located in the project's root `scripts/` directory.

### Architecture and Key Files

The directory utilizes a flat structure of Markdown-based templates that define operational constraints and decision-making frameworks for LLM-driven agents.

*   **`README.md`**: Functions as a deprecation manifest. It identifies current canonical replacements and documents specific technical reasons for the retirement of these files, such as stale patterns and lack of modern integration flags.
*   **`ralph-loop-v2.md`**: Defines a "Self-Healing PR Protocol." This file outlines a complex state machine for autonomous agents, including CI monitoring via `gh pr checks`, automated remediation of build failures, and signaling protocols using specific terminal output patterns (e.g., `TASK_COMPLETE` or `BLOCKED`).
*   **`orientation-phase.md`**: Establishes a mandatory pre-task protocol focused on context loading and assumption verification. It instructs agents to ingest historical data from `MEMORY.md` and `CLAUDE.md`, verify the current repository state through active testing, and generate structured plans (`PLAN.md`) and post-task documentation (`LEARNINGS.md`) to facilitate knowledge transfer across the agent fleet.

### Dependencies and Technical Constraints

The archived logic relies on a specific suite of external tools and environment configurations:

*   **GitHub CLI (`gh`)**: Extensively utilized for polling PR status, retrieving logs, and managing comments via the GitHub API.
*   **Git**: Required for branch management and identity configuration, with templates often enforcing a hardcoded identity (specifically the `kaylee-mistystep` profile).
*   **Claude API**: The templates are optimized for the Claude LLM and are designed to be executed via dispatch scripts using specific permission bypass flags.
*   **Brittle Polling**: The architecture depends on manual polling loops and hardcoded `sleep` intervals (e.g., 120 seconds) rather than event-driven triggers or webhooks.

### Key Gotchas

*   **Stale Status Detection**: The templates rely on `stdout` detection for task status signaling, which has been superseded by more reliable file-based signaling in current versions of the project.
*   **Configuration Drift**: These files lack the necessary flags for modern "Claude Code" integrations and contain outdated Git configurations that may conflict with active developer environments.
*   **Hardcoded Identities**: The inclusion of hardcoded email and username configurations enforces a specific agent identity, which restricts flexibility and may cause attribution issues in modern workflows.