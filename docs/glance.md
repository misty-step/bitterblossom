The `/docs` directory is the technical reference for Bitterblossom's current two-surface architecture:

- `cmd/bb`: thin deterministic transport for setup, dispatch, status, logs, and recovery
- `scripts/conductor.py`: run-centric control plane for intake, review orchestration, CI waits, and merge

### Architecture and Purpose
The docs exist to keep that boundary explicit. `bb` owns mechanical transport concerns such as connectivity probes, repo sync, streaming execution, and recovery. The conductor and Ralph loop own workflow judgment, durable run state, and task completion semantics.

### Key File Roles
- `adr/001-claude-code-canonical-harness.md` and `adr/002-architecture-minimalism.md`: the canonical harness and thin-CLI decisions.
- `CLI-REFERENCE.md`: the supported `bb` surface.
- `CONDUCTOR.md`: the control-plane contract and operator workflow.
- `COMPLETION-PROTOCOL.md`: the signal-file contract used by Ralph and transport checks.
- `shakedowns/`: operator evidence and recovery learnings.

### Dependencies and Technical Constraints
- The transport is pinned to the `sprites-go` SDK and Claude Code Sonnet 4.6 via OpenRouter-backed sprite settings.
- `TASK_COMPLETE` is canonical; `TASK_COMPLETE.md` remains a compatibility fallback.
- Historical docs in `archive/` describe older designs and should not be treated as the current CLI contract.
