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

## Update 2026-06-13: 050 bearer-only read auth

Backlog item: `050-event-plane-hardening-before-growth`, child 1.

Work:

- Removed query-string read auth from `bb serve`; read APIs and the HTML
  operator view now accept only `Authorization: Bearer <BB_API_TOKEN>` when a
  token is configured.
- Added a live-server regression test proving missing token, bad bearer, and
  `?token=` are rejected while bearer auth succeeds for `/api/runs` and `/`.
- Updated `docs/spine.md` and the Bitterblossom operator recipe so agents and
  humans no longer learn the unsafe URL-token path.
- Updated backlog 050 to mark the bearer-auth child done while leaving the
  larger hardening epic open.

Verification:

- Red test first: `cargo test --test serve
  read_api_requires_bearer_and_rejects_query_token -- --nocapture` failed
  because `/api/runs?token=test-token` returned `200`.
- Focused green: `cargo test --test serve`.
- Full gate: `./scripts/verify.sh` passed with `src LOC: 4991`.
- Live bearer QA:
  - `GET /api/status` without header -> `401`.
  - `GET /api/status?token=test-token` -> `401`.
  - bad bearer -> `401`.
  - bearer `/api/status` -> status JSON with `8 tasks, parked=1, dlq=4`.
  - bearer `/` -> `200`.
  - no-token loopback `/api/status` and `/` remain open for local dev.

Friction:

- A failing live-server test can leave a spawned `bb serve` behind unless the
  test owns child cleanup explicitly. The new test now uses a drop guard, but
  this is easy to miss when writing process-level tests.
- The existing query-param helper is still needed for safe non-secret filters
  like `task`, `state`, `submission`, and `change`; the code needed careful
  wording so removing query-token auth did not look like deleting query
  parsing wholesale.

Delight:

- The red test was exact: it failed only on the unsafe query-token path, then
  passed after a one-line auth change.
- The live QA path was straightforward because `/api/status` now provides a
  compact proof response; no manual task/run/DLQ synthesis was needed.

Backlog implications:

- 050 still needs panic-safe in-flight cleanup, bounded notification dispatch,
  command/docs parity checks, and containment/storm drills.
- The process-test cleanup guard pattern should be reused for future `bb serve`
  live QA tests.

## Update 2026-06-13: bearer-auth submission storm

Change: `8c1be3a1747a34a5d32864c152b101e750ec0ba5`
(`fix: reject query-token read auth`).

Submission attempts:

- First submission `4f6a9da5b948` exposed an operator error: I ran canonical
  `verify` without `GH_TOKEN`. Run `0d5c50785324` failed before execution and
  dead-lettered as `6`.
- `GH_TOKEN=$(gh auth token) ./target/debug/bb --config plane dlq replay 6`
  created replay run `9b7982da52fa`, which succeeded in 45.394s.
- `bb gate --submission 4f6a9da5b948 --json` still returned
  `decision: escalated` because the gate honors only the canonical
  `storm:<submission>:verify` run; the successful replay did not repair that
  member.
- Clean submission `955383bfcda1` then ran the available member set with
  `GH_TOKEN` from the start.

Clean submission results:

- `verify`: `verdict:pass`, run `101d51ef08a1`, duration 40.628s.
- `correctness`: `verdict:pass`, run `2963e813a6c9`, cost `$0.0537442645`,
  duration 157.123s, 110750 input tokens, 6006 output tokens.
- `simplification`: `verdict:pass`, run `8715c27a366b`, cost
  `$0.0171592283`, duration 150.413s, 87171 input tokens, 8333 output tokens.
- `product`: `verdict:pass`, run `b50adf735ba1`, cost `$0.0342742`,
  duration 32.095s, 10940 input tokens, 2024 output tokens.
- `security`: `not_started`; task stayed parked because the prior run cost
  `$0.2539` exceeded `max_cost_per_run_usd $0.25`.
- `bb gate --submission 955383bfcda1 --json` returned `decision: pending`
  with no blocking/advisory findings.

Friction:

- The dogfood skill incorrectly showed `verify` without `GH_TOKEN`, even
  though the verifier task needs GitHub access. Fixed in the skill after this
  run.
- Canonical storm member failure is intentionally strict, but the recovery UX
  is not obvious: replay proves the command can pass yet cannot make the gate
  count that member.
- `bb dlq replay 6 --json` failed with `unexpected argument '--json'`, unlike
  the read-heavy operator commands.
- Long verdict runs still have no heartbeat in the foreground command; the
  operator waits silently for minutes before seeing the final JSON.

Delight:

- The canonical-key behavior is rigorous once understood: the gate did not
  silently forgive a failed required member just because a non-canonical replay
  passed.
- The clean submission gave a compact, auditable packet: exact run ids, costs,
  durations, token counts, and a pending gate explained solely by the parked
  security member.

Backlog implications:

- Added `backlog.d/059-submission-retry-and-operator-heartbeats.md` for
  canonical retry guidance, `dlq replay --json` parity, and long-run heartbeat
  feedback.

## Update 2026-06-13: 059 dlq replay JSON

Backlog item: `059-submission-retry-and-operator-heartbeats`, child 3.

Work:

- Added `bb dlq replay <id> --json`.
- JSON replay output now matches the existing run bundle shape:
  `run`, `attempts`, and `events`.
- Updated `docs/spine.md`, `skills/bitterblossom/SKILL.md`, and the operator
  recipes so agents can rely on the JSON replay surface.

Verification:

- Red test first: `cargo test --test dlq_cli
  dlq_replay_json_returns_replayed_run_bundle -- --nocapture` failed with
  `unexpected argument '--json'`.
- Focused green: `cargo test --test dlq_cli -- --nocapture`.
- Help check: `bb --config plane dlq replay --help` now shows
  `Usage: bb dlq replay [OPTIONS] <ID>` and `--json`.

Friction:

- Before this change, replay was the one recovery action in the dogfood path
  that forced text parsing or a follow-up `runs show` command.

Delight:

- The implementation could reuse `print_run`, so the replay surface now shares
  the same schema as the other run receipts instead of introducing another
  shape.

Backlog implications:

- 059 still needs canonical storm failure guidance, gate/status safe actions,
  long-run human heartbeat output, and the final retry-path documentation.

Submission storm:

- Change: `3d9ef4939718de8f10c5298471d339653848fec2`
  (`feat: add json dlq replay output`).
- Submission: `./target/debug/bb --config plane submit open --change
  dlq-replay-json-3d9ef49 --rev
  3d9ef4939718de8f10c5298471d339653848fec2 --context ... --json`
  created `20a3b600d4d3`.
- `verify`: `verdict:pass`, run `26c1b7ab0dc6`, duration 47.648s,
  attempt `99`, artifact dir `plane/.bb/runs/26c1b7ab0dc6/attempt-1`.
- `correctness`: `verdict:pass`, run `92cd4e5ab313`, cost
  `$0.031799428`, duration 312.092s, 52782 input tokens, 8551 output
  tokens.
- `simplification`: `verdict:pass`, run `932fa9b230ab`, cost
  `$0.0160832835`, duration 172.094s, 89341 input tokens, 8060 output
  tokens.
- `product`: `verdict:pass`, run `34586597ae2b`, cost `$0.04957295`,
  duration 34.039s, 15085 input tokens, 2231 output tokens.
- `security`: `not_started`; task stayed parked because the prior run cost
  `$0.2539` exceeded `max_cost_per_run_usd $0.25`.
- `bb gate --submission 20a3b600d4d3 --json` returned
  `decision: pending` with no blocking, advisory, or rejected findings.

Additional friction:

- A broad `pi` fresh critic returned a useful compact `pass`, but the
  narrower follow-up critic ignored the requested output shape, wandered into
  local file exploration, and had to be stopped with Ctrl-C. Cross-model
  critique is valuable, but the local harness needs stronger output bounding
  or timeout/default receipt behavior for critic lanes.
- Foreground `bb run --json` remained silent during long remote lanes:
  `correctness` waited more than five minutes and `simplification` nearly
  three minutes before returning the final bundle. The ledger proved progress,
  but the operator had to run a separate `runs list` read.
- A quick `task list --json` `jq` probe used the wrong field names and returned
  nulls for task name and parked reason. The data is present, but the schema is
  not self-describing enough for ad hoc shell consumers.

Additional delight:

- The new `dlq replay --json` shape fits naturally into the existing
  `run`/`attempts`/`events` receipt model; no new parser path is needed.
- Sequentially running `simplification` and `product` avoided the host lease
  contention seen in earlier dogfood storms while preserving exact costs and
  artifacts.

## Update 2026-06-13: 059 human-mode run heartbeat

Backlog item: `059-submission-retry-and-operator-heartbeats`, child 4.

Work:

- Added human-mode progress output for `bb run`: an immediate run receipt on
  stderr plus periodic heartbeat lines with current run state and latest
  attempt phase while the run is pending/running.
- Kept `bb run --json` quiet until the final `run`/`attempts`/`events` bundle
  so agent consumers do not need a streaming JSON parser.
- Moved source-counted helper tests out of `src` pressure where integration
  coverage already exercises the public surface, keeping the spine at the
  5k-line budget.
- Updated `docs/spine.md`, the Bitterblossom skill, operator recipes, and this
  backlog item.

Verification:

- Red test first: `cargo test --test run_cli
  run_human_mode_prints_early_receipt_and_heartbeat_without_json_noise
  -- --nocapture` failed because stderr did not contain `accepted`.
- Focused green: `cargo test --test run_cli -- --nocapture`.
- Regression check: `cargo test --test serve
  read_api_requires_bearer_and_rejects_query_token -- --nocapture` passed with
  the query-substring check moved to the live HTTP route.
- Source budget check: repo gate reported `src LOC: 4999`.

Friction:

- The 5k Rust spine cap forced a design choice. The first joined heartbeat
  helper was more polished internally, but it added a small lifecycle
  abstraction the operator did not need. The final version is deliberately
  thinner: human-only stderr polling around the blocking dispatch.
- A hidden short heartbeat interval was needed for the CLI integration test;
  the production default remains 30 seconds.
- Fresh critic passed the diff, but first used raw `wc -l` instead of the
  repo's nonblank LOC oracle; the repo gate remains the source of truth.

Delight:

- The final behavior matches the dogfood pain precisely: human runs stop
  feeling opaque, while agent-readable `--json` remains stable and clean.
- The LOC cap again pushed the code toward a narrower interface instead of a
  general progress framework.

Backlog implications:

- 059 still needs canonical storm failure guidance, gate/status safe actions,
  and the final retry-path documentation.

Submission storm:

- Change: `18c7697ccadaa2ae493d0d9b6e3d5ddc61ef5a00`
  (`feat: add human run heartbeat`).
- Submission: `./target/debug/bb --config plane submit open --change
  run-heartbeat-18c7697 --rev
  18c7697ccadaa2ae493d0d9b6e3d5ddc61ef5a00 --context ... --json`
  created `6f745ee1b606`.
- `verify`: `verdict:pass`, run `2fb6d23b297f`, duration 51.880s,
  artifact dir `plane/.bb/runs/2fb6d23b297f/attempt-1`.
- `correctness`: `verdict:pass`, run `0e3eba5cd9ad`, cost
  `$0.075776449`, duration 201.781s, 152243 input tokens, 10116 output
  tokens.
- `simplification`: `verdict:pass`, run `bf843f0d06f8`, cost
  `$0.0166148673`, duration 226.221s, 97759 input tokens, 11044 output
  tokens.
- `product`: `verdict:pass`, run `3bbc8b9c0ac8`, cost `$0.03993955`,
  duration 25.552s, 12543 input tokens, 1968 output tokens.
- `security`: `not_started`; task stayed parked because the prior run cost
  `$0.2539` exceeded `max_cost_per_run_usd $0.25`.
- `bb gate --submission 6f745ee1b606 --json` returned
  `decision: pending` with no blocking, advisory, or rejected findings.

Additional friction:

- The critic lane again ignored the compact-output request for a long stretch
  and invoked its own shell commands. It eventually returned a useful `pass`,
  but the operator experience still needs bounded critic receipts.
- The storm itself used `--json`, so the new heartbeat did not apply. That is
  the intended contract, but it means agent-mode silence is still expected and
  must be paired with separate ledger reads when a human wants progress.

Additional delight:

- The gate stayed clean across the exact member set we can safely run while
  `security` remains parked.
- The heartbeat change did not disturb the submission run bundle shape; all
  member receipts were the same `run`/`attempts`/`events` JSON as before.

## Update 2026-06-13: 059 gate safe action

Backlog item: `059-submission-retry-and-operator-heartbeats`, child 2.

Work:

- Added `safe_next_command` and `safe_next_reason` to `bb gate --json` member
  entries when a canonical storm member is `run:failure`.
- The safe command opens a clean replacement submission for the same change and
  revision after the operator fixes the infrastructure issue. It does not
  suggest DLQ replay as gate recovery.
- Updated `docs/spine.md`, the Bitterblossom skill, operator recipes, and
  backlog `059`.

Verification:

- Red test first: `cargo test --test submission
  required_member_terminal_failure_escalates_with_one_notify -- --nocapture`
  failed because `MemberStatus` had no safe-action fields.
- Focused green: the same test now proves both the Rust report struct and the
  serialized JSON include `safe_next_command`.
- Fresh critic: `pi --provider openrouter --model deepseek/deepseek-v4-flash`
  found no blocking issues. Nonblocking note: the generated command relies on
  ambient plane config because it does not include `--config`.
- Full gate: first `./scripts/verify.sh` hit a one-off
  `ConnectionReset` in `read_api_requires_bearer_and_rejects_query_token`;
  focused rerun passed, and the second full `./scripts/verify.sh` passed with
  `src LOC: 4996`.

Friction:

- The first richer `safe_next_actions` array was more future-proof than the
  immediate need, but it exceeded the 5k spine cap. The final scalar
  `safe_next_command`/`safe_next_reason` shape is narrower and better matched
  to this recovery path.
- The source LOC cap forced another cleanup of redundant CLI doc comments.
  That is acceptable, but it is a reminder that every new operator affordance
  competes with existing inline explanation.
- The critic lane emitted a useful verdict, but the peer CLI streamed internal
  event/reasoning noise before the JSON and then exited nonzero with an
  `Extension ctx is stale` watchdog error. The model result was usable; the
  receipt surface was not.
- The live-server test flake did not reproduce, but the failure mode was
  expensive: a full green gate became a focused diagnosis loop because one
  readiness race surfaced as `ConnectionReset` instead of a clear server-start
  failure.

Delight:

- The gate can now say the thing I had to infer during dogfood: a replay may
  prove the failed pre-execute path, but a clean replacement submission is the
  canonical gate recovery path.
