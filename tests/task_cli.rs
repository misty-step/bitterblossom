use std::fs;
use std::process::Command;

fn bb() -> &'static str {
    env!("CARGO_BIN_EXE_bb")
}

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

fn run_bb(root: &std::path::Path, args: &[&str]) -> std::process::Output {
    Command::new(bb())
        .arg("--config")
        .arg(root)
        .args(args)
        .output()
        .unwrap()
}

fn park_demo(root: &std::path::Path) {
    let park = run_bb(
        root,
        &["task", "park", "demo", "--reason", "operator pause"],
    );
    assert!(
        park.status.success(),
        "{}",
        String::from_utf8_lossy(&park.stderr)
    );
}

fn blocked_manual_run(root: &std::path::Path, key: &str) {
    let run = run_bb(root, &["run", "demo", "--idempotency-key", key, "--json"]);
    assert!(
        run.status.success(),
        "{}",
        String::from_utf8_lossy(&run.stderr)
    );
}

fn set_run_timestamp(root: &std::path::Path, key: &str, ts: &str) {
    let conn = rusqlite::Connection::open(root.join(".bb/plane.db")).unwrap();
    let changed = conn
        .execute(
            "UPDATE runs SET created_at = ?1, updated_at = ?1 WHERE idempotency_key = ?2",
            rusqlite::params![ts, key],
        )
        .unwrap();
    assert_eq!(changed, 1);
}

fn run_by_key<'a>(runs: &'a serde_json::Value, key: &str) -> &'a serde_json::Value {
    runs.as_array()
        .unwrap()
        .iter()
        .find(|run| run["idempotency_key"] == key)
        .unwrap()
}

#[test]
fn task_list_cli_reports_parked_state() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());

    park_demo(dir.path());

    let json = run_bb(dir.path(), &["task", "list", "--json"]);
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

#[test]
fn task_unpark_refuses_bulk_release_without_confirmation() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    park_demo(dir.path());
    blocked_manual_run(dir.path(), "old");
    blocked_manual_run(dir.path(), "new");
    set_run_timestamp(dir.path(), "old", "2026-06-01T00:00:00Z");
    set_run_timestamp(dir.path(), "new", "2026-07-04T00:00:00Z");

    let unpark = run_bb(dir.path(), &["task", "unpark", "demo"]);
    assert!(
        !unpark.status.success(),
        "bulk unpark should require --yes; stdout={} stderr={}",
        String::from_utf8_lossy(&unpark.stdout),
        String::from_utf8_lossy(&unpark.stderr)
    );
    let out = format!(
        "{}{}",
        String::from_utf8_lossy(&unpark.stdout),
        String::from_utf8_lossy(&unpark.stderr)
    );
    assert!(out.contains("2 blocked_budget run(s)"), "{out}");
    assert!(out.contains("oldest 2026-06-01T00:00:00Z"), "{out}");
    assert!(out.contains("newest 2026-07-04T00:00:00Z"), "{out}");
    assert!(out.contains("--yes"), "{out}");
}

#[test]
fn task_unpark_since_only_requeues_recent_blocked_runs() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    park_demo(dir.path());
    blocked_manual_run(dir.path(), "old");
    blocked_manual_run(dir.path(), "new");
    set_run_timestamp(dir.path(), "old", "2026-06-01T00:00:00Z");
    set_run_timestamp(dir.path(), "new", "2026-07-04T00:00:00Z");

    let unpark = run_bb(
        dir.path(),
        &[
            "task",
            "unpark",
            "demo",
            "--since",
            "2026-07-01T00:00:00Z",
            "--yes",
        ],
    );
    assert!(
        unpark.status.success(),
        "{}",
        String::from_utf8_lossy(&unpark.stderr)
    );
    let out = String::from_utf8_lossy(&unpark.stdout);
    assert!(out.contains("2 blocked_budget run(s)"), "{out}");
    assert!(out.contains("1 selected for release"), "{out}");

    let list = run_bb(dir.path(), &["runs", "list", "--task", "demo", "--json"]);
    assert!(
        list.status.success(),
        "{}",
        String::from_utf8_lossy(&list.stderr)
    );
    let rows: serde_json::Value = serde_json::from_slice(&list.stdout).unwrap();
    assert_eq!(run_by_key(&rows, "old")["state"], "blocked_budget");
    assert_eq!(run_by_key(&rows, "new")["state"], "pending");
}
