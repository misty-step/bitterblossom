use std::process::Command;
use std::time::{Duration, Instant};

fn shim() -> Command {
    let mut cmd = Command::new("sh");
    cmd.arg(format!(
        "{}/scripts/bb-register.sh",
        env!("CARGO_MANIFEST_DIR")
    ));
    cmd
}

#[test]
fn bb_register_shim_noops_when_plane_env_is_unset() {
    let out = shim()
        .arg("start")
        .env_remove("BB_URL")
        .env_remove("BB_API_TOKEN")
        .output()
        .unwrap();

    assert!(out.status.success());
    assert!(out.stdout.is_empty());
    assert!(out.stderr.is_empty());
}

#[test]
fn bb_register_shim_fire_and_forget_when_plane_is_unreachable() {
    let start = Instant::now();
    let out = shim()
        .arg("start")
        .env("BB_URL", "http://127.0.0.1:1")
        .env("BB_API_TOKEN", "test-token")
        .env("BB_REGISTER_AGENT", "codex-bb-909")
        .env("BB_REGISTER_ROLE", "implementer")
        .env("BB_REGISTER_REPO", "misty-step/bitterblossom")
        .env("BB_REGISTER_BRIEF_HASH", "sha256:test")
        .env("BB_REGISTER_STARTED_AT", "2026-07-04T12:00:00Z")
        .output()
        .unwrap();

    assert!(out.status.success());
    assert!(out.stdout.is_empty());
    assert!(out.stderr.is_empty());
    assert!(
        start.elapsed() < Duration::from_secs(3),
        "unreachable plane should be bounded by a short timeout"
    );
}
