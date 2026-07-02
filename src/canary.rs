use std::io::Write;
use std::process::{Command, Stdio};
use std::time::Duration;

const CHECKIN_INTERVAL: Duration = Duration::from_secs(60);
const CHECKIN_TIMEOUT: &str = "10";

pub fn enabled() -> bool {
    endpoint().is_some() && ingest_key().is_some()
}

fn endpoint() -> Option<String> {
    std::env::var("CANARY_ENDPOINT")
        .ok()
        .filter(|v| !v.is_empty())
}

fn ingest_key() -> Option<String> {
    std::env::var("CANARY_INGEST_KEY")
        .ok()
        .filter(|v| !v.is_empty())
}

pub fn check_in() {
    let (Some(ep), Some(key)) = (endpoint(), ingest_key()) else {
        return;
    };
    let payload = serde_json::json!({
        "monitor": "bb-plane",
        "status": "alive",
        "summary": "BB plane heartbeat",
        "ttl_ms": 120_000,
    });
    let body = payload.to_string();
    let url = format!("{ep}/api/v1/check-ins");
    let bin = notify_bin();
    let spawned = Command::new(&bin)
        .args([
            "-fsS",
            "-m",
            CHECKIN_TIMEOUT,
            "-XPOST",
            "-H",
            &format!("Authorization: Bearer {key}"),
            "-H",
            "Content-Type: application/json",
            "-d@-",
        ])
        .arg(&url)
        .stdin(Stdio::piped())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn();
    if let Ok(mut child) = spawned {
        if let Some(mut stdin) = child.stdin.take() {
            let _ = stdin.write_all(body.as_bytes());
        }
        let _ = child.wait();
    }
}

pub fn report_error(class: &str, message: &str) {
    let (Some(ep), Some(key)) = (endpoint(), ingest_key()) else {
        return;
    };
    let payload = serde_json::json!({
        "service": "bb-plane",
        "error_class": class,
        "message": message,
        "severity": "error",
    });
    let body = payload.to_string();
    let url = format!("{ep}/api/v1/errors");
    let bin = notify_bin();
    let spawned = Command::new(&bin)
        .args([
            "-fsS",
            "-m",
            CHECKIN_TIMEOUT,
            "-XPOST",
            "-H",
            &format!("Authorization: Bearer {key}"),
            "-H",
            "Content-Type: application/json",
            "-d@-",
        ])
        .arg(&url)
        .stdin(Stdio::piped())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn();
    if let Ok(mut child) = spawned {
        if let Some(mut stdin) = child.stdin.take() {
            let _ = stdin.write_all(body.as_bytes());
        }
        let _ = child.wait();
    }
}

pub fn start_health_loop() {
    if !enabled() {
        return;
    }
    std::thread::Builder::new()
        .name("bb-canary-health".into())
        .spawn(move || loop {
            std::thread::sleep(CHECKIN_INTERVAL);
            check_in();
        })
        .ok();
}

fn notify_bin() -> String {
    std::env::var("BB_NOTIFY_BIN").unwrap_or_else(|_| "curl".into())
}
