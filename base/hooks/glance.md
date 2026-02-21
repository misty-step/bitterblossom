### Technical Overview: /base/hooks

This directory contains a suite of Python-based lifecycle hooks designed to manage the safety, quality, and context retention of automated AI agents (referred to as "sprites") operating within a development environment. The architecture follows an event-driven pattern where scripts are triggered at specific stages of a tool's execution—specifically before a tool runs (`PreToolUse`), after a tool completes (`PostToolUse`), or when a session terminates (`Stop`).

#### Core Architecture and Key Files

*   **Destructive Command Guard (`destructive-command-guard.py`)**: 
    Acts as a security middleware for the `Bash` tool. It intercepts shell commands to prevent operations that could irrecoverably alter remote Git history or bypass quality gates. Key features include:
    *   **Recursive Shell Parsing**: Uses `shlex` and custom regex to decompose compound commands (linked by `&&`, `||`, `;`, or `|`) and extract nested subshells (`$()` or backticks).
    *   **Wrapper Awareness**: Recognizes and strips command prefixes like `sudo`, `env`, `nice`, and `time` to inspect the underlying executable.
    *   **Protection Logic**: Specifically blocks direct pushes to `main`/`master`, force pushes without lease, `git rebase`, `git reset --hard`, and destructive `gh` (GitHub CLI) operations.
    *   **Decision Interface**: Communicates with the host system via a structured JSON schema, returning a `deny` decision and a descriptive reason when violations are detected.

*   **Fast Feedback (`fast-feedback.py`)**: 
    A performance-oriented quality assurance hook that runs immediately after file modification tools. 
    *   **Project Detection**: Identifies the programming language (TypeScript, Python, Rust, or Go) by scanning the current working directory for markers like `tsconfig.json`, `pyproject.toml`, or `Cargo.toml`.
    *   **Asynchronous-style Validation**: Executes lightweight checks—such as `ruff check`, `tsc --noEmit`, or `cargo check`—with strict timeouts (15–60 seconds). 
    *   **Non-Blocking Design**: Always exits with code 0; it reports errors to the agent's standard output to encourage self-correction without halting the workflow.

*   **Memory Reminder (`memory-reminder.py`)**: 
    A session-end hook triggered by "Stop" events. It serves as a prompt for the agent to persist context, insights, and patterns into a `MEMORY.md` file, ensuring continuity across disparate work sessions.

*   **Validation Suite**: 
    The directory includes comprehensive `pytest` implementations (`test_destructive_command_guard.py`, `test_fast_feedback.py`, etc.) that mock `stdin` and `subprocess` calls to validate parsing logic and edge-case handling. `test_workspace_contract.py` specifically enforces a structural contract ensuring that workspace environment variables are consistently defined across the broader project's shell library.

#### Key Dependencies and Gotchas

*   **External Binaries**: The hooks rely on the presence of system-level tools including `git`, `gh`, `ruff`, `npx/tsc`, `cargo`, and `go`. If a binary is missing, the hooks are designed to fail silently or skip the check rather than crashing the agent session.
*   **Shell Complexity**: The `destructive-command-guard.py` uses a conservative parsing strategy. While it handles nested subshells and brace grouping, highly unconventional shell syntax or aliased commands may bypass the guard or trigger false positives.
*   **Input Protocol**: All hooks expect context (like the current working directory or tool inputs) to be passed via `stdin` as JSON. They are sensitive to the structure of this payload, typically falling back to default behaviors (like `os.getcwd()`) if the JSON is malformed.
*   **Execution Time**: `fast-feedback.py` is subject to hardcoded timeouts. In very large monorepos, these checks may time out before completion, resulting in no feedback.