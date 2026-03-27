# Break The Conductor Bootstrap Cycle

Priority: medium
Status: ready
Estimate: L

## Goal
Remove the `bootstrap -> sprite -> workspace` dependency cycle and start paying down the largest conductor modules in the hot path.

## Oracle
- [ ] `mix xref graph --format cycles` reports no cycle across `bootstrap`, `sprite`, and `workspace`
- [ ] The cycle break preserves current runtime behavior and test coverage
- [ ] The resulting module boundaries are documented or obvious from the code

## Notes
- Derived from the 2026-03-27 agent-readiness audit
- Corresponds to umbrella issue #810
