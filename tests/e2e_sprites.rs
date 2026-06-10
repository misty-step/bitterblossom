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
echo '{"type":"result","subtype":"success","result":"sprite says hi","total_cost_usd":0.005,"num_turns":1,"usage":{"input_tokens":10,"output_tokens":5}}'
"#;

fn write_executable(path: &Path, content: &str) {
    fs::write(path, content).unwrap();
    fs::set_permissions(path, fs::Permissions::from_mode(0o755)).unwrap();
}

#[test]
fn sprites_task_runs_end_to_end_with_identical_row_shape() {
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

    // The card was materialized into the (fake) remote workspace.
    assert!(fake_home.join("bb/demo/LANE_CARD.md").exists());
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

    std::env::remove_var("BB_SPRITE_BIN");
    std::env::remove_var("BB_TEST_SECRET");
}
