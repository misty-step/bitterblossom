//! End-to-end: a task defined purely in config executes on the local
//! substrate via a stub harness binary, and the ledger records cost,
//! tokens, attempts, and artifacts. The stub stands in for the external
//! harness CLI boundary only — everything else is the real spine.

use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::Path;

use bitterblossom::dispatch;
use bitterblossom::ledger::{IngressRequest, Ledger};
use bitterblossom::spec::Plane;

const CLAUDE_STUB: &str = r#"#!/bin/sh
# Claude-shaped result object on stdout; ignores its arguments.
cat > /dev/null
echo '{"type":"result","subtype":"success","result":"commission complete","total_cost_usd":0.0123,"num_turns":3,"usage":{"input_tokens":120,"output_tokens":45}}'
"#;

const BROKEN_STUB: &str = r#"#!/bin/sh
cat > /dev/null
echo 'this is not json'
"#;

fn write_executable(path: &Path, content: &str) {
    fs::write(path, content).unwrap();
    fs::set_permissions(path, fs::Permissions::from_mode(0o755)).unwrap();
}

/// Build a plane config dir with one agent + one local task.
fn make_plane(root: &Path, stub: &str, task_extra: &str) -> Plane {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    let stub_path = root.join("stub-harness.sh");
    write_executable(&stub_path, stub);
    fs::write(
        root.join("agents/stub.toml"),
        format!(
            "version = 2\nharness = \"claude\"\nmodel = \"claude-fable-5\"\nbin = \"{}\"\n",
            stub_path.display()
        ),
    )
    .unwrap();
    fs::write(
        root.join("tasks/demo/card.md"),
        "# Demo lane card\nSay hello.\n",
    )
    .unwrap();
    fs::write(
        root.join("tasks/demo/task.toml"),
        format!(
            "agent = \"stub\"\nsubstrate = \"local\"\n{task_extra}\n[[trigger]]\nkind = \"manual\"\n"
        ),
    )
    .unwrap();
    Plane::load(root).unwrap()
}

fn manual_run(ledger: &mut Ledger, task: &str, key: Option<&str>) -> String {
    ledger
        .ingest(IngressRequest {
            task,
            trigger_kind: "manual",
            idempotency_key: key,
            source_event_id: None,
            payload: None,
            parent_run_id: None,
        })
        .unwrap()
        .run_id
}

#[test]
fn demo_task_runs_end_to_end_with_cost_and_artifacts() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path(), CLAUDE_STUB, "");
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let run_id = manual_run(&mut ledger, "demo", None);
    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();

    assert_eq!(run.state, "success");
    assert_eq!(run.cost_usd, Some(0.0123));
    assert!(run.duration_ms.is_some());
    assert_eq!(run.agent_name.as_deref(), Some("stub"));
    assert_eq!(run.agent_version, Some(2));

    let attempts = ledger.attempts(&run_id).unwrap();
    assert_eq!(attempts.len(), 1);
    let a = &attempts[0];
    assert_eq!(a.outcome.as_deref(), Some("success"));
    assert_eq!(a.phase, "released");
    assert_eq!(a.tokens_in, Some(120));
    assert_eq!(a.tokens_out, Some(45));
    assert_eq!(a.turns, Some(3));
    assert_eq!(a.harness, "claude");

    let artifact_dir = Path::new(a.artifact_dir.as_deref().unwrap());
    let result = fs::read_to_string(artifact_dir.join("result.md")).unwrap();
    assert_eq!(result, "commission complete");
    assert!(artifact_dir.join("workspace/LANE_CARD.md").exists());
}

#[test]
fn unparseable_harness_output_is_failure_not_silent_success() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path(), BROKEN_STUB, "");
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let run_id = manual_run(&mut ledger, "demo", None);
    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();

    assert_eq!(run.state, "failure");
    assert!(run.state_reason.unwrap().contains("unparseable"));
    let attempts = ledger.attempts(&run_id).unwrap();
    // Executed once; no mechanical retry after the execute phase.
    assert_eq!(attempts.len(), 1);
    let artifact_dir = attempts[0].artifact_dir.as_deref().unwrap();
    let raw = fs::read_to_string(Path::new(artifact_dir).join("stdout.txt")).unwrap();
    assert!(raw.contains("this is not json"));
}

#[test]
fn pre_execute_failure_dead_letters_after_bounded_retries_and_replays() {
    let dir = tempfile::tempdir().unwrap();
    // pre_command failure happens in prepare — before executing.
    let plane = make_plane(dir.path(), CLAUDE_STUB, "pre_command = \"exit 7\"");
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let run_id = manual_run(&mut ledger, "demo", None);
    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();

    assert_eq!(run.state, "failure");
    assert!(run
        .state_reason
        .as_deref()
        .unwrap()
        .starts_with("dead_letter:"));
    // Initial attempt + 2 mechanical retries.
    assert_eq!(ledger.attempts(&run_id).unwrap().len(), 3);

    let dls = ledger.list_dead_letters().unwrap();
    assert_eq!(dls.len(), 1);

    // Replay mints a new run with lineage.
    let replay_id = manual_run(&mut ledger, "demo", Some("replay-key"));
    ledger
        .mark_dead_letter_replayed(dls[0].id, &replay_id)
        .unwrap();
    let replayed = ledger.dead_letter(dls[0].id).unwrap();
    assert_eq!(
        replayed.replayed_run_id.as_deref(),
        Some(replay_id.as_str())
    );
}

#[test]
fn timeout_kills_the_harness_and_fails_the_run() {
    let dir = tempfile::tempdir().unwrap();
    let slow = "#!/bin/sh\ncat > /dev/null\nsleep 60\n";
    // timeout_minutes only has minute granularity; use the budget field via
    // a tiny fractional minute is impossible, so set 0 -> rejected? Use 1
    // minute is too slow for tests; instead exercise the substrate timeout
    // directly through a 2-second budget by patching the task budget.
    let plane = make_plane(dir.path(), slow, "[budget]\ntimeout_minutes = 0\n");
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let run_id = manual_run(&mut ledger, "demo", None);
    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();
    assert_eq!(run.state, "failure");
    assert!(run.state_reason.unwrap().contains("timeout"));
}

#[test]
fn duplicate_manual_key_yields_one_run_two_ingress_events() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path(), CLAUDE_STUB, "");
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let first = manual_run(&mut ledger, "demo", Some("X"));
    let second = manual_run(&mut ledger, "demo", Some("X"));
    assert_eq!(first, second);
    assert_eq!(ledger.list_runs(Some("demo"), None).unwrap().len(), 1);
    assert_eq!(ledger.ingress_event_count("demo").unwrap(), 2);
    let _ = plane;
}

#[test]
fn max_runs_per_day_parks_task_and_blocks_dispatch() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path(), CLAUDE_STUB, "[budget]\nmax_runs_per_day = 1\n");
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let r1 = manual_run(&mut ledger, "demo", None);
    assert_eq!(
        dispatch::dispatch_run(&plane, &mut ledger, &r1)
            .unwrap()
            .state,
        "success"
    );

    let r2 = manual_run(&mut ledger, "demo", None);
    let blocked = dispatch::dispatch_run(&plane, &mut ledger, &r2).unwrap();
    assert_eq!(blocked.state, "blocked_budget");
    assert!(ledger.parked_reason("demo").unwrap().is_some());

    // Ingress while parked records the event but never dispatches.
    let r3 = manual_run(&mut ledger, "demo", None);
    assert_eq!(ledger.run(&r3).unwrap().state, "blocked_budget");
}
