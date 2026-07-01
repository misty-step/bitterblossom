use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::Path;
use std::process::Command;

use bitterblossom::ledger::Ledger;
use bitterblossom::spec::Plane;

const NOTIFY_STUB: &str = r#"#!/bin/sh
cat >> "$BB_NOTIFY_LOG"
echo >> "$BB_NOTIFY_LOG"
"#;

const FAIL_NOTIFY_STUB: &str = r#"#!/bin/sh
cat > /dev/null
exit 9
"#;

fn write_executable(path: &Path, content: &str) {
    fs::write(path, content).unwrap();
    fs::set_permissions(path, fs::Permissions::from_mode(0o755)).unwrap();
}

fn write_plane(root: &Path) -> Plane {
    fs::write(
        root.join("plane.toml"),
        "dev = true\n[notify]\nwebhook_url = \"http://example.invalid/hook\"\n",
    )
    .unwrap();
    Plane::load(root).unwrap()
}

fn bb(root: &Path, args: &[&str]) -> Command {
    let mut cmd = Command::new(env!("CARGO_BIN_EXE_bb"));
    cmd.args(["--config", root.to_str().unwrap()]).args(args);
    cmd
}

#[test]
fn notify_retry_delivers_pending_outbox_rows() {
    let dir = tempfile::tempdir().unwrap();
    let plane = write_plane(dir.path());
    let ledger = Ledger::open(&plane.db_path()).unwrap();
    ledger
        .enqueue_notification("probe", r#"{"event":"probe"}"#)
        .unwrap();
    let stub = dir.path().join("notify-stub.sh");
    let log = dir.path().join("notify.log");
    write_executable(&stub, NOTIFY_STUB);

    let out = bb(dir.path(), &["notify", "retry", "--json"])
        .env("BB_NOTIFY_BIN", &stub)
        .env("BB_NOTIFY_LOG", &log)
        .output()
        .unwrap();
    assert!(
        out.status.success(),
        "stderr: {}",
        String::from_utf8_lossy(&out.stderr)
    );
    let report: serde_json::Value = serde_json::from_slice(&out.stdout).unwrap();
    assert_eq!(report["attempted"], 1);
    assert_eq!(report["delivered"], 1);
    assert_eq!(report["failed"], 0);

    let list = bb(dir.path(), &["notify", "list", "--json"])
        .output()
        .unwrap();
    let rows: serde_json::Value = serde_json::from_slice(&list.stdout).unwrap();
    assert_eq!(rows[0]["status"], "delivered");
    assert_eq!(rows[0]["attempts"], 1);
    assert!(fs::read_to_string(log)
        .unwrap()
        .contains("\"event\":\"probe\""));
}

#[test]
fn notify_retry_retries_failed_rows_until_delivery() {
    let dir = tempfile::tempdir().unwrap();
    let plane = write_plane(dir.path());
    let ledger = Ledger::open(&plane.db_path()).unwrap();
    ledger
        .enqueue_notification("retry_probe", r#"{"event":"retry_probe"}"#)
        .unwrap();
    let fail = dir.path().join("fail-notify-stub.sh");
    let ok = dir.path().join("notify-stub.sh");
    let log = dir.path().join("notify.log");
    write_executable(&fail, FAIL_NOTIFY_STUB);
    write_executable(&ok, NOTIFY_STUB);

    let failed = bb(dir.path(), &["notify", "retry", "--json"])
        .env("BB_NOTIFY_BIN", &fail)
        .env("BB_NOTIFY_LOG", &log)
        .output()
        .unwrap();
    assert!(failed.status.success());
    let report: serde_json::Value = serde_json::from_slice(&failed.stdout).unwrap();
    assert_eq!(report["attempted"], 1);
    assert_eq!(report["failed"], 1);

    let delivered = bb(dir.path(), &["notify", "retry", "--json"])
        .env("BB_NOTIFY_BIN", &ok)
        .env("BB_NOTIFY_LOG", &log)
        .output()
        .unwrap();
    assert!(delivered.status.success());
    let report: serde_json::Value = serde_json::from_slice(&delivered.stdout).unwrap();
    assert_eq!(report["attempted"], 1);
    assert_eq!(report["delivered"], 1);

    let list = bb(dir.path(), &["notify", "list", "--json"])
        .output()
        .unwrap();
    let rows: serde_json::Value = serde_json::from_slice(&list.stdout).unwrap();
    assert_eq!(rows[0]["status"], "delivered");
    assert_eq!(rows[0]["attempts"], 2);
}

#[test]
fn notify_ack_closes_row_without_retrying_it() {
    let dir = tempfile::tempdir().unwrap();
    let plane = write_plane(dir.path());
    let ledger = Ledger::open(&plane.db_path()).unwrap();
    ledger
        .enqueue_notification("ack_probe", r#"{"event":"ack_probe"}"#)
        .unwrap();
    let stub = dir.path().join("notify-stub.sh");
    let log = dir.path().join("notify.log");
    write_executable(&stub, NOTIFY_STUB);

    let ack = bb(
        dir.path(),
        &[
            "notify",
            "ack",
            "1",
            "--reason",
            "handled out of band",
            "--json",
        ],
    )
    .output()
    .unwrap();
    assert!(
        ack.status.success(),
        "stderr: {}",
        String::from_utf8_lossy(&ack.stderr)
    );
    let row: serde_json::Value = serde_json::from_slice(&ack.stdout).unwrap();
    assert_eq!(row["status"], "acknowledged");
    assert_eq!(row["acknowledged_reason"], "handled out of band");

    let retry = bb(dir.path(), &["notify", "retry", "--json"])
        .env("BB_NOTIFY_BIN", &stub)
        .env("BB_NOTIFY_LOG", &log)
        .output()
        .unwrap();
    assert!(retry.status.success());
    let report: serde_json::Value = serde_json::from_slice(&retry.stdout).unwrap();
    assert_eq!(report["attempted"], 0);
    assert!(!log.exists(), "acknowledged row should not be delivered");
}
