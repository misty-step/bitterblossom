use anyhow::{bail, Context, Result};
use chrono::{DateTime, Utc};
use hmac::{Hmac, Mac};
use sha2::Sha256;

use crate::ledger::{IngressOutcome, IngressRequest, Ledger};
use crate::spec::{Plane, Task, TriggerSpec, WebhookActionSpec};
pub struct WebhookResponse {
    pub status: u16,
    pub body: String,
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
    let signature = header(headers, "x-hub-signature-256")
        .or_else(|| header(headers, "x-signature-256"))
        .unwrap_or_default();
    if !verify_hmac(&secret, body.as_bytes(), &signature) {
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

    let derived = match dedupe_key {
        Some(expr) => Some(derive_dedupe_key(expr, headers, body)?),
        None => Some(format!("body:{}", body_hash(body))),
    };
    let key = derived.map(|k| format!("wh:{route}:{k}"));

    let source_event_id = header(headers, "x-github-delivery");
    let outcome = ledger.ingest(IngressRequest {
        task: &task.name,
        trigger_kind: "webhook",
        idempotency_key: key.as_deref(),
        source_event_id: source_event_id.as_deref(),
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
    match ledger.open_submission(change, rev, None) {
        Ok(submission) => {
            remember_submission_version(ledger, &submission.id, version)?;
            Ok(Some(submission))
        }
        Err(err) => {
            let latest = ledger
                .latest_submission(change)?
                .with_context(|| err.to_string())?;
            if latest.state == "open" && latest.rev == rev {
                return Ok(Some(latest));
            }
            if latest.state != "open" {
                return Err(err);
            }
            if !allow_supersede {
                return Ok(None);
            }
            if version
                .zip(latest.report_json.as_deref())
                .is_some_and(|(new, old)| new <= old)
            {
                return Ok(None);
            }
            ledger.settle_submission(&latest.id, "abandoned", "{}")?;
            let submission = ledger.open_submission(change, rev, None)?;
            remember_submission_version(ledger, &submission.id, version)?;
            Ok(Some(submission))
        }
    }
}

fn remember_submission_version(ledger: &mut Ledger, id: &str, version: Option<&str>) -> Result<()> {
    if let Some(version) = version {
        ledger.conn.execute(
            "UPDATE submissions SET report_json = ?2 WHERE id = ?1 AND state = 'open'",
            rusqlite::params![id, version],
        )?;
    }
    Ok(())
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
    if !s.len().is_multiple_of(2) {
        bail!("odd-length hex");
    }
    (0..s.len())
        .step_by(2)
        .map(|i| u8::from_str_radix(&s[i..i + 2], 16).context("bad hex"))
        .collect()
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
