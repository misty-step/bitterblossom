This directory serves as a historical archive for superseded Large Language Model (LLM) prompt templates and workflow protocols within the `bitterblossom` project. Its primary purpose is to preserve legacy logic and patterns that have been replaced by more robust automation scripts, specifically `scripts/ralph-prompt-template.md`.

### Architecture and Key Files

The directory follows a flat structure containing documentation and template files that once governed the autonomous behavior of "Sprites" (automated agents).

*   **`README.md`**: Acts as the deprecation manifest. It identifies the current canonical replacement and provides specific reasons for the retirement of legacy files, such as stale patterns and missing integration flags.
*   **`ralph-loop-v2.md`**: A detailed technical prompt template defining a "Self-Healing PR Protocol." It outlines a complex state machine for autonomous agents including:
    *   **CI Monitoring**: Polling logic using `gh pr checks` and `gh run view` to detect and debug build failures.
    *   **Automated Remediation**: Instructions for fixing CI errors, addressing GitHub review comments via API interaction, and resolving merge conflicts through rebasing.
    *   **Signaling**: A protocol for task completion (`TASK_COMPLETE`) or failure (`BLOCKED`) using specific file creation and terminal output patterns.

### Dependencies and Technical Constraints

The legacy logic within this directory relies on several external tools and specific environment configurations:

*   **GitHub CLI (`gh`)**: Extensively used for PR status checks, log retrieval, and API-based comment management.
*   **Git**: Required for branch management, rebasing, and identity configuration (specifically hardcoded to the `kaylee-mistystep` profile).
*   **Claude API**: The templates are designed to be wrapped in a dispatch script and piped into the Claude LLM using specific flags (e.g., `--permission-mode bypassPermissions`).
*   **Brittle Polling**: The architecture relies on hardcoded `sleep` intervals (e.g., 120 seconds) and manual polling loops rather than event-driven triggers.

### Key Gotchas

*   **Stale Patterns**: The templates utilize `stdout` detection for task status, which has been superseded by more reliable file-based signaling in newer versions.
*   **Configuration Drift**: These files lack necessary flags for modern "Claude Code" integrations and contain outdated Git configurations that may conflict with current developer environments.
*   **Hardcoded Identities**: The templates contain hardcoded email and username configurations that enforce a specific agent identity, limiting flexibility.