# Estate execution through Bitterblossom

Status: execution-boundary contract for Powder
`bitterblossom-estate-execution-contract`, 2026-07-14.

Estate is the sole infrastructure mutation authority. Bitterblossom does not
classify infrastructure actions, widen capabilities, reconstruct provider
arguments, or decide whether a plan is safe. It executes a workflow whose
deterministic adapter consumes Estate's already-authorized exact payload and
returns Estate's receipt unchanged.

## Boundary

The workflow declaration owns only execution mechanics:

- an immutable Estate Git commit plus exact Git blob identities for every
  provider/tool lock file;
- a plane-owned agent binding and role, recorded in `RUN.json` and the ledger;
- an isolated substrate workspace, lease, timeout, run budget, lifecycle, and
  recovery state;
- a command adapter fixed by the task declaration, never selected by an event;
- declared receipt paths collected as opaque regular-file evidence.

Estate owns the signed capability, runtime public-key binding, runtime-signed
request, exact artifact and payload digest, authorization decision, replay
reservation, state precondition, mutation adapter, and signed action receipt.
An event may carry Estate's signed data, but it cannot choose the run's agent,
role, substrate, command, repository revision, lock identities, or receipt
contract. Estate rejects a caller-supplied role without possession of the
capability-bound runtime key.

`examples/estate-execution-plane/` is a disposable integration proof. It runs
Estate's real external-fixture CLI test from commit
`225272aa827b3616a45daa9273b8aceaa7a13b96` under `cargo --locked`, proving the
capability/runtime signature, exact-plan, disposable apply, and signed receipt
laws without contacting a provider. The example's evidence packet is a
Bitterblossom proof receipt, not a provider action receipt. A production task
must declare the real Estate adapter's receipt path (for example
`receipts/estate.receipt.jsonl`) in `required_artifacts`; Bitterblossom then
retains those exact bytes without parsing them. Collection is bounded to one
MiB per declared receipt, walks every path component without following
symlinks, preserves binary bytes across remote transports, and snapshots
nested receipts into the durable evidence ledger. Bitterblossom-owned evidence
names such as `RUN.json`, `stdout.txt`, and `result.md` cannot be redeclared.

`GH_TOKEN`, when declared, exists only during immutable repository checkout.
It is removed before pre-commands and harness execution on local, Sprites, and
tailnet substrates. It authenticates clone transport; it is never an Estate
capability or workload credential. Remote host adapters require `python3` for
the bounded no-follow receipt collector.

## Recovery law

Failures before harness execution may use Bitterblossom's bounded mechanical
retry. At or after execution, Bitterblossom does not replay automatically.
Estate's transaction/replay journal and observed target state decide whether
execution completed, must recover, or conflicts. Bitterblossom records that
result and exposes the retained Estate receipt; it does not invent an apply
outcome from process exit alone.

## Local break-glass

Break-glass is a pre-provisioned local Estate path, not a fallback workflow.
It must remain usable when Bitterblossom, Mint, Sprites, GitHub, and the network
are unavailable. Keep the pinned Estate Git object, Rust toolchain/cache,
signed input bundle, configured trust roots, execution data directory, and
receipt-head store on the authorized operator host before an incident.

Verify and execute directly:

```sh
test "$(git -C /opt/estate rev-parse HEAD)" = 225272aa827b3616a45daa9273b8aceaa7a13b96
test "$(git -C /opt/estate rev-parse HEAD:Cargo.lock)" = 57198968cb956be86a9aa91eadc54a9df5264574
env CARGO_NET_OFFLINE=true \
  http_proxy=http://127.0.0.1:9 https_proxy=http://127.0.0.1:9 \
  GIT_TERMINAL_PROMPT=0 \
  cargo build --manifest-path /opt/estate/Cargo.toml --release --locked --offline
/opt/estate/target/release/estate prove-authorization \
  --fixture-dir /secure/estate/authorized-input \
  --data-dir /var/lib/estate/execution \
  --receipt-head /var/lib/estate-head/estate-head.json
```

The fixture paths above are operator-managed examples, not Git content. The
command neither requests credentials from Mint nor contacts GitHub or a remote
substrate. Normal exact-plan authorization, receipt anchoring, and recovery
rules still apply; break-glass bypasses orchestration availability, never
Estate authority.
