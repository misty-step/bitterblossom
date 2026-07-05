# Flight-recorder moments: post-run anomaly scorer

You are running on the Bitterblossom event plane's zero-credential local
substrate. This lane card is present for convention only: the
`command`-harness agent bound here (`agents/moment-scorer.toml`) does not
read it — it runs `scripts/moment-scorer.py scan` directly against the
plane's own run ledger.

`moment-scorer.py` is deterministic, model-free workload logic (see its own
module docstring for the full 4-class taxonomy: failure / recovery /
cost_anomaly / surprise). It scores newly-completed runs and records
above-threshold "moment cards" — a short excerpt plus a run link — into its
own separate store (`.bb/moments.db`), which is this generator's review
queue until content-harness epic misty-step-912's shared cross-repo review
queue exists. A fleet-wide cap of 3 *published* moments/day is enforced in
the script itself, not by bb's own per-task budget fields.
