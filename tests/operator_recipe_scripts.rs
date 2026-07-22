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
elif cmd == ["dlq", "list", "--json"]:
    if os.environ.get("BB_FAKE_BAD_DLQ"):
        print(json.dumps({"not": "a list"}))
    elif os.environ.get("BB_FAKE_OPEN_DLQ"):
        print(json.dumps([{"id":29,"task":"powder-chew","status":"open","error":"missing POWDER_API_BASE_URL"}]))
    elif os.environ.get("BB_FAKE_HISTORY_DLQ"):
        print(json.dumps([{"id":27,"task":"old","status":"replayed"},{"id":28,"task":"old","status":"acknowledged","acknowledged_at":"2026-07-20T00:00:00Z"}]))
    else:
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

#[cfg(unix)]
fn set_mode(path: &std::path::Path, mode: u32) {
    use std::os::unix::fs::PermissionsExt;
    fs::set_permissions(path, fs::Permissions::from_mode(mode)).unwrap();
}

#[cfg(unix)]
fn mode(path: &std::path::Path) -> u32 {
    use std::os::unix::fs::PermissionsExt;
    fs::metadata(path).unwrap().permissions().mode() & 0o777
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
    .env("BB_FAKE_HISTORY_DLQ", "1")
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
fn submit_storm_recipe_returns_operational_error_for_malformed_dlq() {
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

    let out = Command::new("python3")
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
        .env("BB_FAKE_BAD_DLQ", "1")
        .output()
        .unwrap();

    assert_eq!(out.status.code(), Some(2));
    assert!(String::from_utf8_lossy(&out.stderr).contains("malformed DLQ"));
    let log = fs::read_to_string(dir.path().join("bb.log")).unwrap();
    assert!(log.contains("dlq"));
    assert!(!log.contains("submit"));
}

#[test]
fn submit_storm_recipe_blocks_open_dlq_before_submission_mutation() {
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

    let out = Command::new("python3")
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
        .env("BB_FAKE_OPEN_DLQ", "1")
        .output()
        .unwrap();

    assert_eq!(out.status.code(), Some(3));
    let stderr = String::from_utf8_lossy(&out.stderr);
    assert!(stderr.contains("status=open"), "stderr:\n{stderr}");
    assert!(stderr.contains("#29"), "stderr:\n{stderr}");
    let log = fs::read_to_string(dir.path().join("bb.log")).unwrap();
    assert!(log.contains("dlq"), "readiness must inspect DLQ:\n{log}");
    assert!(
        !log.contains("submit"),
        "open DLQ must block submit:\n{log}"
    );
    assert!(
        !log.contains("\"run\""),
        "open DLQ must block member fanout:\n{log}"
    );
    assert!(
        !log.contains("gate"),
        "open DLQ must block gate mutation:\n{log}"
    );
    assert!(
        !log.contains("ack") && !log.contains("replay"),
        "readiness must not mutate DLQ:\n{log}"
    );
}

#[test]
fn submit_storm_recipe_allows_replayed_and_acknowledged_history() {
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

    let out = recipe(
        &[
            "--config",
            "plane",
            "--bb",
            fake.to_str().unwrap(),
            "--payload-file",
            payload.to_str().unwrap(),
            "--member",
            "verify",
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
    let log = fs::read_to_string(dir.path().join("bb.log")).unwrap();
    assert!(
        log.contains("submit"),
        "resolved history must not block submit: {log}"
    );
    assert!(
        log.contains("\"run\""),
        "resolved history must allow member dispatch: {log}"
    );
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
        r#"{"repo":"misty-step/bitterblossom","change":"change-x","rev":"abc123","backlog":"bitterblossom-086","base_ref":"origin/master"}"#,
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

#[test]
fn local_primary_installer_stages_release_atomically_and_requires_explicit_legacy_cleanup() {
    let dir = tempfile::tempdir().unwrap();
    let repo = dir.path().join("repo&xml");
    let home = dir.path().join("home");
    let fake_bin = dir.path().join("fake-bin");
    fs::create_dir_all(repo.join("scripts")).unwrap();
    fs::create_dir_all(repo.join("deploy/launchd")).unwrap();
    fs::create_dir_all(repo.join("plane")).unwrap();
    fs::create_dir_all(&home).unwrap();
    fs::create_dir_all(&fake_bin).unwrap();
    let root = std::path::Path::new(env!("CARGO_MANIFEST_DIR"));
    for rel in [
        "scripts/install-bb-local-primary.sh",
        "deploy/launchd/com.misty-step.bb-serve.plist.template",
        "deploy/launchd/com.misty-step.bb-plane-litestream.plist.template",
    ] {
        let destination = repo.join(rel);
        fs::copy(root.join(rel), destination).unwrap();
    }
    let release = repo.join("target/release/bb");
    fs::create_dir_all(release.parent().unwrap()).unwrap();
    fs::write(&release, "release-v1").unwrap();
    make_executable(&release);
    fs::write(
        repo.join("plane/plane.toml"),
        "dev = false\nallow_local_substrate = true\n[ingress]\nbind = \"127.0.0.1:7093\"\n",
    )
    .unwrap();
    let env_file = repo.join(".env.bb");
    fs::write(&env_file, "BB_API_TOKEN=sentinel\n").unwrap();
    set_mode(&env_file, 0o644);

    let launch_log = dir.path().join("launchctl.log");
    let plutil_log = dir.path().join("plutil.log");
    let launchctl = fake_bin.join("launchctl");
    fs::write(
        &launchctl,
        "#!/bin/sh\nif [ \"$1\" = \"print\" ]; then exit 1; fi\nprintf '%s\\n' \"$*\" >> \"$BB_LAUNCH_LOG\"\nexit 0\n",
    )
    .unwrap();
    make_executable(&launchctl);
    let uname = fake_bin.join("uname");
    fs::write(&uname, "#!/bin/sh\nprintf '%s\\n' Linux\n").unwrap();
    make_executable(&uname);
    let plutil = fake_bin.join("plutil");
    fs::write(
        &plutil,
        "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$BB_PLUTIL_LOG\"\nexit 127\n",
    )
    .unwrap();
    make_executable(&plutil);
    let path = format!("{}:{}", fake_bin.display(), std::env::var("PATH").unwrap());
    let install_dir = home.join(".local/libexec/bitterblossom");
    let installer = repo.join("scripts/install-bb-local-primary.sh");

    let run = |extra: &[&str]| {
        Command::new("sh")
            .arg(&installer)
            .args(extra)
            .env("HOME", &home)
            .env("BB_INSTALL_DIR", &install_dir)
            .env("BB_LOG_DIR", home.join(".local/state/bitterblossom"))
            .env("BB_LAUNCH_LOG", &launch_log)
            .env("BB_PLUTIL_LOG", &plutil_log)
            .env("PATH", &path)
            .output()
            .unwrap()
    };

    let first = run(&[]);
    assert!(
        first.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&first.stdout),
        String::from_utf8_lossy(&first.stderr)
    );
    assert_eq!(
        fs::read_to_string(install_dir.join("bb")).unwrap(),
        "release-v1"
    );
    assert!(!install_dir.join("bb.previous").exists());
    assert_eq!(mode(&env_file), 0o600);
    let rendered =
        fs::read_to_string(home.join("Library/LaunchAgents/com.misty-step.bb-serve.plist"))
            .unwrap();
    assert!(rendered.contains("BB_LOCAL_PRIMARY_BIN"));
    assert!(rendered.contains(&install_dir.join("bb").display().to_string()));
    assert!(rendered.contains("repo&amp;xml"));
    assert!(
        !plutil_log.exists(),
        "Linux validation must not invoke plutil: {}",
        fs::read_to_string(&plutil_log).unwrap_or_default()
    );

    let serve_template = repo.join("deploy/launchd/com.misty-step.bb-serve.plist.template");
    let valid_serve_template = fs::read(&serve_template).unwrap();
    fs::write(&serve_template, "<plist><dict>").unwrap();
    fs::remove_file(&launch_log).unwrap();
    let malformed = run(&[]);
    assert!(!malformed.status.success());
    assert!(String::from_utf8_lossy(&malformed.stderr).contains("invalid launchd plist"));
    assert!(
        !launch_log.exists() || fs::read_to_string(&launch_log).unwrap().is_empty(),
        "malformed plist must fail before launchctl mutation"
    );
    fs::write(&serve_template, valid_serve_template).unwrap();

    fs::write(&uname, "#!/bin/sh\nprintf '%s\\n' Darwin\n").unwrap();
    fs::write(
        &plutil,
        "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$BB_PLUTIL_LOG\"\nexit 0\n",
    )
    .unwrap();
    let macos_check = run(&[]);
    assert!(
        macos_check.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&macos_check.stdout),
        String::from_utf8_lossy(&macos_check.stderr)
    );
    let plutil_calls = fs::read_to_string(&plutil_log).unwrap();
    assert!(plutil_calls.contains("-lint"));
    assert!(plutil_calls.contains("com.misty-step.bb-serve.plist"));
    assert!(plutil_calls.contains("com.misty-step.bb-plane-litestream.plist"));

    let legacy = home.join("Library/LaunchAgents/com.misty-step.bb-dashboard.plist");
    fs::write(&legacy, "legacy").unwrap();
    let detected = run(&[]);
    assert!(detected.status.success());
    assert_eq!(
        fs::read_to_string(install_dir.join("bb.previous")).unwrap(),
        "release-v1"
    );
    assert!(
        legacy.exists(),
        "default install must not silently delete legacy label"
    );
    assert!(String::from_utf8_lossy(&detected.stderr).contains("--retire-legacy-dashboard"));

    fs::remove_file(&release).unwrap();
    fs::create_dir(&release).unwrap();
    let failed = run(&[]);
    assert!(
        !failed.status.success(),
        "copying a directory must fail before replacement"
    );
    assert_eq!(
        fs::read_to_string(install_dir.join("bb")).unwrap(),
        "release-v1"
    );
    fs::remove_dir(&release).unwrap();
    fs::write(&release, "release-v2").unwrap();
    make_executable(&release);

    let retired = run(&["--retire-legacy-dashboard"]);
    assert!(retired.status.success());
    assert!(
        !legacy.exists(),
        "explicit cleanup must remove retired plist"
    );
    let launch_calls = fs::read_to_string(&launch_log).unwrap();
    assert!(launch_calls.contains("bootout gui/"));
    assert!(launch_calls.contains("com.misty-step.bb-dashboard"));
}
