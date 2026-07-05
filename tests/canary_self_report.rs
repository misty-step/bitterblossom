use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::sync::Mutex;
use std::time::{Duration, Instant};

static ENV_LOCK: Mutex<()> = Mutex::new(());

fn unset_canary_env() {
    std::env::remove_var("CANARY_ENDPOINT");
    std::env::remove_var("CANARY_INGEST_KEY");
    std::env::remove_var("BB_NOTIFY_BIN");
}

fn make_executable(path: &std::path::Path) {
    let mut permissions = fs::metadata(path).unwrap().permissions();
    permissions.set_mode(0o755);
    fs::set_permissions(path, permissions).unwrap();
}

#[test]
fn check_in_noops_when_env_is_unset() {
    let _guard = ENV_LOCK.lock().unwrap();
    let dir = tempfile::tempdir().unwrap();
    let called = dir.path().join("called");
    let stub = dir.path().join("curl-stub.sh");
    fs::write(
        &stub,
        format!("#!/bin/sh\ntouch '{}'\nexit 0\n", called.display()),
    )
    .unwrap();
    make_executable(&stub);

    unset_canary_env();
    std::env::set_var("BB_NOTIFY_BIN", &stub);
    bitterblossom::canary::check_in();

    assert!(!called.exists());
    unset_canary_env();
}

#[test]
fn check_in_sends_secret_and_body_through_curl_config_stdin() {
    let _guard = ENV_LOCK.lock().unwrap();
    let dir = tempfile::tempdir().unwrap();
    let args_log = dir.path().join("args");
    let stdin_log = dir.path().join("stdin");
    let stub = dir.path().join("curl-stub.sh");
    fs::write(
        &stub,
        format!(
            "#!/bin/sh\nprintf '%s\\n' \"$*\" > '{}'\ncat > '{}'\nexit 0\n",
            args_log.display(),
            stdin_log.display()
        ),
    )
    .unwrap();
    make_executable(&stub);

    unset_canary_env();
    std::env::set_var("CANARY_ENDPOINT", "https://canary.example.test/");
    std::env::set_var("CANARY_INGEST_KEY", "secret-ingest-key");
    std::env::set_var("BB_NOTIFY_BIN", &stub);
    bitterblossom::canary::check_in();

    let args = fs::read_to_string(args_log).unwrap();
    let stdin = fs::read_to_string(stdin_log).unwrap();
    assert_eq!(args.trim(), "--config -");
    assert!(!args.contains("secret-ingest-key"));
    assert!(stdin.contains("url = \"https://canary.example.test/api/v1/check-ins\""));
    assert!(stdin.contains("header = \"Authorization: Bearer secret-ingest-key\""));
    assert!(stdin.contains("\\\"monitor\\\":\\\"bb-plane\\\""));
    unset_canary_env();
}

#[test]
fn check_in_fleet_heartbeat_targets_the_fleet_monitor() {
    let _guard = ENV_LOCK.lock().unwrap();
    let dir = tempfile::tempdir().unwrap();
    let stdin_log = dir.path().join("stdin");
    let stub = dir.path().join("curl-stub.sh");
    fs::write(
        &stub,
        format!("#!/bin/sh\ncat > '{}'\nexit 0\n", stdin_log.display()),
    )
    .unwrap();
    make_executable(&stub);

    unset_canary_env();
    std::env::set_var("CANARY_ENDPOINT", "https://canary.example.test/");
    std::env::set_var("CANARY_INGEST_KEY", "secret-ingest-key");
    std::env::set_var("BB_NOTIFY_BIN", &stub);
    bitterblossom::canary::check_in_fleet_heartbeat();

    let stdin = fs::read_to_string(stdin_log).unwrap();
    assert!(stdin.contains("\\\"monitor\\\":\\\"bitterblossom-plane-fleet-heartbeat\\\""));
    unset_canary_env();
}

#[test]
fn delivery_failure_returns_after_bounded_curl_exit() {
    let _guard = ENV_LOCK.lock().unwrap();
    unset_canary_env();
    std::env::set_var("CANARY_ENDPOINT", "http://127.0.0.1:1");
    std::env::set_var("CANARY_INGEST_KEY", "test-key");

    let started = Instant::now();
    bitterblossom::canary::check_in();

    assert!(started.elapsed() < Duration::from_secs(12));
    unset_canary_env();
}
