# Feedback Loops

The first debugging deliverable is a loop that can prove the symptom is present
and later prove it is gone. Pick the smallest loop that exercises the real
failure, then improve the loop before changing production code.

## Loop Menu

Prefer loops in this order when they fit the symptom:

1. **Failing test** at the highest seam that reaches the bug.
2. **HTTP/API script** such as `curl` or a checked-in request fixture.
3. **Local third-party API emulator** such as
   `npx emulate --service github,stripe` plus request replay when `emulate.dev`
   supports the provider; it exercises SDK/protocol seams without network.
4. **CLI fixture** that runs one command and diffs stdout, stderr, exit code,
   or a generated artifact.
5. **Browser script** for UI behavior, asserting DOM, console, network, or
   visible output.
6. **Trace replay** from a captured request, payload, event stream, HAR file,
   log bundle, or production-safe fixture.
7. **Throwaway harness** that starts the smallest runnable subset of the
   system and calls the failing path.
8. **Property or fuzz loop** when the bug is broad-domain, intermittent, or
   input-sensitive.
9. **Bisect harness** for regressions between known-good and known-bad states.
10. **Differential loop** that compares old vs new version, config A vs config
   B, or two implementations on the same input.
11. **HITL loop** only when a person must reproduce it — full protocol below.

## Loop Quality

Before moving to root-cause hypotheses, improve the loop:

- **Faster:** remove unrelated setup, cache heavy initialization, narrow scope.
- **Sharper:** assert the specific symptom, not "did not crash."
- **More deterministic:** pin time, seed randomness, isolate filesystem and
  network, freeze external data where possible.
- **Closer to the real path:** avoid shallow seams that cannot reproduce the
  caller behavior that failed.

For nondeterministic bugs, raise the reproduction rate. A 50 percent flake is
debuggable; a 1 percent flake usually needs stress, parallel repetition,
timing injection, or better instrumentation before investigation can proceed.

Before trusting a new regression test as the loop, verify it fails for the
right reason — the bug itself, not a syntax or import error.

## Correct Seam Rule

A regression test earns trust only when it exercises the bug pattern as it
appeared. If the only available test seam is too shallow, document that as the
finding instead of writing a false-confidence test. Fix the bug when the root
cause is proven, then fix the missing seam directly or file a ticket with a
focused `/critique --lens ousterhout` follow-up.

## Instrumented Reproduction Loop

The full protocol for HITL loop 11, when you can't reproduce the bug yourself
(auth-gated, mobile, timing-dependent, hardware-specific, user-flow-dependent):

```
INSTRUMENT → USER REPRODUCES → READ LOGS → REFINE → REPEAT
```

1. **Plan loop probes** — form 2-3 candidate observations that would
   discriminate between possible causes. These are instrumentation targets,
   not root-cause conclusions.
2. **Instrument** — add targeted logging that discriminates between
   hypotheses. Write to a log file the user can share back:
   ```bash
   LOG_FILE="${HOME}/Desktop/debug-$(date +%s).log"
   ```
   Log at decision points: function entry/exit, branch taken, values at
   boundaries. Tag each line with the hypothesis it tests:
   `[H1] auth token expired: ${token.exp}`.
3. **Hand off** — tell the user: "Reproduce the bug, then say done." Give
   exact steps if known.
4. **Read & analyze** — when the user signals done, read the log file. For
   each hypothesis: supported (design the next experiment), disproved
   (eliminate it, remove its instrumentation, add a new hypothesis), or
   insufficient data (add more targeted logging at the next layer).
5. **Iterate** — repeat until one hypothesis survives all evidence. Max 3
   rounds — if still ambiguous, escalate to Multi-Hypothesis Mode (agent
   teams).
6. **Clean up** — remove all instrumentation before fixing. Instrumentation
   is diagnostic, not the fix.

Use when: flaky tests, user-reported bugs you can't trigger, environment-
specific issues. Skip when the bug reproduces in your own environment.

## No-Loop Stop

If no believable loop can be built with current access, stop and report:

- what loops were attempted;
- the missing artifact or access;
- the smallest useful request to the user or operator;
- any temporary instrumentation needed and how it will be removed.

Do not proceed to speculative fixes without a loop.
