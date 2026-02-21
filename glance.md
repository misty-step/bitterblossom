Bitterblossom is a Go-based orchestration framework and command-line interface (`bb`) designed to provision, manage, and supervise autonomous AI agents, referred to as "sprites." These agents operate within persistent, stateful Linux sandboxes on the Sprites.dev platform (hosted on Fly.io). The system follows a "Thin CLI, Thick Skills" architecture, where a minimal Go control plane handles deterministic transport and lifecycle management, while the agent's intelligence is governed by modular Markdown-based skills and personas.

### Architecture and Core Components

The system architecture is structured into three distinct layers:
*   **Transport & Orchestration Layer (Go):** The `cmd/bb` utility serves as the entry point, utilizing the `sprites-go` SDK to manage remote execution, file synchronization, and fleet status reconciliation. It bridges local user input with remote environments, bypassing the execution timeouts of standard agent tools.
*   **Supervisory Layer (Bash/Ralph):** The "Ralph" loop (`scripts/ralph.sh`) is a "sacred" autonomous execution harness that wraps the agent's primary process. It manages iteration limits, enforces wall-clock timeouts, monitors for "off-rails" behavior (e.g., error loops), and handles filesystem-based signaling for task completion.
*   **Intelligence Layer (Claude Code):** The system uses Claude Code (pinned to Sonnet 4.6) as the primary agentic harness. Agent behavior is specialized through persona files in `/sprites` and modular instruction sets in `/base/skills`.

### Key Directory and File Roles

*   **`cmd/bb/`**: Contains the Go implementation of the CLI. Key files include `dispatch.go` (task orchestration and repo synchronization), `setup.go` (environment provisioning and persona injection), `status.go` (concurrent fleet health probing), and `logs.go` (pretty-printed or JSON-structured log streaming).
*   **`/base`**: Defines the foundational configuration for all sprites. It includes `settings.json` for LLM model routing and event hooks, `CLAUDE.md` for engineering philosophy, and `/hooks` containing Python-based safety middleware like the `destructive-command-guard.py`.
*   **`/scripts`**: Houses the operational logic for the remote environments. `sprite-agent.sh` acts as the remote supervisor, while `ralph-prompt-template.md` provides the standardized instruction set for task execution.
*   **`/compositions`**: Uses versioned YAML files to define "Fae Court" models—hierarchical agent rosters with specific specializations (e.g., Bramble for backend, Fern for DevOps) and model provider routing.
*   **`/observations` & `/reports`**: Serve as the system's telemetry and feedback loop, housing qualitative journals (`OBSERVATIONS.md`) and quantitative E2E shakedown reports that track dispatch reliability and model performance.

### Key Dependencies

*   **Infrastructure**: Heavily dependent on the **Sprites.dev API** and **Fly.io** for machine lifecycle management.
*   **LLM Routing**: Relies on **OpenRouter** and specific **Claude 3.5 Sonnet** iterations. It requires the `ANTHROPIC_BASE_URL` to be redirected to OpenRouter to support non-Anthropic model backends through the Claude Code harness.
*   **Software Stack**: Requires **Go 1.24+** for the CLI, **Python 3** for lifecycle hooks, and the **GitHub CLI (`gh`)** for issue context and credential management.

### Technical Gotchas

*   **Completion Protocol**: The system relies on specific, extensionless signal files (e.g., `TASK_COMPLETE`, `BLOCKED.md`) created by the agent. Failure to produce these exact filenames can cause the Ralph loop to hang or exhaust its iteration limit.
*   **Credential Precedence**: The presence of both `ANTHROPIC_AUTH_TOKEN` and `ANTHROPIC_API_KEY` can cause authentication collisions. The system is designed to prioritize OpenRouter-based tokens to mitigate billing risks and model access issues.
*   **State Fragmentation**: Context is maintained through local `MEMORY.md` and `PLAN.md` files on the remote sprite. If a sprite is destroyed or the filesystem is not synchronized, the agent loses historical context for the project.
*   **Log Pollution**: Non-structured text (e.g., token exchange logs or terminal escape sequences) occasionally leaks into the JSON stdout, which can break machine-parseability for downstream telemetry tools.
*   **Shell Safety**: The `destructive-command-guard.py` uses regex and `shlex` for command interception; complex compound commands or obfuscated subshells may occasionally bypass these filters or trigger false positives.