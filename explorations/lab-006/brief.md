# LAB-006 converged operator flow

LAB-006 is the single review candidate selected from the LAB-005 design spine. It
is a fixture-backed interaction contract, not a claim that Bitterblossom's
blocked workflow-store and workflow-runtime APIs already exist.

## Binding direction

- Use one Precision Register system with a persistent, icon-forward left rail.
- Keep Bitterblossom's flower mark and Lucide-style product icons.
- Put Workflows, Runs, Agents, and Spend in the rail. Never use bottom navigation.
- Make workflow detail a standalone page led by name, description, stable
  configuration, an actually connected execution topology, and `View runs`.
- Treat runs as durable history. Sort executing work first and make every row a
  route to a standalone detail page. Do not invent a `current run` ontology.
- Treat agents as reusable declarations, not scarce slots. Use one expandable
  roster and the same declarations in workflow authoring.
- Treat spend as reporting plus enforceable governors: receipts, estimates,
  unavailable coverage, ceilings, workflow attribution, and control scopes stay
  visibly distinct.
- Author workflows in seven wired steps: Goal, Trigger, Agents, Authority,
  Limits, Test, Activate. LLM goal enhancement is an optional proposal and can
  never silently replace the operator's original.

## Aesthetic fence

Misty Step Aesthetic is pinned at commit `2bf1d8a`. Light and dark are equal.
The composition uses the kit's square controls, hairlines, semantic ink, status
colors, typography, spacing, and logo contract. One local derived line token
fills a missing strong-line role without introducing a new palette.

The known typography weakness is routed upstream rather than hidden with a
Bitterblossom-only type system. Powder card `aesthetic-typography-hierarchy`
owns that follow-up.

## Review boundary

The review surface must expose every top-level route, both workflow definitions,
every run row and run detail, collapsible agent declarations, editable spend
governors, and every creation step at desktop and phone widths. Fixture
provenance remains visible in product chrome. Controls that imply persistence
save only to the in-memory review fixture and say so.
