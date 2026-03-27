# Multi-repo agent support

Priority: high
Status: ready
Estimate: M

## Goal
Each sprite can work across multiple repos. fleet.toml already supports `repo` per sprite defaults. Agents reference their assigned repo in the loop prompt. Infrastructure provisions workspace per repo.

## Non-Goals
- Cross-repo PRs (one sprite, one repo per loop iteration)

## Oracle
- [ ] fleet.toml can declare sprites assigned to different repos
- [ ] Launcher passes repo to loop prompt
- [ ] Agent picks issues from its assigned repo
- [ ] Tested with at least 2 repos
