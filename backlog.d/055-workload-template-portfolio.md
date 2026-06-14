# Create production-shaped workload templates beyond the demo plane

Priority: P1 | Status: pending | Estimate: XL

## Goal

Make Bitterblossom's value prop copyable by shipping a small portfolio of
production-shaped workload templates after the control plane is hardened.

## Oracle

- [ ] At least three templates exist beyond `examples/demo-plane`: review
      factory, canary/incident responder, and docs or monitor watcher.
- [ ] Each template includes plane/task/agent files, lane card, budgets,
      trigger examples, containment filters, expected JSON outputs, and a
      local validation recipe.
- [ ] Each template runs through `bb check`; dev/test versions do not require
      live credentials.
- [ ] The README points cold users to a template selection path.
- [ ] No template adds workload-specific Rust code.

## Children

1. Extract a credential-free review-factory template from `plane/`.
2. Shape a canary/incident responder template from the Tansy direction.
3. Shape a docs-sync or monitor watcher template.
4. Add template validation to the repo gate where feasible.
5. Document when to use a template versus authoring a custom task.

## Notes

Why: the product lane found a demo-only public example set while `project.md`
names a broader workload roadmap.

Evidence:

- `project.md:93-100` names review factory, canary incident responder, and
  monitor/deploy watchers.
- `examples/demo-plane/` is currently the only public cloneable example plane.
- `plane/` has real review/verdict tasks but is production-owned, not a clean
  starter template.

050 is complete. Before increasing real reflex volume, pair these templates
with the recovery and operations evidence from 051/054 rather than treating
template creation alone as production readiness.
