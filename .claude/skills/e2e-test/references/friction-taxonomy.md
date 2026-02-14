# Friction Taxonomy

Categories for classifying findings during e2e shakedown. Each category has a severity floor — the minimum severity regardless of how minor the specific instance seems. This prevents systemic issues from being downgraded.

## Categories

### F1: Silent Failure
**Severity floor: P1**

Something failed but the user wasn't told. The operation appeared to succeed or produced no output at all.

Examples:
- Dispatch exits 0 but agent never started
- Credential validation passes but token is expired
- Signal file written but content empty

### F2: Confusing Output
**Severity floor: P2**

Output exists but doesn't help the user understand what happened or what to do next.

Examples:
- Error message references internal state the user can't inspect
- Success message when task partially completed
- Progress text interleaved with JSON output

### F3: Missing Feedback
**Severity floor: P2**

Expected information is absent. The system is working but the user can't tell.

Examples:
- No progress during long operations
- No confirmation after dispatch
- No summary at completion

### F4: Stale State
**Severity floor: P0**

Old state from a previous operation interferes with the current one. The most dangerous category — causes false positives and masks real failures.

Examples:
- TASK_COMPLETE from previous run triggers premature success
- Cached sprite status doesn't reflect current state
- Old branch on sprite conflicts with new dispatch

### F5: Credential Pain
**Severity floor: P1**

Authentication or authorization blocks the user's work. Includes unclear requirements, silent failures, and unnecessary manual steps.

Examples:
- Token required but error doesn't say which
- `GITHUB_TOKEN` must be exported manually before every dispatch
- Credential check passes locally but fails on sprite

### F6: Timing & Performance
**Severity floor: P2**

Unexpectedly slow, wrong timeouts, or timing-related confusion.

Examples:
- Build takes >2 minutes without explanation
- Timeout value doesn't match actual wait duration
- Progress messages stop for >90s without explanation

### F7: Flag & CLI Ergonomics
**Severity floor: P3**

The CLI fights the user. Flags are confusing, inconsistent, or require unnecessary ceremony.

Examples:
- `--json` on some commands, `--format json` on others
- Required flag order not documented
- Flag name doesn't match its behavior

### F8: Documentation Gap
**Severity floor: P3**

Behavior is correct but undocumented. User must read source code or experiment to understand it.

Examples:
- Skill mount path resolution undocumented
- Signal file protocol not in user-facing docs
- Error code meanings unexplained

### F9: Infrastructure Fragility
**Severity floor: P1**

Transport, network, or platform issues that break the workflow. Not application bugs — environment-level problems.

Examples:
- SSH/exec transport times out intermittently
- Fly.io API rate limiting during fleet status
- Sprite VM not responding after provision

## Using This Taxonomy

When documenting a finding:
1. Assign the category (F1-F9)
2. Check the severity floor — your assigned severity cannot be lower
3. Describe observed behavior, expected behavior, and user impact
4. Note whether this is a known issue (check MEMORY.md filed issues)
