### Technical Overview: Bitterblossom CLI (`bb`)

The `bb` directory contains the source code for the Bitterblossom command-line interface, a tool designed to orchestrate and monitor agentic tasks—specifically the "ralph" loop—running on remote "sprites" (ephemeral virtual environments). The CLI acts as a control plane for setting up remote environments, dispatching prompts, and managing the lifecycle of agent processes.

#### Architecture and Purpose
The application is built using the Cobra CLI framework and follows a provider-consumer model where the CLI communicates with the Sprites API via the `sprites-go` SDK. The architecture emphasizes remote execution and observability, delegating heavy lifting to bash scripts on the sprite while maintaining local control over task safety and log streaming.

#### Key File Roles
- **`main.go`**: The entry point that initializes the Cobra command hierarchy. It handles authentication by exchanging Fly.io API tokens for Sprites-specific bearer tokens and manages global error handling through a custom `exitError` type.
- **`dispatch.go`**: The core orchestration engine. It performs pre-flight connectivity checks, synchronizes GitHub repositories on the remote sprite, renders prompt templates, and executes the `ralph.sh` agent loop. It utilizes a `context.CancelCauseFunc` to terminate tasks if "off-rails" behavior is detected.
- **`setup.go`**: Manages the initial provisioning of a sprite. It creates directory structures, uploads base configuration files (e.g., `CLAUDE.md`, `settings.json`), configures Git credentials, and clones target repositories.
- **`offrails.go`**: Implements a safety monitoring system. It tracks activity via an `atomic.Int64` timestamp and monitors for "silence" (lack of output) or "error loops" (repeated identical tool errors), triggering an automatic abort if thresholds are exceeded.
- **`stream_json.go`**: A specialized writer that parses the Claude Code `stream-json` output format. It provides two modes: a "pretty" mode for human-readable terminal output and a raw JSON mode for structured data processing. It also extracts tool errors to feed the off-rails detector.
- **`status.go` & `logs.go`**: Provide observability. `status.go` uses concurrency to probe the availability and activity state of an entire fleet of sprites, while `logs.go` implements remote `tail` logic to stream agent logs to the local terminal.
- **`kill.go`**: A maintenance utility that executes a remote bash script to identify and terminate stale or orphaned agent processes using `pgrep` and `pkill` with specific regex patterns.
- **`sprite_workspace.go`**: A utility for dynamically discovering the active working directory on a remote sprite by searching for specific signal files like `.dispatch-prompt.md` or `ralph.log`.

#### Dependencies and Gotchas
- **SDK Dependency**: Relies heavily on `github.com/superfly/sprites-go` for remote command execution and filesystem abstraction.
- **Token Exchange**: The tool expects either `SPRITE_TOKEN` or `FLY_API_TOKEN`. If using the latter, it performs a network request to exchange it for a macaroon-based Sprite token, which requires `SPRITES_ORG` to be correctly configured.
- **Remote Environment Assumptions**: Commands assume a Linux environment on the sprite with `bash`, `pgrep`, and `pkill` available. The setup process specifically targets the `/home/sprite/` directory structure.
- **Error Normalization**: The off-rails detector uses basic string truncation for error grouping; exact-match logic is used for deduplication, meaning minor variations in error messages (like timestamps) may bypass the repeat-threshold detector.
- **Concurrency**: The fleet status check uses a semaphore pattern to limit concurrent network probes, preventing local resource exhaustion when managing many sprites.