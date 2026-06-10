//! Notification webhook: state transitions only — dead letters, budget
//! breaches, boot recovery. No heartbeats, no per-run noise. The operator
//! gets pinged instead of watching anything.

use std::io::Write as _;
use std::process::{Command, Stdio};

use crate::spec::Plane;

/// Fire-and-forget POST of `{event, ...detail}` to the configured webhook.
/// Failures are logged, never fatal: notification is best-effort by design.
pub fn notify(plane: &Plane, event: &str, detail: &serde_json::Value) {
    let Some(url) = &plane.spec.notify.webhook_url else {
        return;
    };
    let mut payload = serde_json::json!({ "event": event });
    if let (Some(obj), Some(extra)) = (payload.as_object_mut(), detail.as_object()) {
        for (k, v) in extra {
            obj.insert(k.clone(), v.clone());
        }
    }
    let body = payload.to_string();

    // curl keeps the plane free of an HTTP-client dependency; this is a
    // best-effort one-shot POST, not a transport abstraction. Tests
    // override the transport binary via BB_NOTIFY_BIN.
    let bin = std::env::var("BB_NOTIFY_BIN").unwrap_or_else(|_| "curl".into());
    let spawned = Command::new(bin)
        .args([
            "-fsS",
            "-m",
            "10",
            "-X",
            "POST",
            "-H",
            "Content-Type: application/json",
            "-d",
            "@-",
        ])
        .arg(url)
        .stdin(Stdio::piped())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn();
    let event = event.to_string();
    match spawned {
        Ok(mut child) => {
            if let Some(mut stdin) = child.stdin.take() {
                let _ = stdin.write_all(body.as_bytes());
            }
            std::thread::spawn(move || {
                if let Ok(status) = child.wait() {
                    if !status.success() {
                        eprintln!("notify: webhook POST failed (exit {status}) event={event}");
                    }
                }
            });
        }
        Err(e) => eprintln!("notify: cannot spawn curl: {e}"),
    }
}
