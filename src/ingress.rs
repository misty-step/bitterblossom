use anyhow::{bail, Context, Result};
use chrono::{DateTime, Utc};
use hmac::{Hmac, Mac};
use rusqlite::OptionalExtension;
use sha2::Sha256;
use std::fmt;
use time::{format_description::well_known::Rfc3339, OffsetDateTime};

const DELIVERY_MAX_CLOCK_SKEW_SECONDS: i64 = 300;

use crate::attention::{self, AttentionDebt};
use crate::budget::{self, Violation};
use crate::ledger::{IngressOutcome, IngressRequest, Ledger};
use crate::spec::{AttentionDebtPolicy, Plane, Task, TriggerSpec, WebhookActionSpec};
pub struct WebhookResponse {
    pub status: u16,
    pub body: String,
}

#[derive(Debug)]
pub enum IngressClientError {
    BadRequest(String),
    Backpressure(String),
}

impl fmt::Display for IngressClientError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::BadRequest(detail) => write!(f, "{detail}"),
            Self::Backpressure(detail) => write!(f, "{detail}"),
        }
    }
}
impl std::error::Error for IngressClientError {}

#[derive(serde::Serialize)]
struct AdmissionRefusal {
    kind: String,
    detail: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    debt: Option<AttentionDebt>,
}
pub fn webhook_target<'p>(
    plane: &'p Plane,
    route: &str,
) -> Result<Option<(&'p Task, &'p TriggerSpec)>> {
    let route = normalize_route(route);
    let mut found = None;
    for task in plane.tasks.values() {
        for trigger in &task.spec.triggers {
            if let TriggerSpec::Webhook { route: r, .. } = trigger {
                if normalize_route(r) != route {
                    continue;
                }
                if found.is_some() {
                    bail!("webhook route '{route}' is claimed by multiple tasks");
                }
                found = Some((task, trigger));
            }
        }
    }
    Ok(found)
}

pub fn normalize_route(route: &str) -> String {
    route.trim().trim_matches('/').to_ascii_lowercase()
}

pub fn task_webhook_routes(plane: &Plane) -> Vec<String> {
    plane
        .tasks
        .values()
        .flat_map(|task| task.spec.triggers.iter())
        .filter_map(|trigger| match trigger {
            TriggerSpec::Webhook { route, .. } => Some(normalize_route(route)),
            _ => None,
        })
        .collect()
}
pub fn handle_webhook(
    plane: &Plane,
    ledger: &mut Ledger,
    route: &str,
    headers: &[(String, String)],
    body: &str,
) -> Result<WebhookResponse> {
    let Some((task, trigger)) = (match webhook_target(plane, route) {
        Ok(target) => target,
        Err(_) => {
            return Ok(WebhookResponse {
                status: 409,
                body: "{\"error\":\"webhook route is ambiguous\"}".to_string(),
            })
        }
    }) else {
        return Ok(WebhookResponse {
            status: 404,
            body: format!("{{\"error\":\"no webhook route '{route}'\"}}"),
        });
    };
    let max_body = plane.spec.ingress.max_body_bytes;
    let oversized = body.len() > max_body;
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
    match verify_delivery_hmac(&secret, headers, body) {
        DeliveryVerification::Valid => {}
        DeliveryVerification::InvalidSignature => {
            return Ok(WebhookResponse {
                status: 401,
                body: "{\"error\":\"invalid signature\"}".to_string(),
            });
        }
        DeliveryVerification::Stale => {
            return Ok(WebhookResponse {
                status: 401,
                body: "{\"error\":\"signature timestamp outside allowed clock skew\"}".to_string(),
            });
        }
        DeliveryVerification::MalformedTimestamp
        | DeliveryVerification::IncompleteTimestampedHeaders => {
            return Ok(WebhookResponse {
                status: 400,
                body: "{\"error\":\"malformed timestamped signature envelope\"}".to_string(),
            });
        }
    }
    if oversized {
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
    let payload: serde_json::Value = match serde_json::from_str(body) {
        Ok(v) => v,
        Err(_) => {
            ledger.record_guard_event(
                "ingress_malformed",
                Some(&task.name),
                &format!("route={route} body is not valid JSON"),
                1,
            )?;
            return Ok(WebhookResponse {
                status: 400,
                body: "{\"error\":\"webhook body must be valid JSON\"}".to_string(),
            });
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

    // Derive the key before admission guards. This lets the ledger preserve
    // duplicate dispositions even while attention debt or queue pressure is active.
    let derived = match dedupe_key {
        Some(expr) => match derive_dedupe_key(expr, headers, body) {
            Ok(value) => Some(value),
            Err(_) => {
                return Ok(WebhookResponse {
                    status: 400,
                    body: "{\"error\":\"invalid webhook dedupe key\"}".to_string(),
                })
            }
        },
        None => Some(format!("body:{}", body_hash(body))),
    };
    let key = derived.map(|k| format!("wh:{}:{k}", normalize_route(route)));
    let duplicate = match key.as_deref() {
        Some(key) => ledger
            .conn
            .query_row(
                "SELECT 1 FROM runs WHERE task = ?1 AND idempotency_key = ?2",
                rusqlite::params![task.name, key],
                |_row| Ok(()),
            )
            .optional()?
            .is_some(),
        None => false,
    };

    if !duplicate {
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
    }

    let storm_plan = action
        .as_ref()
        .map(|action| prepare_submission_storm(plane, headers, body, action))
        .transpose()?;
    let source = header(headers, "x-github-delivery").or_else(|| header(headers, "x-delivery-id"));
    let outcome = if let Some((change, rev, version, members)) = storm_plan {
        match ledger.ingest_submission_storm(
            IngressRequest {
                task: &task.name,
                trigger_kind: "webhook",
                idempotency_key: key.as_deref(),
                source_event_id: source.as_deref(),
                payload: Some(body),
                parent_run_id: None,
            },
            &change,
            &rev,
            version.as_deref(),
            &members,
        ) {
            Ok(outcome) => outcome,
            Err(error) if error.to_string().contains("queue backpressure") => {
                ledger.record_guard_event(
                    "queue_backpressure",
                    Some(&task.name),
                    &format!("source=webhook {error}"),
                    1,
                )?;
                return Ok(WebhookResponse {
                    status: 429,
                    body: serde_json::json!({
                        "error": "queue backpressure refused reflex admission",
                        "task": task.name,
                        "detail": error.to_string(),
                    })
                    .to_string(),
                });
            }
            Err(error) => return Err(error),
        }
    } else {
        match ledger.ingest(IngressRequest {
            task: &task.name,
            trigger_kind: "webhook",
            idempotency_key: key.as_deref(),
            source_event_id: source.as_deref(),
            payload: Some(body),
            parent_run_id: None,
        }) {
            Ok(outcome) => outcome,
            Err(error) if error.to_string().contains("queue backpressure") => {
                ledger.record_guard_event(
                    "queue_backpressure",
                    Some(&task.name),
                    &format!("source=webhook {error}"),
                    1,
                )?;
                return Ok(WebhookResponse {
                    status: 429,
                    body: serde_json::json!({
                        "error": "queue backpressure refused reflex admission",
                        "task": task.name,
                        "detail": error.to_string(),
                    })
                    .to_string(),
                });
            }
            Err(error) => return Err(error),
        }
    };
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
    match verify_delivery_hmac(&secret, headers, body) {
        DeliveryVerification::Valid => {}
        DeliveryVerification::InvalidSignature => {
            return Ok(WebhookResponse {
                status: 401,
                body: "{\"error\":\"invalid signature\"}".to_string(),
            });
        }
        DeliveryVerification::Stale => {
            return Ok(WebhookResponse {
                status: 401,
                body: "{\"error\":\"signature timestamp outside allowed clock skew\"}".to_string(),
            });
        }
        DeliveryVerification::MalformedTimestamp
        | DeliveryVerification::IncompleteTimestampedHeaders => {
            return Ok(WebhookResponse {
                status: 400,
                body: "{\"error\":\"malformed timestamped signature envelope\"}".to_string(),
            });
        }
    }
    if serde_json::from_str::<serde_json::Value>(body).is_err() {
        return Ok(WebhookResponse {
            status: 400,
            body: "{\"error\":\"webhook body must be valid JSON\"}".to_string(),
        });
    }
    let derived = match &trigger.dedupe_key {
        Some(expr) => match derive_dedupe_key(expr, headers, body) {
            Ok(value) => value,
            Err(_) => {
                return Ok(WebhookResponse {
                    status: 400,
                    body: "{\"error\":\"invalid workflow dedupe key\"}".to_string(),
                })
            }
        },
        None => format!("body:{}", body_hash(body)),
    };
    let route = trigger.route.as_deref().unwrap_or_default();
    let outcome = match crate::workflow_runtime::accept(
        plane,
        ledger,
        &crate::workflow_runtime::TriggerEnvelope {
            workflow: workflow.to_string(),
            source: crate::workflow_runtime::TriggerSource::External,
            payload: Some(body.to_string()),
            dedupe_key: Some(format!("wh:{}:{derived}", normalize_route(route))),
        },
    ) {
        Ok(outcome) => outcome,
        Err(error) if error.to_string().contains("payload must be JSON") => return Ok(WebhookResponse {
            status: 400,
            body: "{\"error\":\"invalid workflow webhook payload\"}".to_string(),
        }),
        Err(error) => return Err(error),
    };
        ledger,
        &crate::workflow_runtime::TriggerEnvelope {
            workflow: workflow.to_string(),
            source: crate::workflow_runtime::TriggerSource::External,
            payload: Some(body.to_string()),
            dedupe_key: Some(format!("wh:{}:{derived}", normalize_route(route))),
        },
    )?;
    use crate::workflow::AcceptOutcome;
    let body = match &outcome {
        AcceptOutcome::Accepted { run } => {
            serde_json::json!({"workflow_run_id": run.id, "duplicate": false})
        }
        AcceptOutcome::Duplicate { run } => {
            serde_json::json!({"workflow_run_id": run.id, "duplicate": true})
        }
        AcceptOutcome::Suppressed { workflow, reason } => {
            serde_json::json!({"suppressed": reason, "workflow": workflow})
        }
        AcceptOutcome::Denied {
            workflow,
            kind,
            reason,
        } => {
            serde_json::json!({"denied": reason, "kind": kind, "workflow": workflow})
        }
    };
    let status = if matches!(outcome, AcceptOutcome::Denied { .. }) {
        429
    } else {
        202
    };
    Ok(WebhookResponse {
        status,
        body: body.to_string(),
    })
}

type SubmissionStormPlan = (
    String,
    String,
    Option<String>,
    Vec<(String, String, String)>,
);

fn prepare_submission_storm(
    plane: &Plane,
    headers: &[(String, String)],
    body: &str,
    action: &WebhookActionSpec,
) -> Result<SubmissionStormPlan> {
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
    let derive = |expr: &str| {
        derive_dedupe_key(expr, headers, body).map_err(|_| {
            anyhow::Error::new(IngressClientError::BadRequest(
                "submission_storm action field must resolve to a non-empty value".into(),
            ))
        })
    };
    let change = derive(change)?;
    let rev = derive(rev)?;
    let repo = repo.as_deref().map(derive).transpose()?;
    let version = version.as_deref().map(derive).transpose()?;
    let mut members = Vec::with_capacity(gate.required.len());
    for kind in &gate.required {
        let task = plane
            .tasks
            .values()
            .find(|task| task.spec.verdict.as_deref() == Some(kind.as_str()))
            .with_context(|| format!("no task declares verdict = \"{kind}\""))?;
        let key = kind.clone();
        let mut payload = serde_json::json!({
            "change": change,
            "rev": rev,
        });
        if let Some(repo) = &repo {
            payload["repo"] = repo.clone().into();
        }
        members.push((task.name.clone(), key, payload.to_string()));
    }
    Ok((change, rev, version, members))
}

/// Validate the canonical trigger dedupe expression before activation.
///
/// Expressions are one or more header:<name> or json:<pointer> terms joined
/// by a pipe. Keeping syntax validation here lets the workflow store and every
/// ingress path share one parser rather than accepting a trigger that can only
/// fail after the first delivery arrives.
pub fn validate_dedupe_key_expression(expr: &str) -> Result<()> {
    let expr = expr.trim();
    if expr.is_empty() {
        bail!("dedupe_key expression must not be empty");
    }
    if let Some((left, right)) = expr.split_once('|') {
        validate_dedupe_key_expression(left)?;
        validate_dedupe_key_expression(right)?;
        return Ok(());
    }
    let Some((kind, value)) = expr.split_once(':') else {
        bail!("unknown dedupe_key expression '{expr}' (use header:<Name> or json:<ptr>)");
    };
    match kind {
        "header" if !value.trim().is_empty() && !value.contains(':') => Ok(()),
        "json" if !value.is_empty() && value.starts_with('/') => Ok(()),
        "header" => bail!("dedupe header expression '{expr}' needs a non-empty name"),
        "json" => bail!("dedupe json expression '{expr}' needs a JSON pointer starting with '/'"),
        _ => bail!("unknown dedupe_key expression '{expr}' (use header:<Name> or json:<ptr>)"),
    }
}

pub fn derive_dedupe_key(expr: &str, headers: &[(String, String)], body: &str) -> Result<String> {
    validate_dedupe_key_expression(expr)?;
    let expr = expr.trim();
    if let Some((left, right)) = expr.split_once('|') {
        let left = derive_dedupe_key(left, headers, body)?;
        let right = derive_dedupe_key(right, headers, body)?;
        if left.is_empty() || right.is_empty() {
            bail!("dedupe expression resolved to an empty value");
        }
        return Ok(format!("{left}|{right}"));
    }
    let (kind, value) = expr
        .split_once(':')
        .expect("validated dedupe expression contains a kind separator");
    let result = match kind {
        "header" => header(headers, &value.to_ascii_lowercase())
            .with_context(|| format!("dedupe header '{value}' missing from delivery"))?,
        "json" => {
            let payload: serde_json::Value =
                serde_json::from_str(body).context("dedupe json: body is not JSON")?;
            let found = payload
                .pointer(value)
                .with_context(|| format!("dedupe pointer '{value}' missing from body"))?;
            match found {
                serde_json::Value::Null => {
                    bail!("dedupe pointer '{value}' resolved to null")
                }
                serde_json::Value::String(s) if s.trim().is_empty() => {
                    bail!("dedupe pointer '{value}' resolved to an empty string")
                }
                serde_json::Value::String(s) => s.trim().to_string(),
                serde_json::Value::Number(number) => number.to_string(),
                serde_json::Value::Bool(value) => value.to_string(),
                _ => bail!("dedupe pointer '{value}' must resolve to a scalar"),
            }
        }
        _ => unreachable!("validated dedupe expression kind"),
    };
    if result.trim().is_empty() {
        bail!("dedupe expression '{expr}' resolved to an empty value");
    }
    Ok(result)
}
fn header(headers: &[(String, String)], name: &str) -> Option<String> {
    headers
        .iter()
        .find(|(k, _)| k.eq_ignore_ascii_case(name))
        .map(|(_, v)| v.trim().to_string())
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

#[derive(Debug, PartialEq, Eq)]
enum DeliveryVerification {
    Valid,
    InvalidSignature,
    Stale,
    MalformedTimestamp,
    IncompleteTimestampedHeaders,
}

fn verify_delivery_hmac(
    secret: &str,
    headers: &[(String, String)],
    body: &str,
) -> DeliveryVerification {
    let canary_signature = header(headers, "x-canary-signature");
    let timestamp_header = header(headers, "x-timestamp");
    let delivery_id = header(headers, "x-delivery-id");
    if canary_signature.is_some() || timestamp_header.is_some() {
        let (Some(signature), Some(timestamp), Some(delivery_id)) =
            (canary_signature, timestamp_header, delivery_id)
        else {
            // Never downgrade a partially supplied timestamped envelope to
            // legacy body HMAC; doing so would bypass freshness/replay policy.
            return DeliveryVerification::IncompleteTimestampedHeaders;
        };
        let timestamp_text = timestamp;
        let timestamp = match parse_delivery_timestamp(&timestamp_text) {
            Ok(timestamp) => timestamp,
            Err(_) => return DeliveryVerification::MalformedTimestamp,
        };
        let now = OffsetDateTime::now_utc().unix_timestamp();
        if (i128::from(now) - i128::from(timestamp)).abs()
            > i128::from(DELIVERY_MAX_CLOCK_SKEW_SECONDS)
        {
            return DeliveryVerification::Stale;
        }
        let signed = format!("{timestamp_text}.{delivery_id}.{body}");
        return if verify_hmac(secret, signed.as_bytes(), &signature) {
            DeliveryVerification::Valid
        } else {
            DeliveryVerification::InvalidSignature
        };
    }

    ["x-hub-signature-256", "x-signature-256", "x-signature"]
        .iter()
        .find_map(|name| header(headers, name))
        .map(|signature| {
            if verify_hmac(secret, body.as_bytes(), &signature) {
                DeliveryVerification::Valid
            } else {
                DeliveryVerification::InvalidSignature
            }
        })
        .unwrap_or(DeliveryVerification::InvalidSignature)
}

fn parse_delivery_timestamp(value: &str) -> Result<i64> {
    if let Ok(seconds) = value.trim().parse::<i64>() {
        return Ok(seconds);
    }
    Ok(OffsetDateTime::parse(value.trim(), &Rfc3339)?.unix_timestamp())
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

pub fn due_fires_bounded_for_runtime(
    schedule: &cron::Schedule,
    after: DateTime<Utc>,
    until: DateTime<Utc>,
    max_fires: usize,
) -> (Vec<DateTime<Utc>>, usize) {
    due_fires_bounded(schedule, after, until, max_fires)
}

const MAX_CRON_SCAN_FIRES: usize = 10_000;

fn due_fires_bounded(
    schedule: &cron::Schedule,
    after: DateTime<Utc>,
    until: DateTime<Utc>,
    max_fires: usize,
) -> (Vec<DateTime<Utc>>, usize) {
    let cap = max_fires.max(1);
    let scan_cap = MAX_CRON_SCAN_FIRES.max(cap).min(MAX_CRON_SCAN_FIRES);
    let mut retained = std::collections::VecDeque::with_capacity(cap.min(scan_cap));
    let mut scanned = 0usize;
    for fire in schedule
        .after(&after)
        .take_while(|t| *t <= until)
        .take(scan_cap.saturating_add(1))
    {
        scanned = scanned.saturating_add(1);
        if retained.len() == cap {
            retained.pop_front();
        }
        retained.push_back(fire);
    }
    // Once the scan cap is reached this is a lower bound. We retain the
    // newest fires and never spend unbounded CPU counting an ancient backlog.
    (retained.into_iter().collect(), scanned.saturating_sub(cap))
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

/// Ingest one task's cron triggers together. Advancing the task cursor once
/// after this call prevents a second trigger from losing its own catch-up
/// window. Equal timestamps share the task's idempotency key and are kept as
/// one fire.
pub fn cron_catchup_guarded_multi(
    plane: &Plane,
    ledger: &mut Ledger,
    task: &str,
    schedules: &[&cron::Schedule],
    last: DateTime<Utc>,
    now: DateTime<Utc>,
    max_fires: u32,
) -> Result<CronCatchupOutcome> {
    let cap = max_fires.max(1) as usize;
    let mut fires = Vec::new();
    let mut skipped = 0usize;
    for schedule in schedules {
        let (mut retained, collapsed) = due_fires_bounded(schedule, last, now, cap);
        fires.append(&mut retained);
        skipped = skipped.saturating_add(collapsed);
    }
    fires.sort_unstable();
    fires.dedup();
    if fires.len() > cap {
        let drop_count = fires.len() - cap;
        fires.drain(..drop_count);
        skipped = skipped.saturating_add(drop_count);
    }
    cron_catchup_fires(Some(plane), ledger, task, fires, skipped)
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
    let (fires, bounded_skipped) = due_fires_bounded(schedule, last, now, max_fires as usize);
    cron_catchup_fires(plane, ledger, task, fires, bounded_skipped)
}

fn cron_catchup_fires(
    plane: Option<&Plane>,
    ledger: &mut Ledger,
    task: &str,
    ingest_fires: Vec<DateTime<Utc>>,
    skipped: usize,
) -> Result<CronCatchupOutcome> {
    if ingest_fires.is_empty() {
        return Ok(CronCatchupOutcome::default());
    }
    if let Some(plane) = plane {
        let refused = match plane.task(task) {
            Ok(task_spec) => reflex_admission_refusal(plane, ledger, task_spec, "cron")?.is_some(),
            Err(_) => attention_debt_brake(plane, ledger, task, "cron")?.is_some(),
        };
        if refused {
            // Refused fires are not collapsed: preserve the distinction in
            // the returned counters and guard stream.
            return Ok(CronCatchupOutcome {
                skipped: ingest_fires.len() + skipped,
                ..Default::default()
            });
        }
    }
    let mut outcome = CronCatchupOutcome {
        skipped,
        ..Default::default()
    };
    for fire in &ingest_fires {
        match ingest_cron_fire(ledger, task, *fire) {
            Ok(o) if o.duplicate => outcome.duplicates += 1,
            Ok(_) => outcome.ingested += 1,
            Err(e) if e.to_string().contains("queue backpressure") => {
                outcome.skipped += 1;
                ledger.record_guard_event(
                    "queue_backpressure",
                    Some(task),
                    &format!("source=cron {e}"),
                    1,
                )?;
            }
            Err(e) => return Err(e),
        }
    }
    if skipped > 0 {
        ledger.record_guard_event(
            "cron_collapse",
            Some(task),
            &format!("skipped={skipped} fired={}", ingest_fires.len() + skipped),
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
