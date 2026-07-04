use std::io::{Read, Write};
use std::net::TcpListener;
use std::process::Command;
use std::thread;
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

#[test]
fn bb_register_shim_posts_start_payload_and_prints_registered_id() {
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let port = listener.local_addr().unwrap().port();
    listener.set_nonblocking(true).unwrap();
    let server = thread::spawn(move || {
        let deadline = Instant::now() + Duration::from_secs(3);
        let mut stream = loop {
            match listener.accept() {
                Ok((stream, _)) => break stream,
                Err(err) if err.kind() == std::io::ErrorKind::WouldBlock => {
                    assert!(
                        Instant::now() < deadline,
                        "shim did not POST to test server"
                    );
                    thread::sleep(Duration::from_millis(20));
                }
                Err(err) => panic!("accept failed: {err}"),
            }
        };
        let mut request = Vec::new();
        let mut buf = [0; 1024];
        loop {
            let n = stream.read(&mut buf).unwrap();
            if n == 0 {
                break;
            }
            request.extend_from_slice(&buf[..n]);
            if String::from_utf8_lossy(&request).contains("\"brief_hash\":\"sha256:test\"") {
                break;
            }
        }
        let request = String::from_utf8_lossy(&request);
        assert!(request.starts_with("POST /api/external-runs HTTP/1.1"));
        assert!(request.contains("Authorization: Bearer test-token"));
        assert!(request.contains("\"agent\":\"codex-bb-909\""));
        let body = r#"{"id":"external123","source":"external","status":"running"}"#;
        write!(
            stream,
            "HTTP/1.1 201 Created\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
            body.len(),
            body
        )
        .unwrap();
    });

    let out = shim()
        .arg("start")
        .env("BB_URL", format!("http://127.0.0.1:{port}"))
        .env("BB_API_TOKEN", "test-token")
        .env("BB_REGISTER_AGENT", "codex-bb-909")
        .env("BB_REGISTER_ROLE", "implementer")
        .env("BB_REGISTER_REPO", "misty-step/bitterblossom")
        .env("BB_REGISTER_BRIEF_HASH", "sha256:test")
        .env("BB_REGISTER_STARTED_AT", "2026-07-04T12:00:00Z")
        .output()
        .unwrap();

    server.join().unwrap();
    assert!(
        out.status.success(),
        "stderr:\n{}",
        String::from_utf8_lossy(&out.stderr)
    );
    assert_eq!(String::from_utf8_lossy(&out.stdout).trim(), "external123");
    assert!(out.stderr.is_empty());
}
