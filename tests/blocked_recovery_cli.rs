//! `bb runs release` must refuse a budget-blocked run that would just re-block
//! after the park is cleared (e.g. the daily run quota is still exhausted) —
//! otherwise the operator gets a false "released" and the task silently re-parks.

use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::process::Command;

const STUB: &str = r#"#!/bin/sh
cat > /dev/null
echo '{"type":"result","result":"ok","total_cost_usd":0.0,"num_turns":1,"usage":{"input_tokens":1,"output_tokens":1}}'
"#;

fn bb() -> &'static str {
    env!("CARGO_BIN_EXE_bb")
}

#[test]
fn release_cli_refuses_a_run_that_would_re_block_on_max_runs_per_day() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    let stub = root.join("stub.sh");
    fs::write(&stub, STUB).unwrap();
    fs::set_permissions(&stub, fs::Permissions::from_mode(0o755)).unwrap();
    fs::write(
        root.join("agents/a.toml"),
        format!(
            "version=1\nharness=\"claude\"\nmodel=\"m\"\nbin=\"{}\"\n",
            stub.display()
        ),
    )
    .unwrap();
    fs::write(root.join("tasks/demo/card.md"), "card\n").unwrap();
    fs::write(
        root.join("tasks/demo/task.toml"),
        "agent=\"a\"\nsubstrate=\"local\"\n[budget]\nmax_runs_per_day=1\n\
         [[trigger]]\nkind=\"manual\"\n",
    )
    .unwrap();
    fs::write(root.join("plane.toml"), "dev=true\n").unwrap();
    let cfg = root.to_str().unwrap();

    // First run consumes the daily quota; the second blocks (and parks) on it.
    for _ in 0..2 {
        Command::new(bb())
            .args(["--config", cfg, "run", "demo", "--json"])
            .output()
            .unwrap();
    }
    let list = Command::new(bb())
        .args([
            "--config",
            cfg,
            "runs",
            "list",
            "--state",
            "blocked_budget",
            "--json",
        ])
        .output()
        .unwrap();
    let rows: serde_json::Value = serde_json::from_slice(&list.stdout).unwrap();
    let id = rows[0]["id"].as_str().expect("a blocked_budget run");

    // release must REFUSE: clearing the park would not help — the run re-blocks
    // on the still-exhausted quota, which would also re-park the siblings.
    let released = Command::new(bb())
        .args(["--config", cfg, "runs", "release", id])
        .output()
        .unwrap();
    assert!(
        !released.status.success(),
        "release should refuse a run that would re-block on max_runs_per_day"
    );
    let err = String::from_utf8_lossy(&released.stderr);
    assert!(
        err.contains("max_runs_per_day") || err.contains("cannot release"),
        "{err}"
    );

    // retire still closes it.
    let retired = Command::new(bb())
        .args([
            "--config",
            cfg,
            "runs",
            "retire",
            id,
            "--reason",
            "over quota",
        ])
        .output()
        .unwrap();
    assert!(
        retired.status.success(),
        "{}",
        String::from_utf8_lossy(&retired.stderr)
    );
}
