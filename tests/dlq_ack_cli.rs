use std::fs;
use std::process::Command;

fn write_plane(root: &std::path::Path) {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/broken")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
    // `true` exists but is never reached: pre_command fails pre-execute, so the
    // run dead-letters after the mechanical retry budget.
    fs::write(
        root.join("agents/true.toml"),
        "version = 1\nharness = \"command\"\nmodel = \"\"\nbin = \"true\"\n",
    )
    .unwrap();
    fs::write(root.join("tasks/broken/card.md"), "card\n").unwrap();
    fs::write(
        root.join("tasks/broken/task.toml"),
        "agent = \"true\"\nsubstrate = \"local\"\npre_command = \"exit 7\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
}

fn bb(root: &str, args: &[&str]) -> std::process::Output {
    Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", root])
        .args(args)
        .output()
        .unwrap()
}

#[test]
fn acknowledge_persists_and_is_listed_as_acknowledged() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let root = dir.path().to_str().unwrap();

    // Produce a pre-execute dead letter.
    let failed = bb(root, &["run", "broken", "--json"]);
    assert!(!failed.status.success(), "broken run should fail");
    let initial: serde_json::Value = serde_json::from_slice(&failed.stdout).unwrap();
    assert!(initial["run"]["state_reason"]
        .as_str()
        .unwrap()
        .starts_with("dead_letter:"));

    // Acknowledge it with an explicit reason.
    let ack = bb(
        root,
        &[
            "dlq",
            "ack",
            "1",
            "--reason",
            "superseded by run r2",
            "--json",
        ],
    );
    assert!(
        ack.status.success(),
        "ack stdout:\n{}\nack stderr:\n{}",
        String::from_utf8_lossy(&ack.stdout),
        String::from_utf8_lossy(&ack.stderr)
    );
    let row: serde_json::Value = serde_json::from_slice(&ack.stdout).unwrap();
    assert_eq!(row["status"], "acknowledged");
    assert_eq!(row["acknowledged_reason"], "superseded by run r2");
    assert!(row["acknowledged_at"].as_str().unwrap().ends_with('Z'));
    assert!(
        row["replayed_run_id"].is_null(),
        "replay history must be untouched"
    );

    // list --json distinguishes acknowledged rows.
    let list = bb(root, &["dlq", "list", "--json"]);
    let docs: serde_json::Value = serde_json::from_slice(&list.stdout).unwrap();
    let entry = docs
        .as_array()
        .unwrap()
        .iter()
        .find(|d| d["id"] == 1)
        .unwrap();
    assert_eq!(entry["status"], "acknowledged");
    assert_eq!(entry["acknowledged_reason"], "superseded by run r2");
}

#[test]
fn acknowledge_requires_a_reason() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    assert!(!bb(root, &["run", "broken", "--json"]).status.success());

    // No --reason: clap rejects the missing required argument.
    let no_reason = bb(root, &["dlq", "ack", "1", "--json"]);
    assert!(!no_reason.status.success());
}

#[test]
fn replay_is_rejected_after_acknowledgement() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    assert!(!bb(root, &["run", "broken", "--json"]).status.success());
    assert!(bb(
        root,
        &["dlq", "ack", "1", "--reason", "superseded", "--json"]
    )
    .status
    .success());

    let replay = bb(root, &["dlq", "replay", "1", "--json"]);
    assert!(
        !replay.status.success(),
        "replay must be rejected after acknowledgement"
    );
    let err = String::from_utf8_lossy(&replay.stderr);
    assert!(
        err.contains("acknowledged") && err.contains("replay rejected"),
        "stderr should name acknowledgement and reject replay, got: {err}"
    );

    // Replay history stays immutable: the row is still acknowledged, not replayed.
    let list = bb(root, &["dlq", "list", "--json"]);
    let docs: serde_json::Value = serde_json::from_slice(&list.stdout).unwrap();
    let entry = docs
        .as_array()
        .unwrap()
        .iter()
        .find(|d| d["id"] == 1)
        .unwrap();
    assert_eq!(entry["status"], "acknowledged");
    assert!(entry["replayed_run_id"].is_null());
}

#[test]
fn acknowledge_is_rejected_for_already_replayed_or_already_acknowledged() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    assert!(!bb(root, &["run", "broken", "--json"]).status.success());

    // Double-acknowledge: the second is rejected.
    assert!(
        bb(root, &["dlq", "ack", "1", "--reason", "first", "--json"])
            .status
            .success()
    );
    let second = bb(root, &["dlq", "ack", "1", "--reason", "second", "--json"]);
    assert!(!second.status.success());
    let err = String::from_utf8_lossy(&second.stderr);
    assert!(err.contains("already acknowledged"), "got: {err}");
}

#[test]
fn status_excludes_acknowledged_dlq_from_open_and_drops_replay_action() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    assert!(!bb(root, &["run", "broken", "--json"]).status.success());

    // Before acknowledgement: the broken task has one open DLQ and a replay action.
    let before = bb(root, &["status", "--json"]);
    let doc: serde_json::Value = serde_json::from_slice(&before.stdout).unwrap();
    let broken = doc["tasks"]
        .as_array()
        .unwrap()
        .iter()
        .find(|t| t["task"] == "broken")
        .unwrap();
    assert_eq!(broken["dlq"]["open"], 1);
    assert!(broken["safe_next_actions"]
        .as_array()
        .unwrap()
        .iter()
        .any(|a| a["kind"] == "replay_pre_execute_dlq"));
    assert_eq!(doc["summary"]["open_dlq"], 1);

    assert!(bb(
        root,
        &["dlq", "ack", "1", "--reason", "superseded", "--json"]
    )
    .status
    .success());

    // After acknowledgement: the DLQ is no longer open operator work.
    let after = bb(root, &["status", "--json"]);
    let doc: serde_json::Value = serde_json::from_slice(&after.stdout).unwrap();
    let broken = doc["tasks"]
        .as_array()
        .unwrap()
        .iter()
        .find(|t| t["task"] == "broken")
        .unwrap();
    assert_eq!(
        broken["dlq"]["open"], 0,
        "acknowledged DLQ must not count as open"
    );
    assert_eq!(broken["dlq"]["acknowledged"], 1);
    assert!(
        !broken["safe_next_actions"]
            .as_array()
            .unwrap()
            .iter()
            .any(|a| a["kind"] == "replay_pre_execute_dlq"),
        "no replay action for an acknowledged DLQ"
    );
    assert_eq!(doc["summary"]["open_dlq"], 0);
}
