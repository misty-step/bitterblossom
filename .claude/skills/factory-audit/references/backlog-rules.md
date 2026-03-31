# Backlog Rules

Convert findings into backlog changes with minimal churn.

## Create a new backlog item when

- the finding is new and not covered by any existing `backlog.d/` item
- the run exposed a sharper acceptance criterion than current backlog text
- the friction pattern is repeatable, not a one-off

## Update an existing backlog item when

- the problem is already tracked in `backlog.d/`
- the run adds timestamps, reproduction steps, or stronger priority evidence
- the item scope is still right but needs refinement

## Do not file when

- the note is cosmetic and isolated
- it does not change behavior, operator cost, or architectural clarity
- it is just a one-off artifact cleanup with no repeated pattern

## Severity guide

- `P0/critical`: stale state, false success, destructive merge, or misleading correctness signal
- `P1/high`: merge-governance hole, security/control-plane risk, auth breakage, sprite liveness failure
- `P2/medium`: observability gaps, manual recovery pain, poor progress visibility, noisy surfaces
- `P3/low`: docs polish, naming cleanup, cosmetic hygiene
