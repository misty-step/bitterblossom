# Observation Journal

Kaylee's notes on what's working and what isn't. **Update after every meaningful task dispatch.**

This is the core feedback loop. Without observations, compositions are just guesses.

## How to Log

```markdown
### YYYY-MM-DD — <Sprite> — <Task Type>
**Task:** Brief description
**Repo:** org/repo (if applicable)
**Outcome:** Success / Partial / Failed
**Time:** Approximate duration
**Routing decision:** Why this sprite? Was it the right call?
**Quality notes:** What went well, what didn't
**Action:** Keep / Adjust routing / Change base config / Investigate
```

## What to Watch For

- **Routing accuracy:** Did the sprite's specialization help, or would any sprite have done equally well?
- **Cross-domain tasks:** Tasks that touch multiple specializations — who handles them best?
- **Config drift:** Are sprites modifying their own CLAUDE.md in useful ways? Harvest those changes for base.
- **Failure patterns:** Same type of mistake across sprites = base config issue. Sprite-specific mistake = persona issue.
- **Time patterns:** Some task types consistently faster on certain sprites? That's signal.

## Experiment Log

Track A/B tests and composition experiments here.

### Composition: Fae Court v1 (2026-02-05)
**Hypothesis:** 5 specialized sprites > generalists
**Status:** Active, collecting observations
**Observations so far:** 0

---

## Observations

_Begin logging after first dispatched task completes._

### 2026-02-05 — Thorn — Testing (First Dispatch Ever)
**Task:** Write notification throttling tests (Heartbeat issue #57)
**Repo:** misty-step/heartbeat
**Outcome:** Success — PR #92 opened
**Time:** ~20 minutes (dispatched ~12:55, PR opened ~13:15)
**Routing decision:** Thorn (Quality & Security) — testing task, perfect match
**Quality notes:**
- Wrote 172+ lines of tests across 2 files
- Proper mock setup for email module
- Created helper mutation for test setup (testSetNotifiedAt)
- Conventional commit, proper branch naming
- Opened PR automatically
**Issues observed:**
- Did not create feature branch until commit time (worked on master, then branched)
- Log output fully buffered (0 bytes until completion) — need tmux-based observability
- Still running after PR opened (probably memory-reminder hook or final checks)
**Action:** Keep routing testing work to Thorn. Fix log observability for next dispatch.
