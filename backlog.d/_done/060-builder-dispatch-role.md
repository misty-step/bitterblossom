# Add a manual Rust builder dispatch role

Priority: P1 | Status: done | Estimate: M

## Goal

Let operators dispatch focused code-authoring work through Bitterblossom
without introducing a hidden multi-agent orchestrator or a generic
`deliverer` agent.

## Oracle

- [x] Agent launch metadata supports a role and curated skill/profile
      references.
- [x] The operator plane includes a manual-only `bb-builder-rust` agent and
      `build` task.
- [x] The builder card requires a branch, report, repo gate, and no-merge
      boundary.
- [x] `bb check --json` exposes the builder role and skills to agents.

## Notes

This is the first tractable slice of the broader versioned-agent-contract
work in `053`. Runtime skill projection, Daedalus-generated launch contracts,
and multi-step deliver orchestration remain outside this slice.
