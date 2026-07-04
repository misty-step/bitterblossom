use std::process::{Command, Output, Stdio};

use anyhow::{bail, Result};
use serde::Serialize;

use crate::ledger::Ledger;
use crate::spec::Plane;

/// Bound on what a webhook response snippet persists (backlog 109): a large
/// or secret-bearing response body must never be stored in full.
const NOTIFICATION_RESPONSE_BYTES_CAP: usize = 2000;

/// Sentinel curl's `-w` write-out appends after the response body so the
/// actual HTTP status is recoverable even without `-f` (which would
/// otherwise swallow the body and status on a non-2xx response).
const STATUS_MARKER: &str = "\n__bb_notify_http_status__:";

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

/// One delivery attempt: a completed HTTP round trip carries the observed
/// status code and a bounded response snippet regardless of whether that
/// status was 2xx; `error` is set only when the attempt should count as a
/// failed/retryable delivery (curl itself failed, or the status was not 2xx).
struct DeliveryOutcome {
    status_code: Option<i64>,
    response: Option<String>,
    error: Option<String>,
}

impl DeliveryOutcome {
    fn delivered(status_code: Option<i64>, response: Option<String>) -> Self {
        Self {
            status_code,
            response,
            error: None,
        }
    }

    fn failed(error: String, status_code: Option<i64>, response: Option<String>) -> Self {
        Self {
            status_code,
            response,
            error: Some(error),
        }
    }

    fn is_failure(&self) -> bool {
        self.error.is_some()
    }
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
    let outcome = deliver(url, event, &body);
    record_delivery_result(ledger, outbox_id, outcome);
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
        let outcome = deliver(url, &row.event, &row.payload);
        let is_failure = outcome.is_failure();
        let error = outcome.error.clone();
        if is_failure {
            report.failed += 1;
        } else {
            report.delivered += 1;
        }
        record_delivery_result(ledger, Some(row.id), outcome);
        report.rows.push(NotificationRetryRow {
            id: row.id,
            event: row.event,
            status: if is_failure { "failed" } else { "delivered" }.into(),
            error,
        });
    }
    Ok(report)
}

fn deliver(url: &str, event: &str, body: &str) -> DeliveryOutcome {
    let bin = std::env::var("BB_NOTIFY_BIN").unwrap_or_else(|_| "curl".into());
    let write_out = format!("{STATUS_MARKER}%{{http_code}}");
    let spawned = Command::new(bin)
        .args([
            "-sS",
            "-m10",
            "-XPOST",
            "-HContent-Type: application/json",
            "-d@-",
            "-w",
            &write_out,
        ])
        .arg(url)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::null())
        .spawn();
    let mut child = match spawned {
        Ok(child) => child,
        Err(e) => {
            return DeliveryOutcome::failed(
                format!("event={event} cannot_spawn_curl={e}"),
                None,
                None,
            )
        }
    };
    if let Some(mut stdin) = child.stdin.take() {
        let _ = std::io::Write::write_all(&mut stdin, body.as_bytes());
    }
    match child.wait_with_output() {
        Ok(output) => parse_delivery_output(event, &output),
        Err(e) => {
            DeliveryOutcome::failed(format!("event={event} cannot_wait_curl={e}"), None, None)
        }
    }
}

/// Split curl's stdout into the response body (bounded, before the trailing
/// `STATUS_MARKER`) and the HTTP status code that followed it. A stub
/// harness that ignores `-w` entirely (as several test fixtures do) simply
/// produces no marker; that's `status_code: None`, not an error.
fn parse_delivery_output(event: &str, output: &Output) -> DeliveryOutcome {
    let stdout = String::from_utf8_lossy(&output.stdout);
    let (body_part, status_code) = match stdout.rfind(STATUS_MARKER) {
        Some(idx) => {
            let code = stdout[idx + STATUS_MARKER.len()..]
                .trim()
                .parse::<i64>()
                .ok();
            (&stdout[..idx], code)
        }
        None => (stdout.as_ref(), None),
    };
    let response = truncate_response(body_part);

    if !output.status.success() {
        // curl itself could not complete the round trip (DNS, connection
        // refused, timeout) — a hard failure independent of any HTTP status.
        return DeliveryOutcome::failed(
            format!("event={event} webhook_exit={}", output.status),
            status_code,
            response,
        );
    }

    match status_code {
        Some(code) if !(200..300).contains(&code) => DeliveryOutcome::failed(
            format!("event={event} http_status={code}"),
            status_code,
            response,
        ),
        _ => DeliveryOutcome::delivered(status_code, response),
    }
}

/// Bound and scrub the response snippet before it is ever persisted: a
/// large body is truncated, never stored whole. Truncation lands on a char
/// boundary so it never splits a multi-byte UTF-8 sequence.
fn truncate_response(body: &str) -> Option<String> {
    let trimmed = body.trim();
    if trimmed.is_empty() {
        return None;
    }
    if trimmed.len() <= NOTIFICATION_RESPONSE_BYTES_CAP {
        return Some(trimmed.to_string());
    }
    let mut end = NOTIFICATION_RESPONSE_BYTES_CAP;
    while !trimmed.is_char_boundary(end) {
        end -= 1;
    }
    Some(format!("{}… (truncated)", &trimmed[..end]))
}

fn record_delivery_result(ledger: &Ledger, outbox_id: Option<i64>, outcome: DeliveryOutcome) {
    if let Some(error) = &outcome.error {
        if let Some(id) = outbox_id {
            let _ = ledger.mark_notification_failed(
                id,
                error,
                outcome.status_code,
                outcome.response.as_deref(),
            );
        }
        eprintln!("notify: {error}");
        let _ = ledger.record_guard_event("notify_failed", None, error, 1);
    } else if let Some(id) = outbox_id {
        let _ = ledger.mark_notification_delivered(
            id,
            outcome.status_code,
            outcome.response.as_deref(),
        );
    }
}
