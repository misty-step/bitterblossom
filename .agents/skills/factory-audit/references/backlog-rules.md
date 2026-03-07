# Backlog Rules

Convert findings into backlog changes with minimal churn.

## File a new issue when

- the finding is new
- the existing issue does not clearly cover the failure mode
- the run exposed a sharper acceptance criterion than current backlog text

## Comment on an existing issue when

- the problem is already tracked
- the run adds timestamps, reproduction steps, or stronger priority evidence
- the issue scope is still right

## Do not file when

- the note is cosmetic and isolated
- it does not change behavior, operator cost, or architectural clarity
- it is just a one-off artifact cleanup with no repeated pattern

## Severity guide

- `P0`: stale state, false success, destructive merge, or misleading correctness signal
- `P1`: merge-governance hole, security/control-plane risk, auth breakage, worker liveness failure
- `P2`: observability gaps, manual recovery pain, poor progress visibility, noisy surfaces
- `P3`: docs polish, naming cleanup, cosmetic hygiene
