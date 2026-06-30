//! CLI integration for `bb artifacts`: a real local-plane run produces
//! REPORT.json, then `list --json` and `read` consume it through the public
//! binary surface. Edge-case fixtures discover the attempt artifact dir through
//! `bb runs show --json` instead of hardcoding storage layout.

use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::process::{Command, Output};

use bitterblossom::artifacts::READ_LIMIT;

fn write_executable(path: &std::path::Path, content: &str) {
    fs::write(path, content).unwrap();
    fs::set_permissions(path, fs::Permissions::from_mode(0o755)).unwrap();
}

/// A zero-credential command-harness plane: the agent writes REPORT.json into
/// its workspace, which the local substrate copies to the attempt artifact
/// dir on release.
fn write_plane(root: &std::path::Path) {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/hello")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
    let stub = root.join("agent.sh");
    write_executable(
        &stub,
        "#!/bin/sh\nprintf '{\"task\":\"hello\",\"ok\":true}\\n' > REPORT.json\necho hello\n",
    );
    fs::write(
        root.join("agents/local.toml"),
        format!(
            "version = 1\nharness = \"command\"\nmodel = \"\"\nbin = \"sh\"\nargs = [\"-c\", \"sh {stub}\"]\n",
            stub = stub.display()
        ),
    )
    .unwrap();
    fs::write(root.join("tasks/hello/card.md"), "# hello\n").unwrap();
    fs::write(
        root.join("tasks/hello/task.toml"),
        "agent = \"local\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
}

fn bb(root: &str, args: &[&str]) -> Output {
    Command::new(env!("CARGO_BIN_EXE_bb"))
        .arg("--config")
        .arg(root)
        .args(args)
        .output()
        .unwrap()
}

fn run_hello(root: &str) -> String {
    let run = bb(root, &["run", "hello", "--json"]);
    assert!(
        run.status.success(),
        "{}",
        String::from_utf8_lossy(&run.stderr)
    );
    let doc: serde_json::Value = serde_json::from_slice(&run.stdout).unwrap();
    doc["run"]["id"].as_str().unwrap().to_string()
}

fn artifact_dir(root: &str, run_id: &str) -> std::path::PathBuf {
    let show = bb(root, &["runs", "show", run_id, "--json"]);
    assert!(
        show.status.success(),
        "{}",
        String::from_utf8_lossy(&show.stderr)
    );
    let doc: serde_json::Value = serde_json::from_slice(&show.stdout).unwrap();
    doc["attempts"][0]["artifact_dir"]
        .as_str()
        .expect("attempt artifact_dir")
        .into()
}

#[test]
fn artifacts_list_and_read_report_json_through_cli() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let root = dir.path().to_str().unwrap();

    let run = bb(
        root,
        &["run", "hello", "--payload", "{\"ok\":true}", "--json"],
    );
    assert!(
        run.status.success(),
        "{}",
        String::from_utf8_lossy(&run.stderr)
    );
    let doc: serde_json::Value = serde_json::from_slice(&run.stdout).unwrap();
    let run_id = doc["run"]["id"].as_str().unwrap().to_string();

    let list = bb(root, &["artifacts", "list", &run_id, "--json"]);
    assert!(
        list.status.success(),
        "{}",
        String::from_utf8_lossy(&list.stderr)
    );
    let entries: serde_json::Value = serde_json::from_slice(&list.stdout).unwrap();
    let report = entries
        .as_array()
        .unwrap()
        .iter()
        .find(|e| e["path"] == "REPORT.json")
        .expect("REPORT.json listed");
    assert_eq!(report["content_type"], "application/json");
    assert_eq!(report["binary"], false);

    let read = bb(root, &["artifacts", "read", &run_id, "REPORT.json"]);
    assert!(
        read.status.success(),
        "{}",
        String::from_utf8_lossy(&read.stderr)
    );
    let body = String::from_utf8_lossy(&read.stdout);
    assert!(body.contains(r#""ok":true"#), "got: {body}");

    // --json envelope for read.
    let read_json = bb(
        root,
        &["artifacts", "read", &run_id, "REPORT.json", "--json"],
    );
    assert!(read_json.status.success());
    let env: serde_json::Value = serde_json::from_slice(&read_json.stdout).unwrap();
    assert_eq!(env["kind"], "text");
    assert!(env["content"].as_str().unwrap().contains(r#""ok":true"#));
}

#[test]
fn artifacts_read_missing_exits_nonzero_with_structured_error() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    let run_id = run_hello(root);

    let missing = bb(root, &["artifacts", "read", &run_id, "NOPE.json", "--json"]);
    assert!(!missing.status.success());
    let env: serde_json::Value = serde_json::from_slice(&missing.stdout).unwrap();
    assert_eq!(env["kind"], "missing");
}

#[test]
fn artifacts_read_binary_json_exits_nonzero_with_binary_envelope() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    let run_id = run_hello(root);
    fs::write(artifact_dir(root, &run_id).join("binary.bin"), [0xff, 0x00]).unwrap();

    let list = bb(root, &["artifacts", "list", &run_id, "--json"]);
    assert!(list.status.success());
    let entries: serde_json::Value = serde_json::from_slice(&list.stdout).unwrap();
    let binary = entries
        .as_array()
        .unwrap()
        .iter()
        .find(|e| e["path"] == "binary.bin")
        .expect("binary artifact listed");
    assert_eq!(binary["binary"], true);

    let read = bb(
        root,
        &["artifacts", "read", &run_id, "binary.bin", "--json"],
    );
    assert!(!read.status.success());
    let env: serde_json::Value = serde_json::from_slice(&read.stdout).unwrap();
    assert_eq!(env["kind"], "binary");
    assert_eq!(env["path"], "binary.bin");
}

#[test]
fn artifacts_read_incomplete_utf8_tail_is_binary_not_io_error() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    let run_id = run_hello(root);
    fs::write(
        artifact_dir(root, &run_id).join("incomplete.txt"),
        [b'a', 0xc3],
    )
    .unwrap();

    let read = bb(
        root,
        &["artifacts", "read", &run_id, "incomplete.txt", "--json"],
    );
    assert!(!read.status.success());
    let env: serde_json::Value = serde_json::from_slice(&read.stdout).unwrap();
    assert_eq!(env["kind"], "binary");
    assert_eq!(env["path"], "incomplete.txt");
}

#[test]
fn artifacts_read_oversized_json_exits_nonzero_with_oversized_envelope() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    let run_id = run_hello(root);
    fs::write(
        artifact_dir(root, &run_id).join("huge.txt"),
        vec![b'a'; READ_LIMIT as usize + 1],
    )
    .unwrap();

    let read = bb(root, &["artifacts", "read", &run_id, "huge.txt", "--json"]);
    assert!(!read.status.success());
    let env: serde_json::Value = serde_json::from_slice(&read.stdout).unwrap();
    assert_eq!(env["kind"], "oversized");
    assert_eq!(env["path"], "huge.txt");
    assert_eq!(env["limit"], READ_LIMIT);
}

#[test]
fn artifacts_list_does_not_mark_oversized_utf8_split_at_sniff_boundary_as_binary() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    let run_id = run_hello(root);
    let mut content = vec![b'a'; 8191];
    content.extend_from_slice("é".as_bytes());
    content.resize(READ_LIMIT as usize + 1, b'a');
    fs::write(artifact_dir(root, &run_id).join("huge-utf8.txt"), content).unwrap();

    let list = bb(root, &["artifacts", "list", &run_id, "--json"]);
    assert!(
        list.status.success(),
        "{}",
        String::from_utf8_lossy(&list.stderr)
    );
    let entries: serde_json::Value = serde_json::from_slice(&list.stdout).unwrap();
    let huge = entries
        .as_array()
        .unwrap()
        .iter()
        .find(|e| e["path"] == "huge-utf8.txt")
        .expect("huge UTF-8 artifact listed");
    assert_eq!(huge["content_type"], "text/plain");
    assert_eq!(huge["binary"], false);
}

#[test]
fn artifacts_read_rejects_path_traversal_at_cli_boundary() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    let run_id = run_hello(root);

    for bad in ["../escape", "/etc/passwd", ".."] {
        let out = bb(root, &["artifacts", "read", &run_id, bad]);
        assert!(!out.status.success(), "traversal {bad:?} succeeded");
        let err = String::from_utf8_lossy(&out.stderr);
        assert!(
            err.contains("must be a non-empty relative path"),
            "traversal {bad:?} not rejected: {err}"
        );
    }

    let json = bb(root, &["artifacts", "read", &run_id, "../escape", "--json"]);
    assert!(!json.status.success());
    let env: serde_json::Value = serde_json::from_slice(&json.stdout).unwrap();
    assert_eq!(env["kind"], "invalid_path");
    assert_eq!(env["path"], "../escape");
}

#[test]
fn artifacts_json_errors_cover_missing_run_and_stat_failure() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    let run_id = run_hello(root);

    let list = bb(root, &["artifacts", "list", "no-such-run", "--json"]);
    assert!(!list.status.success());
    let env: serde_json::Value = serde_json::from_slice(&list.stdout).unwrap();
    assert_eq!(env["kind"], "missing_run");
    assert_eq!(env["run_id"], "no-such-run");

    let read = bb(
        root,
        &["artifacts", "read", "no-such-run", "REPORT.json", "--json"],
    );
    assert!(!read.status.success());
    let env: serde_json::Value = serde_json::from_slice(&read.stdout).unwrap();
    assert_eq!(env["kind"], "missing_run");
    assert_eq!(env["path"], "REPORT.json");

    fs::write(
        artifact_dir(root, &run_id).join("not-dir"),
        "not a directory",
    )
    .unwrap();
    let stat = bb(
        root,
        &["artifacts", "read", &run_id, "not-dir/child", "--json"],
    );
    assert!(!stat.status.success());
    let env: serde_json::Value = serde_json::from_slice(&stat.stdout).unwrap();
    assert_eq!(env["kind"], "io_error");
    assert_eq!(env["path"], "not-dir/child");
}
