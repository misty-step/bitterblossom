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
    let socket_path = dir.path().join("litestream.sock");
    let secret = "replica-url-value-that-must-not-leak";

    write_executable(
        &bin_dir.join("bb"),
        r#"#!/bin/sh
set -eu
printf 'bb:%s\n' "$*" >>"$BB_TEST_LOG"
if [ "$1" = "status" ]; then
  mkdir -p "$(dirname "$BB_LITESTREAM_DB_PATH")"
  [ -f "$BB_LITESTREAM_DB_PATH" ] || : >"$BB_LITESTREAM_DB_PATH"
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
        .env("BB_LITESTREAM_SOCKET_PATH", &socket_path)
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
    assert!(config.contains("socket:"));
    assert!(config.contains(&format!("path: {}", socket_path.display())));
    assert!(config.contains("url: ${LITESTREAM_REPLICA_URL}"));
    assert!(!config.contains(secret));

    let log = fs::read_to_string(log_path).unwrap();
    assert!(log.contains("litestream:replicate -config"));
    assert!(log.contains(&format!(
        "litestream:sync -socket {} -wait -timeout 5",
        socket_path.display()
    )));
    assert!(log.contains("bb:serve"));
    assert!(!log.contains(secret));

    let heartbeat = fs::read_to_string(heartbeat_path).unwrap();
    assert!(heartbeat.trim_end().ends_with('Z'));
}

#[test]
fn litestream_entrypoint_restores_missing_db_before_initializing_ledger() {
    let dir = tempfile::tempdir().unwrap();
    let bin_dir = dir.path().join("bin");
    fs::create_dir(&bin_dir).unwrap();
    let log_path = dir.path().join("calls.log");
    let db_path = dir.path().join("plane/.bb/plane.db");
    let config_path = dir.path().join("litestream.yml");
    let heartbeat_path = dir.path().join("plane/.bb/backup-last-success");
    let socket_path = dir.path().join("litestream.sock");

    write_executable(
        &bin_dir.join("bb"),
        r#"#!/bin/sh
set -eu
printf 'bb:%s\n' "$*" >>"$BB_TEST_LOG"
if [ "$1" = "status" ]; then
  mkdir -p "$(dirname "$BB_LITESTREAM_DB_PATH")"
  [ -f "$BB_LITESTREAM_DB_PATH" ] || : >"$BB_LITESTREAM_DB_PATH"
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
  restore)
    mkdir -p "$(dirname "$BB_LITESTREAM_DB_PATH")"
    printf 'restored\n' >"$BB_LITESTREAM_DB_PATH"
    exit 0
    ;;
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
        .env("LITESTREAM_REPLICA_URL", "s3://example/plane.db")
        .env("BB_LITESTREAM_HEARTBEAT_PATH", &heartbeat_path)
        .env("BB_LITESTREAM_SOCKET_PATH", &socket_path)
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

    let log = fs::read_to_string(log_path).unwrap();
    let restore_index = log
        .find("litestream:restore -if-replica-exists")
        .expect("missing restore call");
    let status_index = log.find("bb:status").expect("missing status call");
    let serve_index = log.find("bb:serve").expect("missing serve call");
    assert!(restore_index < status_index, "{log}");
    assert!(restore_index < serve_index, "{log}");
    assert_eq!(fs::read_to_string(db_path).unwrap(), "restored\n");
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
        .env(
            "BB_LITESTREAM_SOCKET_PATH",
            dir.path().join("litestream.sock"),
        )
        .args(["bb", "serve"])
        .output()
        .unwrap();

    assert!(!output.status.success());
    let stderr = String::from_utf8_lossy(&output.stderr);
    assert!(stderr.contains("LITESTREAM_REPLICA_URL is required"));
    assert!(!stderr.contains("s3://"));
}

#[test]
fn entrypoint_materializes_tailnet_ssh_credentials_without_logging_them() {
    let dir = tempfile::tempdir().unwrap();
    let ssh_dir = dir.path().join("root-ssh");
    let private_key = "private-key-sentinel-line-1\nprivate-key-sentinel-line-2";
    let known_hosts = "10.108.0.4 ssh-ed25519 pinned-host-key-sentinel";

    let output = Command::new(repo_root().join("scripts/bb-litestream-entrypoint.sh"))
        .env_remove("LITESTREAM_REPLICA_URL")
        .env_remove("BB_LITESTREAM_REQUIRED")
        .env("BB_TAILNET_SSH_DIR", &ssh_dir)
        .env("BB_TAILNET_SSH_PRIVATE_KEY", private_key)
        .env("BB_TAILNET_SSH_KNOWN_HOSTS", known_hosts)
        .args([
            "sh",
            "-c",
            "test -z \"${BB_TAILNET_SSH_PRIVATE_KEY:-}\" && test -z \"${BB_TAILNET_SSH_KNOWN_HOSTS:-}\"",
        ])
        .output()
        .unwrap();

    assert!(
        output.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );
    for output in [&output.stdout, &output.stderr] {
        let output = String::from_utf8_lossy(output);
        assert!(!output.contains("private-key-sentinel"));
        assert!(!output.contains("pinned-host-key-sentinel"));
    }

    assert_eq!(
        fs::read_to_string(ssh_dir.join("id_ed25519")).unwrap(),
        format!("{private_key}\n")
    );
    assert_eq!(
        fs::read_to_string(ssh_dir.join("known_hosts")).unwrap(),
        format!("{known_hosts}\n")
    );
    assert_eq!(
        fs::metadata(&ssh_dir).unwrap().permissions().mode() & 0o777,
        0o700
    );
    for file in ["id_ed25519", "known_hosts"] {
        assert_eq!(
            fs::metadata(ssh_dir.join(file))
                .unwrap()
                .permissions()
                .mode()
                & 0o777,
            0o600,
            "wrong permissions for {file}"
        );
    }
}

#[test]
fn entrypoint_does_not_touch_ssh_directory_when_tailnet_credentials_are_unset() {
    let dir = tempfile::tempdir().unwrap();
    let ssh_dir = dir.path().join("root-ssh");
    let output = Command::new(repo_root().join("scripts/bb-litestream-entrypoint.sh"))
        .env_remove("LITESTREAM_REPLICA_URL")
        .env_remove("BB_LITESTREAM_REQUIRED")
        .env_remove("BB_TAILNET_SSH_PRIVATE_KEY")
        .env_remove("BB_TAILNET_SSH_KNOWN_HOSTS")
        .env("BB_TAILNET_SSH_DIR", &ssh_dir)
        .args(["sh", "-c", "true"])
        .output()
        .unwrap();

    assert!(
        output.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );
    assert!(!ssh_dir.exists());
}
