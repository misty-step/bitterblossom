# Preserve incident-triage REPORT.json when the model stream is capped

Priority: P1 | Status: ready | Estimate: M

## Goal

The incident-triage wrapper should preserve the agent's real `REPORT.json` when
the model has already completed Canary writebacks, even if the model stdout
stream hits a wrapper cap or the model process exits non-zero late.

## Evidence

Live drill on 2026-07-02:

- BB run: `3f2e52af59e5`
- Canary incident: `INC-ay76lctwao3z`
- Canary claim: `CLM-n68gzl9nzmbd`, state `dismissed`
- Canary writebacks succeeded:
  - `ANN-9ti3spuxgvvq` investigation started
  - `ANN-59dgh0uk11f5` / `ANN-fbx8690uzkql` hypotheses written
  - `ANN-fxrohk6vo6oy` / `ANN-d78sih8vxvvi` local verification
  - `ANN-vv7bz6dob7r0` / `ANN-zfyynexpfiev` no defect / no fix needed
- BB run state: success
- Collected artifact: `REPORT.json` status `blocked`, reason
  `agent command exited 153 before REPORT.json`

The operational flow reached the right Canary-side state, but the durable BB
artifact did not reflect that success.

## Oracle

- [x] If the agent writes `REPORT.json` before a late model-stream failure, the
      wrapper preserves and validates that report instead of overwriting it.
- [x] If the agent reaches Canary terminal writebacks but fails before writing
      `REPORT.json`, the wrapper synthesizes a report that reflects observed
      writebacks rather than `blocked_before_agent`.
- [x] The wrapper's transcript cap is enforced by byte-counting the subprocess
      stream, not by a shell `ulimit` that varies by shell/platform.
- [x] Regression tests cover late non-zero exit with existing `REPORT.json` and
      late non-zero exit after mocked writeback receipt artifacts.
- [x] `./scripts/verify.sh` passes.

## Non-goals

Do not remove the stdout cap. The cap is needed; the missing piece is preserving
the truthful completion artifact under capped-stream conditions.

## 2026-07-04 Slice

The wrapper now enforces `INCIDENT_TRIAGE_AGENT_STDOUT_MAX_BYTES` by streaming
the agent stdout through a byte-counting FIFO writer instead of shell
`ulimit -f`. If the cap or a late non-zero exit occurs after `REPORT.json`
exists, the wrapper preserves and validates the agent's report. If no report
exists but terminal Canary writeback receipt JSON files are present under
`incident-triage/writebacks/`, the wrapper synthesizes
`canary_writebacks_preserved` with the observed writebacks, a truthful
`agent_failed_after_canary_terminal_writebacks` stop reason, and residual risk
naming the late failure. Regression tests cover both paths plus the removal of
the shell `ulimit` cap. `./scripts/verify.sh` passed with `src LOC: 11528`.
