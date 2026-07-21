use anyhow::{bail, Context, Result};
use chrono::{DateTime, Utc};
use hmac::{Hmac, Mac};
use sha2::Sha256;
use time::OffsetDateTime;

use crate::attention::{self, AttentionDebt};
use crate::budget::{self, Violation};
use crate::ledger::{IngressOutcome, IngressRequest, Ledger};
use crate::spec::{AttentionDebtPolicy, Plane, Task, TriggerSpec, WebhookActionSpec};
pub struct WebhookResponse {
    pub status: u16,
    pub body: String,
}

#[derive(serde::Serialize)]
struct AdmissionRefusal {
    kind: String,
    detail: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    debt: Option<AttentionDebt>,
}
pub fn webhook_target<'p>(plane: &'p Plane, route: &str) -> Option<(&'p Task, &'p TriggerSpec)> {
    for task in plane.tasks.values() {
        for trigger in &task.spec.triggers {
            if let TriggerSpec::Webhook { route: r, .. } = trigger {
                if r == route {
                    return Some((task, trigger));
                }
            }
        }
    }
    None
}
pub fn handle_webhook(
    plane: &Plane,
    ledger: &mut Ledger,
    route: &str,
    headers: &[(String, String)],
    body: &str,
) -> Result<WebhookResponse> {
    let Some((task, trigger)) = webhook_target(plane, route) else {
        return Ok(WebhookResponse {
            status: 404,
            body: format!("{{\"error\":\"no webhook route '{route}'\"}}"),
        });
    };
    let max_body = plane.spec.ingress.max_body_bytes;
    if body.len() > max_body {
        ledger.record_guard_event(
            "ingress_oversized",
            Some(&task.name),
            &format!("route={route} bytes={} max={max_body}", body.len()),
            1,
        )?;
        return Ok(WebhookResponse {
            status: 413,
            body: format!(
                "{{\"error\":\"webhook body {} bytes exceeds max_body_bytes {max_body}\"}}",
                body.len()
            ),
        });
    }
    let TriggerSpec::Webhook {
        secret_env,
        dedupe_key,
        action,
        filter,
        ..
    } = trigger
    else {
        unreachable!("webhook_target returns webhook triggers only");
    };

    let Ok(secret) = std::env::var(secret_env) else {
        return Ok(WebhookResponse {
            status: 503,
            body: format!("{{\"error\":\"secret env '{secret_env}' not set on the plane\"}}"),
        });
    };
    if !verify_delivery_hmac(&secret, headers, body) {
        return Ok(WebhookResponse {
            status: 401,
            body: "{\"error\":\"invalid signature\"}".to_string(),
        });
    }
    if !filter.is_empty() {
        let payload: serde_json::Value = match serde_json::from_str(body) {
            Ok(v) => v,
            Err(_) => {
                return Ok(WebhookResponse {
                    status: 200,
                    body: "{\"filtered\":\"payload is not JSON\"}".to_string(),
                })
            }
        };
        for f in filter {
            if let Some(reason) = f.reject_reason(&payload) {
                return Ok(WebhookResponse {
                    status: 200,
                    body: serde_json::json!({ "filtered": reason }).to_string(),
                });
            }
        }
    }

    if let Some(refusal) = reflex_admission_refusal(plane, ledger, task, "webhook")? {
        return Ok(WebhookResponse {
            status: 429,
            body: serde_json::json!({
                "error": "attention debt brake refused reflex admission",
                "task": task.name,
                "debt": refusal.debt.as_ref(),
                "refusal": refusal,
            })
            .to_string(),
        });
    }

    let derived = match dedupe_key {
        Some(expr) => Some(derive_dedupe_key(expr, headers, body)?),
        None => Some(format!("body:{}", body_hash(body))),
    };
    let key = derived.map(|k| format!("wh:{route}:{k}"));

    let source = header(headers, "x-github-delivery").or_else(|| header(headers, "x-delivery-id"));
    let outcome = ledger.ingest(IngressRequest {
        task: &task.name,
        trigger_kind: "webhook",
        idempotency_key: key.as_deref(),
        source_event_id: source.as_deref(),
        payload: Some(body),
        parent_run_id: None,
    })?;
    if let Some(action) = action {
        start_submission_storm(
            plane,
            ledger,
            &outcome.run_id,
            headers,
            body,
            action,
            !outcome.duplicate,
        )?;
    }
    Ok(WebhookResponse {
        status: 202,
        body: serde_json::json!({"run_id": outcome.run_id, "duplicate": outcome.duplicate})
            .to_string(),
    })
}

/// Webhook delivery for a database-defined workflow trigger
/// (bitterblossom-workflow-runtime-v1). Same HMAC and dedupe-derivation
/// mechanics as task webhooks, converging on the workflow runtime's single
/// normalized acceptance contract.
pub fn handle_workflow_webhook(
    plane: &Plane,
    ledger: &Ledger,
    workflow: &str,
    trigger: &crate::workflow::WorkflowTrigger,
    headers: &[(String, String)],
    body: &str,
) -> Result<WebhookResponse> {
    let secret_env = trigger.secret_env.as_deref().unwrap_or_default();
    let Ok(secret) = std::env::var(secret_env) else {
        return Ok(WebhookResponse {
            status: 503,
            body: format!("{{\"error\":\"secret env '{secret_env}' not set on the plane\"}}"),
        });
    };
    if !verify_delivery_hmac(&secret, headers, body) {
        return Ok(WebhookResponse {
            status: 401,
            body: "{\"error\":\"invalid signature\"}".to_string(),
        });
    }
    let derived = match &trigger.dedupe_key {
        Some(expr) => derive_dedupe_key(expr, headers, body)?,
        None => format!("body:{}", body_hash(body)),
    };
    let route = trigger.route.as_deref().unwrap_or_default();
    let outcome = crate::workflow_runtime::accept(
        plane,
        ledger,
        &crate::workflow_runtime::TriggerEnvelope {
            workflow: workflow.to_string(),
            source: crate::workflow_runtime::TriggerSource::External,
            payload: Some(body.to_string()),
            dedupe_key: Some(format!("wh:{route}:{derived}")),
        },
    )?;
    use crate::workflow::AcceptOutcome;
    let (status, body) = match &outcome {
        AcceptOutcome::Accepted { run } => (
            202,
            serde_json::json!({"workflow_run_id": run.id, "duplicate": false}),
        ),
        AcceptOutcome::Duplicate { run } => (
            202,
            serde_json::json!({"workflow_run_id": run.id, "duplicate": true}),
        ),
        AcceptOutcome::Denied { workflow, reason } => (
            429,
            serde_json::json!({"denied": reason, "workflow": workflow}),
        ),
        AcceptOutcome::Suppressed { workflow, reason } => (
            202,
            serde_json::json!({"suppressed": reason, "workflow": workflow}),
        ),
    };
    Ok(WebhookResponse {
        status,
        body: body.to_string(),
    })
}

fn start_submission_storm(
    plane: &Plane,
    ledger: &mut Ledger,
    parent_run_id: &str,
    headers: &[(String, String)],
    body: &str,
    action: &WebhookActionSpec,
    allow_supersede: bool,
) -> Result<()> {
    let WebhookActionSpec::SubmissionStorm {
        change,
        rev,
        repo,
        version,
    } = action;
    let gate = plane
        .spec
        .gate
        .as_ref()
        .context("submission_storm action requires [gate]")?;
    let change = derive_dedupe_key(change, headers, body)?;
    let rev = derive_dedupe_key(rev, headers, body)?;
    let repo = derive_optional(repo, headers, body)?;
    let version = derive_optional(version, headers, body)?;
    let Some(submission) =
        open_webhook_submission(ledger, &change, &rev, allow_supersede, version.as_deref())?
    else {
        return Ok(());
    };
    for kind in &gate.required {
        let task = plane
            .tasks
            .values()
            .find(|task| task.spec.verdict.as_deref() == Some(kind.as_str()))
            .with_context(|| format!("no task declares verdict = \"{kind}\""))?;
        let key = format!("storm:{}:{kind}", submission.id);
        let mut payload = serde_json::json!({
            "submission": submission.id,
            "change": submission.change_key,
            "rev": submission.rev,
        });
        if let Some(repo) = &repo {
            payload["repo"] = repo.clone().into();
        }
        let payload = payload.to_string();
        let _ = ledger.ingest(IngressRequest {
            task: &task.name,
            trigger_kind: "webhook",
            idempotency_key: Some(&key),
            source_event_id: None,
            payload: Some(&payload),
            parent_run_id: Some(parent_run_id),
        })?;
    }
    Ok(())
}

fn derive_optional(
    expr: &Option<String>,
    headers: &[(String, String)],
    body: &str,
) -> Result<Option<String>> {
    expr.as_deref()
        .map(|expr| derive_dedupe_key(expr, headers, body))
        .transpose()
}

fn open_webhook_submission(
    ledger: &mut Ledger,
    change: &str,
    rev: &str,
    allow_supersede: bool,
    version: Option<&str>,
) -> Result<Option<crate::submit::SubmissionRow>> {
    // `open_submission` errors only when a submission is already open, so a redelivery
    // that lands after one settles would re-open and re-storm. Decide from the latest
    // submission's head and state instead.
    match ledger.latest_submission(change)? {
        // Same head we already handled: reuse an open submission (the member loop
        // repairs missing rows); a settled one is an idempotent no-op.
        Some(l) if l.rev == rev => {
            if l.state == "open" {
                return Ok(Some(l));
            }
            return Ok(None);
        }
        // Different head while one is open: supersede only on a non-duplicate, strictly
        // newer delivery.
        Some(l) if l.state == "open" => {
            if !allow_supersede
                || version
                    .zip(submission_head_version(&l))
                    .is_some_and(|(new, old)| new <= old)
            {
                return Ok(None);
            }
            ledger.settle_submission(&l.id, "abandoned", "{}")?;
        }
        // Different head after settle: open only when it is newer than the last
        // processed head.
        Some(l)
            if version
                .zip(l.head_version.as_deref())
                .is_some_and(|(new, old)| new <= old) =>
        {
            return Ok(None);
        }
        // No prior submission, or a settled one with a newer or unversioned head: open
        // the next round below.
        _ => {}
    }
    let mut submission = ledger.open_submission(change, rev, None)?;
    remember_submission_version(ledger, &submission.id, version)?;
    submission.head_version = version.map(ToOwned::to_owned);
    Ok(Some(submission))
}

fn remember_submission_version(ledger: &mut Ledger, id: &str, version: Option<&str>) -> Result<()> {
    if let Some(version) = version {
        ledger.conn.execute(
            "UPDATE submissions SET head_version = ?2 WHERE id = ?1 AND state = 'open'",
            rusqlite::params![id, version],
        )?;
    }
    Ok(())
}

fn submission_head_version(submission: &crate::submit::SubmissionRow) -> Option<&str> {
    submission
        .head_version
        .as_deref()
        .or(submission.report_json.as_deref())
}

pub fn derive_dedupe_key(expr: &str, headers: &[(String, String)], body: &str) -> Result<String> {
    if let Some((left, right)) = expr.split_once('|') {
        let left = derive_dedupe_key(left, headers, body)?;
        let right = derive_dedupe_key(right, headers, body)?;
        return Ok(format!("{left}|{right}"));
    }
    match expr.split_once(':') {
        Some(("header", name)) => header(headers, &name.to_ascii_lowercase())
            .with_context(|| format!("dedupe header '{name}' missing from delivery")),
        Some(("json", pointer)) => {
            let v: serde_json::Value =
                serde_json::from_str(body).context("dedupe json: body is not JSON")?;
            let found = v
                .pointer(pointer)
                .with_context(|| format!("dedupe pointer '{pointer}' missing from body"))?;
            Ok(match found {
                serde_json::Value::String(s) => s.clone(),
                other => other.to_string(),
            })
        }
        _ => bail!("unknown dedupe_key expression '{expr}' (use header:<Name> or json:<ptr>)"),
    }
}

fn header(headers: &[(String, String)], name: &str) -> Option<String> {
    headers
        .iter()
        .find(|(k, _)| k.eq_ignore_ascii_case(name))
        .map(|(_, v)| v.clone())
}
pub fn verify_hmac(secret: &str, body: &[u8], signature_header: &str) -> bool {
    let Some(hex_sig) = signature_header.strip_prefix("sha256=") else {
        return false;
    };
    let Ok(expected) = hex_decode(hex_sig) else {
        return false;
    };
    let mut mac = Hmac::<Sha256>::new_from_slice(secret.as_bytes()).expect("hmac accepts any key");
    mac.update(body);
    mac.verify_slice(&expected).is_ok()
}

fn verify_delivery_hmac(secret: &str, headers: &[(String, String)], body: &str) -> bool {
    if let (Some(signature), Some(timestamp), Some(delivery_id)) = (
        header(headers, "x-canary-signature"),
        header(headers, "x-timestamp"),
        header(headers, "x-delivery-id"),
    ) {
        let signed = format!("{timestamp}.{delivery_id}.{body}");
        return verify_hmac(secret, signed.as_bytes(), &signature);
    }

    ["x-hub-signature-256", "x-signature-256", "x-signature"]
        .iter()
        .any(|name| {
            header(headers, name)
                .is_some_and(|signature| verify_hmac(secret, body.as_bytes(), &signature))
        })
}

pub fn sign_hmac(secret: &str, body: &[u8]) -> String {
    let mut mac = Hmac::<Sha256>::new_from_slice(secret.as_bytes()).expect("hmac accepts any key");
    mac.update(body);
    format!("sha256={}", hex_encode(&mac.finalize().into_bytes()))
}

fn body_hash(body: &str) -> String {
    use sha2::Digest;
    let mut h = Sha256::new();
    h.update(body.as_bytes());
    hex_encode(&h.finalize())
}

fn hex_encode(bytes: &[u8]) -> String {
    bytes.iter().map(|b| format!("{b:02x}")).collect()
}

fn hex_decode(s: &str) -> Result<Vec<u8>> {
    let bytes = s.as_bytes();
    if !bytes.len().is_multiple_of(2) {
        bail!("odd-length hex");
    }
    bytes
        .chunks_exact(2)
        .map(|pair| {
            let hi = hex_nibble(pair[0])?;
            let lo = hex_nibble(pair[1])?;
            Ok((hi << 4) | lo)
        })
        .collect()
}

fn hex_nibble(byte: u8) -> Result<u8> {
    match byte {
        b'0'..=b'9' => Ok(byte - b'0'),
        b'a'..=b'f' => Ok(byte - b'a' + 10),
        b'A'..=b'F' => Ok(byte - b'A' + 10),
        _ => bail!("bad hex"),
    }
}
pub fn parse_schedule(expr: &str) -> Result<cron::Schedule> {
    let normalized = if expr.split_whitespace().count() == 5 {
        format!("0 {expr}")
    } else {
        expr.to_string()
    };
    normalized
        .parse::<cron::Schedule>()
        .with_context(|| format!("invalid cron schedule '{expr}'"))
}
pub fn due_fires(
    schedule: &cron::Schedule,
    after: DateTime<Utc>,
    until: DateTime<Utc>,
) -> Vec<DateTime<Utc>> {
    schedule.after(&after).take_while(|t| *t <= until).collect()
}
pub fn ingest_cron_fire(
    ledger: &mut Ledger,
    task: &str,
    scheduled: DateTime<Utc>,
) -> Result<IngressOutcome> {
    let key = format!("cron:{}", scheduled.to_rfc3339());
    ledger.ingest(IngressRequest {
        task,
        trigger_kind: "cron",
        idempotency_key: Some(&key),
        source_event_id: None,
        payload: None,
        parent_run_id: None,
    })
}

/// Outcome of one cron catch-up tick for a task (backlog 083).
#[derive(Debug, Default, PartialEq, Eq)]
pub struct CronCatchupOutcome {
    pub ingested: usize,
    pub duplicates: usize,
    pub skipped: usize,
}

/// Ingest due cron fires for `task` between `last` and `now`, bounded by
/// `max_fires`. When more fires are due than `max_fires`, the oldest are
/// collapsed (skipped) and only the latest `max_fires` are ingested — reflex
/// tasks catch up to the newest state, not the whole backlog. Skipped fires
/// are recorded as a `cron_collapse` guard event so the count is visible in
/// status. Returns counts for assertion/drill visibility.
pub fn cron_catchup(
    ledger: &mut Ledger,
    task: &str,
    schedule: &cron::Schedule,
    last: DateTime<Utc>,
    now: DateTime<Utc>,
    max_fires: u32,
) -> Result<CronCatchupOutcome> {
    cron_catchup_inner(None, ledger, task, schedule, last, now, max_fires)
}

pub fn cron_catchup_guarded(
    plane: &Plane,
    ledger: &mut Ledger,
    task: &str,
    schedule: &cron::Schedule,
    last: DateTime<Utc>,
    now: DateTime<Utc>,
    max_fires: u32,
) -> Result<CronCatchupOutcome> {
    cron_catchup_inner(Some(plane), ledger, task, schedule, last, now, max_fires)
}

fn cron_catchup_inner(
    plane: Option<&Plane>,
    ledger: &mut Ledger,
    task: &str,
    schedule: &cron::Schedule,
    last: DateTime<Utc>,
    now: DateTime<Utc>,
    max_fires: u32,
) -> Result<CronCatchupOutcome> {
    let fires = due_fires(schedule, last, now);
    if fires.is_empty() {
        return Ok(CronCatchupOutcome::default());
    }
    if let Some(plane) = plane {
        let refused = match plane.task(task) {
            Ok(task_spec) => reflex_admission_refusal(plane, ledger, task_spec, "cron")?.is_some(),
            Err(_) => attention_debt_brake(plane, ledger, task, "cron")?.is_some(),
        };
        if refused {
            return Ok(CronCatchupOutcome {
                skipped: fires.len(),
                ..Default::default()
            });
        }
    }
    let max = max_fires.max(1) as usize;
    let (ingest_fires, skipped) = if fires.len() > max {
        let skipped = fires.len() - max;
        (fires[fires.len() - max..].to_vec(), skipped)
    } else {
        (fires.clone(), 0)
    };
    let mut outcome = CronCatchupOutcome {
        skipped,
        ..Default::default()
    };
    for fire in &ingest_fires {
        match ingest_cron_fire(ledger, task, *fire) {
            Ok(o) if o.duplicate => outcome.duplicates += 1,
            Ok(_) => outcome.ingested += 1,
            Err(e) => return Err(e),
        }
    }
    if skipped > 0 {
        ledger.record_guard_event(
            "cron_collapse",
            Some(task),
            &format!("skipped={skipped} fired={}", fires.len()),
            skipped as i64,
        )?;
    }
    Ok(outcome)
}

pub fn attention_debt_brake(
    plane: &Plane,
    ledger: &Ledger,
    task: &str,
    source: &str,
) -> Result<Option<AttentionDebt>> {
    let debt = attention::scan(plane, ledger, OffsetDateTime::now_utc())?;
    if !debt.blocking {
        return Ok(None);
    }
    ledger.record_guard_event(
        "attention_debt_brake",
        Some(task),
        &format!("source={source} {}", debt.reason),
        1,
    )?;
    Ok(Some(debt))
}

fn reflex_admission_refusal(
    plane: &Plane,
    ledger: &Ledger,
    task: &Task,
    source: &str,
) -> Result<Option<AdmissionRefusal>> {
    match task.spec.admission.attention_debt {
        AttentionDebtPolicy::Global => {
            attention_debt_brake(plane, ledger, &task.name, source).map(|debt| {
                debt.map(|debt| AdmissionRefusal {
                    kind: "attention_debt".to_string(),
                    detail: debt.reason.clone(),
                    debt: Some(debt),
                })
            })
        }
        AttentionDebtPolicy::Task => {
            if let Some(violation) = budget::pre_dispatch_check(plane, ledger, task)? {
                record_admission_refusal(ledger, &task.name, source, &violation)?;
                return Ok(Some(AdmissionRefusal {
                    kind: violation.kind.to_string(),
                    detail: violation.detail,
                    debt: None,
                }));
            }
            let debt = attention::scan_task(ledger, &task.name, OffsetDateTime::now_utc())?;
            if !debt.blocking {
                return Ok(None);
            }
            ledger.record_guard_event(
                "attention_debt_brake",
                Some(&task.name),
                &format!("source={source} policy=task {}", debt.reason),
                1,
            )?;
            Ok(Some(AdmissionRefusal {
                kind: "attention_debt".to_string(),
                detail: debt.reason.clone(),
                debt: Some(debt),
            }))
        }
    }
}

fn record_admission_refusal(
    ledger: &Ledger,
    task: &str,
    source: &str,
    violation: &Violation,
) -> Result<()> {
    ledger.record_guard_event(
        "attention_debt_brake",
        Some(task),
        &format!(
            "source={source} policy=task {}={}",
            violation.kind, violation.detail
        ),
        1,
    )
}
