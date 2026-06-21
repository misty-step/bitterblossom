use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::process::Command;

fn write_executable(path: &std::path::Path, content: &str) {
    fs::write(path, content).unwrap();
    fs::set_permissions(path, fs::Permissions::from_mode(0o755)).unwrap();
}

fn bb(root: &str, args: &[&str]) -> std::process::Output {
    Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", root])
        .args(args)
        .output()
        .unwrap()
}

fn mkdirs(root: &std::path::Path, tasks: &[&str]) {
    fs::create_dir_all(root.join("agents")).unwrap();
    for t in tasks {
        fs::create_dir_all(root.join("tasks").join(t)).unwrap();
        fs::write(root.join("tasks").join(t).join("card.md"), "card\n").unwrap();
    }
}

#[test]
fn preflight_reports_missing_secret() {
    let dir = tempfile::tempdir().unwrap();
    let root_path = dir.path();
    mkdirs(root_path, &["demo"]);
    fs::write(root_path.join("plane.toml"), "dev = true\n").unwrap();
    let stub = root_path.join("stub.sh");
    write_executable(&stub, "#!/bin/sh\ncat > /dev/null\n");
    fs::write(
        root_path.join("agents/stub.toml"),
        format!(
            "version = 1\nharness = \"command\"\nmodel = \"\"\nbin = \"{}\"\nsecrets = [\"BB_PREFLIGHT_TEST_MISSING_TOKEN\"]\n",
            stub.display()
        ),
    )
    .unwrap();
    fs::write(
        root_path.join("tasks/demo/task.toml"),
        "agent = \"stub\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();

    let out = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args([
            "--config",
            root_path.to_str().unwrap(),
            "preflight",
            "demo",
            "--json",
        ])
        .env_remove("BB_PREFLIGHT_TEST_MISSING_TOKEN")
        .output()
        .unwrap();
    assert!(
        !out.status.success(),
        "preflight with findings must exit non-zero"
    );
    let doc: serde_json::Value = serde_json::from_slice(&out.stdout).unwrap();
    assert!(doc["tasks_checked"]
        .as_array()
        .unwrap()
        .contains(&serde_json::json!("demo")));
    let finding = doc["findings"]
        .as_array()
        .unwrap()
        .iter()
        .find(|f| f["kind"] == "missing_secret")
        .expect("missing_secret finding");
    assert!(finding["detail"]
        .as_str()
        .unwrap()
        .contains("BB_PREFLIGHT_TEST_MISSING_TOKEN"));
}

#[test]
fn preflight_reports_unspawnable_command_binary() {
    let dir = tempfile::tempdir().unwrap();
    let root_path = dir.path();
    mkdirs(root_path, &["demo"]);
    fs::write(root_path.join("plane.toml"), "dev = true\n").unwrap();
    fs::write(
        root_path.join("agents/stub.toml"),
        "version = 1\nharness = \"command\"\nmodel = \"\"\nbin = \"/no/such/binary/here\"\n",
    )
    .unwrap();
    fs::write(
        root_path.join("tasks/demo/task.toml"),
        "agent = \"stub\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();

    let out = bb(
        root_path.to_str().unwrap(),
        &["preflight", "demo", "--json"],
    );
    assert!(!out.status.success());
    let doc: serde_json::Value = serde_json::from_slice(&out.stdout).unwrap();
    let finding = doc["findings"]
        .as_array()
        .unwrap()
        .iter()
        .find(|f| f["kind"] == "unspawnable_binary")
        .expect("unspawnable_binary finding");
    assert!(finding["detail"]
        .as_str()
        .unwrap()
        .contains("/no/such/binary/here"));
}

#[test]
fn preflight_ok_when_secrets_and_binary_present() {
    let dir = tempfile::tempdir().unwrap();
    let root_path = dir.path();
    mkdirs(root_path, &["demo"]);
    fs::write(root_path.join("plane.toml"), "dev = true\n").unwrap();
    let stub = root_path.join("stub.sh");
    write_executable(&stub, "#!/bin/sh\ncat > /dev/null\n");
    fs::write(
        root_path.join("agents/stub.toml"),
        format!(
            "version = 1\nharness = \"command\"\nmodel = \"\"\nbin = \"{}\"\n",
            stub.display()
        ),
    )
    .unwrap();
    fs::write(
        root_path.join("tasks/demo/task.toml"),
        "agent = \"stub\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();

    let out = bb(
        root_path.to_str().unwrap(),
        &["preflight", "demo", "--json"],
    );
    assert!(
        out.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&out.stdout),
        String::from_utf8_lossy(&out.stderr)
    );
    let doc: serde_json::Value = serde_json::from_slice(&out.stdout).unwrap();
    assert!(doc["findings"].as_array().unwrap().is_empty());
    assert!(doc["tasks_checked"]
        .as_array()
        .unwrap()
        .contains(&serde_json::json!("demo")));
}

#[test]
fn preflight_storm_covers_gate_required_members() {
    let dir = tempfile::tempdir().unwrap();
    let root_path = dir.path();
    mkdirs(root_path, &["verify"]);
    fs::write(
        root_path.join("plane.toml"),
        "dev = true\n[gate]\nrequired = [\"verify\"]\n",
    )
    .unwrap();
    // verify member declares verdict = "verify" but is missing a secret.
    fs::write(
        root_path.join("agents/stub.toml"),
        "version = 1\nharness = \"command\"\nmodel = \"\"\nbin = \"true\"\nsecrets = [\"BB_PREFLIGHT_STORM_MISSING\"]\n",
    )
    .unwrap();
    fs::write(
        root_path.join("tasks/verify/task.toml"),
        "agent = \"stub\"\nsubstrate = \"local\"\nverdict = \"verify\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();

    let out = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args([
            "--config",
            root_path.to_str().unwrap(),
            "preflight",
            "--storm",
            "--json",
        ])
        .env_remove("BB_PREFLIGHT_STORM_MISSING")
        .output()
        .unwrap();
    assert!(!out.status.success());
    let doc: serde_json::Value = serde_json::from_slice(&out.stdout).unwrap();
    assert!(doc["tasks_checked"]
        .as_array()
        .unwrap()
        .contains(&serde_json::json!("verify")));
    assert!(doc["findings"]
        .as_array()
        .unwrap()
        .iter()
        .any(|f| f["kind"] == "missing_secret" && f["task"] == "verify"));
}

#[test]
fn preflight_needs_a_target() {
    let dir = tempfile::tempdir().unwrap();
    let root_path = dir.path();
    mkdirs(root_path, &[]);
    fs::write(root_path.join("plane.toml"), "dev = true\n").unwrap();
    let out = bb(root_path.to_str().unwrap(), &["preflight", "--json"]);
    assert!(!out.status.success());
    let err = String::from_utf8_lossy(&out.stderr);
    assert!(
        err.contains("task") || err.contains("--storm"),
        "got: {err}"
    );
}
