# powder-chew local drill (dev plane)

The local-drill twin of the live `powder-chew` workload
(`plane/agents/powder-chewer.toml`, `plane/tasks/powder-chew/`, gitignored
instance config -- see `docs/operations/README.md` for how it reaches the
live plane). This example is real, tracked, and `bb check`-covered so the
pull-model shape (poll Powder yourself, filter to an allowlist, never claim
during a drill) has a repeatable proof, matching the pattern
`examples/demo-plane` and `tests/e2e_local.rs` already use for stub-harness
local execution.

Validate from a clean checkout:

```bash
cargo build
./target/debug/bb --config examples/powder-chew-dev-plane check --json
```

To actually run the drill against the REAL Powder board (read-only):

```bash
export POWDER_API_BASE_URL=... POWDER_API_KEY=...   # a read-capable key; never printed
export PATH="$PWD/examples/powder-chew-dev-plane:$PATH"
cargo run -- --config examples/powder-chew-dev-plane run powder-chew --json
```

Inspect the result:

```bash
./target/debug/bb --config examples/powder-chew-dev-plane runs show <run-id> --json
./target/debug/bb --config examples/powder-chew-dev-plane artifacts read <run-id> REPORT.json --json
```

## Why this shape

- `substrate = "local"` requires `dev = true` in `plane.toml`
  (docs/spine.md) -- this plane declares it explicitly so it can never be
  mistaken for a production config.
- The bound agent's harness is `command`
  (`agents/powder-chewer-stub.toml`), a deterministic shell script
  (`powder-chew-dev-stub.sh`), not a model -- "real Powder, fake execution":
  the script calls the real, live `powder list-ready` (declared `secrets`
  resolve `POWDER_API_BASE_URL`/`POWDER_API_KEY` from the operator's own
  environment at dispatch time, the same mechanism every other agent uses),
  applies the production card's own repo-allowlist prefix filter, and writes
  `REPORT.json` -- but it never calls `claim`, never checks out a repo, and
  never opens a PR. This proves the selection logic against live data
  without any risk of claiming real work during a drill.
- `bin` is a bare filename (`powder-chew-dev-stub.sh`), resolved on `PATH` --
  the local substrate's exec working directory is a temporary attempt
  workspace, not the repo root, so a relative path would not resolve. Add
  this directory to `PATH` before dispatching (see the run recipe above).
- The stub uses `harness = "command"`, whose output contract
  (`src/harness.rs::parse_command`) accepts any exit-0 stdout -- no
  claude/pi/opencode-shaped JSON is required, matching
  `plane/agents/self-drill-runner.toml`'s existing precedent for a
  deterministic, model-free stub.
