//! End-to-end: a task defined purely in config executes on the local
//! substrate via a stub harness binary, and the ledger records cost,
//! tokens, attempts, and artifacts. The stub stands in for the external
//! harness CLI boundary only — everything else is the real spine.

use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::Path;
use std::process::Command;
use std::sync::Mutex;

use bitterblossom::ledger::{IngressRequest, Ledger};
use bitterblossom::spec::Plane;
use bitterblossom::{artifacts, dispatch};

static ENV_LOCK: Mutex<()> = Mutex::new(());

const CLAUDE_STUB: &str = r#"#!/bin/sh
# Claude-shaped result object on stdout; ignores its arguments.
cat > /dev/null
printf '{"status":"ok","artifact_paths":["REPORT.json"]}\n' > REPORT.json
echo '{"type":"result","subtype":"success","result":"commission complete","total_cost_usd":0.0123,"num_turns":3,"usage":{"input_tokens":120,"output_tokens":45}}'
"#;

const BROKEN_STUB: &str = r#"#!/bin/sh
cat > /dev/null
echo 'this is not json'
"#;

// bitterblossom-930: the agent parked on an ask instead of finishing --
// writes its own episodic handoff packet (ASK_PACKET.json) and a normal
// success-shaped result on stdout. The packet's presence, not the exit
// code or parsed stdout, is what makes dispatch classify this as parked.
const PARKED_STUB: &str = r#"#!/bin/sh
cat > /dev/null
printf '{"packet_version":1,"note":"parked mid-turn"}\n' > ASK_PACKET.json
echo '{"type":"result","subtype":"success","result":"parked, awaiting answer","total_cost_usd":0.0004,"num_turns":1,"usage":{"input_tokens":10,"output_tokens":5}}'
"#;

// Parseable claude-shaped result on stdout, but no REPORT.json written.
const NOREPORT_STUB: &str = r#"#!/bin/sh
cat > /dev/null
echo '{"type":"result","subtype":"success","result":"commission complete","total_cost_usd":0.0123,"num_turns":3,"usage":{"input_tokens":120,"output_tokens":45}}'
"#;

const ESTATE_RECEIPT_STUB: &str = r#"#!/bin/sh
cat > /dev/null
mkdir -p receipts
cp EVENT.json receipts/estate-action.json
printf '{"schema_version":"bb.command_result.v1","result":"estate receipt relayed"}\n'
"#;

fn write_executable(path: &Path, content: &str) {
    fs::write(path, content).unwrap();
    fs::set_permissions(path, fs::Permissions::from_mode(0o755)).unwrap();
}

/// Build a plane config dir with one agent + one local task.
fn make_plane(root: &Path, stub: &str, task_extra: &str) -> Plane {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
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
    let report = fs::read_to_string(artifact_dir.join("REPORT.json")).unwrap();
    assert!(report.contains(r#""artifact_paths":["REPORT.json"]"#));
    assert!(artifact_dir.join("workspace/LANE_CARD.md").exists());
}

#[test]
fn trigger_payload_materializes_as_event_json_in_workspace() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path(), CLAUDE_STUB, "");
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let run_id = ledger
        .ingest(IngressRequest {
            task: "demo",
            trigger_kind: "manual",
            idempotency_key: Some("manual:demo:7"),
            source_event_id: None,
            payload: Some(r#"{"pr":7,"action":"opened"}"#),
            parent_run_id: None,
        })
        .unwrap()
        .run_id;
    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();
    assert_eq!(run.state, "success");

    let attempts = ledger.attempts(&run_id).unwrap();
    let artifact_dir = std::path::Path::new(attempts[0].artifact_dir.as_deref().unwrap());
    let event = fs::read_to_string(artifact_dir.join("workspace/EVENT.json")).unwrap();
    assert_eq!(event, r#"{"pr":7,"action":"opened"}"#);

    let run_context: serde_json::Value =
        serde_json::from_str(&fs::read_to_string(artifact_dir.join("workspace/RUN.json")).unwrap())
            .unwrap();
    assert_eq!(run_context["run_id"], run_id);
    assert_eq!(run_context["task"], "demo");
    assert_eq!(run_context["trigger"]["kind"], "manual");
    assert_eq!(run_context["trigger"]["idempotency_key"], "manual:demo:7");
    assert_eq!(run_context["agent"]["name"], "stub");
    assert_eq!(run_context["agent"]["harness"], "claude");
}

#[test]
fn declared_estate_receipt_round_trips_without_reinterpretation() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(
        dir.path(),
        ESTATE_RECEIPT_STUB,
        "required_artifacts = [\"receipts/estate-action.json\"]",
    );
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let exact = r#"{ "schema":"estate.receipt.v1", "artifact_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "phase":"applied" }"#;
    let run_id = ledger
        .ingest(IngressRequest {
            task: "demo",
            trigger_kind: "manual",
            idempotency_key: Some("estate:receipt:1"),
            source_event_id: None,
            payload: Some(exact),
            parent_run_id: None,
        })
        .unwrap()
        .run_id;

    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();
    assert_eq!(run.state, "success", "{:?}", run.state_reason);
    assert_eq!(run.agent_name.as_deref(), Some("stub"));

    let attempts = ledger.attempts(&run_id).unwrap();
    let artifact_dir = Path::new(attempts[0].artifact_dir.as_deref().unwrap());
    assert_eq!(
        fs::read_to_string(artifact_dir.join("receipts/estate-action.json")).unwrap(),
        exact
    );
    let run_context: serde_json::Value =
        serde_json::from_str(&fs::read_to_string(artifact_dir.join("workspace/RUN.json")).unwrap())
            .unwrap();
    assert_eq!(run_context["agent"]["name"], "stub");
    fs::remove_dir_all(artifact_dir).unwrap();
    assert!(matches!(
        artifacts::read(&ledger, &run_id, "receipts/estate-action.json").unwrap(),
        artifacts::ReadOutcome::Text { content, .. } if content == exact
    ));
}

#[test]
fn isolated_worker_checks_out_exact_commit_and_lock_blob() {
    let _guard = ENV_LOCK.lock().unwrap();
    let dir = tempfile::tempdir().unwrap();
    let source = dir.path().join("estate-source");
    fs::create_dir(&source).unwrap();
    fs::write(source.join("Cargo.lock"), "version = 4\n").unwrap();
    fs::write(source.join("estate.txt"), "pinned\n").unwrap();
    for args in [
        vec!["init", "-q"],
        vec!["config", "user.name", "Bitterblossom Test"],
        vec!["config", "user.email", "bb-test@example.invalid"],
        vec!["add", "Cargo.lock", "estate.txt"],
        vec!["commit", "-q", "-m", "fixture"],
    ] {
        assert!(Command::new("git")
            .args(args)
            .current_dir(&source)
            .status()
            .unwrap()
            .success());
    }
    let git = |spec: &str| {
        String::from_utf8(
            Command::new("git")
                .args(["rev-parse", spec])
                .current_dir(&source)
                .output()
                .unwrap()
                .stdout,
        )
        .unwrap()
        .trim()
        .to_string()
    };
    let commit = git("HEAD");
    let lock_blob = git("HEAD:Cargo.lock");

    let root = dir.path().join("plane");
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/estate")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
    let stub = root.join("estate-stub.sh");
    write_executable(
        &stub,
        &format!(
            "#!/bin/sh\nset -eu\ncat >/dev/null\ntest \"$(git -C estate-source rev-parse HEAD)\" = {commit:?}\ntest \"$(git -C estate-source rev-parse HEAD:Cargo.lock)\" = {lock_blob:?}\nprintf '{{\"schema\":\"estate.receipt.v1\",\"phase\":\"applied\"}}\\n' > ESTATE_RECEIPT.json\nprintf '{{\"schema_version\":\"bb.command_result.v1\",\"result\":\"pinned estate execution\"}}\\n'\n"
        ),
    );
    fs::write(
        root.join("agents/estate.toml"),
        format!(
            "harness = \"command\"\nmodel = \"\"\nbin = \"{}\"\nrole = \"estate-executor\"\n",
            stub.display()
        ),
    )
    .unwrap();
    fs::write(
        root.join("tasks/estate/card.md"),
        "deterministic Estate adapter\n",
    )
    .unwrap();
    fs::write(
        root.join("tasks/estate/task.toml"),
        format!(
            "agent = \"estate\"\nsubstrate = \"local\"\nrequired_artifacts = [\"ESTATE_RECEIPT.json\"]\n[workspace]\nrepos = [{{ url = {:?}, ref = \"master\", commit = {:?}, locks = [{{ path = \"Cargo.lock\", git_blob = {:?} }}] }}]\n[[trigger]]\nkind = \"manual\"\n",
            source.to_string_lossy(), commit, lock_blob
        ),
    )
    .unwrap();

    let plane = Plane::load(&root).unwrap();
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let run_id = manual_run(&mut ledger, "estate", None);
    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();
    assert_eq!(run.state, "success", "{:?}", run.state_reason);
    assert_eq!(run.agent_name.as_deref(), Some("estate"));
    let attempts = ledger.attempts(&run_id).unwrap();
    let artifact_dir = Path::new(attempts[0].artifact_dir.as_deref().unwrap());
    assert!(artifact_dir.join("ESTATE_RECEIPT.json").exists());

    // A tree object alone is insufficient: checkout filters can materialize
    // different bytes. Pin a second commit whose attributes force CRLF and
    // prove the working-byte hash rejects it before the harness executes.
    fs::write(source.join(".gitattributes"), "Cargo.lock text eol=crlf\n").unwrap();
    for args in [
        vec!["add", ".gitattributes"],
        vec!["commit", "-q", "-m", "hostile checkout filter"],
    ] {
        assert!(Command::new("git")
            .args(args)
            .current_dir(&source)
            .status()
            .unwrap()
            .success());
    }
    let hostile_commit = git("HEAD");
    fs::write(
        root.join("tasks/estate/task.toml"),
        format!(
            "agent = \"estate\"\nsubstrate = \"local\"\nrequired_artifacts = [\"ESTATE_RECEIPT.json\"]\n[workspace]\nrepos = [{{ url = {:?}, ref = \"master\", commit = {:?}, locks = [{{ path = \"Cargo.lock\", git_blob = {:?} }}] }}]\n[[trigger]]\nkind = \"manual\"\n",
            source.to_string_lossy(), hostile_commit, lock_blob
        ),
    )
    .unwrap();
    let hostile_plane = Plane::load(&root).unwrap();
    let hostile_run_id = manual_run(&mut ledger, "estate", Some("hostile-filter"));
    let hostile_run = dispatch::dispatch_run(&hostile_plane, &mut ledger, &hostile_run_id).unwrap();
    assert_eq!(hostile_run.state, "failure");
    assert!(
        hostile_run
            .state_reason
            .as_deref()
            .is_some_and(|reason| reason.starts_with("dead_letter:")),
        "{:?}",
        hostile_run.state_reason
    );
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
fn required_artifact_missing_zero_exit_marks_run_failed() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(
        dir.path(),
        NOREPORT_STUB,
        "required_artifacts = [\"REPORT.json\"]",
    );
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let run_id = manual_run(&mut ledger, "demo", None);
    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();

    // Zero-exit harness with a parseable final message, but the required
    // artifact was never produced: the run is a failure, not a silent success.
    assert_eq!(run.state, "failure");
    let reason = run.state_reason.unwrap();
    assert!(
        reason.contains("missing required artifact"),
        "reason={reason}"
    );
    assert!(reason.contains("REPORT.json"), "reason={reason}");

    let attempts = ledger.attempts(&run_id).unwrap();
    assert_eq!(attempts.len(), 1);
    let a = &attempts[0];
    assert_eq!(a.outcome.as_deref(), Some("failure"));
    assert_eq!(a.exit_code, Some(0));

    // The artifact directory is preserved for inspection even on contract
    // failure: stdout/result.md are still there, REPORT.json is not.
    let artifact_dir = Path::new(a.artifact_dir.as_deref().unwrap());
    assert!(artifact_dir.join("stdout.txt").exists());
    assert!(artifact_dir.join("result.md").exists());
    assert!(!artifact_dir.join("REPORT.json").exists());
}

#[test]
fn required_artifact_present_stays_success() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(
        dir.path(),
        CLAUDE_STUB,
        "required_artifacts = [\"REPORT.json\"]",
    );
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let run_id = manual_run(&mut ledger, "demo", None);
    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();

    // Declaring the contract does not false-positive when the artifact lands.
    assert_eq!(run.state, "success");
    let attempts = ledger.attempts(&run_id).unwrap();
    assert_eq!(attempts[0].outcome.as_deref(), Some("success"));
    let artifact_dir = Path::new(attempts[0].artifact_dir.as_deref().unwrap());
    assert!(artifact_dir.join("REPORT.json").exists());
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
fn hermetic_exec_passes_secrets_but_never_the_planes_env() {
    let _guard = ENV_LOCK.lock().unwrap();
    // The stub echoes back what it sees: a plane env var that must NOT
    // cross, the declared secret that must, and HOME which must be
    // workspace-local for api-auth (hermetic) agents.
    const PI_ENV_STUB: &str = r#"#!/bin/sh
cat > /dev/null
printf '{"type":"turn_end"}\n'
printf '{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"leak=[%s] secret=[%s] home=[%s]"}],"usage":{"input":1,"output":2,"cost":{"total":0.0001}}}}\n' "$BB_TEST_LEAK" "$BB_TEST_SECRET" "$HOME"
"#;
    std::env::set_var("BB_TEST_LEAK", "plane-private");
    std::env::set_var("BB_TEST_SECRET", "declared-secret");

    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
    let stub_path = root.join("stub-pi.sh");
    write_executable(&stub_path, PI_ENV_STUB);
    fs::write(
        root.join("agents/stub.toml"),
        format!(
            "harness = \"pi\"\nmodel = \"m\"\nbin = \"{}\"\nsecrets = [\"BB_TEST_SECRET\"]\n",
            stub_path.display()
        ),
    )
    .unwrap();
    fs::write(root.join("tasks/demo/card.md"), "card\n").unwrap();
    fs::write(
        root.join("tasks/demo/task.toml"),
        "agent = \"stub\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
    let plane = Plane::load(root).unwrap();
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let run_id = manual_run(&mut ledger, "demo", None);
    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();
    assert_eq!(run.state, "success", "{:?}", run.state_reason);

    let attempts = ledger.attempts(&run_id).unwrap();
    let artifact_dir = Path::new(attempts[0].artifact_dir.as_deref().unwrap());
    let result = fs::read_to_string(artifact_dir.join("result.md")).unwrap();
    assert!(result.contains("leak=[]"), "plane env leaked: {result}");
    assert!(result.contains("secret=[declared-secret]"), "{result}");
    assert!(
        result.contains("workspace/.home"),
        "HOME not hermetic: {result}"
    );
}

#[test]
fn missing_optional_secret_degrades_the_run_instead_of_dead_lettering_it() {
    let _guard = ENV_LOCK.lock().unwrap();
    std::env::remove_var("BB_TEST_OPTIONAL");

    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
    let stub_path = root.join("stub-pi.sh");
    write_executable(&stub_path, CLAUDE_STUB);
    fs::write(
        root.join("agents/stub.toml"),
        format!(
            "harness = \"claude\"\nmodel = \"m\"\nbin = \"{}\"\noptional_secrets = [\"BB_TEST_OPTIONAL\"]\n",
            stub_path.display()
        ),
    )
    .unwrap();
    fs::write(root.join("tasks/demo/card.md"), "card\n").unwrap();
    fs::write(
        root.join("tasks/demo/task.toml"),
        "agent = \"stub\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
    let plane = Plane::load(root).unwrap();
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let run_id = manual_run(&mut ledger, "demo", None);
    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();

    // The whole point: an unset OPTIONAL secret never dead-letters the run.
    // A REQUIRED secret in this exact shape would fail at "acquired" before
    // the harness ever spawned (see dispatch.rs's hard-required path).
    assert_eq!(run.state, "success", "{:?}", run.state_reason);
}

#[test]
fn present_optional_secret_still_reaches_the_workload_when_set() {
    let _guard = ENV_LOCK.lock().unwrap();
    const PI_ENV_STUB: &str = r#"#!/bin/sh
cat > /dev/null
printf '{"type":"turn_end"}\n'
printf '{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"optional=[%s]"}],"usage":{"input":1,"output":2,"cost":{"total":0.0001}}}}\n' "$BB_TEST_OPTIONAL"
"#;
    std::env::set_var("BB_TEST_OPTIONAL", "present-value");

    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
    let stub_path = root.join("stub-pi.sh");
    write_executable(&stub_path, PI_ENV_STUB);
    fs::write(
        root.join("agents/stub.toml"),
        format!(
            "harness = \"pi\"\nmodel = \"m\"\nbin = \"{}\"\noptional_secrets = [\"BB_TEST_OPTIONAL\"]\n",
            stub_path.display()
        ),
    )
    .unwrap();
    fs::write(root.join("tasks/demo/card.md"), "card\n").unwrap();
    fs::write(
        root.join("tasks/demo/task.toml"),
        "agent = \"stub\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
    let plane = Plane::load(root).unwrap();
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let run_id = manual_run(&mut ledger, "demo", None);
    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();
    assert_eq!(run.state, "success", "{:?}", run.state_reason);

    let attempts = ledger.attempts(&run_id).unwrap();
    let artifact_dir = Path::new(attempts[0].artifact_dir.as_deref().unwrap());
    let result = fs::read_to_string(artifact_dir.join("result.md")).unwrap();
    assert!(result.contains("optional=[present-value]"), "{result}");
    std::env::remove_var("BB_TEST_OPTIONAL");
}

#[test]
fn workspace_clone_can_use_declared_gh_token_without_url_credentials() {
    let _guard = ENV_LOCK.lock().unwrap();
    const COMMAND_STUB: &str = r#"#!/bin/sh
cat > /dev/null
[ -z "${GH_TOKEN:-}" ] || { echo "GH_TOKEN leaked into workload" >&2; exit 8; }
printf '{"artifact_paths":["REPORT.json"]}\n' > REPORT.json
printf '{"schema_version":"bb.command_result.v1","result":"ok"}\n'
"#;
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    let fake_bin = root.join("bin");
    fs::create_dir(&fake_bin).unwrap();
    let git_log = root.join("git-auth.log");
    write_executable(
        &fake_bin.join("git"),
        r#"#!/bin/sh
set -eu
case "$1" in
  init)
    dest=""
    for arg in "$@"; do dest="$arg"; done
    mkdir -p "$dest/.git"
    ;;
  -C)
    case "$3" in
      fetch)
        user="$("$GIT_ASKPASS" "Username for https://github.com")"
        pass="$("$GIT_ASKPASS" "Password for https://github.com")"
        printf '%s|%s|%s|%s\n' "$user" "$pass" "${GIT_TERMINAL_PROMPT:-}" "${GIT_CONFIG_KEY_0:-}" >> "$BB_GIT_AUTH_LOG"
        ;;
    esac
    ;;
  *)
    printf 'unexpected git invocation: %s\n' "$*" >&2
    exit 9
    ;;
esac
"#,
    );
    let old_path = std::env::var("PATH").unwrap_or_default();
    let old_gh_token = std::env::var_os("GH_TOKEN");
    let old_git_auth_log = std::env::var_os("BB_GIT_AUTH_LOG");
    std::env::set_var("PATH", format!("{}:{old_path}", fake_bin.display()));
    std::env::set_var("GH_TOKEN", "test-gh-token");
    std::env::set_var("BB_GIT_AUTH_LOG", &git_log);

    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
    let stub_path = root.join("stub-command.sh");
    write_executable(&stub_path, COMMAND_STUB);
    fs::write(
        root.join("agents/stub.toml"),
        format!(
            "harness = \"command\"\nmodel = \"\"\nbin = \"{}\"\nsecrets = [\"GH_TOKEN\", \"BB_GIT_AUTH_LOG\"]\n",
            stub_path.display()
        ),
    )
    .unwrap();
    fs::write(root.join("tasks/demo/card.md"), "card\n").unwrap();
    fs::write(
        root.join("tasks/demo/task.toml"),
        "agent = \"stub\"\nsubstrate = \"local\"\nrequired_artifacts = [\"REPORT.json\"]\n[workspace]\nrepos = [{ url = \"https://github.com/misty-step/bastion.git\", ref = \"master\" }]\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
    let plane = Plane::load(root).unwrap();
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let run_id = manual_run(&mut ledger, "demo", None);
    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();
    std::env::set_var("PATH", old_path);
    match old_gh_token {
        Some(value) => std::env::set_var("GH_TOKEN", value),
        None => std::env::remove_var("GH_TOKEN"),
    }
    match old_git_auth_log {
        Some(value) => std::env::set_var("BB_GIT_AUTH_LOG", value),
        None => std::env::remove_var("BB_GIT_AUTH_LOG"),
    }

    assert_eq!(run.state, "success", "{:?}", run.state_reason);
    let log = fs::read_to_string(git_log).unwrap();
    assert_eq!(
        log.trim(),
        "x-access-token|test-gh-token|0|credential.helper"
    );
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

#[test]
fn pi_exit_code_survives_the_flood_filter_pipeline() {
    // A pi that emits a parseable message_end and then dies must fail the
    // run — the grep filter's exit status must not mask the harness's.
    const DYING_PI: &str = r#"#!/bin/sh
cat > /dev/null
printf '{"type":"turn_end"}\n{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"looks done"}],"usage":{"input":1,"output":1,"cost":{"total":0.001}}}}\n'
exit 3
"#;
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
    let stub_path = root.join("dying-pi.sh");
    write_executable(&stub_path, DYING_PI);
    fs::write(
        root.join("agents/stub.toml"),
        format!(
            "harness = \"pi\"\nmodel = \"m\"\nbin = \"{}\"\n",
            stub_path.display()
        ),
    )
    .unwrap();
    fs::write(root.join("tasks/demo/card.md"), "card\n").unwrap();
    fs::write(
        root.join("tasks/demo/task.toml"),
        "agent = \"stub\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
    let plane = Plane::load(root).unwrap();
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let run_id = manual_run(&mut ledger, "demo", None);
    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();
    assert_eq!(run.state, "failure", "{:?}", run.state_reason);
    assert!(
        run.state_reason.as_deref().unwrap().contains("exit 3"),
        "{:?}",
        run.state_reason
    );
}

#[test]
fn ask_packet_marker_parks_the_run_instead_of_succeeding() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path(), PARKED_STUB, "");
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let run_id = manual_run(&mut ledger, "demo", None);
    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();

    assert_eq!(run.state, "parked_on_ask", "{:?}", run.state_reason);
    assert!(
        run.cost_usd.is_some(),
        "parked runs still record attempt cost"
    );

    let attempts = ledger.attempts(&run_id).unwrap();
    assert_eq!(attempts.len(), 1);
    assert_eq!(attempts[0].outcome.as_deref(), Some("parked_on_ask"));
    assert_eq!(attempts[0].phase, "released");

    let artifact_dir = Path::new(attempts[0].artifact_dir.as_deref().unwrap());
    let packet = fs::read_to_string(artifact_dir.join(dispatch::ASK_PACKET_FILENAME)).unwrap();
    assert!(packet.contains("parked mid-turn"));

    // No retry: a single attempt is terminal for a parked outcome, unlike
    // pre-execute failures which retry up to MAX_RETRIES.
    assert_eq!(ledger.attempt_count(&run_id).unwrap(), 1);
}

#[test]
fn ask_packet_bypasses_the_required_artifact_contract() {
    let dir = tempfile::tempdir().unwrap();
    // required_artifacts names something the PARKED_STUB never writes; a
    // parked outcome must not be reclassified as a missing-artifact failure.
    let plane = make_plane(
        dir.path(),
        PARKED_STUB,
        "required_artifacts = [\"REPORT.json\"]\n",
    );
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let run_id = manual_run(&mut ledger, "demo", None);
    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();

    assert_eq!(run.state, "parked_on_ask", "{:?}", run.state_reason);
}
