use std::fs;
use std::process::Command;

fn write_plane(root: &std::path::Path) {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    fs::write(
        root.join("agents/a.toml"),
        "version = 7\nharness = \"command\"\nmodel = \"\"\nbin = \"true\"\n",
    )
    .unwrap();
    fs::write(root.join("tasks/demo/card.md"), "card\n").unwrap();
    fs::write(
        root.join("tasks/demo/task.toml"),
        "agent = \"a\"\nsubstrate = \"local\"\nverdict = \"verify\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
}

#[test]
fn task_list_cli_reports_parked_state() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let bb = env!("CARGO_BIN_EXE_bb");

    let park = Command::new(bb)
        .args([
            "--config",
            dir.path().to_str().unwrap(),
            "task",
            "park",
            "demo",
            "--reason",
            "operator pause",
        ])
        .output()
        .unwrap();
    assert!(
        park.status.success(),
        "{}",
        String::from_utf8_lossy(&park.stderr)
    );

    let json = Command::new(bb)
        .args([
            "--config",
            dir.path().to_str().unwrap(),
            "task",
            "list",
            "--json",
        ])
        .output()
        .unwrap();
    assert!(
        json.status.success(),
        "{}",
        String::from_utf8_lossy(&json.stderr)
    );
    let rows: serde_json::Value = serde_json::from_slice(&json.stdout).unwrap();
    assert_eq!(rows[0]["task"], "demo");
    assert_eq!(rows[0]["agent"], "a@v7");
    assert_eq!(rows[0]["substrate"], "local");
    assert_eq!(rows[0]["triggers"], 1);
    assert_eq!(rows[0]["verdict"], "verify");
    assert_eq!(rows[0]["parked"], "operator pause");
}
