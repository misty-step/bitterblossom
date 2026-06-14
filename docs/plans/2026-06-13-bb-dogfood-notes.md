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

Submission storm:

- Change: `e0f8fecf329af8b99475a041fc8fea89f4bcd215`
  (`feat: add gate safe recovery command`).
- Submission `9dd735ad16ce` ran `verify` and `correctness` successfully:
  `verify` run `39f404d8f879`, duration 50.649s; `correctness` run
  `7c2511a4539e`, cost `$0.18108557`, duration 247.467s.
- Canonical `simplification` run `e9edd6960249` failed in 68.922s with
  `harness exit 1: Error: connection closed`.
- `bb gate --submission 9dd735ad16ce --json` returned
  `decision: escalated` and the failed member carried:
  `safe_next_command: bb submit open --change gate-safe-action-e0f8fec --rev
  e0f8fecf329af8b99475a041fc8fea89f4bcd215 --json` and
  `safe_next_reason: canonical simplification run e9edd6960249 failed:
  harness exit 1: Error: connection closed`.
- Replacement submission `02e587ec4533` was opened with `--config plane`
  added manually. Available members then passed:
  `verify` run `a131ef006aea`, duration 43.851s;
  `correctness` run `ec4e8cb11b50`, cost `$0.37316533`, duration 419.171s;
  `simplification` run `267b841b4cc8`, cost `$0.0319799066`, duration
  228.989s; `product` run `d44541cca3eb`, cost `$0.07496845`, duration
  35.205s.
- `bb gate --submission 02e587ec4533 --json` returned `decision: pending`:
  all unparked members were `verdict:pass`, while `security` remained
  `not_started` because it is still parked.

Additional friction:

- The generated `safe_next_command` is directionally right but not
  self-contained in this repo checkout: it omitted `--config plane`, so the
  command needed manual context before it would run from the repo root.
- A plain command string is probably the wrong final safe-action primitive.
  If Bitterblossom wants agent-grade recovery, it should consider structured
  argv plus display text, or a shell-escaped command that includes plane
  context.
- The transient `connection closed` failure consumed the canonical
  `simplification` key exactly like a real operator/infrastructure failure.
  The new gate guidance made recovery obvious, but the replacement storm still
  required rerunning every unparked member for the new submission.

Additional delight:

- This slice dogfooded itself: the first live storm hit a canonical member
  failure, and the new JSON fields immediately explained the clean replacement
  path.
- The strict canonical-key model felt good once the recovery command existed:
  the plane stayed rigorous without forcing the operator to reverse-engineer
  what to do next.

Backlog implications:

- 059 child 5 is satisfied by the skill/recipe updates plus this real
  submission example.
- Follow-up: safe actions should become self-contained agent actions, not only
  human-readable command strings with ambient config assumptions.

## Update 2026-06-13: 059 self-contained gate safe command

Backlog item: `059-submission-retry-and-operator-heartbeats`, child 6.

Work:

- Changed failed-member `safe_next_command` to include the loaded plane
  `--config` path, shell-quoted in the displayed command, so an agent can run
  the suggested clean replacement submission command from another cwd.
- Kept the existing scalar `safe_next_command`/`safe_next_reason` JSON shape
  instead of adding a new structured argv field in this slice.
- Updated `docs/spine.md`, the Bitterblossom skill, operator recipes, and
  backlog `059`.

Local verification:

- Red test first: `cargo test --test submission
  required_member_terminal_failure_escalates_with_one_notify -- --nocapture`
  failed because the command omitted `--config <plane-root>`.
- Focused green: the same test now proves the report struct and serialized JSON
  include the loaded plane path in `safe_next_command`.
- Source budget check after implementation: repo LOC oracle returned
  `src LOC: 4998`; full `./scripts/verify.sh` later passed with
  `src LOC: 4996`.
- Fresh critic: Claude found no blocking issues. Nonblocking notes: Rust
  `Debug` formatting is ordinary path quoting rather than a full shell-escape
  contract, and the test shares the same formatting primitive as production.

Friction:

- The plain command string remains less agent-native than structured argv; it
  still depends on shell interpretation. This is acceptable for the immediate
  retry path, but versioned agent schemas in backlog 053 should revisit action
  shape.

Delight:

- The fix was tiny because the gate evaluator already has the canonical
  `Plane`; the product lesson from the previous storm could be folded back into
  the exact JSON surface that caused the friction.

Submission storm:

- Change: `ed14c9b6b343d7973e2d52431046ce171b50593b`
  (`fix: include plane config in gate recovery`).
- Submission: `./target/debug/bb --config plane submit open --change
  gate-config-safe-command-ed14c9b --rev
  ed14c9b6b343d7973e2d52431046ce171b50593b --context ... --json`
  created `14418aa38938`.
- Available members passed on Misty Step Sprites:
  `verify` run `9668dd6d2256`, duration 48.216s;
  `correctness` run `ef5d73e24bff`, cost `$0.096619474`, duration
  219.380s; `simplification` run `b7de92bace1b`, cost `$0.0179772326`,
  duration 224.407s; `product` run `d72ecd1c8963`, cost `$0.0432494`,
  duration 29.878s.
- `bb gate --submission 14418aa38938 --json` returned `decision: pending`:
  all unparked members were `verdict:pass`, while `security` remained
  `not_started` because it is still parked.

Backlog implications:

- `backlog.d/059-submission-retry-and-operator-heartbeats.md` is complete and
  moved to `backlog.d/_done/`.
- Backlog 053 remains the right place to upgrade action-shaped JSON from a
  human command string to a versioned structured argv/schema contract.

## Update 2026-06-14: 050 panic-safe dispatch in-flight cleanup

Backlog item: `050-event-plane-hardening-before-growth`, child 2.

Work:

- Made `bb serve` run workers remove their task from the in-memory `in_flight`
  set even when the worker panics after dispatching a run, then resume the
  unwind so the panic remains visible.
- Added a live-server regression that seeds two pending runs for the same task,
  forces the first worker to panic after dispatch, and proves the second run
  still drains.
- Updated backlog `050` to mark the in-flight cleanup oracle complete.

Verification so far:

- Red test first: `cargo test --test serve
  dispatch_worker_panic_does_not_strand_task_in_flight -- --nocapture` failed
  because the second run stayed `pending`.
- Focused green: the same test now passes.
- Full local gate: `./scripts/verify.sh` passed; final source budget output was
  `src LOC: 5000`.
- Fresh critic status: an earlier Claude critic found no blockers and caught
  an overzealous CLI help cleanup. After the final mutex-lifetime fix, two peer
  CLI critic attempts (`claude -p`, then `pi -p`) hung without output and were
  stopped; a native read-only critic lane then failed at launch because its
  fixed model was unavailable on this Codex account.

Friction:

- Testing a true Rust worker panic through the binary required a debug-only
  `BB_TEST_PANIC_AFTER_RUN_ID` seam because ordinary harness failures are
  correctly modeled as run failures, not thread panics.
- The 5000-line spine cap forced another small cleanup loop. The first cleanup
  removed useful Clap help comments; the critic caught that as operator-facing
  scope creep, so the final cleanup preserved the help and recovered the budget
  from incidental formatting.
- Peer harness CLIs were a bad critic path in this run: both Claude and Pi
  accepted the prompt but produced no output before manual cancellation.
- Native critic lanes can fail before review when the role pins a model that
  the current account cannot use; the tool surfaced this only after launch.

Delight:

- The failure was easy to reproduce once the seam existed: one live `bb serve`
  process, two real pending ledger rows, and no mocked dispatcher.
- The fix preserves the useful panic signal while removing the operational
  starvation hazard.

Submission storm:

- Commit: `eecb169b108d1edbd5a252cd589013168252b472`
  (`fix: release in-flight task on worker panic`), pushed to `master`.
- Preflight: `flyctl orgs list` showed Misty Step, `sprite org list` selected
  `misty-step`, `sprite use -o misty-step lane-1` succeeded, and
  `sprite exec -- whoami` returned `sprite`.
- Submission: `./target/debug/bb --config plane submit open --change
  serve-panic-cleanup-eecb169 --rev
  eecb169b108d1edbd5a252cd589013168252b472 --context ... --json` created
  `b1286039a7de`.
- Available members passed on Misty Step Sprites:
  `verify` run `6320e959d4db`, duration 52.559s;
  `correctness` run `eebac44e4518`, cost `$0.115813849`, duration 288.139s;
  `simplification` run `151a5f65de3f`, cost `$0.0236074608`, duration
  204.706s; `product` run `eb95dc0ac445`, cost `$0.0268762`, duration
  25.618s.
- `bb gate --submission b1286039a7de --json` returned `decision: pending`:
  all unparked members were `verdict:pass`, while `security` remained
  `not_started` because it is still parked for
  `run cost $0.2539 > max_cost_per_run_usd $0.25`.

More UX notes:

- Friction: `bb run --json` is perfect for scripts but gives the supervising
  operator no heartbeat during multi-minute verdict runs; I had to poll
  `bb status --json` from another shell to distinguish "quiet by design" from
  "stuck process".
- Delight: the ledger made that workaround reliable. Status showed the exact
  running task and run id (`eebac44e4518`, then `151a5f65de3f`) without
  disturbing the blocking `bb run --json` process.

## Update 2026-06-14: 050 CLI/docs/skill parity

Backlog item: `050-event-plane-hardening-before-growth`, child 4.

Work:

- Added `tests/cli_contract_docs.rs` to execute live `bb` help for
  `run`, `runs export`, and `gate`, then scan current user-facing docs and
  skills for stale or missing agent-facing examples.
- Updated `skills/bitterblossom/SKILL.md` and
  `skills/bitterblossom/references/operator-recipes.md` so the portable skill
  names `bb --config <plane> runs export` and keeps submission placeholders
  consistent.
- Updated backlog `050` to mark the CLI/docs/skill parity oracle complete.

Verification so far:

- Red test first: `cargo test --test cli_contract_docs -- --nocapture` failed
  because the portable skill did not document `bb --config <plane> runs export`.
- Focused green: the same test now passes.
- Full local gate: `./scripts/verify.sh` passed with `src LOC: 5000`.
- Live help read before edits: `bb run --help` exposed `--payload <PAYLOAD>`
  and `--json`; `bb runs export --help` exposed no `--since`; `bb gate --help`
  exposed `--submission`, `--change`, and `--json`.
- Fresh critic: a headless Codex artifact-only review returned
  `BLOCKING: none` and `VERDICT: pass`; it also caught that the new test file
  was still untracked before staging.

Friction:

- The current docs were already fixed, but the skill was missing the export
  seam entirely. A stale-command grep would not have caught an omitted command;
  the parity test had to assert required positive examples too.
- Backlog and archival docs intentionally contain stale strings as evidence, so
  the regression has to scope itself to current user-facing contracts instead
  of sweeping the whole repo.
- The headless Codex critic worked this time, but emitted noisy plugin/token
  warnings before the final verdict.

Delight:

- This is a compact, cheap gate for the agent interface. It caught a real skill
  omission without spending model or Sprite time.
- The live help output is now the oracle; skills and docs no longer rely on
  memory of what `bb` used to accept.
- The critic's untracked-file note was exactly the kind of mechanical closeout
  check that keeps a dogfood slice honest.

Submission storm:

- Commit: `87c959e41e2dbae2f2619474a1e98d5bff0b7eaf`
  (`test: lock bb cli skill parity`), pushed to `master`.
- Submission: `./target/debug/bb --config plane submit open --change
  cli-skill-parity-87c959e --rev
  87c959e41e2dbae2f2619474a1e98d5bff0b7eaf --context ... --json` created
  `d03da5f2c8d3`.
- Available members passed on Misty Step Sprites:
  `verify` run `8635f92d89e2`, duration 51.416s;
  `product` run `794f4610d32d`, cost `$0.03753575`, duration 28.814s;
  `correctness` run `e3b346a9f5cd`, cost `$0.041094914`, duration 117.859s;
  `simplification` run `bdb2b17139cf`, cost `$0.0102486382`, duration
  166.469s.
- `bb gate --submission d03da5f2c8d3 --json` returned `decision: pending`:
  all unparked members were `verdict:pass`, while `security` remained
  `not_started` because it is still parked for
  `run cost $0.2539 > max_cost_per_run_usd $0.25`.

## Update 2026-06-14: 050 bounded notification accounting

Backlog item: `050-event-plane-hardening-before-growth`, child 3.

Work:

- Removed the detached waiter thread from `notify::notify`; notification
  delivery still uses the dependency-free `curl -m 10` seam, but the caller now
  waits for the child and logs non-zero or wait failures before returning.
- Added `notification_storm_is_synchronously_accounted`, which uses a slow fake
  `BB_NOTIFY_BIN` and asserts a burst cannot return while child processes are
  still outstanding.
- Updated backlog `050` to mark the notification accounting oracle complete.

Verification so far:

- Red test first: `cargo test --test budgets
  notification_storm_is_synchronously_accounted -- --nocapture` failed with
  `left: 0`, `right: 8`; no fake notify children had finished when `notify()`
  returned.
- Focused green: the same test now passes.
- Affected suites: `cargo test --test budgets -- --nocapture` and
  `cargo test --test submission -- --nocapture` pass.
- Full local gate: `./scripts/verify.sh` passed with `src LOC: 4999`.

Friction:

- The existing `curl -m 10` already bounded child lifetime, so the bug was not
  obvious from a casual read; the actual hazard was the unbounded detached
  waiter thread per notification.
- The backlog wording asked for failed or saturated notification attempts to be
  recorded. A durable notification-attempt ledger would be a larger observability
  feature; this slice stayed inside the existing `curl` seam and synchronously
  logs failures instead.

Delight:

- `BB_NOTIFY_BIN` made the storm regression cheap and hermetic: no network, no
  server process, no sleeps in production code.
- The small fix reduces spine LOC instead of consuming more of the 5k budget.

Residual:

- Synchronous notification means a bad webhook can now block the caller for up
  to the existing `curl -m 10` timeout per notification. That is intentionally
  bounded, but a future operator setting for timeout/concurrency may be worth
  shaping if storms become frequent.

Submission storm:

- Commit: `4b15d57433e57e6012b266673ac65c870487d5cd`
  (`fix: account notification delivery synchronously`), pushed to `master`.
- Fresh critic: headless Codex artifact-only review returned
  `BLOCKING: <none>`, `SERIOUS: <none>`, `VERDICT: pass`.
- First submission: `./target/debug/bb --config plane submit open --change
  notify-accounting-4b15d57 --rev
  4b15d57433e57e6012b266673ac65c870487d5cd --context ... --json` created
  `dcb0632a4f86`.
- First storm results: `verify` run `e90c2ea30689` passed in 53.286s;
  `correctness` run `df0df09cfff7` passed with cost `$0.067627101` in
  169.151s; `product` run `ea5c03e825cb` failed before execution after three
  acquire attempts because `bb-polisher-3` was unreachable, creating DLQ `7`.
- First gate: `./target/debug/bb --config plane gate --submission
  dcb0632a4f86 --json` returned `decision: escalated` and included the safe
  next command to open a clean replacement submission.
- Host recovery check: `sprite -o misty-step -s bb-polisher-3 exec -- whoami`
  then returned `sprite`.
- Replacement submission: `8d56e6cc6a13`.
- Replacement available members passed on Misty Step Sprites:
  `verify` run `bdd50c064401`, duration 45.793s;
  `product` run `becd95dae3ca`, cost `$0.02678975`, duration 22.786s;
  `correctness` run `0460ed26d870`, cost `$0.052301152`, duration 147.042s;
  `simplification` run `5e4b0b978242`, cost `$0.0166731446`, duration
  191.033s.
- Replacement gate: `./target/debug/bb --config plane gate --submission
  8d56e6cc6a13 --json` returned `decision: pending`: all unparked members were
  `verdict:pass`, while `security` remained `not_started` because it is still
  parked for `run cost $0.2539 > max_cost_per_run_usd $0.25`.

More UX notes:

- Friction: a pre-execute DLQ on a canonical storm member escalates the
  submission correctly, but the safe-next replacement came back as `round: 1`
  with `prior_report_json: null`; the operator has to preserve the escalated
  context manually in notes.
- Friction: `bb-polisher-3` recovered immediately under a direct Sprite probe
  after three timed-out product acquire attempts. The storm told the truth, but
  there is no built-in "probe this task host" action in `bb status`.
- Delight: the gate output gave the exact safe next command and safe next
  reason, so I did not have to infer whether DLQ replay would count for the
  canonical member. It would not.

## Update 2026-06-14: 050 live control-loop drill

Backlog item: `050-event-plane-hardening-before-growth`, child 6 and final
verification oracle.

Work:

- Added `scripts/control-loop-drill.sh`, a repeatable live drill for the
  control-loop evidence that should not live only in prose memory.
- The script creates a temp dev plane with a local command harness, starts
  `bb serve`, curls open-loopback read API/HTML, fires five signed webhook
  deliveries against a `max_runs_per_day = 1` task, asserts containment, then
  restarts with `BB_API_TOKEN` and verifies bearer-only read access.
- Updated `CLAUDE.md` verification guidance and backlog `050` with the new
  drill command and evidence.

Verification so far:

- `./scripts/control-loop-drill.sh` passed:
  open-loopback `/health`, `/api/status`, `/api/tasks`, `/api/runs`,
  `/api/dlq`, `/api/submissions`, and `/` returned `200`.
- The webhook storm accepted five signed deliveries with `202`, then settled
  to one `success` and four `blocked_budget` runs; the task parked with
  `1 runs today >= max_runs_per_day 1` and the notify stub recorded four
  `budget_blocked` notifications.
- With `BB_API_TOKEN`, `/health` remained `200` unauthenticated;
  no-token `/api/status`, `?token=`, and bad bearer returned `401`; bearer
  `/api/status`, `/api/tasks`, `/api/runs`, `/api/dlq`, `/api/submissions`,
  and `/` returned `200`.
- `./scripts/verify.sh` passed with `src LOC: 4999`.
- Fresh-context critic: a read-only Claude CLI lane produced no output after
  roughly 90 seconds and was stopped; `pi --no-tools --no-context-files
  --no-skills --no-extensions --no-session -p` reviewed the artifact-only diff
  and returned `BLOCKING: none`, `SERIOUS-NONBLOCKING: none`, `VERDICT: pass`.

Friction:

- This was exactly the kind of evidence that belonged in a script. Re-running
  the live loopback and storm checks from notes would otherwise require
  rebuilding temp configs, HMAC signing, curl assertions, and JSON state checks
  by hand.
- The storm exposes current behavior honestly: once a task is parked, later
  pending rows also emit `budget_blocked` notifications with `task_parked`
  semantics. Useful, but the notification event name is broad.
- Peer CLI critic tools were uneven: Claude stalled without output, and an
  initial `pi` critic returned a pass verdict but then crashed in the
  ops-watchdog extension with a stale extension context. Disabling extensions
  made `pi` reliable for this artifact-only review.

Delight:

- A command-harness temp plane lets the full `serve -> webhook -> ledger ->
  dispatch -> notify -> read API` loop run in a few seconds with no model
  credentials and no permanent ledger mutation.
- This gives future dogfood runs one crisp command for the control-loop proof.

Backlog cleanup:

- `047`, `048`, `049`, and `050` moved to `_done/`: 047 and 049 were direct
  children of 050, 048 was delivered by 052, and 050 now has its final live
  verification evidence.

Submission storm:

- Commit: `8a144608a0a61fab08a781f4186ed37e242a88ec`
  (`test: add live control-loop drill`).
- Misty Step preflight stayed correct: `sprite org list` selected
  `misty-step`, and `sprite exec -- whoami` plus explicit host probes for
  `lane-1`, `bb-polisher-2`, and `bb-polisher-3` returned `sprite`.
- `./target/debug/bb --config plane submit open --change
  control-loop-drill-8a14460 --rev
  8a144608a0a61fab08a781f4186ed37e242a88ec --context ... --json` created
  submission `b339972dfea1`.
- Available members:
  - `verify`: pass, run `b907ea21b8ef`, duration `48671ms`.
  - `correctness`: pass, run `8d639e8fdf39`, cost `$0.06311096`, duration
    `300059ms`.
  - `product`: pass, run `8bd946ed1214`, cost `$0.05243745`, duration
    `43158ms`.
  - `simplification`: advisory, run `b8c44e5f53cb`, cost `$0.0201310126`,
    duration `190781ms`.
- `security` was not run because the task remains parked with
  `run cost $0.2539 > max_cost_per_run_usd $0.25`.
- `./target/debug/bb --config plane gate --submission b339972dfea1 --json`
  returned `decision: pending`, no blocking findings, two minor advisory
  findings from simplification, and `security` as `not_started`.
- Post-storm `bb status --json`: `cost_today_usd: 0.7120934888`,
  `open_dlq: 5`, `parked_tasks: 1`, and no active pending/running queues.

Storm UX notes:

- Friction: `bb gate` still reports parked required work as `not_started`
  without joining the task's parked reason. I had to read `bb status --json`
  and `bb task list --json` to confirm the safe decision was to leave security
  parked.
- Friction: the simplification advisory claimed `webhook_url` was dead because
  `BB_NOTIFY_BIN` was set, but `notify::notify` still requires `webhook_url`
  to emit anything; this is a reviewer false positive that the gate preserved
  as advisory rather than blocking.
- Delight: probing each Sprite host before dispatch avoided repeating the
  previous `bb-polisher-3` acquire DLQ, and staggering product/simplification
  on the shared host produced a clean available-member storm.
- Lean in: the gate's advisory structure is useful even when I reject an
  advisory; fingerprinted minor findings are easy to discuss without
  conflating them with blocking correctness.

## Update 2026-06-14: 051 malformed recovery probe evidence

Backlog item: `051-deterministic-recovery-and-probe-contract`, malformed
probe/evidence slice.

Preflight:

- `git status --short --branch --untracked-files=all`: clean
  `## master...origin/master`.
- `flyctl orgs list`: `Misty Step` / `misty-step` available.
- `sprite org list`: selected org `misty-step`; `sprite use -o misty-step
  lane-1` confirmed the repo context; `sprite exec -- whoami` returned
  `sprite`.
- `./target/debug/bb --config plane check`: green.
- `./target/debug/bb --config plane status --json`: `cost_today_usd:
  0.7120934888`, `open_dlq: 5`, `parked_tasks: 1`, no active queues.
- `./target/debug/bb --config plane task list --json`: `security` remains
  parked for `run cost $0.2539 > max_cost_per_run_usd $0.25`.

Work:

- Selected 051 because 050 is closed and the next custom-spine proof
  obligation is recovery trust under uncertain substrate probes.
- Added a Sprite probe regression test proving malformed remote pidfiles are
  `Unknown`, not `Dead`.
- Added a CLI-level recovery test proving malformed local pidfile evidence is
  visible in `recover --json`, `runs show --json`, and the `boot_probe` event,
  while the host lease remains held.
- Fixed `src/substrate/sprites.rs` so a non-numeric remote pidfile cannot
  release a host lease as if the process were proven dead.
- Updated `docs/spine.md`, `skills/bitterblossom/references/operator-recipes.md`,
  and backlog 051 with the unknown-probe contract.

Verification so far:

- Red proof: `cargo test --test e2e_sprites
  sprite_probe_treats_malformed_pidfile_as_unknown -- --nocapture` failed
  with `malformed pidfile must be unknown, got Dead`.
- Green focused tests:
  - `cargo test --test e2e_sprites
    sprite_probe_treats_malformed_pidfile_as_unknown -- --nocapture`
  - `cargo test --test recovery
    unknown_probe_evidence_is_visible_for_operator_resolution -- --nocapture`
- `cargo test --test e2e_sprites -- --nocapture` passed after cleaning up
  test env vars noted by the critic.
- `./scripts/verify.sh` passed with `src LOC: 4997`.
- Fresh-context critic: `pi --no-tools --no-context-files --no-skills
  --no-extensions --no-session -p` stalled with no output and was stopped;
  `codex exec --sandbox read-only --ephemeral` returned `BLOCKING: none` and
  `VERDICT: pass`, with one nonblocking test-env cleanup that was fixed.

UX notes:

- Friction: the recovery backlog is large enough that a single turn should not
  pretend to close it. I kept 051 open and recorded this as a slice because
  stale-age escalation and a full probe state-machine spec still remain.
- Friction: the source budget immediately shaped the implementation. It caught
  a small growth in `src` and forced the fix to stay at the substrate seam.
- Friction: the peer-critic path is still noisy. `pi` stalled this time, while
  `codex exec` completed but emitted a large volume of plugin/MCP warnings
  before the useful verdict.
- Delight: `runs show --json` already had the right event bundle; the missing
  piece was a test that proved the `boot_probe` evidence actually reached the
  operator.
- Mitigate: 051 still needs stale `awaiting_recovery` policy and broader
  malformed/missing/stale fixture coverage before it can move to `_done/`.

Submission storm:

- Commit: `82618d769fc608e6c5a0ae2b4eb005f85881b1fe`
  (`fix: keep malformed sprite probes unknown`).
- Host probes before dispatch:
  - `lane-1`: `sprite`.
  - `bb-polisher-2`: `sprite`.
  - `bb-polisher-3`: failed after about 60s with
    `failed to connect ... i/o timeout`.
- `./target/debug/bb --config plane submit open --change
  malformed-probe-82618d7 --rev 82618d769fc608e6c5a0ae2b4eb005f85881b1fe
  --context ... --json` created submission `d98e75753cf3`.
- Available members:
  - `verify`: pass, run `0bea26171ddc`, duration `49076ms`.
  - `correctness`: pass, run `aa402b054b05`, cost `$0.1175718`, duration
    `245269ms`.
- `security` was not run because it remains parked for
  `run cost $0.2539 > max_cost_per_run_usd $0.25`.
- `product` and `simplification` were not run because both use
  `bb-polisher-3`, which failed the pre-dispatch host probe. I avoided creating
  another predictable acquire DLQ.
- `./target/debug/bb --config plane gate --submission d98e75753cf3 --json`
  returned `decision: pending`, `verify` and `correctness` as pass, no
  blocking or advisory findings, and `security`, `simplification`, and
  `product` as `not_started`.

Storm UX notes:

- Friction: `bb` has no first-class "probe this task host" command, so I had
  to use `sprite -o misty-step -s <host> exec -- whoami` outside the plane to
  decide whether product/simplification were safe to run.
- Delight: doing that host probe before `bb run` prevented another open DLQ
  for a known-unreachable shared Sprite.
- Mitigate: 051/054 should consider an operator-safe host probe or production
  smoke command that reports task host reachability without dispatching a
  workload.
