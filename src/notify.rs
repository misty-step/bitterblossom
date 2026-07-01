use std::process::{Command, Stdio};

use crate::ledger::Ledger;
use crate::spec::Plane;
/// Deliver a notification for `event` to the plane's notify webhook, if any.
/// Failures are recorded as durable `notify_failed` guard events (backlog 083)
/// so they surface in `status` instead of dying on stderr. The webhook POST
/// is synchronous and bounded to 10s; a missing webhook_url is a no-op.
pub fn notify(plane: &Plane, ledger: &Ledger, event: &str, detail: &serde_json::Value) {
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
    let outbox_id = ledger.enqueue_notification(event, &body).ok();
    let bin = std::env::var("BB_NOTIFY_BIN").unwrap_or_else(|_| "curl".into());
    let spawned = Command::new(bin)
        .args([
            "-fsS",
            "-m10",
            "-XPOST",
            "-HContent-Type: application/json",
            "-d@-",
        ])
        .arg(url)
        .stdin(Stdio::piped())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn();
    let failure: Option<String> = match spawned {
        Ok(mut child) => {
            if let Some(mut stdin) = child.stdin.take() {
                let _ = std::io::Write::write_all(&mut stdin, body.as_bytes());
            }
            match child.wait() {
                Ok(status) if !status.success() => {
                    Some(format!("event={event} webhook_exit={status}"))
                }
                Err(e) => Some(format!("event={event} cannot_wait_curl={e}")),
                _ => None,
            }
        }
        Err(e) => Some(format!("event={event} cannot_spawn_curl={e}")),
    };
    if let Some(detail) = failure {
        if let Some(id) = outbox_id {
            let _ = ledger.mark_notification_failed(id, &detail);
        }
        eprintln!("notify: {detail}");
        let _ = ledger.record_guard_event("notify_failed", None, &detail, 1);
    } else if let Some(id) = outbox_id {
        let _ = ledger.mark_notification_delivered(id);
    }
}
