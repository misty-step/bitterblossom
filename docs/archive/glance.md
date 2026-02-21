The `/docs/archive` directory contains the legacy architectural blueprints, design decisions, and migration strategies for Bitterblossom, an AI agent fleet management system. The documentation traces the evolution from a shell-script-based orchestration layer to a robust Go-based control plane (`bb` CLI) designed to manage persistent, stateful agent environments known as "Sprites."

### Purpose and Architecture
The directory documents a system designed to automate software development through a fleet of autonomous agents. The architecture is built on three primary layers:
*   **CLI Layer (`cmd/bb/`):** A Go-based entry point facilitating fleet reconciliation, task dispatch, and health monitoring.
*   **Domain Layer:** Core logic governing the "Sprite" state machine (provisioned, idle, working, blocked, dead) and fleet reconciliation (converging actual VM state to desired YAML state).
*   **Infrastructure Layer:** Integration with the Sprites.dev API for compute substrate, GitHub for version control, and OpenRouter for LLM inference.

### Key File Roles
*   **`REDESIGN.md` & `MIGRATION.md`:** Outline the transition from brittle shell scripts to a Go control plane. They define the "Ousterhout × UNIX" philosophy, emphasizing deep modules, information hiding, and JSONL-based event streaming for composability.
*   **`SPRITE-ARCHITECTURE.md` & `SPRITES.md`:** Define Sprites as persistent Linux sandboxes rather than ephemeral containers. They establish Claude Code as the canonical agent harness and detail the "Ralph Loop"—a self-correcting execution cycle.
*   **`DESIGN-DECISIONS.md`:** Documents the visual standards (Overmind hexagonal aesthetic), the permission hierarchy between the primary user (Kaylee) and agents (Sprites), and the "Option A/B" evolution of GitHub account isolation.
*   **`QUALITY-GATEWAY.md` & `SENTRY-SETUP.md`:** Describe the observability and safety infrastructure, including a declarative quality spec (`quality-spec.yml`), PR shepherding scripts, and standardized Sentry alert rules across the `misty-step` organization.
*   **`contracts.md`:** Specifies the JSON/NDJSON interface standards for machine readability, including response envelopes and deterministic exit codes.
*   **`PROVIDERS.md`:** Codifies the canonical runtime environment, specifically Moonshot Kimi K2.5 via OpenRouter, and the required environment variables for sprite injection.

### Key Dependencies
*   **Compute:** [Sprites.dev](https://sprites.dev) for isolated, persistent Linux environments with 100GB durable storage.
*   **LLM Orchestration:** OpenRouter as the unified provider layer for Kimi K2.5 and GLM models.
*   **Agent Harness:** Claude Code (restored as the primary harness via a proxy provider).
*   **Security & Monitoring:** TruffleHog for verified secret detection and Sentry for error tracking across Next.js projects.

### Important Gotchas
*   **Model Compatibility:** Claude Code traditionally hangs when using non-Anthropic models; a custom proxy provider is required to translate requests for OpenRouter/Moonshot compatibility.
*   **Dry-Run Defaults:** All mutating Go CLI commands (`provision`, `dispatch`, `teardown`) are dry-run by default and require an explicit `--execute` flag to prevent accidental infrastructure changes.
*   **Permission Boundaries:** Initial sprite implementations share a single GitHub identity, relying on client-side hooks (`destructive-command-guard`) rather than server-side IAM for protection against destructive operations.
*   **Environment Injection:** The `sprite exec -env` flag is noted as unreliable; persistent configuration must be written directly to `.bashrc` or local environment files on the sprite.
*   **Auth Token Fallbacks:** While `OPENROUTER_API_KEY` is the canonical auth variable, the system maintains a legacy fallback to `ANTHROPIC_AUTH_TOKEN` for backward compatibility.