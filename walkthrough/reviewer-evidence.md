# Reviewer Evidence

## Claim

This branch removes a dead compatibility layer that advertised `bb` commands and flags the binary does not implement, then aligns repo-local docs and shipped skills to the real transport surface.

## Before

- `scripts/provision.sh`, `scripts/sync.sh`, `scripts/status.sh`, and `scripts/teardown.sh` pretended to preserve an old wrapper API.
- `scripts/test_legacy_wrappers.sh` only asserted argument forwarding, not that the forwarded commands existed.
- `go run ./cmd/bb provision fern` failed with `unknown command "provision" for "bb"`.
- `go run ./cmd/bb status --format text` failed with `unknown flag: --format`.
- Multiple docs and Bitterblossom skills still taught those stale commands and flags.

## After

- The dead wrapper scripts and their wrapper-only test are gone.
- `cmd/bb/main_test.go` now codifies the real CLI boundary by asserting that legacy entrypoints are rejected.
- Repo-local docs and Bitterblossom skills now point to the supported commands: `setup`, `dispatch`, `status`, `logs`, `kill`, and `version`.

## Why This Matters

The repo’s ADRs say `bb` is a thin, deterministic transport. Leaving a fake compatibility surface in place made the operator boundary shallower and more confusing: readers had to know which docs were real, which wrappers were dead, and which flags only existed in history. This branch collapses that split-brain surface back to one truth.

## Evidence Bundle

### Files that prove the boundary

- `cmd/bb/main.go`
- `cmd/bb/main_test.go`
- `docs/CLI-REFERENCE.md`
- `README.md`
- `QA.md`
- `base/skills/bitterblossom-dispatch/SKILL.md`
- `base/skills/bitterblossom-monitoring/SKILL.md`

### Deleted surface

- `scripts/lib_bb.sh`
- `scripts/provision.sh`
- `scripts/sync.sh`
- `scripts/status.sh`
- `scripts/teardown.sh`
- `scripts/test_legacy_wrappers.sh`

## Protecting Checks

- `go test ./cmd/bb/...`
- `python3 -m pytest -q base/hooks scripts/test_conductor.py`

## Residual Gap

Older historical reports in `docs/shakedowns/` and `observations/` still mention now-removed commands as part of their historical narrative. They were left intact because they are evidence, not current operator guidance.
