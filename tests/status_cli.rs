use std::fs;
use std::process::Command;

fn write_plane(root: &std::path::Path) {
    fs::create_dir_all(root.join("agents")).unwrap();
    for task in ["ok", "broken"] {
        fs::create_dir_all(root.join("tasks").join(task)).unwrap();
        fs::write(root.join("tasks").join(task).join("card.md"), "card\n").unwrap();
    }
    fs::write(
        root.join("agents/true.toml"),
        "version = 1\nharness = \"command\"\nmodel = \"\"\nbin = \"true\"\n",
    )
    .unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
    fs::write(
        root.join("tasks/ok/task.toml"),
        "agent = \"true\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
    fs::write(
        root.join("tasks/broken/task.toml"),
        "agent = \"true\"\nsubstrate = \"local\"\npre_command = \"exit 7\"\n[[trigger]]\nkind = \"manual\"\n",
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
fn status_cli_clusters_tasks_runs_dlq_and_safe_actions() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let root = dir.path().to_str().unwrap();

    assert!(bb(&["--config", root, "run", "ok", "--json"])
        .status
        .success());
    assert!(bb(&[
        "--config",
        root,
        "task",
        "park",
        "ok",
        "--reason",
        "budget paused",
    ])
    .status
    .success());
    assert!(bb(&[
        "--config",
        root,
        "run",
        "ok",
        "--idempotency-key",
        "blocked-on-park",
        "--json",
    ])
    .status
    .success());

    let broken = bb(&["--config", root, "run", "broken", "--json"]);
    assert!(!broken.status.success());

    let status = bb(&["--config", root, "status", "--json"]);
    assert!(
        status.status.success(),
        "{}",
        String::from_utf8_lossy(&status.stderr)
    );
    let doc: serde_json::Value = serde_json::from_slice(&status.stdout).unwrap();
    let tasks = doc["tasks"].as_array().unwrap();
    let ok = tasks.iter().find(|t| t["task"] == "ok").unwrap();
    assert_eq!(ok["parked"], "budget paused");
    assert_eq!(ok["runs"]["by_state"]["success"], 1);
    assert_eq!(ok["runs"]["by_state"]["blocked_budget"], 1);
    assert!(ok["safe_next_actions"].as_array().unwrap().iter().any(|a| {
        a["kind"] == "unpark_after_reason_cleared" && a["command"] == "bb task unpark ok"
    }));

    let broken = tasks.iter().find(|t| t["task"] == "broken").unwrap();
    assert_eq!(broken["dlq"]["open"], 1);
    assert!(broken["safe_next_actions"]
        .as_array()
        .unwrap()
        .iter()
        .any(|a| { a["kind"] == "replay_pre_execute_dlq" && a["command"] == "bb dlq replay 1" }));
    assert_eq!(doc["summary"]["open_dlq"], 1);
    assert_eq!(doc["summary"]["parked_tasks"], 1);
}
