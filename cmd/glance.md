### Technical Overview: Bitterblossom Command-Line Interface (`cmd/bb`)

The `cmd` directory serves as the entry point for the Bitterblossom ecosystem's command-line tools, primarily housing the `bb` utility. This tool functions as a centralized control plane for orchestrating agentic tasks—specifically the "ralph" execution loop—across a fleet of "sprites" (ephemeral, remote virtual environments). It bridges local user input and remote execution by managing the full lifecycle of agent processes, from environment provisioning to real-time safety monitoring.

#### Architecture and Purpose
The CLI is built on the Cobra framework and follows a provider-consumer architecture. It acts as a client to the Sprites API, utilizing the `sprites-go` SDK for remote command execution and filesystem abstraction. The architecture is designed to delegate high-compute agentic loops to remote Linux environments while maintaining local observability and administrative control. It emphasizes a "safety-first" approach, incorporating active monitoring to detect and terminate runaway or stalled processes.

#### Key File Roles
- **`main.go`**: The application entry point. It initializes the command hierarchy, configures global error handling, and manages authentication by exchanging Fly.io API tokens for Macaroon-based Sprites bearer tokens.
- **`dispatch.go`**: The primary orchestration engine. It handles pre-flight connectivity checks, synchronizes GitHub repositories to the remote environment, renders prompt templates, and initiates the `ralph.sh` loop. It maintains a cancellation context to terminate remote tasks if safety thresholds are breached.
- **`setup.go`**: Manages the initial provisioning of remote sprites. It automates directory creation, uploads base configuration files (e.g., `CLAUDE.md`, `settings.json`), and configures Git credentials to prepare the environment for agent activity.
- **`offrails.go`**: Implements a safety monitoring system that tracks agent activity via atomic timestamps. It detects "silence" (inactivity) or "error loops" (repetitive tool failures) and triggers an automatic abort to prevent resource waste.
- **`stream_json.go`**: A specialized parser for the Claude Code `stream-json` format. It provides dual-mode output: a "pretty" mode for terminal-based human consumption and a raw JSON mode for structured logging. It also extracts tool-specific errors for the off-rails detection system.
- **`status.go` & `logs.go`**: Provide fleet-wide observability. `status.go` uses concurrent probes to report the health and activity of all managed sprites, while `logs.go` implements remote `tail` logic to stream agent logs to the local console.
- **`kill.go`**: A maintenance utility that executes remote bash scripts to identify and terminate orphaned or stale agent processes using `pgrep` and `pkill` with specific regex patterns.
- **`sprite_workspace.go`**: A utility for dynamic path discovery on remote sprites, locating active working directories by searching for signal files like `.dispatch-prompt.md`.

#### Dependencies and Gotchas
- **SDK Dependency**: Relies heavily on `github.com/superfly/sprites-go` for API interaction and remote execution.
- **Authentication Flow**: Requires either `SPRITE_TOKEN` or `FLY_API_TOKEN`. When using the latter, `SPRITES_ORG` must be configured to facilitate the token exchange process.
- **Remote Environment Requirements**: The tool assumes the remote sprite is a Linux environment with `bash`, `pgrep`, and `pkill` pre-installed. Setup logic is hardcoded to the `/home/sprite/` directory structure.
- **Error Detection Limits**: The off-rails detector uses string-matching for error deduplication; minor variations in error messages (such as unique timestamps or IDs) may prevent the detector from identifying a repeating error loop.
- **Concurrency Management**: Fleet status checks utilize a semaphore pattern to throttle concurrent network requests, preventing local socket exhaustion when querying large numbers of sprites.