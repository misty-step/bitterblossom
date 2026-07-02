# Preserve low-severity Canary incident fidelity for drills and responder routing

Priority: P1 | Status: ready | Estimate: S

## Goal

When a low-severity Canary error opens an incident, the incident payload and
incident detail should either preserve `severity=low` or expose the explicit
normalization rule that raised it.

## Evidence

Live drill on 2026-07-02:

- Error event: `ERR-y4y82bm9zq61`
- Error class: `BitterblossomIncidentTriageDrill_20260702T201143Z`
- Posted severity: `low`
- Timeline event: `EVT-knom6q0limw8`, `error.new_class`, severity `low`
- Incident: `INC-ay76lctwao3z`
- Timeline event: `EVT-nmbg4bf9h6xq`, `incident.opened`, severity `medium`
- Incident detail also reports severity `medium`

This made a low-severity drill look like a medium incident to downstream
responders.

## Oracle

- [ ] Canary-owned incident creation either preserves the originating signal's
      low severity or includes a machine-readable `severity_reason` /
      `normalized_from` field.
- [ ] BB's pinned `canary.incident_event.v1` fixture is refreshed only if the
      producer contract intentionally changes.
- [ ] A low-severity synthetic incident drill demonstrates the expected
      severity in timeline, incident detail, and webhook payload.
- [ ] `./scripts/verify.sh` passes in BB after any pinned-contract update.

## Non-goals

Do not add BB-side severity heuristics to guess the producer's intent. Canary
owns the incident severity contract.
