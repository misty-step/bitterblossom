use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::process::Command;

fn write_executable(path: &std::path::Path, content: &str) {
    fs::write(path, content).unwrap();
    fs::set_permissions(path, fs::Permissions::from_mode(0o755)).unwrap();
}

fn write_plane(root: &std::path::Path, allow: &std::path::Path) {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
    let stub = root.join("stub.sh");
    write_executable(&stub, "#!/bin/sh\ncat > /dev/null\nprintf replayed\n");
    fs::write(
        root.join("agents/stub.toml"),
        format!(
            "version = 1\nharness = \"command\"\nmodel = \"\"\nbin = \"{}\"\n",
            stub.display()
        ),
    )
    .unwrap();
    fs::write(root.join("tasks/demo/card.md"), "card\n").unwrap();
    fs::write(
        root.join("tasks/demo/task.toml"),
        format!(
            "agent = \"stub\"\nsubstrate = \"local\"\npre_command = \"test -f '{}'\"\n[[trigger]]\nkind = \"manual\"\n",
            allow.display()
        ),
    )
    .unwrap();
}

fn bb(args: &[&str]) -> std::process::Output {
    Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(args)
        .output()
        .unwrap()
}

#[test]
fn dlq_replay_json_returns_replayed_run_bundle() {
    let dir = tempfile::tempdir().unwrap();
    let allow = dir.path().join("allow-replay");
    write_plane(dir.path(), &allow);
    let root = dir.path().to_str().unwrap();

    let failed = bb(&["--config", root, "run", "demo", "--json"]);
    assert!(!failed.status.success());
    let initial: serde_json::Value = serde_json::from_slice(&failed.stdout).unwrap();
    assert_eq!(initial["run"]["state"], "failure");
    assert!(initial["run"]["state_reason"]
        .as_str()
        .unwrap()
        .starts_with("dead_letter:"));

    fs::write(&allow, "").unwrap();
    let replay = bb(&["--config", root, "dlq", "replay", "1", "--json"]);
    assert!(
        replay.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&replay.stdout),
        String::from_utf8_lossy(&replay.stderr)
    );
    let doc: serde_json::Value = serde_json::from_slice(&replay.stdout).unwrap();
    assert_eq!(doc["run"]["task"], "demo");
    assert_eq!(doc["run"]["trigger_kind"], "replay");
    assert_eq!(doc["run"]["state"], "success");
    assert_eq!(
        doc["run"]["parent_run_id"], initial["run"]["id"],
        "replay should preserve dead-letter lineage"
    );
    assert_eq!(doc["attempts"].as_array().unwrap().len(), 1);
}
