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

### 2026-02-12 — Bramble — Dispatch UX + Skills Mounting (Issue #252)
**Task:** Dispatch `misty-step/bitterblossom#252` via `bb dispatch --execute --wait` and verify end-to-end agent workflow.
**Repo:** misty-step/bitterblossom
**Outcome:** Partial — local implementation shipped; live sprite run exposed blocking UX/reliability friction.
**Time:** ~45 minutes
**Routing decision:** Bramble chosen as default systems sprite. Reasonable choice.
**Quality notes:**
- Dry-run plan quality was good and explicit.
- New `--skill` feature implemented locally with tests and docs.

**Friction observed (detailed):**
- `gh issue view <n>` failed locally due deprecated `projectCards` GraphQL field; had to use `gh api repos/.../issues/<n>` as workaround.
- Pre-dispatch issue validation hard-blocked execution because issue `#252` lacked `ralph-ready` label.
  - Command: `bb dispatch bramble --issue 252 --repo misty-step/bitterblossom --execute --wait`
  - Required workaround: `--skip-validation`
- After bypass, dispatch transitioned to `ready` then hung in repo setup step (`sprite exec ... git fetch/pull or clone`) with no further progress output.
- `bb status bramble` and `bb watchdog --sprite bramble` also hung while waiting on `sprite exec`.
- Direct fallback `sprite exec` calls hung too, including trivial commands like `pwd`.
- `sprite --debug exec` showed: `Failed to load sprite tracking for recording: failed to parse tracking file: unexpected end of JSON input` before hanging.
- `bb fleet --format text` showed `bramble` as `orphaned` while other status surfaces were blocked/hung.
- Local environment had no `GH_TOKEN`/`GITHUB_TOKEN` exported by default, so sprite GitHub operations are likely fragile unless explicitly configured.

**Action:**
- Keep improving dispatch ergonomics, but prioritize timeout/telemetry hardening around `sprite exec` and repo setup.
- Validation gating should distinguish hard safety invariants from advisory issue metadata requirements.
- Add explicit auth preflight checks (`gh`, OpenRouter, sprite token) with actionable remediation before starting remote ops.
