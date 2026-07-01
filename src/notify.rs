use std::process::{Command, Stdio};

use anyhow::{bail, Result};
use serde::Serialize;

use crate::ledger::Ledger;
use crate::spec::Plane;

#[derive(Debug, Serialize)]
pub struct NotificationRetryReport {
    pub attempted: usize,
    pub delivered: usize,
    pub failed: usize,
    pub rows: Vec<NotificationRetryRow>,
}

#[derive(Debug, Serialize)]
pub struct NotificationRetryRow {
    pub id: i64,
    pub event: String,
    pub status: String,
    pub error: Option<String>,
}

/// Deliver a notification for `event` to the plane's notify webhook, if any.
/// Failures are recorded in the notification outbox plus durable
/// `notify_failed` guard events so they surface in `status` instead of dying
/// on stderr. The webhook POST is synchronous and bounded to 10s; a missing
/// webhook_url is a no-op.
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
    let failure = deliver(url, event, &body);
    record_delivery_result(ledger, outbox_id, event, failure);
}

pub fn retry_pending(
    plane: &Plane,
    ledger: &Ledger,
    limit: i64,
) -> Result<NotificationRetryReport> {
    let Some(url) = &plane.spec.notify.webhook_url else {
        bail!("notify webhook_url is not configured");
    };
    let rows = ledger.retryable_notifications(limit)?;
    let mut report = NotificationRetryReport {
        attempted: rows.len(),
        delivered: 0,
        failed: 0,
        rows: Vec::new(),
    };
    for row in rows {
        let failure = deliver(url, &row.event, &row.payload);
        let status = if failure.is_some() {
            report.failed += 1;
            "failed"
        } else {
            report.delivered += 1;
            "delivered"
        };
        record_delivery_result(ledger, Some(row.id), &row.event, failure.clone());
        report.rows.push(NotificationRetryRow {
            id: row.id,
            event: row.event,
            status: status.into(),
            error: failure,
        });
    }
    Ok(report)
}

fn deliver(url: &str, event: &str, body: &str) -> Option<String> {
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
    match spawned {
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
    }
}

fn record_delivery_result(
    ledger: &Ledger,
    outbox_id: Option<i64>,
    _event: &str,
    failure: Option<String>,
) {
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
