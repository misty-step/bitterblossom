use std::fs;
use std::process::{Command, Output};

fn write_fake_bb(path: &std::path::Path) {
    fs::write(
        path,
        r#"#!/usr/bin/env python3
import json, os, pathlib, sys

log = pathlib.Path(os.environ["BB_FAKE_LOG"])
payload_log = pathlib.Path(os.environ["BB_FAKE_PAYLOAD_LOG"])
args = sys.argv[1:]
with log.open("a") as handle:
    handle.write(json.dumps(args) + "\n")

if "--payload-file" in args:
    p = pathlib.Path(args[args.index("--payload-file") + 1])
    with payload_log.open("a") as handle:
        handle.write(p.read_text())

try:
    idx = args.index("--config")
except ValueError:
    print("missing --config", file=sys.stderr)
    sys.exit(9)
cmd = args[idx + 2:]

if cmd == ["preflight", "--storm", "--json"]:
    if os.environ.get("BB_FAKE_PREFLIGHT_FAIL"):
        print(json.dumps([{"task": "security", "missing_secret": "OPENROUTER_API_KEY"}]))
        sys.exit(2)
    print("[]")
elif cmd == ["preflight", "build", "--json"]:
    if os.environ.get("BB_FAKE_PREFLIGHT_FAIL"):
        print(json.dumps([{"task": "build", "missing_secret": "OPENROUTER_API_KEY"}]))
        sys.exit(2)
    print("[]")
elif cmd == ["runs", "list", "--json"]:
    if os.environ.get("BB_FAKE_ACTIVE_BUILD"):
        print(json.dumps([{"id":"run-active","task":"build","idempotency_key":os.environ["BB_FAKE_ACTIVE_BUILD"],"state":"running"}]))
    else:
        print("[]")
elif cmd == ["submit", "list", "--limit", "200", "--json"]:
    if os.environ.get("BB_FAKE_OPEN_SUBMISSION"):
        print(json.dumps([{"submission":{"id":"open123","change_key":"change-x","rev":"oldrev","state":"open"},"verdicts":[],"rejections":[]}]))
    else:
        print("[]")
elif cmd[:2] == ["submit", "open"]:
    print(json.dumps({"id":"sub123","change_key":cmd[cmd.index("--change") + 1],"rev":cmd[cmd.index("--rev") + 1],"state":"open"}))
elif cmd and cmd[0] == "run":
    task = cmd[1]
    if os.environ.get("BB_FAKE_FAIL_TASK") == task:
        print(json.dumps({"run":{"id":"run-" + task,"state":"failure"}}))
        print("forced member failure", file=sys.stderr)
        sys.exit(2)
    print(json.dumps({"run":{"id":"run-" + task,"state":"success"},"attempts":[],"events":[]}))
elif cmd == ["gate", "--submission", "sub123", "--json"]:
    print(json.dumps({"submission":"sub123","decision":"clear"}))
else:
    print("unexpected command: " + json.dumps(cmd), file=sys.stderr)
    sys.exit(8)
"#,
    )
    .unwrap();
}

#[cfg(unix)]
fn make_executable(path: &std::path::Path) {
    use std::os::unix::fs::PermissionsExt;
    fs::set_permissions(path, fs::Permissions::from_mode(0o755)).unwrap();
}

fn recipe(args: &[&str], dir: &std::path::Path) -> Output {
    let mut cmd = Command::new("python3");
    cmd.arg(format!(
        "{}/scripts/bb-submit-storm",
        env!("CARGO_MANIFEST_DIR")
    ))
    .args(args)
    .env("BB_FAKE_LOG", dir.join("bb.log"))
    .env("BB_FAKE_PAYLOAD_LOG", dir.join("payloads.log"))
    .output()
    .unwrap()
}

fn build_recipe(args: &[&str], dir: &std::path::Path) -> Output {
    let mut cmd = Command::new("python3");
    cmd.arg(format!(
        "{}/scripts/bb-dispatch-build",
        env!("CARGO_MANIFEST_DIR")
    ))
    .args(args)
    .env("BB_FAKE_LOG", dir.join("bb.log"))
    .env("BB_FAKE_PAYLOAD_LOG", dir.join("payloads.log"))
    .output()
    .unwrap()
}

#[test]
fn dispatch_build_recipe_validates_required_fields_before_bb_calls() {
    let dir = tempfile::tempdir().unwrap();
    let payload = dir.path().join("payload.json");
    fs::write(
        &payload,
        r#"{"repo":"misty-step/bitterblossom","prompt":"do it"}"#,
    )
    .unwrap();

    let out = build_recipe(
        &[
            "--config",
            "plane",
            "--bb",
            "does-not-exist-bb",
            "--payload-file",
            payload.to_str().unwrap(),
            "--json",
        ],
        dir.path(),
    );

    assert!(!out.status.success());
    assert!(
        String::from_utf8_lossy(&out.stderr)
            .contains("required field(s) 'backlog', 'base_ref', 'branch_slug'"),
        "stderr:\n{}",
        String::from_utf8_lossy(&out.stderr)
    );
    assert!(!dir.path().join("bb.log").exists());
}

#[test]
fn dispatch_build_recipe_stops_on_preflight_before_run_mutation() {
    let dir = tempfile::tempdir().unwrap();
    let fake = dir.path().join("fake-bb.py");
    write_fake_bb(&fake);
    make_executable(&fake);
    let payload = dir.path().join("payload.json");
    fs::write(
        &payload,
        r#"{"repo":"misty-step/bitterblossom","backlog":"bitterblossom-086","base_ref":"origin/master","branch_slug":"operator-recipes","prompt":"Ship the builder recipe."}"#,
    )
    .unwrap();

    let mut cmd = Command::new("python3");
    let out = cmd
        .arg(format!(
            "{}/scripts/bb-dispatch-build",
            env!("CARGO_MANIFEST_DIR")
        ))
        .args([
            "--config",
            "plane",
            "--bb",
            fake.to_str().unwrap(),
            "--payload-file",
            payload.to_str().unwrap(),
            "--json",
        ])
        .env("BB_FAKE_LOG", dir.path().join("bb.log"))
        .env("BB_FAKE_PAYLOAD_LOG", dir.path().join("payloads.log"))
        .env("BB_FAKE_PREFLIGHT_FAIL", "1")
        .output()
        .unwrap();

    assert_eq!(out.status.code(), Some(2));
    assert!(String::from_utf8_lossy(&out.stderr).contains("preflight failed before build dispatch"));
    let log = fs::read_to_string(dir.path().join("bb.log")).unwrap();
    assert!(log.contains("preflight"));
    assert!(!log.contains("\"run\""), "log:\n{log}");
}

#[test]
fn dispatch_build_recipe_refuses_duplicate_active_work_unless_forced() {
    let dir = tempfile::tempdir().unwrap();
    let fake = dir.path().join("fake-bb.py");
    write_fake_bb(&fake);
    make_executable(&fake);
    let payload = dir.path().join("payload.json");
    fs::write(
        &payload,
        r#"{"repo":"misty-step/bitterblossom","backlog":"bitterblossom-086","base_ref":"origin/master","branch_slug":"operator-recipes","prompt":"Ship the builder recipe."}"#,
    )
    .unwrap();
    let active_key = "build:operator-recipes:bac5551b074a6b1a";

    let mut blocked = Command::new("python3");
    let out = blocked
        .arg(format!(
            "{}/scripts/bb-dispatch-build",
            env!("CARGO_MANIFEST_DIR")
        ))
        .args([
            "--config",
            "plane",
            "--bb",
            fake.to_str().unwrap(),
            "--payload-file",
            payload.to_str().unwrap(),
            "--json",
        ])
        .env("BB_FAKE_LOG", dir.path().join("bb.log"))
        .env("BB_FAKE_PAYLOAD_LOG", dir.path().join("payloads.log"))
        .env("BB_FAKE_ACTIVE_BUILD", active_key)
        .output()
        .unwrap();
    assert_eq!(out.status.code(), Some(3));
    assert!(String::from_utf8_lossy(&out.stderr).contains("duplicate active build"));
    let log = fs::read_to_string(dir.path().join("bb.log")).unwrap();
    assert!(log.contains("runs"));
    assert!(!log.contains("\"run\""), "log:\n{log}");

    let mut forced = Command::new("python3");
    let out = forced
        .arg(format!(
            "{}/scripts/bb-dispatch-build",
            env!("CARGO_MANIFEST_DIR")
        ))
        .args([
            "--config",
            "plane",
            "--bb",
            fake.to_str().unwrap(),
            "--payload-file",
            payload.to_str().unwrap(),
            "--force",
            "--json",
        ])
        .env("BB_FAKE_LOG", dir.path().join("bb-force.log"))
        .env("BB_FAKE_PAYLOAD_LOG", dir.path().join("payloads-force.log"))
        .env("BB_FAKE_ACTIVE_BUILD", active_key)
        .output()
        .unwrap();
    assert!(
        out.status.success(),
        "stderr:\n{}",
        String::from_utf8_lossy(&out.stderr)
    );
    let receipt: serde_json::Value = serde_json::from_slice(&out.stdout).unwrap();
    assert_eq!(receipt["forced"], true);
    assert_eq!(receipt["duplicate_active_run"], "run-active");
}

#[test]
fn dispatch_build_recipe_runs_with_payload_file_and_returns_receipt() {
    let dir = tempfile::tempdir().unwrap();
    let fake = dir.path().join("fake-bb.py");
    write_fake_bb(&fake);
    make_executable(&fake);
    let payload = dir.path().join("payload.json");
    fs::write(
        &payload,
        r#"{"repo":"misty-step/bitterblossom","backlog":"bitterblossom-086","base_ref":"origin/master","branch_slug":"operator-recipes","prompt":"Ship the builder recipe.","model":"openrouter/test"}"#,
    )
    .unwrap();

    let out = build_recipe(
        &[
            "--config",
            "plane",
            "--bb",
            fake.to_str().unwrap(),
            "--payload-file",
            payload.to_str().unwrap(),
            "--json",
        ],
        dir.path(),
    );

    assert!(
        out.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&out.stdout),
        String::from_utf8_lossy(&out.stderr)
    );
    let receipt: serde_json::Value = serde_json::from_slice(&out.stdout).unwrap();
    assert_eq!(receipt["run"], "run-build");
    assert_eq!(receipt["task"], "build");
    assert_eq!(receipt["backlog"], "bitterblossom-086");
    assert_eq!(receipt["base_ref"], "origin/master");
    assert_eq!(receipt["branch_slug"], "operator-recipes");
    assert_eq!(
        receipt["idempotency_key"],
        "build:operator-recipes:bac5551b074a6b1a"
    );
    assert!(receipt["safe_next_command"]
        .as_str()
        .unwrap()
        .contains("logs -f run-build"));

    let log = fs::read_to_string(dir.path().join("bb.log")).unwrap();
    assert!(log.contains("--payload-file"));
    assert!(
        !log.contains("Ship the builder recipe."),
        "prompt must not travel via argv:\n{log}"
    );
    assert!(
        log.contains("\"--idempotency-key\", \"build:operator-recipes:bac5551b074a6b1a\""),
        "{log}"
    );

    let payloads = fs::read_to_string(dir.path().join("payloads.log")).unwrap();
    assert!(
        payloads.contains("\"schema_version\": \"bb.dispatch_job.v1\""),
        "{payloads}"
    );
    assert!(
        payloads.contains("\"prompt\": \"Ship the builder recipe.\""),
        "{payloads}"
    );
    assert!(
        payloads.contains("\"model\": \"openrouter/test\""),
        "{payloads}"
    );
}

#[test]
fn submit_storm_recipe_validates_required_fields_before_bb_calls() {
    let dir = tempfile::tempdir().unwrap();
    let payload = dir.path().join("payload.json");
    fs::write(&payload, r#"{"change":"change-x","rev":"abc123"}"#).unwrap();

    let out = recipe(
        &[
            "--config",
            "plane",
            "--bb",
            "does-not-exist-bb",
            "--payload-file",
            payload.to_str().unwrap(),
            "--json",
        ],
        dir.path(),
    );

    assert!(!out.status.success());
    assert!(
        String::from_utf8_lossy(&out.stderr).contains("required field(s) 'repo'"),
        "stderr:\n{}",
        String::from_utf8_lossy(&out.stderr)
    );
    assert!(!dir.path().join("bb.log").exists());
}

#[test]
fn submit_storm_recipe_stops_on_preflight_before_submission_mutation() {
    let dir = tempfile::tempdir().unwrap();
    let fake = dir.path().join("fake-bb.py");
    write_fake_bb(&fake);
    make_executable(&fake);
    let payload = dir.path().join("payload.json");
    fs::write(
        &payload,
        r#"{"repo":"misty-step/bitterblossom","change":"change-x","rev":"abc123"}"#,
    )
    .unwrap();

    let mut cmd = Command::new("python3");
    let out = cmd
        .arg(format!(
            "{}/scripts/bb-submit-storm",
            env!("CARGO_MANIFEST_DIR")
        ))
        .args([
            "--config",
            "plane",
            "--bb",
            fake.to_str().unwrap(),
            "--payload-file",
            payload.to_str().unwrap(),
            "--json",
        ])
        .env("BB_FAKE_LOG", dir.path().join("bb.log"))
        .env("BB_FAKE_PAYLOAD_LOG", dir.path().join("payloads.log"))
        .env("BB_FAKE_PREFLIGHT_FAIL", "1")
        .output()
        .unwrap();

    assert_eq!(out.status.code(), Some(2));
    assert!(String::from_utf8_lossy(&out.stderr).contains("failed before submission mutation"));
    let log = fs::read_to_string(dir.path().join("bb.log")).unwrap();
    assert!(log.contains("preflight"));
    assert!(!log.contains("submit",), "log:\n{log}");
}

#[test]
fn submit_storm_recipe_opens_dispatches_with_payload_files_and_returns_receipt() {
    let dir = tempfile::tempdir().unwrap();
    let fake = dir.path().join("fake-bb.py");
    write_fake_bb(&fake);
    make_executable(&fake);
    let payload = dir.path().join("payload.json");
    fs::write(
        &payload,
        r#"{"repo":"misty-step/bitterblossom","change":"change-x","rev":"abc123","backlog":"backlog.d/086-first-class-operator-dispatch-recipes.md","base_ref":"origin/master"}"#,
    )
    .unwrap();

    let out = recipe(
        &[
            "--config",
            "plane",
            "--bb",
            fake.to_str().unwrap(),
            "--payload-file",
            payload.to_str().unwrap(),
            "--require-field",
            "backlog",
            "--member",
            "verify",
            "--member",
            "correctness",
            "--json",
        ],
        dir.path(),
    );

    assert!(
        out.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&out.stdout),
        String::from_utf8_lossy(&out.stderr)
    );
    let receipt: serde_json::Value = serde_json::from_slice(&out.stdout).unwrap();
    assert_eq!(receipt["submission"], "sub123");
    assert_eq!(receipt["gate"]["decision"], "clear");
    assert_eq!(receipt["members"].as_array().unwrap().len(), 2);

    let log = fs::read_to_string(dir.path().join("bb.log")).unwrap();
    assert!(log.contains("submit"));
    assert!(log.contains("open"));
    assert!(log.contains("--payload-file"));
    assert!(
        !log.contains("--payload\""),
        "payload must not travel via argv:\n{log}"
    );

    let payloads = fs::read_to_string(dir.path().join("payloads.log")).unwrap();
    assert!(
        payloads.contains("\"submission\": \"sub123\""),
        "{payloads}"
    );
    assert!(
        payloads.contains("\"repo\": \"misty-step/bitterblossom\""),
        "{payloads}"
    );
}

#[test]
fn submit_storm_recipe_stops_member_fanout_on_runtime_failure() {
    let dir = tempfile::tempdir().unwrap();
    let fake = dir.path().join("fake-bb.py");
    write_fake_bb(&fake);
    make_executable(&fake);
    let payload = dir.path().join("payload.json");
    fs::write(
        &payload,
        r#"{"repo":"misty-step/bitterblossom","change":"change-x","rev":"abc123"}"#,
    )
    .unwrap();

    let mut cmd = Command::new("python3");
    let out = cmd
        .arg(format!(
            "{}/scripts/bb-submit-storm",
            env!("CARGO_MANIFEST_DIR")
        ))
        .args([
            "--config",
            "plane",
            "--bb",
            fake.to_str().unwrap(),
            "--payload-file",
            payload.to_str().unwrap(),
            "--member",
            "verify",
            "--member",
            "security",
            "--member",
            "correctness",
            "--json",
        ])
        .env("BB_FAKE_LOG", dir.path().join("bb.log"))
        .env("BB_FAKE_PAYLOAD_LOG", dir.path().join("payloads.log"))
        .env("BB_FAKE_FAIL_TASK", "security")
        .output()
        .unwrap();

    assert_eq!(out.status.code(), Some(2));
    assert!(String::from_utf8_lossy(&out.stderr).contains("storm member security failed"));
    let log = fs::read_to_string(dir.path().join("bb.log")).unwrap();
    assert!(log.contains("verify"), "{log}");
    assert!(log.contains("security"), "{log}");
    assert!(
        !log.contains("correctness"),
        "fanout should stop after failed member:\n{log}"
    );
    assert!(
        !log.contains("gate"),
        "gate should not run after failed member:\n{log}"
    );
}
