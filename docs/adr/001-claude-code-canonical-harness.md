# ADR-001: Claude Code as Canonical Sprite Harness

- **Status:** Accepted
- **Date:** 2026-02-10
- **Supersedes:** OpenCode-only decision (2026-02-09)

## Context

On February 9, 2026, Bitterblossom documentation was updated to declare OpenCode as the sole agent harness, deprecating Claude Code. The rationale was that Claude Code could not reliably use non-Anthropic models — it silently hung when pointed at OpenRouter or Moonshot endpoints for Kimi K2.5 and GLM 4.7.

However, this limitation was discovered to be solvable. PR #136 (`feat/proxy-provider`) implements a proxy provider that enables Claude Code to route through OpenRouter to any supported model. With this proxy in place, Claude Code can dispatch to Kimi K2.5, GLM 4.7, and any other OpenRouter-hosted model without hanging.

Additionally, during the brief OpenCode-only period, several stability and usability issues with OpenCode were encountered that required workarounds.

## Decision

**Claude Code is the canonical agent harness for Bitterblossom sprite dispatch.** OpenCode remains available as an alternative harness, but is not the default path.

All sprite dispatch flows, documentation, and tooling should assume Claude Code as the default harness, using the proxy provider for non-Anthropic model routing when needed.

## Rationale

1. **Superior tool use and agentic capabilities.** Claude Code has mature, battle-tested tool calling, file editing, and multi-step reasoning that outperforms OpenCode in production coding tasks.

2. **Proxy provider solves the model limitation.** PR #136 enables Claude Code to route requests through OpenRouter to any model (Kimi K2.5, GLM 4.7, etc.), eliminating the original reason for the OpenCode decision.

3. **OpenCode stability issues.** OpenCode required multiple workarounds for reliability in sprite environments — including indexing overhead on S3-backed filesystems, no persistent daemon mode, and configuration brittleness.

4. **PTY mode is the proven production pattern.** Claude Code's `--yolo` PTY mode is the established, tested dispatch pattern across the Misty Step infrastructure. Switching to OpenCode introduced unnecessary risk.

5. **Better ecosystem and support.** Claude Code has stronger documentation, wider adoption, and direct Anthropic support. This reduces maintenance burden and onboarding friction.

## Consequences

- All Bitterblossom documentation (`AGENTS.md`, `docs/SPRITE-ARCHITECTURE.md`) updated to reflect Claude Code as canonical.
- The OpenCode-only migration checklist in `docs/SPRITE-ARCHITECTURE.md` is cancelled.
- Cerberus Council reviewers should accept Claude Code references in PRs — they are not deprecated.
- Sprite environment configuration uses `ANTHROPIC_BASE_URL` / `ANTHROPIC_AUTH_TOKEN` for proxy routing (not `OPENROUTER_API_KEY` alone).
- OpenCode remains available as an alternative harness but is not the canonical dispatch path.
