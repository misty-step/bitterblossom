# Cerberus Substrate Findings

Date: 2026-06-19

Source: local pasted report, "Modern coding-agent systems as execution
substrates", current through 2026-06-19.

## Finding

The report argues OpenCode is a better substrate than OMP for a production
automated code-review system because OpenCode is server/session first. That
fits durable review execution, event collection, concurrent reviewer sessions,
model routing, and future control-plane integration better than a terminal-first
local wrapper.

The Cerberus restart now reflects this by treating OpenCode as the preferred
production master substrate and OMP as a local/power-user fallback. The product
rule remains one master reviewer with dynamic runtime lanes, not predefined
static reviewer subagents.

## Implication for Bitterblossom

Bitterblossom remains the event plane. It should not learn code-review judgment
or hardcode Cerberus reviewer topology. Its responsibility is to dispatch,
record, budget, and surface trusted external review artifacts.

For Cerberus-backed work, Bitterblossom should care about:

- request and artifact paths;
- run id, substrate name, model, cost, timeout, and state;
- whether the artifact is `completed`, `completed_degraded`, or `failed`;
- finding fingerprints and publication/trusted-surface status;
- external side effects such as PR comments or Check Runs.

It should not care whether Cerberus's master chose zero, one, or many runtime
subagents.

## Compatibility Note

This note does not reverse Bitterblossom's existing production harness decision.
Bitterblossom's canonical dispatch runtime is still governed by its own ADRs
and live sprite evidence. The OpenCode preference here is specifically for
Cerberus as a review substrate, where session-level programmatic execution is
the central product requirement.

## Follow-Up Shape

When Cerberus becomes a trusted external review surface, prefer a simple
Bitterblossom task that invokes Cerberus, stores the resulting
`ReviewArtifact.v1`, and lets existing governance read that artifact. Avoid a
new internal review council unless live evidence shows Cerberus artifacts are
insufficient.
