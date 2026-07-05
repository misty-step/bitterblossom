# Flight-recorder moments plane template

This is a production-shaped starter plane for the flight-recorder anomaly
scorer (bitterblossom-914, content-harness epic misty-step-912, generator
#4). It defines one report-first, zero-credential, model-free workload:

- `moment-scorer`: scores newly-completed runs in the plane's own ledger
  against a small fixed 4-class taxonomy (`failure`, `recovery`,
  `cost_anomaly`, `surprise` — see `scripts/moment-scorer.py`'s module
  docstring for the exact deterministic signal set) and records
  above-threshold "moment cards" — an excerpt plus a run link — into its own
  separate store, `.bb/moments.db`. A fleet-wide cap of 3 *published*
  moments/day is enforced; capped-out moments are still recorded, just
  marked unpublished, so a real signal is never silently dropped.

Validate from a clean checkout:

```bash
cargo build
./target/debug/bb --config examples/moments-plane check
./target/debug/bb --config examples/moments-plane task list --json
```

`bb check` does not require live credentials or a real ledger — the scorer
itself needs no external secrets, no network, and no model. Run it directly
(not through the plane's own dispatch, since the command-harness attempt
workspace is isolated from the plane root — see the absolute-path note
below) against a real ledger:

```bash
python3 scripts/moment-scorer.py scan --db /path/to/your/plane/.bb/plane.db --moments-db /path/to/your/plane/.bb/moments.db
python3 scripts/moment-scorer.py list --moments-db /path/to/your/plane/.bb/moments.db --json
```

## Graduating to a real deployment

`agents/moment-scorer.toml`'s `args` carries a placeholder
`/absolute/path/to/your/plane/.bb/plane.db` — replace both `--db` and
`--moments-db` with your live plane's absolute paths before wiring the cron
trigger into your real plane. This is the one operational step every
command-harness workload in this repo needs when it reads plane-local state
rather than an inbound webhook payload: the substrate runs each attempt in
its own isolated workspace (same as `incident-triage-wrapper.sh`), so a
workspace-relative default would silently miss the real ledger.
