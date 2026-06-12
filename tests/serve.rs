use std::fs;
use std::process::{Child, Command, Output, Stdio};
use std::time::{Duration, Instant};

fn write_plane(root: &std::path::Path) {
    fs::write(
        root.join("plane.toml"),
        "dev = true\n[ingress]\nbind = \"127.0.0.1:0\"\n",
    )
    .unwrap();
}

fn wait_for_exit(mut child: Child, timeout: Duration) -> Output {
    let deadline = Instant::now() + timeout;
    loop {
        if child.try_wait().unwrap().is_some() {
            return child.wait_with_output().unwrap();
        }
        if Instant::now() >= deadline {
            child.kill().unwrap();
            let output = child.wait_with_output().unwrap();
            panic!("bb serve did not exit within {timeout:?}: {output:?}");
        }
        std::thread::sleep(Duration::from_millis(20));
    }
}

#[test]
fn public_bind_without_api_token_refuses_startup() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());

    let child = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", dir.path().to_str().unwrap(), "serve"])
        .env("BB_INGRESS_BIND", "0.0.0.0:0")
        .env_remove("BB_API_TOKEN")
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .unwrap();
    let output = wait_for_exit(child, Duration::from_secs(2));

    assert!(!output.status.success());
    let stderr = String::from_utf8_lossy(&output.stderr);
    assert!(stderr.contains("BB_API_TOKEN must be set"));
}

#[test]
fn public_bind_with_api_token_starts() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());

    let mut child = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", dir.path().to_str().unwrap(), "serve"])
        .env("BB_INGRESS_BIND", "0.0.0.0:0")
        .env("BB_API_TOKEN", "test-token")
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .unwrap();

    std::thread::sleep(Duration::from_millis(300));
    if let Some(status) = child.try_wait().unwrap() {
        panic!("bb serve exited early: {status}");
    }
    child.kill().unwrap();
    child.wait().unwrap();
}
