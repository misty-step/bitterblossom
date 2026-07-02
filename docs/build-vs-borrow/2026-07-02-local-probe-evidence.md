# Local Substrate Probe Evidence

Date: 2026-07-02
Backlog: 058 child 4

## Purpose

This records the smallest adapter probe now available for 058 candidate
scoring. The probe exercises the current Bitterblossom local substrate against
`bb-coding-harness-probe.v1` without dispatching to Sprites or any external
provider.

## Probe Tool

Script:

```sh
scripts/build-vs-borrow-local-probe.sh [--include-timeout] [--report PATH]
```

Behavior:

- creates a temporary dev plane;
- creates a command-harness agent with a declared `BB_PROBE_SECRET`;
- runs `happy`, `missing_artifact`, and optionally `timeout` modes through
  `bb run --json`;
- queries the temp ledger for run, attempt, event, cost, and artifact evidence;
- fails if the declared secret appears in captured stdout, stderr, or artifacts;
- emits a `bb.build_vs_borrow.local_probe_report.v1` JSON packet.

The timeout probe is opt-in because the current task budget is minute-granular
and the kill-path proof intentionally takes about 60 seconds.

## Live Run

Command:

```sh
scripts/build-vs-borrow-local-probe.sh --include-timeout \
  --report /tmp/bb-build-vs-borrow-local-probe-report.json
```

Result: `status=pass`

| Check | Evidence |
|---|---|
| Happy run succeeds | run `b2b23e573d9b`, state `success`, attempt phase `released`, exit code `0`, cost `$0.00`, duration `107ms`. |
| Declared secret is not captured | `secret_leaked=false` for `happy`, `missing_artifact`, and `timeout`. |
| Missing `REPORT.json` fails the run | run `907c1b84faad`, state `failure`, error `missing required artifact: REPORT.json`, exit code `0`. |
| Timeout kills executing harness | run `686fd812a5a6`, state `failure`, state reason `timeout after 60s`, attempt error `wall-clock timeout (killed)`, exit code `-1`. |

Observed progress events included `phase:prepared`, `phase:executing`,
`phase:collecting`, and terminal state events. The happy run also recorded
structured command usage (`tokens_in=0`, `tokens_out=0`, `turns=1`,
`cost_usd=0.0`).

## Limits

- This is the local substrate baseline only; no Sprite or third-party candidate
  was dispatched overnight.
- The raw attempt directory contains the workspace files used for leak scans;
  public artifact reads still use the `bb artifacts` boundary.
- The script proves candidate-packet mechanics and local failure behavior. It
  does not replace the later vendor/substrate comparison.

## Next Slice Input

External candidates should be probed only after adapting this evidence packet
shape to their launch mechanism. The decision memo should cite candidate packet
results rather than prose claims.
