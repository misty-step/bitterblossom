use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::{Path, PathBuf};
use std::process::Command;

fn repo_root() -> PathBuf {
    Path::new(env!("CARGO_MANIFEST_DIR")).to_path_buf()
}

fn write_executable(path: &Path, body: &str) {
    fs::write(path, body).unwrap();
    let mut permissions = fs::metadata(path).unwrap().permissions();
    permissions.set_mode(0o755);
    fs::set_permissions(path, permissions).unwrap();
}

#[test]
fn litestream_entrypoint_uses_env_name_and_writes_replication_heartbeat() {
    let dir = tempfile::tempdir().unwrap();
    let bin_dir = dir.path().join("bin");
    fs::create_dir(&bin_dir).unwrap();
    let log_path = dir.path().join("calls.log");
    let db_path = dir.path().join("plane/.bb/plane.db");
    let config_path = dir.path().join("litestream.yml");
    let heartbeat_path = dir.path().join("plane/.bb/backup-last-success");
    let secret = "replica-url-value-that-must-not-leak";

    write_executable(
        &bin_dir.join("bb"),
        r#"#!/bin/sh
set -eu
printf 'bb:%s\n' "$*" >>"$BB_TEST_LOG"
if [ "$1" = "status" ]; then
  mkdir -p "$(dirname "$BB_LITESTREAM_DB_PATH")"
  : >"$BB_LITESTREAM_DB_PATH"
  printf '{}\n'
  exit 0
fi
if [ "$1" = "serve" ]; then
  sleep 1
  exit 0
fi
exit 9
"#,
    );
    write_executable(
        &bin_dir.join("litestream"),
        r#"#!/bin/sh
set -eu
printf 'litestream:%s\n' "$*" >>"$BB_TEST_LOG"
case "$1" in
  replicate) sleep 30 ;;
  sync) exit 0 ;;
  *) exit 0 ;;
esac
"#,
    );

    let path = format!(
        "{}:{}",
        bin_dir.display(),
        std::env::var("PATH").unwrap_or_default()
    );
    let output = Command::new(repo_root().join("scripts/bb-litestream-entrypoint.sh"))
        .env("PATH", path)
        .env("BB_TEST_LOG", &log_path)
        .env("BB_TEST_ENTRYPOINT_ONCE", "1")
        .env("BB_PLANE_DIR", dir.path().join("plane"))
        .env("BB_LITESTREAM_REQUIRED", "1")
        .env("BB_LITESTREAM_DB_PATH", &db_path)
        .env("BB_LITESTREAM_CONFIG", &config_path)
        .env("BB_LITESTREAM_REPLICA_URL_ENV", "LITESTREAM_REPLICA_URL")
        .env("LITESTREAM_REPLICA_URL", secret)
        .env("BB_LITESTREAM_HEARTBEAT_PATH", &heartbeat_path)
        .env("BB_LITESTREAM_SYNC_INTERVAL_SECONDS", "1")
        .env("BB_LITESTREAM_SYNC_TIMEOUT_SECONDS", "5")
        .args(["bb", "serve"])
        .output()
        .unwrap();

    assert!(
        output.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );

    let stdout = String::from_utf8_lossy(&output.stdout);
    let stderr = String::from_utf8_lossy(&output.stderr);
    assert!(!stdout.contains(secret));
    assert!(!stderr.contains(secret));

    let config = fs::read_to_string(config_path).unwrap();
    assert!(config.contains("replica:"));
    assert!(config.contains("url: ${LITESTREAM_REPLICA_URL}"));
    assert!(!config.contains(secret));

    let log = fs::read_to_string(log_path).unwrap();
    assert!(log.contains("litestream:replicate -config"));
    assert!(log.contains("litestream:sync -wait -timeout 5"));
    assert!(log.contains("bb:serve"));
    assert!(!log.contains(secret));

    let heartbeat = fs::read_to_string(heartbeat_path).unwrap();
    assert!(heartbeat.trim_end().ends_with('Z'));
}

#[test]
fn litestream_entrypoint_fails_closed_when_required_secret_is_missing() {
    let dir = tempfile::tempdir().unwrap();
    let output = Command::new(repo_root().join("scripts/bb-litestream-entrypoint.sh"))
        .env("BB_LITESTREAM_REQUIRED", "1")
        .env(
            "BB_LITESTREAM_DB_PATH",
            dir.path().join("plane/.bb/plane.db"),
        )
        .env("BB_LITESTREAM_CONFIG", dir.path().join("litestream.yml"))
        .env("BB_LITESTREAM_REPLICA_URL_ENV", "LITESTREAM_REPLICA_URL")
        .env("BB_LITESTREAM_HEARTBEAT_PATH", dir.path().join("heartbeat"))
        .args(["bb", "serve"])
        .output()
        .unwrap();

    assert!(!output.status.success());
    let stderr = String::from_utf8_lossy(&output.stderr);
    assert!(stderr.contains("LITESTREAM_REPLICA_URL is required"));
    assert!(!stderr.contains("s3://"));
}
