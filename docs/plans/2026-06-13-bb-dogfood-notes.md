# 2026-06-13 Bitterblossom Dogfood Notes

Goal: use Bitterblossom while finding and delivering one new issue.

## Notes

- `bb check` is useful and fast, but it is doing double duty as a task
  inventory view. It validates config and prints loaded tasks, but does not
  show parked state.
- `bb task --help` exposes `park` and `unpark` only. My next instinct was
  `bb task list`, which failed with `unrecognized subcommand 'list'`.
- `/api/tasks` has the right shape, but needing `bb serve` for local task
  inventory is friction when an operator or agent is already in a shell.
- The first `bb task list` implementation proved the value immediately:
  it surfaced that the `security` verdict task is parked with
  `run cost $0.2539 > max_cost_per_run_usd $0.25`.
- After pushing the branch, I tried to use `bb run verify` as a plane-native
  gate receipt. The CLI help accepts arbitrary JSON payload, but the verdict
  task failed before execution because the payload had no `submission` field.
  The run dead-lettered after three pre-execute attempts, and there is no
  obvious "acknowledge this intentional failed probe" command.
- Long model names made fixed-width table output hard to scan. The final
  implementation uses compact JSON-lines in text mode to stay inside the
  5k-LOC spine budget; a richer table belongs in a future cleanup only if
  it can delete or share code.

## Selected Issue

Backlog `045`: add `bb task list` with text and JSON output.

## Desired Future Improvements

- `bb run` could print artifact paths for failures and successes; today I
  have to know to follow with `bb runs show`.
- `bb run <verdict-task>` should either validate the required `submission`
  payload before dispatch or make the verdict-task requirement visible in
  help/errors without spending three attempts.
- Failed manual dogfood probes need a clear operator disposition path:
  replay is not the same as acknowledging a known bad invocation.
- The CLI could expose one canonical "operator snapshot" command that joins
  tasks, recent runs, DLQ, parked state, and gate status.
- Parked verdict tasks need an operational follow-up path: `bb task list`
  reveals the state, but it does not tell me whether unpark is safe.

## Update 2026-06-13: backlog dogfood goal + skill creation

Goal: use Bitterblossom to work through its own backlog and capture primary
user experience notes while doing it.

Preflight:

- `flyctl orgs list` showed the Misty Step Fly org is available.
- `sprite org list` initially reported `adminifi` as the selected Sprite org,
  even though `misty-step` is configured. This is dangerous for dogfood work.
- `sprite use -o misty-step lane-1` fixed the checkout context; subsequent
  `sprite org list` reported `misty-step`, and both bare `sprite exec -- whoami`
  and explicit `sprite -o misty-step -s lane-1 exec -- whoami` returned
  `sprite`.
- `./target/debug/bb --config plane task list --json` made the current parked
  `security` verdict task obvious:
  `run cost $0.2539 > max_cost_per_run_usd $0.25`.
- `./target/debug/bb --config plane dlq list --json` still shows the prior
  direct `verify` invocation dead-lettered because the payload lacked a
  `submission` field.

Friction:

- Sprite has an account footgun: a user can have multiple org tokens and the
  selected org may be wrong for the repo. Dogfood runs need an explicit
  `misty-step` preflight before any remote execution.
- The skill-creator init command is not executable directly on this machine;
  it had to be run through `python3`.
- Passing `$bitterblossom-dogfood` through a shell command without escaping
  `$` caused the generated OpenAI default prompt to become `Use -dogfood...`.
- `bb runs list --json` is complete but too raw for a human to triage during a
  dogfood run; this reinforces backlog 052.
- The submission gate is mechanically clear, but a parked required member
  leaves the operator needing judgment: should we unpark, run a partial storm,
  or stop? The system tells the truth but does not yet guide the safe action.

Delight:

- `bb task list --json` immediately surfaced the parked verdict task and budget
  reason; this is exactly the sort of agent-readable truth surface to lean into.
- `sprite use -o misty-step lane-1` made the local checkout context explicit
  and verifiable.
- The submission loop docs make the earlier failed direct `verify` run easy to
  understand after the fact: verdict members require a submission payload.

Backlog implications:

- 052 should include a concise dogfood snapshot view that joins task health,
  DLQ, recent failures, and safe next actions.
- 053 should keep tightening skill/test parity so generated skill metadata does
  not drift or lose `$skill` references through shell expansion.
- 054 should include Sprite account/org preflight in production operation
  runbooks.

## Update 2026-06-13: first dogfood submission for the dogfood skill

Change: `fcb490012288acad9fa8763a25b9413af0926990`
(`docs: add bitterblossom dogfood skill`).

Submission:

- `./target/debug/bb --config plane submit open --change dogfood-skill-fcb4900
  --rev fcb490012288acad9fa8763a25b9413af0926990 --context ... --json`
  created submission `df17211fcfb3`.
- `GH_TOKEN=$(gh auth token) ./target/debug/bb --config plane run verify
  --idempotency-key storm:df17211fcfb3:verify --payload
  '{"submission":"df17211fcfb3","repo":"misty-step/bitterblossom","rev":"fcb490012288acad9fa8763a25b9413af0926990","change":"dogfood-skill-fcb4900"}'
  --json` ran on the Sprites substrate and returned success.
- Verify run: `d7c33f8f86f4`, exit code 0, duration 49.802s, attempt phase
  `released`, artifact dir
  `plane/.bb/runs/d7c33f8f86f4/attempt-1`.
- `./target/debug/bb --config plane gate --submission df17211fcfb3 --json`
  returned `decision: pending`; `verify` was `verdict:pass`, while
  `correctness`, `security`, `simplification`, and `product` were
  `not_started`.
- `./target/debug/bb --config plane task list --json` still shows `security`
  parked with `run cost $0.2539 > max_cost_per_run_usd $0.25`.

Friction:

- `bb run --json` emitted no progress for about 50 seconds while the remote
  verifier was running. The final JSON was good, but the wait was opaque.
- The gate output says required members are `not_started`; it does not combine
  that with task parked state, so the operator has to run `task list` and join
  the explanation manually.
- The safe path around a parked required member is still a judgment call. The
  dogfood skill now says not to unpark just to make the gate pass, but the
  product should eventually guide that decision.

Delight:

- The submission-shaped verify run worked cleanly and quickly once invoked
  correctly. The previous direct-verdict dead letter now reads like a useful
  teaching failure rather than a mystery.
- The run receipt is precise: run id, trace id, attempt id, duration, artifact
  dir, state events, and exit code were all available from one `bb run --json`
  / `runs show --json` path.

Backlog implications:

- 052 should include a joined gate/task snapshot: pending member + parked
  reason + safe next action in one response.
- 053 should include contract tests for verdict-task invocation recipes so
  `verify` cannot be documented as a generic `bb run` target without the
  submission payload shape.

## Update 2026-06-13: backlog 052 status surface

Backlog item: `052-ledger-native-operator-truth-surface`.

Work:

- Added `bb status [--json]` and `GET /api/status`.
- Added a generic `src/health.rs` report that groups each task's recent run
  states, cost, duration, latest failure reason, parked state, queue counts,
  oldest pending age, DLQ counts, and safe next actions.
- Updated the Bitterblossom skill, dogfood skill, operator recipes, README,
  and spine command docs to point agents at `bb status --json`.
- Moved ledger and harness parser tests from in-source unit modules to
  integration tests so the new surface could land without breaking the
  5k-line spine budget.

Verification:

- Red test first: `cargo test --test status_cli` failed on
  `unrecognized subcommand 'status'`.
- Focused green: `cargo test --test status_cli --test status_view
  --test skill_artifacts`.
- Full gate: `./scripts/verify.sh` passed with `src LOC: 4992`.
- Live read: `./target/debug/bb --config plane status` reported
  `tasks=8 parked=1 open_dlq=2`; `security` now carries
  `unpark_after_reason_cleared`, and `product`/`verify` carry
  `replay_pre_execute_dlq` plus `inspect_artifact`.

Friction:

- The 5k source budget did its job, but it forced an immediate decision:
  either keep adding spine code or move non-spine test scaffolding out of
  `src`. That was useful pressure, but it made a small product surface feel
  like a refactor until the right move was obvious.
- `git diff --stat` hid untracked new files, so it understated the change
  until `git status --untracked-files=all` was read. This is generic Git
  friction, but it matters in dogfood closeout because untracked tests are
  easy to miss.

Delight:

- The first text `bb status` output immediately replaced three manual joins:
  task list, run list, and DLQ list. It named `security` as parked and pointed
  at replay/inspect actions for open DLQs.
- The LOC gate pushed the implementation toward a deeper module with one
  reusable report consumed by CLI and API, rather than parallel ad hoc
  surfaces.

Backlog implications:

- 050 still needs the remaining hardening children: bearer-only read auth,
  panic-safe in-flight cleanup, bounded notifications, and live loopback
  API/HTML QA.
- 033 can now compare Raindrop against a local baseline instead of raw ledger
  rows.

## Update 2026-06-13: 052 submission storm

Change: `48ed241c015bdab2e9f23539ae530c90d625ab18`
(`feat: add ledger-native status surface`).

First submission:

- `./target/debug/bb --config plane submit open --change
  status-surface-48ed241 --rev 48ed241c015bdab2e9f23539ae530c90d625ab18
  --context ... --json` created submission `6b49226eca48`.
- `verify` passed as run `0039140763d2`, duration 48.480s.
- I incorrectly ran `correctness` and `simplification` without
  `GH_TOKEN=$(gh auth token)`. Both failed pre-execute and dead-lettered:
  `correctness` run `52d9928eada2`, DLQ `5`; `simplification` run
  `a1ef389c96e1`, DLQ `4`.
- `bb gate --submission 6b49226eca48 --json` returned
  `decision: escalated` because the canonical required members were
  terminal failures.

Rerun submission:

- `./target/debug/bb --config plane submit open --change
  status-surface-48ed241-rerun --rev
  48ed241c015bdab2e9f23539ae530c90d625ab18 --context ... --json`
  created submission `52a45f27efb6`.
- With `GH_TOKEN=$(gh auth token)`, available members passed:
  `verify` run `72db597e1b2b`, duration 41.333s;
  `correctness` run `db20c05119c2`, cost $0.1506, duration 259.554s;
  `simplification` run `197e1586e24a`, cost $0.0198, duration 129.804s;
  `product` run `61de1f78d539`, cost $0.0737, duration 36.939s.
- `./target/debug/bb --config plane gate --submission 52a45f27efb6 --json`
  returned `decision: pending`: all unparked members were `verdict:pass`,
  while `security` remained `not_started` because the task is parked.
- `./target/debug/bb --config plane status` reported
  `tasks=8 parked=1 open_dlq=4 cost_today=$0.2441` and surfaced replay
  actions for the new `GH_TOKEN` DLQs.

Friction:

- Forgetting `GH_TOKEN=$(gh auth token)` is still too easy. The task specs
  know the required secret, but `bb run` only discovers the missing env after
  creating and retrying the canonical storm member.
- Once a canonical storm member dead-letters from pre-execute operator error,
  `bb gate` escalates and there is no obvious same-submission recovery path
  that keeps the canonical `storm:<submission>:<kind>` key.
- Long remote verdict runs are still silent until completion; `correctness`
  ran for more than four minutes with no heartbeat.

Delight:

- `bb status` made the failed first storm legible immediately: open DLQ count
  moved from 2 to 4, and the affected tasks named `replay_pre_execute_dlq`
  plus `inspect_artifact`.
- The rerun storm showed the submission loop can run the available member set
  cleanly on Misty Step Sprites and preserve exact costs, durations, run ids,
  and artifacts.

Backlog implications:

- 050/053 should add a pre-dispatch secret availability check or a clearer
  operator preflight for verdict tasks so missing env is caught before a
  canonical storm key is consumed.
- 052 follow-up: status could join open submissions/gate state too. Today it
  explains task/DLQ health, but a pending gate still requires a separate
  `bb gate` read.
