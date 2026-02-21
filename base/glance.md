The `/base` directory serves as the foundational configuration and behavioral framework for the Bitterblossom ecosystem, an autonomous AI-agent development environment. It functions as a "manual-as-code" repository that defines the safety constraints, operational protocols, and modular capabilities for AI "sprites" operating within a coordinated development workflow.

### Architecture and Core Components

The architecture is built on a tiered system of configuration, lifecycle hooks, and instructional templates that govern how LLM-based agents interact with codebases and external services.

*   **Orchestration Layer (`settings.json`)**: This file configures the runtime environment for the agents. It maps specific LLM models (primarily Claude 3.5 Sonnet via OpenRouter) and defines an event-driven hook system. It integrates Python-based security and quality scripts into the agent's tool-use lifecycle, specifically targeting `PreToolUse`, `PostToolUse`, and `Stop` events.
*   **Behavioral Framework (`CLAUDE.md`)**: Acts as the primary engineering philosophy and operational manual. It enforces a strict "Plan-Execute-Verify" loop, mandating the creation of `PLAN.md` for tasks and `LEARNINGS.md` for post-task reflection. It establishes technical standards such as TDD (Test-Driven Development) as the default, CLI-first interactions, and idiomatic code styles.
*   **Initialization Protocol (`DISPATCH-TEMPLATE.md`)**: A standardized prompt template used to instantiate agents for specific GitHub issues. It enforces a uniform execution protocol: issue analysis, branch creation, implementation, testing, and pull request generation.

### Subdirectory Roles

*   **`/commands`**: Contains Markdown-based playbooks for standardized workflows. The primary focus is repository hygiene and Git lifecycle management, enforcing Conventional Commits and linear history.
*   **`/hooks`**: Houses Python middleware that provides safety and immediate feedback. This includes the `destructive-command-guard.py` for intercepting dangerous shell commands and `fast-feedback.py` for non-blocking, language-specific linting and type-checking.
*   **`/skills`**: A modular library of "Skill Mounts"—specialized instruction sets and tool configurations (e.g., monitoring, deployment, git mastery) that are injected into the agent's environment at runtime.
*   **`/archive`**: Preserves legacy prompt templates and superseded state-machine patterns, providing historical context for the evolution of the sprite protocols.

### Key Dependencies

*   **LLM Infrastructure**: Heavily dependent on the **OpenRouter API** and specific **Claude 3.5 Sonnet** model iterations.
*   **External Tooling**: Relies on system-level binaries including **Git**, **GitHub CLI (`gh`)**, and **Fly.io CLI** for infrastructure management.
*   **Runtime Environments**: Requires **Python 3** for lifecycle hooks and standard web development stacks (**Node.js/pnpm**, **Rust/Cargo**, **Go**) for the automated validation suites.

### Technical Gotchas

*   **Permission Bypass**: The environment is configured to skip dangerous mode prompts (`skipDangerousModePermissionPrompt: true`). This places critical reliance on the `destructive-command-guard.py` to prevent accidental history rewrites or unauthorized data deletion.
*   **State Fragmentation**: Context retention is managed via local Markdown files (`MEMORY.md`, `PLAN.md`). If an agent fails to update these files during a session, subsequent dispatches may lack necessary context, leading to redundant work or architectural drift.
*   **Shell Parsing Limitations**: The security guard uses `shlex` and regex for command analysis. While robust against standard shell syntax, highly complex or obfuscated compound commands may trigger false positives or potentially bypass the filter.
*   **Environment Sensitivity**: The system is sensitive to the presence of specific environment variables (e.g., `ANTHROPIC_BASE_URL`, `FLY_ORG`) and treats malformed configurations or trailing whitespace in `.env.bb` files as fatal errors.