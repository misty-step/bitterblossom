# Bitterblossom Operator Recipes

Use these recipes when the top-level skill points here. Replace `<plane>` with a
directory containing `plane.toml`.

## Resolve `bb`

```bash
bb --version
```

If `bb` is not installed and you are in the Bitterblossom source checkout:

```bash
cargo run --quiet -- --config <plane> check
```

After building:

```bash
cargo build
./target/debug/bb --config <plane> check
```

## Inventory

```bash
bb --config <plane> check
bb --config <plane> check --json
bb --config <plane> status --json
bb --config <plane> task list --json
bb --config <plane> runs list --json
bb --config <plane> dlq list --json
```

Read `status --json` first when triaging. It clusters tasks, recent run states,
cost, queue age, parked reasons, DLQ rows, and safe next actions. Use raw
`task list`, `runs list`, and `dlq list` when you need the underlying rows.

## Manual Dispatch

```bash
bb --config <plane> run <task> \
  --idempotency-key "<stable-key>" \
  --payload '{"key":"value"}' \
  --json
```

Use an idempotency key for replayable operator actions. Treat a returned run id
as the receipt, then inspect it:

```bash
bb --config <plane> runs show <run-id> --json
```

## Review Workload

For a real review comment:

```bash
GH_TOKEN=$(gh auth token) bb --config <plane> run review \
  --payload '{"repo":"owner/repo","pr":123}' \
  --json
```

For measurement without posting:

```bash
GH_TOKEN=$(gh auth token) bb --config <plane> run review \
  --payload '{"repo":"owner/repo","pr":123,"measurement":true}' \
  --json
```

Evidence is both the ledger row and the external effect: PR comment for normal
mode, artifact/result output for measurement mode.

## Submission Gate

Open a submission:

```bash
bb --config <plane> submit open \
  --change "<change-id>" \
  --rev "<git-rev>" \
  --json
```

Run verdict members with the returned submission id:

```bash
bb --config <plane> run correctness \
  --idempotency-key "storm:<submission>:correctness" \
  --payload '{"submission":"<submission>"}' \
  --json
```

Evaluate:

```bash
bb --config <plane> gate --submission <submission> --json
```

If the gate blocks, fix the underlying issue and open the next round. Do not
delete or rewrite prior verdict rows.

## Dead Letters and Recovery

List pre-execute failures:

```bash
bb --config <plane> dlq list --json
```

Replay only when the pre-execute failure is understood:

```bash
bb --config <plane> dlq replay <run-id>
```

After a host restart:

```bash
bb --config <plane> recover
bb --config <plane> runs list --json
```

Resolve `awaiting_recovery` only after inspecting side effects:

```bash
bb --config <plane> runs resolve <run-id> success --reason "<why>"
```

## Serve and Read APIs

Loopback development:

```bash
bb --config <plane> serve
curl http://127.0.0.1:7077/health
curl http://127.0.0.1:7077/api/status
curl http://127.0.0.1:7077/api/tasks
```

Non-loopback serving needs `BB_API_TOKEN`. Query with:

```bash
curl -H "Authorization: Bearer $BB_API_TOKEN" "$BB_URL/api/runs"
```

Do not put `BB_API_TOKEN` in a query string. Read APIs and the HTML view accept
only the bearer header when a token is configured.

## Parked Tasks

```bash
bb --config <plane> task list --json
bb --config <plane> task park <task> --reason "<operator reason>"
bb --config <plane> task unpark <task>
```

Never unpark just to make a command succeed. Read the parked reason, inspect
recent runs, and name why the underlying condition is gone.
