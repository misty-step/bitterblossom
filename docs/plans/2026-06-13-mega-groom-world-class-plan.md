# Bitterblossom Mega Groom: World-Class Plan

Date: 2026-06-13

## Goal

Run a full strategic groom under the corrected Harness Kit `/groom` contract:
swarm investigation, local evidence, external exemplars, premise challenge,
and backlog shaping for making Bitterblossom an excellent event plane for
recurring agent workloads.

## Source Matrix

| Surface | Status | Evidence | Contribution |
|---|---|---|---|
| Product and value prop | complete | Product lane; `project.md:28-42`, `README.md:3-6` | The three open tickets are one control-plane trust arc, not enough product strategy. |
| Operator/user experience | complete | UX lane; `docs/spine.md:245-247`, `docs/spine.md:355-360`, live `bb run --help` | Operator trust is broken by query-token auth and CLI/doc drift. |
| Runtime reliability | complete | Runtime lane; `src/serve.rs:102-140`, `src/notify.rs:16-48`, `src/recovery.rs` | 047 needs to expand into resilience: in-flight safety, bounded notify, durable admission, deterministic recovery. |
| Security and privacy | partial | Security lane partial; local read of `src/ingress.rs:48-61`, `src/serve.rs:209-220` | Header-only API auth and secret/log review belong in the hardening arc; webhook secret env is fail-closed today. |
| Architecture and system design | complete | Architecture lane; `docs/adr/005-rust-event-plane.md`, `src/spec.rs`, `src/ledger.rs`, `src/serve.rs` | The Rust spine shape is still coherent, but scheduling primitives need proof under stress. |
| Code quality and simplicity | complete | Simplification lane; `docs/walkthroughs/*terminal.txt`, old walkthrough Go references, `docs/spine.md` | Clean current-contract docs and archive old operational noise. |
| Observability and ops | complete | Ops lane; `.github/workflows/ci.yml:17-20`, `fly.toml:11-14`, `docs/spine.md:224-227` | Volume persistence exists; backup/restore, deploy smoke, and production canaries are not first-class. |
| Tests and verification | local degraded | Verification lane hit context limit; local commands: `./target/debug/bb --help`, `bb run --help`, `bb runs export --help`, `bb --config examples/demo-plane check`, `scripts/verify.sh` read | Existing gate is good but misses command-doc parity, API shape contracts, deploy smoke, and failure-storm drills. |
| Documentation and onboarding | complete | Docs lane; `README.md:79-90`, `docs/adr/001...`, `docs/spine.md`, `skills/bitterblossom` | Current docs mix Rust v3 truth with stale harness and historical artifacts. |
| Agent readiness | complete | Agent lane; `skills/bitterblossom/SKILL.md`, `tests/skill_artifacts.rs:11-29`, `tests/task_cli.rs:60-67` | Portable skill exists; schema/version guarantees for consuming agents do not. |
| Infrastructure and delivery | complete | Ops lane; `.github/workflows/ci.yml`, `fly.toml`, `docs/spine.md:199-228` | Running on Fly is documented, but rollback/restore/canary are manual knowledge. |
| External exemplars | complete | Firecrawl search over Trigger.dev, Inngest, Temporal, OpenTelemetry | Best-in-class adjacent systems make retries, queues, state resume, observability, and traces first-class. |
| Premise challenge | complete | Premise lane; `backlog.d/047...`, `backlog.d/049...`, `docs/plans/2026-06-13-bb-dogfood-notes.md` | Do not grow reflex workloads until the plane proves its hardening and contracts. |

## World-Class Target

Bitterblossom should be the smallest trustworthy runtime for recurring agent
work:

- A workload is files, not runtime code.
- Every trigger produces a durable run row before side effects.
- The plane is boring under storm load: bounded admission, bounded
  notifications, deterministic recovery, and explicit operator actions.
- Every CLI/API surface used by agents has stable JSON schemas and contract
  tests.
- Every public doc and skill recipe is generated from or tested against live
  `bb` help and JSON.
- Production operation has rehearsed deploy, rollback, backup, restore,
  recovery, and canary paths.
- Telemetry exits through a stable export seam that Daedalus and standard
  GenAI observability tools can consume without adding a sidecar by default.
- New workload categories start from template planes/cards/budgets, not tribal
  memory.

## Gap Map

| Gap | Evidence | Backlog |
|---|---|---|
| Control loop is not hardening-complete | `src/serve.rs:102-140`; `src/notify.rs:16-48`; `src/serve.rs:209-220` | 050, 051 |
| Operator truth surface is too raw | `project.md:104-111`; current `bb runs list --json` and `task list --json` are facts, not diagnosis | 052 |
| Agent interface is portable but not schema-backed | `skills/bitterblossom/`; `tests/skill_artifacts.rs:11-29`; `tests/task_cli.rs:60-67` | 053 |
| Production operations lack restore/canary muscle | `fly.toml:11-14`; `.github/workflows/ci.yml:17-20`; `docs/spine.md:224-227` | 054 |
| Only demo-plane is cloneable as a workload starter | `find examples -maxdepth 3 -type f` shows only `examples/demo-plane` | 055 |
| Telemetry vision is named but not exported as a contract | `project.md:72-74`, `project.md:111`, `docs/spine.md:249-255` | 056 |
| Docs include stale or historical contracts at high visibility | `docs/spine.md:355-360`; `docs/adr/001...`; `docs/walkthroughs/*terminal.txt` | 057 |
| Durable workflow competitors force a proof obligation | `project.md:78-82`; external Trigger.dev/Inngest/Temporal docs | 058 |

## Strategy Themes

### 1. Hardening Before Growth

Recommendation: deliver 050 before adding more reflex workload volume.

The live plane has the right shape, but `bb serve` still has three production
footguns: query-token read auth, panic-unsafe in-flight cleanup, and unbounded
notification fan-out. Multiple lanes independently found these as the highest
impact risks. This is the one best next pickup.

### 2. Make Agent Contracts Mechanical

Recommendation: move from prose promises to versioned CLI/API JSON schemas and
help/doc parity tests.

The portable skill is a good start, but consuming agents need a stable contract,
not a folder whose tests only check for strings. `bb --help`, selected subcommand
help, API JSON, and skill recipes should be checked together.

### 3. Make Operations Boring

Recommendation: treat backup/restore, deploy smoke, production canary, and
recovery drills as product surface.

The Fly deployment is real enough that volume-backed SQLite is now system state.
Volume persistence without a restore drill is not an ops contract.

### 4. Productize Workload Authoring

Recommendation: add production-shaped workload templates only after 050.

The value prop says "tasks + agents + triggers as files"; today the only
copyable public example is `demo-plane`. The review factory exists in `plane/`,
but there is no small template portfolio for canary responder, docs sync, or
monitor watcher.

### 5. Telemetry as Feedback, Not Dashboard Decor

Recommendation: export run attempts in a Daedalus/OTel-shaped contract before
adding a sidecar.

The project already names Daedalus and OTel-shaped telemetry. External OTel
GenAI conventions are active enough to design the seam, while the current spine
should stay small.

### 6. Keep the Custom Spine Honest

Recommendation: after 050, run a build-vs-borrow decision against durable
workflow systems using stress evidence.

Trigger.dev, Inngest, and Temporal all sell retries, queues, observability, and
state resume as first-class primitives. Bitterblossom can still be right because
it dispatches coding harnesses onto sandboxes, but that premise should be
periodically re-proved.

## Backlog Diff

Added:

- `050-event-plane-hardening-before-growth.md`
- `051-deterministic-recovery-and-probe-contract.md`
- `052-ledger-native-operator-truth-surface.md`
- `053-versioned-agent-contract-and-skill-projection.md`
- `054-production-operability-drills.md`
- `055-workload-template-portfolio.md`
- `056-run-telemetry-feedback-loop.md`
- `057-current-contract-docs-and-noise-sweep.md`
- `058-durable-workflow-build-vs-borrow-recheck.md`

Existing item disposition:

- `033` stays blocked pending Raindrop/Workshop credentials and a trial window.
- `047` remains valuable evidence and becomes the first child of 050.
- `048` remains valuable evidence and becomes the seed of 052.
- `049` remains valuable evidence and becomes part of 050/057.

No ticket was deleted or silently merged. The consolidation proposal is now
explicit and sequenced.

## Sequence

Now:

1. 050: harden the event plane before growth.
2. 051: make recovery/probe outcomes deterministic under uncertainty.
3. 053: lock the consuming-agent contract while the surface is still small.

Next:

1. 052: build the ledger-native operator truth surface.
2. 054: add deploy, restore, and canary drills.
3. 057: sweep stale docs and historical noise once live contracts are fixed.

Later:

1. 055: expand workload templates beyond demo/review.
2. 056: wire telemetry export to Daedalus/OTel-shaped consumers.
3. 058: re-run durable workflow build-vs-borrow with stress evidence.

Blocked:

- 033 waits on external Raindrop/Workshop access and a real trial target.

## Best Next Pickup

Deliver 050.

It outranks everything else because it validates the premise that `bb serve`
can be trusted as the recurring event plane. If 050 fails, more templates,
telemetry, and docs only make an unsafe loop easier to use.

## Residual Risk

- I did not exercise the live Fly deployment or production API because this
  groom did not have deploy credentials or permission to mutate production.
- The security lane returned partial evidence; local follow-up covered the
  obvious auth/secret surfaces but not a full threat model.
- The verification subagent failed from context length; local checks covered
  help/config/gate inventory, not a full new test run before edits.
- External research used official/public search snippets, not a deep vendor
  evaluation.
- Several older ADRs and walkthroughs may be historically important; 057 should
  archive or mark them, not delete context blindly.
