# Ticket Format

`backlog.d/<nnn>-<kebab-slug>.md`, bare numeric IDs.

```markdown
<Title as imperative sentence>

Priority: P0–P3 · Status: pending|ready|blocked|in-progress|done|shipped|abandoned · Estimate: S–XL

**Goal**
<one sentence — outcome, not mechanism>

**Oracle**
- [ ] <mechanically verifiable; rough oracles are still oracles>

**Verification System**
- Claim:
- Falsifier:
- Driver:
- Grader:
- Evidence packet:
- Cadence:

**Notes**
<constraints, prior art, open questions>
```

The Verification System fields mean what
`primitives/shared/references/verification-system-first.md` says they mean —
this template just gives them a home on the ticket; do not redefine them here.

Epics are the default shape for strategic emissions: same file, plus a
`## Children` section — ordered child outcomes that stay inline until picked
up, then graduate to their own tickets. An epic still needs a Goal and an
Oracle for the whole arc; an "umbrella" file with no done criteria is
storage, not an epic.

Every active ticket has Goal + Oracle; fix or demote anything that doesn't.
For M+ work or anything promoted to `Status: ready`, fill in the Verification
System fields. M+ tickets promoted to `Status: ready` also follow `/shape`'s
`references/prd-ticket-quality.md`; otherwise they stay raw ideas. When
grooming Harness Kit itself, apply the product lens in
`references/backlog-doctrine.md`.
