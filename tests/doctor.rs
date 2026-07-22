use std::fs;
use std::net::{TcpListener, TcpStream};
use std::process::{Child, Command, Stdio};
use std::time::{Duration, Instant};

struct ChildGuard(Child);

impl Drop for ChildGuard {
    fn drop(&mut self) {
        let _ = self.0.kill();
        let _ = self.0.wait();
    }
}

fn write_local_plane(root: &std::path::Path, bind: &str) {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    fs::write(
        root.join("plane.toml"),
        format!("dev = true\n[ingress]\nbind = \"{bind}\"\n"),
    )
    .unwrap();
    let stub = root.join("stub.sh");
    fs::write(&stub, "#!/bin/sh\ncat >/dev/null\necho ok\n").unwrap();
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let mut perms = fs::metadata(&stub).unwrap().permissions();
        perms.set_mode(0o755);
        fs::set_permissions(&stub, perms).unwrap();
    }
    fs::write(
        root.join("agents/stub.toml"),
        format!(
            "harness = \"command\"\nmodel = \"\"\nbin = \"{}\"\n",
            stub.display()
        ),
    )
    .unwrap();
    fs::write(root.join("tasks/demo/card.md"), "demo\n").unwrap();
    fs::write(
        root.join("tasks/demo/task.toml"),
        "agent = \"stub\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
}

fn free_loopback_port() -> u16 {
    TcpListener::bind("127.0.0.1:0")
        .unwrap()
        .local_addr()
        .unwrap()
        .port()
}

fn wait_for_http(port: u16) {
    let deadline = Instant::now() + Duration::from_secs(5);
    while Instant::now() < deadline {
        if TcpStream::connect(("127.0.0.1", port)).is_ok() {
            return;
        }
        std::thread::sleep(Duration::from_millis(20));
    }
    panic!("server did not listen on port {port}");
}

/// The unit tests in src/doctor.rs cover config/database/preflight/dead-serve
/// logic without a real binary; this is the one case that needs the actual
/// compiled `bb` (only available via CARGO_BIN_EXE_bb in integration tests,
/// not src/ unit tests) driving a real `bb serve` process end to end.
#[test]
fn doctor_reports_ok_against_a_live_serve_process() {
    let dir = tempfile::tempdir().unwrap();
    let port = free_loopback_port();
    write_local_plane(dir.path(), &format!("127.0.0.1:{port}"));

    let child = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", dir.path().to_str().unwrap(), "serve"])
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .unwrap();
    let _child = ChildGuard(child);
    wait_for_http(port);

    let output = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args([
            "--config",
            dir.path().to_str().unwrap(),
            "doctor",
            "--expect-serve",
            "--json",
        ])
        .output()
        .unwrap();

    assert!(
        output.status.success(),
        "{:?}",
        String::from_utf8_lossy(&output.stderr)
    );
    let report: serde_json::Value = serde_json::from_slice(&output.stdout).unwrap();
    assert_eq!(report["ok"], true, "{report}");
    let checks = report["checks"].as_array().unwrap();
    assert!(checks
        .iter()
        .any(|c| c["name"] == "serve_api" && c["status"] == "ok"));
    assert!(checks
        .iter()
        .any(|c| c["name"] == "dashboard" && c["status"] == "ok"));
}

#[test]
fn doctor_reports_ok_against_effective_bind_override() {
    let dir = tempfile::tempdir().unwrap();
    let port = free_loopback_port();
    write_local_plane(dir.path(), "127.0.0.0:1");
    let bind = format!("127.0.0.1:{port}");

    let child = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", dir.path().to_str().unwrap(), "serve"])
        .env("BB_INGRESS_BIND", &bind)
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .unwrap();
    let _child = ChildGuard(child);
    wait_for_http(port);

    let output = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args([
            "--config",
            dir.path().to_str().unwrap(),
            "doctor",
            "--expect-serve",
            "--json",
        ])
        .env("BB_INGRESS_BIND", &bind)
        .output()
        .unwrap();
    assert!(
        output.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );
    let report: serde_json::Value = serde_json::from_slice(&output.stdout).unwrap();
    assert_eq!(report["ok"], true, "{report}");
    assert!(report["checks"]
        .as_array()
        .unwrap()
        .iter()
        .any(|check| check["name"] == "serve_api" && check["status"] == "ok"));
}

#[test]
fn doctor_cli_exits_non_zero_and_prints_remediation_on_invalid_config() {
    let dir = tempfile::tempdir().unwrap();
    fs::write(dir.path().join("plane.toml"), "not = [valid").unwrap();

    let mut output = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", dir.path().to_str().unwrap(), "doctor"])
        .output()
        .unwrap();
    output.stdout.append(&mut output.stderr);
    let text = String::from_utf8_lossy(&output.stdout);

    assert!(!output.status.success());
    assert!(text.contains("config"), "{text}");
    assert!(text.contains("remediation"), "{text}");
}
