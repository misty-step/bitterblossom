use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::process::{Command, Output};

fn write_executable(path: &std::path::Path, content: &str) {
    fs::write(path, content).unwrap();
    fs::set_permissions(path, fs::Permissions::from_mode(0o755)).unwrap();
}

fn write_plane(root: &std::path::Path) {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
    let stub = root.join("slow.sh");
    write_executable(&stub, "#!/bin/sh\ncat > /dev/null\nsleep 1\necho done\n");
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
        "agent = \"stub\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
}

fn bb(root: &str, args: &[&str]) -> Output {
    let mut cmd = Command::new(env!("CARGO_BIN_EXE_bb"));
    cmd.arg("--config")
        .arg(root)
        .args(args)
        .env("BB_RUN_HEARTBEAT_MS", "100")
        .output()
        .unwrap()
}

#[test]
fn run_human_mode_prints_early_receipt_and_heartbeat_without_json_noise() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let root = dir.path().to_str().unwrap();

    let human = bb(root, &["run", "demo"]);
    assert!(
        human.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&human.stdout),
        String::from_utf8_lossy(&human.stderr)
    );
    let human_stdout = String::from_utf8_lossy(&human.stdout);
    let human_stderr = String::from_utf8_lossy(&human.stderr);
    assert!(human_stdout.contains("run "));
    assert!(human_stdout.contains(" success "));
    assert!(human_stderr.contains("accepted"));
    assert!(human_stderr.contains("heartbeat"));
    assert!(human_stderr.contains("state=running"));

    let json = bb(root, &["run", "demo", "--json"]);
    assert!(
        json.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&json.stdout),
        String::from_utf8_lossy(&json.stderr)
    );
    let doc: serde_json::Value = serde_json::from_slice(&json.stdout).unwrap();
    assert_eq!(doc["run"]["state"], "success");
    let json_stderr = String::from_utf8_lossy(&json.stderr);
    assert!(!json_stderr.contains("heartbeat"));
    assert!(!json_stderr.contains("accepted"));
}
