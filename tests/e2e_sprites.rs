//! Sprites-substrate e2e against a stub `sprite` CLI (the real CLI is a
//! network boundary). The stub emulates `exec -s` / `restore` locally,
//! mapping /home/sprite onto a temp dir — everything else is the real
//! spine, so the run row must come out identical in shape to local.

use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::Path;

use bitterblossom::dispatch;
use bitterblossom::ledger::{IngressRequest, Ledger};
use bitterblossom::spec::Plane;
use bitterblossom::substrate::sprites::SpritesSubstrate;
use bitterblossom::substrate::{ProbeResult, Substrate};

const SPRITE_STUB: &str = r#"#!/bin/bash
# Fake sprite CLI. Usage it must understand:
#   sprite exec -s <host> [--file src:dest] [--dir d] <cmd...>
#   sprite restore -s <host> <checkpoint-id>
log="$SPRITE_STUB_LOG"
home="$SPRITE_FAKE_HOME"
cmd="$1"; shift
echo "$cmd $*" >> "$log"
case "$cmd" in
  restore)
    exit 0;;
  exec)
    declare -a rest
    while [ $# -gt 0 ]; do
      case "$1" in
        -s|--dir|--env|-o) shift 2;;
        --) shift; break;;
        --file)
          spec="$2"; shift 2
          src="${spec%%:*}"; dest="${spec#*:}"
          dest="${dest//\/home\/sprite/$home}"
          mkdir -p "$(dirname "$dest")"
          cp "$src" "$dest";;
        *) break;;
      esac
    done
    while [ $# -gt 0 ]; do
      rest+=("${1//\/home\/sprite/$home}"); shift
    done
    # macOS test hosts have no setsid; the session-leader semantics are
    # exercised on real sprites, not by the stub.
    if [ "${rest[0]}" = "setsid" ]; then
      rest=("${rest[@]:1}")
      [ "${rest[0]}" = "-w" ] && rest=("${rest[@]:1}")
    fi
    # Bare `sh` means the script arrives on stdin; rewrite paths there too.
    if [ "${rest[0]}" = "sh" ] && [ "${#rest[@]}" -eq 1 ]; then
      sed "s|/home/sprite|$home|g" | sh
      exit $?
    fi
    exec "${rest[@]}";;
  *)
    echo "stub: unknown command $cmd" >&2; exit 64;;
esac
"#;

const CLAUDE_STUB: &str = r#"#!/bin/sh
cat > /dev/null
# Per-exec secret must arrive via the environment (stdin script exports).
[ "$BB_TEST_SECRET" = "s3cret" ] || { echo "secret missing" >&2; exit 5; }
[ ! -e REPORT.json ] || { echo "stale report leaked into run" >&2; exit 6; }
printf '{"status":"ok","artifact_paths":["REPORT.json"]}\n' > REPORT.json
echo '{"type":"result","subtype":"success","result":"sprite says hi","total_cost_usd":0.005,"num_turns":1,"usage":{"input_tokens":10,"output_tokens":5}}'
"#;

// Env vars are process-global; tests setting BB_SPRITE_BIN must serialize.
static ENV_LOCK: std::sync::Mutex<()> = std::sync::Mutex::new(());

fn write_executable(path: &Path, content: &str) {
    fs::write(path, content).unwrap();
    fs::set_permissions(path, fs::Permissions::from_mode(0o755)).unwrap();
}

#[test]
fn sprites_task_runs_end_to_end_with_identical_row_shape() {
    let _guard = ENV_LOCK.lock().unwrap();
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    let fake_home = root.join("fake-sprite-home");
    fs::create_dir_all(&fake_home).unwrap();

    let sprite_stub = root.join("sprite-stub.sh");
    write_executable(&sprite_stub, SPRITE_STUB);
    let claude_stub = root.join("claude-stub.sh");
    write_executable(&claude_stub, CLAUDE_STUB);

    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    fs::write(
        root.join("agents/remote.toml"),
        format!(
            "version = 1\nharness = \"claude\"\nmodel = \"claude-fable-5\"\nbin = \"{}\"\nsecrets = [\"BB_TEST_SECRET\"]\n",
            claude_stub.display()
        ),
    )
    .unwrap();
    fs::write(root.join("tasks/demo/card.md"), "# Sprite demo card\n").unwrap();
    fs::write(
        root.join("tasks/demo/task.toml"),
        "agent = \"remote\"\nsubstrate = \"sprites\"\n\n[workspace]\nhost = \"test-sprite\"\ncheckpoint = \"v999\"\n\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
    let plane = Plane::load(root).unwrap();
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    // Env is process-global: these tests must not run in parallel with
    // other env-sensitive tests in this file (there are none).
    let log = root.join("sprite-stub.log");
    std::env::set_var("BB_SPRITE_BIN", &sprite_stub);
    std::env::set_var("SPRITE_STUB_LOG", &log);
    std::env::set_var("SPRITE_FAKE_HOME", &fake_home);
    std::env::set_var("BB_TEST_SECRET", "s3cret");

    let run_id = ledger
        .ingest(IngressRequest {
            task: "demo",
            trigger_kind: "manual",
            idempotency_key: None,
            source_event_id: None,
            payload: None,
            parent_run_id: None,
        })
        .unwrap()
        .run_id;
    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();

    assert_eq!(run.state, "success", "reason: {:?}", run.state_reason);
    assert_eq!(run.cost_usd, Some(0.005));

    let attempts = ledger.attempts(&run_id).unwrap();
    assert_eq!(attempts.len(), 1);
    let a = &attempts[0];
    assert_eq!(a.outcome.as_deref(), Some("success"));
    assert_eq!(a.phase, "released");
    assert_eq!(a.tokens_in, Some(10));
    let artifact_dir = Path::new(a.artifact_dir.as_deref().unwrap());
    let report = fs::read_to_string(artifact_dir.join("REPORT.json")).unwrap();
    assert!(report.contains(r#""artifact_paths":["REPORT.json"]"#));

    let run_context: serde_json::Value =
        serde_json::from_str(&fs::read_to_string(artifact_dir.join("RUN.json")).unwrap()).unwrap();
    assert_eq!(run_context["run_id"], run_id);
    assert_eq!(run_context["task"], "demo");
    assert_eq!(run_context["agent"]["name"], "remote");
    assert_eq!(run_context["agent"]["harness"], "claude");

    // The card and run metadata were materialized into the (fake) remote workspace.
    assert!(fake_home.join("bb/demo/LANE_CARD.md").exists());
    assert!(fake_home.join("bb/demo/RUN.json").exists());
    // Checkpoint restore was requested before preparing.
    let log_text = fs::read_to_string(&log).unwrap();
    assert!(
        log_text.contains("restore -s test-sprite v999"),
        "{log_text}"
    );
    // The secret reached the harness (the stub asserts it) but never
    // appeared in argv — the stub logs every argv it sees — nor in the db.
    assert!(
        !log_text.contains("s3cret"),
        "secret leaked into argv: {log_text}"
    );
    let db_bytes = fs::read(plane.db_path()).unwrap();
    assert!(!String::from_utf8_lossy(&db_bytes).contains("s3cret"));

    let run_id = ledger
        .ingest(IngressRequest {
            task: "demo",
            trigger_kind: "manual",
            idempotency_key: Some("second"),
            source_event_id: None,
            payload: None,
            parent_run_id: None,
        })
        .unwrap()
        .run_id;
    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();
    assert_eq!(run.state, "success", "reason: {:?}", run.state_reason);

    std::env::remove_var("BB_SPRITE_BIN");
    std::env::remove_var("BB_TEST_SECRET");
}

const HERMETIC_PI_STUB: &str = r#"#!/bin/sh
cat > /dev/null
printf '{"type":"turn_end"}\n{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"home=[%s] leak=[%s] secret=[%s]"}],"usage":{"input":1,"output":1,"cost":{"total":0.0001}}}}\n' "$HOME" "$BB_SPRITE_LEAK" "$BB_TEST_SECRET2"
"#;

#[test]
fn hermetic_sprite_exec_scrubs_inherited_env_and_relocates_home() {
    let _guard = ENV_LOCK.lock().unwrap();
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    let fake_home = root.join("fake-sprite-home");
    fs::create_dir_all(&fake_home).unwrap();
    let sprite_stub = root.join("sprite-stub.sh");
    write_executable(&sprite_stub, SPRITE_STUB);
    let pi_stub = root.join("pi-stub.sh");
    write_executable(&pi_stub, HERMETIC_PI_STUB);

    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo2")).unwrap();
    fs::write(
        root.join("agents/r.toml"),
        format!(
            "harness = \"pi\"\nmodel = \"m\"\nbin = \"{}\"\nsecrets = [\"BB_TEST_SECRET2\"]\n",
            pi_stub.display()
        ),
    )
    .unwrap();
    fs::write(root.join("tasks/demo2/card.md"), "card\n").unwrap();
    fs::write(
        root.join("tasks/demo2/task.toml"),
        "agent = \"r\"\nsubstrate = \"sprites\"\n\n[workspace]\nhost = \"test-sprite-2\"\n\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
    let plane = Plane::load(root).unwrap();
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let log = root.join("sprite-stub-2.log");
    std::env::set_var("BB_SPRITE_BIN", &sprite_stub);
    std::env::set_var("SPRITE_STUB_LOG", &log);
    std::env::set_var("SPRITE_FAKE_HOME", &fake_home);
    // The "sprite image" env that must NOT reach a hermetic workload.
    std::env::set_var("BB_SPRITE_LEAK", "baked-into-image");
    std::env::set_var("BB_TEST_SECRET2", "declared");

    let run_id = ledger
        .ingest(IngressRequest {
            task: "demo2",
            trigger_kind: "manual",
            idempotency_key: None,
            source_event_id: None,
            payload: None,
            parent_run_id: None,
        })
        .unwrap()
        .run_id;
    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();
    assert_eq!(run.state, "success", "reason: {:?}", run.state_reason);

    let attempts = ledger.attempts(&run_id).unwrap();
    let artifact_dir = Path::new(attempts[0].artifact_dir.as_deref().unwrap());
    let result = fs::read_to_string(artifact_dir.join("result.md")).unwrap();
    assert!(result.contains("/.home]"), "HOME not relocated: {result}");
    assert!(result.contains("leak=[]"), "image env leaked: {result}");
    assert!(result.contains("secret=[declared]"), "{result}");

    std::env::remove_var("BB_SPRITE_BIN");
    std::env::remove_var("BB_SPRITE_LEAK");
    std::env::remove_var("BB_TEST_SECRET2");
}

#[test]
fn sprite_probe_treats_malformed_pidfile_as_unknown() {
    let _guard = ENV_LOCK.lock().unwrap();
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    let fake_home = root.join("fake-sprite-home");
    fs::create_dir_all(&fake_home).unwrap();
    let sprite_stub = root.join("sprite-stub.sh");
    write_executable(&sprite_stub, SPRITE_STUB);

    let log = root.join("sprite-stub-probe.log");
    std::env::set_var("BB_SPRITE_BIN", &sprite_stub);
    std::env::set_var("SPRITE_STUB_LOG", &log);
    std::env::set_var("SPRITE_FAKE_HOME", &fake_home);

    let marker = format!("bb-probe-test-{}", std::process::id());
    let pidfile = std::path::PathBuf::from(format!("/tmp/{marker}.pid"));
    fs::write(&pidfile, "not-a-pid").unwrap();

    let probe = SpritesSubstrate.probe("test-sprite", root, &marker);
    match probe {
        ProbeResult::Unknown(reason) => {
            assert!(reason.contains("malformed pidfile"), "{reason}");
        }
        other => panic!("malformed pidfile must be unknown, got {other:?}"),
    }

    let _ = fs::remove_file(pidfile);
    std::env::remove_var("BB_SPRITE_BIN");
    std::env::remove_var("SPRITE_STUB_LOG");
    std::env::remove_var("SPRITE_FAKE_HOME");
}

#[test]
fn sprite_probe_command_failure_is_unknown() {
    let _guard = ENV_LOCK.lock().unwrap();
    let dir = tempfile::tempdir().unwrap();
    let missing = dir.path().join("missing-sprite-binary");
    std::env::set_var("BB_SPRITE_BIN", &missing);

    let probe = SpritesSubstrate.probe("test-sprite", dir.path(), "bb-missing-sprite-bin");
    match probe {
        ProbeResult::Unknown(reason) => {
            assert!(reason.contains("probe failed"), "{reason}");
        }
        other => panic!("probe command failure must be unknown, got {other:?}"),
    }

    std::env::remove_var("BB_SPRITE_BIN");
}
