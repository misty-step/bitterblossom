use std::fs;
use std::io::{Read, Write};
use std::net::{TcpListener, TcpStream};
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

fn http_get(port: u16, path: &str, bearer: Option<&str>) -> (u16, String) {
    let mut stream = TcpStream::connect(("127.0.0.1", port)).unwrap();
    let auth = bearer
        .map(|t| format!("Authorization: Bearer {t}\r\n"))
        .unwrap_or_default();
    write!(
        stream,
        "GET {path} HTTP/1.1\r\nHost: 127.0.0.1\r\n{auth}Connection: close\r\n\r\n"
    )
    .unwrap();
    let mut response = String::new();
    stream.read_to_string(&mut response).unwrap();
    let status = response
        .lines()
        .next()
        .unwrap()
        .split_whitespace()
        .nth(1)
        .unwrap()
        .parse()
        .unwrap();
    (status, response)
}

struct ChildGuard(Child);

impl Drop for ChildGuard {
    fn drop(&mut self) {
        let _ = self.0.kill();
        let _ = self.0.wait();
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

#[test]
fn read_api_requires_bearer_and_rejects_query_token() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let port = free_loopback_port();

    let child = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", dir.path().to_str().unwrap(), "serve"])
        .env("BB_INGRESS_BIND", format!("127.0.0.1:{port}"))
        .env("BB_API_TOKEN", "test-token")
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .unwrap();
    let _child = ChildGuard(child);
    wait_for_http(port);

    assert_eq!(http_get(port, "/api/runs", None).0, 401);
    assert_eq!(http_get(port, "/api/runs", Some("wrong")).0, 401);
    assert_eq!(http_get(port, "/api/runs?token=test-token", None).0, 401);
    assert_eq!(
        http_get(port, "/api/gate?notsubmission=x", Some("test-token")).0,
        400
    );
    let (status, body) = http_get(port, "/api/runs", Some("test-token"));
    assert_eq!(status, 200, "{body}");
    let (status, body) = http_get(port, "/", Some("test-token"));
    assert_eq!(status, 200, "{body}");
}
