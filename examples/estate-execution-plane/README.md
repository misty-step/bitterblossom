# Estate execution contract proof

This development plane proves the Bitterblossom/Estate seam without provider
mutation. From the repository root:

```sh
cargo build --locked
target/debug/bb --config examples/estate-execution-plane check --json
GH_TOKEN="$(gh auth token)" target/debug/bb \
  --config examples/estate-execution-plane \
  run prove-estate-execution --json
```

The run gets an isolated attempt workspace, checks out the exact Estate commit,
verifies the exact `Cargo.lock` Git blob before execution, records the
plane-bound `hephaestus` identity, runs Estate's real disposable signed-data
proof, and retains `ESTATE_EXECUTION_EVIDENCE.json`.

This example intentionally contains no provider credential, provider mutation,
or product-specific Rust branch. `GH_TOKEN` is read-only clone transport for
the private Estate repository; it is not authorization to mutate
infrastructure. For production, replace the command binding
with Estate's registered provider/host adapter, use a remote substrate, and
declare the adapter's real Estate receipt path in `required_artifacts`. The
authority and recovery boundary is detailed in
[`docs/estate-execution-contract.md`](../../docs/estate-execution-contract.md).
