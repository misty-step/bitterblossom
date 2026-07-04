use std::collections::{HashMap, HashSet};
use std::io::Read;
use std::path::Path;
use std::sync::{Arc, Mutex};
use std::time::Duration;

use anyhow::{bail, Context, Result};
use chrono::{DateTime, Utc};
use cron::Schedule;
use subtle::ConstantTimeEq;
use time::{format_description::well_known::Rfc3339, OffsetDateTime};

use crate::ingress;
use crate::ledger::{ExternalRunCreate, Ledger, LEDGER_SCHEMA_VERSION};
use crate::recovery;
use crate::spec::{Plane, TriggerSpec};
use crate::{canary, dispatch, notify, progress};

const DISPATCH_POLL: Duration = Duration::from_millis(500);
const CRON_POLL: Duration = Duration::from_secs(10);
const NOTIFY_RETRY_POLL: Duration = Duration::from_secs(60);
const WATCHDOG_POLL: Duration = Duration::from_secs(60);
const INGRESS_BIND_ENV: &str = "BB_INGRESS_BIND";
const API_TOKEN_ENV: &str = "BB_API_TOKEN";

pub fn serve(root: &Path) -> Result<()> {
    let plane = Plane::load(root)?;
    let bind = ingress_bind(&plane.spec.ingress.bind);
    enforce_public_bind_token(&bind)?;
    let mut ledger = Ledger::open(&plane.db_path())?;

    let reports = recovery::recover_inherited_runs(&plane, &mut ledger)?;
    for r in &reports {
        eprintln!(
            "recovered run {} task={} phase={} probe={} -> {}",
            r.run_id,
            r.task,
            r.attempt_phase.as_deref().unwrap_or("-"),
            r.probe.as_deref().unwrap_or("-"),
            r.disposition,
        );
        notify::notify(
            &plane,
            &ledger,
            "run_recovered_at_boot",
            &serde_json::json!({
                "run_id": r.run_id,
                "task": r.task,
                "disposition": r.disposition,
            }),
        );
    }
    drop(ledger);

    let root_buf = root.to_path_buf();
    {
        let root = root_buf.clone();
        std::thread::Builder::new()
            .name("bb-cron".into())
            .spawn(move || cron_loop(&root))?;
    }
    {
        let root = root_buf.clone();
        std::thread::Builder::new()
            .name("bb-dispatch".into())
            .spawn(move || dispatch_loop(&root))?;
    }
    {
        let root = root_buf.clone();
        std::thread::Builder::new()
            .name("bb-notify".into())
            .spawn(move || notify_loop(&root))?;
    }
    {
        let root = root_buf.clone();
        std::thread::Builder::new()
            .name("bb-watchdog".into())
            .spawn(move || watchdog_loop(&root))?;
    }

    canary::check_in();
    canary::start_health_loop();

    http_loop(&root_buf, &bind)
}

fn cron_loop(root: &Path) {
    let mut default_last = Utc::now();
    let mut last_by_task = HashMap::new();
    loop {
        std::thread::sleep(CRON_POLL);
        let plane = match Plane::load(root) {
            Ok(plane) => plane,
            Err(e) => {
                report_runtime_error("cron: plane load", "bb.plane.load", &e);
                continue;
            }
        };
        let mut ledger = match Ledger::open(&plane.db_path()) {
            Ok(ledger) => ledger,
            Err(e) => {
                report_runtime_error("cron: ledger open", "bb.ledger.open", &e);
                continue;
            }
        };
        let mut schedules = Vec::new();
        for task in plane.tasks.values() {
            for trigger in &task.spec.triggers {
                if let TriggerSpec::Cron { schedule } = trigger {
                    match ingress::parse_schedule(schedule) {
                        Ok(s) => schedules.push((task.name.clone(), s)),
                        Err(e) => {
                            eprintln!("cron: task {}: {e:#}", task.name);
                            canary::report_error(
                                "bb.cron.schedule_parse",
                                &format!("task {}: {e:#}", task.name),
                            );
                        }
                    }
                }
            }
        }
        let now = Utc::now();
        run_cron_tick(
            &plane,
            &mut ledger,
            &schedules,
            &mut last_by_task,
            &mut default_last,
            now,
            plane.spec.ingress.max_cron_catchup_fires,
        );
    }
}

fn run_cron_tick(
    plane: &Plane,
    ledger: &mut Ledger,
    schedules: &[(String, Schedule)],
    last_by_task: &mut HashMap<String, DateTime<Utc>>,
    default_last: &mut DateTime<Utc>,
    now: DateTime<Utc>,
    max_fires: u32,
) {
    let mut seen = HashSet::new();
    // Backlog 083: cron catch-up is bounded per task. A task that catches up
    // advances independently; a task that fails retries its own window without
    // rewinding successful tasks and duplicating their collapse counts.
    for (task, schedule) in schedules {
        seen.insert(task.clone());
        let last = *last_by_task.entry(task.clone()).or_insert(*default_last);
        match ingress::cron_catchup_guarded(plane, ledger, task, schedule, last, now, max_fires) {
            Ok(o) => {
                if o.skipped > 0 {
                    eprintln!("cron: task {task} collapsed {} skipped fires", o.skipped);
                }
                last_by_task.insert(task.clone(), now);
            }
            Err(e) => {
                eprintln!("cron: task {task}: {e:#}");
                canary::report_error("bb.cron.catchup", &format!("task {task}: {e:#}"));
            }
        }
    }
    last_by_task.retain(|task, _| seen.contains(task));
    *default_last = now;
}

fn dispatch_loop(root: &Path) {
    let in_flight: Arc<Mutex<HashSet<String>>> = Arc::default();
    let plane = match Plane::load(root) {
        Ok(plane) => plane,
        Err(e) => {
            report_runtime_error("dispatch: plane load", "bb.plane.load", &e);
            return;
        }
    };
    let ledger = match Ledger::open(&plane.db_path()) {
        Ok(ledger) => ledger,
        Err(e) => {
            report_runtime_error("dispatch: ledger open", "bb.ledger.open", &e);
            return;
        }
    };
    loop {
        std::thread::sleep(DISPATCH_POLL);
        // Backlog 083: a paused plane halts reflex dispatch. Manual `bb run`
        // bypasses this loop and still dispatches — pause is a reflex circuit
        // breaker, not an operator lock. State is visible in `status`.
        if reflex_dispatch_paused(&ledger) {
            continue;
        }
        let pending = match ledger.pending_runs_oldest_first() {
            Ok(p) => p,
            Err(e) => {
                eprintln!("dispatch: {e:#}");
                canary::report_error("bb.dispatch.pending", &format!("{e:#}"));
                continue;
            }
        };
        for run in pending {
            {
                let mut guard = in_flight.lock().expect("in_flight lock");
                if guard.contains(&run.task) {
                    continue;
                }
                guard.insert(run.task.clone());
            }
            let root = root.to_path_buf();
            let in_flight_worker = Arc::clone(&in_flight);
            let task_name = run.task.clone();
            let run_id = run.id.clone();
            let spawned = std::thread::Builder::new()
                .name(format!("bb-run-{run_id}"))
                .spawn(move || {
                    let panic = std::panic::catch_unwind(|| run_one(&root, &run_id));
                    let _ = in_flight_worker
                        .lock()
                        .expect("in_flight lock")
                        .remove(&task_name);
                    panic.unwrap_or_else(|panic| {
                        let msg = format!("{panic:?}");
                        eprintln!("dispatch: thread panic in task {task_name} run {run_id}: {msg}");
                        canary::report_error(
                            "bb.dispatch.panic",
                            &format!("task {task_name} run {run_id}: {msg}"),
                        );
                        std::panic::resume_unwind(panic);
                    });
                });
            if let Err(e) = spawned {
                eprintln!("dispatch: spawn failed: {e}");
                canary::report_error("bb.dispatch.spawn", &format!("{e:#}"));
                in_flight.lock().expect("in_flight lock").remove(&run.task);
            }
        }
    }
}

fn reflex_dispatch_paused(ledger: &Ledger) -> bool {
    match ledger.plane_paused() {
        Ok(Some(_)) => true,
        Ok(None) => false,
        Err(e) => {
            eprintln!("dispatch: pause check: {e:#}");
            canary::report_error("bb.dispatch.pause_check", &format!("{e:#}"));
            true
        }
    }
}

fn notify_loop(root: &Path) {
    loop {
        std::thread::sleep(NOTIFY_RETRY_POLL);
        let plane = match Plane::load(root) {
            Ok(plane) => plane,
            Err(e) => {
                report_runtime_error("notify retry: plane load", "bb.plane.load", &e);
                continue;
            }
        };
        if plane.spec.notify.webhook_url.is_none() {
            continue;
        }
        let ledger = match Ledger::open(&plane.db_path()) {
            Ok(ledger) => ledger,
            Err(e) => {
                report_runtime_error("notify retry: ledger open", "bb.ledger.open", &e);
                continue;
            }
        };
        match notify::retry_pending(&plane, &ledger, 20) {
            Ok(report) if report.attempted > 0 => eprintln!(
                "notify retry: attempted={} delivered={} failed={}",
                report.attempted, report.delivered, report.failed
            ),
            Err(e) => {
                eprintln!("notify retry: {e:#}");
                canary::report_error("bb.notify.retry", &format!("{e:#}"));
            }
            _ => {}
        }
    }
}

fn watchdog_loop(root: &Path) {
    loop {
        std::thread::sleep(watchdog_poll());
        let plane = match Plane::load(root) {
            Ok(plane) => plane,
            Err(e) => {
                report_runtime_error("watchdog: plane load", "bb.plane.load", &e);
                continue;
            }
        };
        let ledger = match Ledger::open(&plane.db_path()) {
            Ok(ledger) => ledger,
            Err(e) => {
                report_runtime_error("watchdog: ledger open", "bb.ledger.open", &e);
                continue;
            }
        };
        match watchdog_scan(&plane, &ledger) {
            Ok(n) if n > 0 => eprintln!("watchdog: emitted {n} stale notification(s)"),
            Err(e) => {
                eprintln!("watchdog: {e:#}");
                canary::report_error("bb.watchdog", &format!("{e:#}"));
            }
            _ => {}
        }
    }
}

fn watchdog_poll() -> Duration {
    std::env::var("BB_WATCHDOG_POLL_MS")
        .ok()
        .and_then(|v| v.parse::<u64>().ok())
        .map(Duration::from_millis)
        .unwrap_or(WATCHDOG_POLL)
}

fn watchdog_scan(plane: &Plane, ledger: &Ledger) -> Result<usize> {
    if plane.spec.notify.webhook_url.is_none() {
        return Ok(0);
    }
    let now = OffsetDateTime::now_utc();
    let mut emitted = 0usize;
    for state in ["running", "awaiting_recovery"] {
        for run in ledger.runs_in_state(state)? {
            let view = progress::from_ledger(ledger, &run, now)?;
            let Some(escalation) = watchdog_escalation(&run, &view, now) else {
                continue;
            };
            if watchdog_notified(ledger, &run.id, &escalation.key)? {
                continue;
            }
            notify::notify(
                plane,
                ledger,
                escalation.event,
                &serde_json::json!({
                    "run_id": run.id.clone(),
                    "task": run.task.clone(),
                    "classification": view.classification,
                    "attempt_phase": view.attempt_phase.clone(),
                    "last_progress_at": view.last_progress_at.clone(),
                    "age_seconds": escalation.age_seconds,
                    "threshold_seconds": escalation.threshold_seconds,
                    "severity": escalation.severity,
                    "safe_next_action": &view.safe_next_action,
                    "stale_key": escalation.key.clone(),
                }),
            );
            ledger.record_event(&run.id, "watchdog:stale_notified", Some(&escalation.key))?;
            ledger.record_guard_event(
                escalation.event,
                Some(&run.task),
                &format!("run={} classification={}", run.id, view.classification),
                1,
            )?;
            emitted += 1;
        }
    }
    Ok(emitted)
}

struct WatchdogEscalation {
    event: &'static str,
    key: String,
    age_seconds: Option<i64>,
    threshold_seconds: i64,
    severity: &'static str,
}

fn watchdog_escalation(
    run: &crate::ledger::RunRow,
    view: &progress::ProgressView,
    now: OffsetDateTime,
) -> Option<WatchdogEscalation> {
    if view.classification == "stale_executing" {
        let freshness = view.last_progress_at.as_deref().unwrap_or(&run.created_at);
        return Some(WatchdogEscalation {
            event: "run_stale_executing",
            key: format!(
                "stale_executing:{}:{}:{}",
                run.id,
                view.attempt_phase.as_deref().unwrap_or("-"),
                freshness
            ),
            age_seconds: view.age_seconds,
            threshold_seconds: view.threshold_seconds,
            severity: "critical",
        });
    }
    if run.state == "awaiting_recovery" {
        let Ok(updated_at) = OffsetDateTime::parse(&run.updated_at, &Rfc3339) else {
            return None;
        };
        let age = (now - updated_at).whole_seconds().max(0);
        if age >= progress::RECOVERY_STALE_SECONDS {
            return Some(WatchdogEscalation {
                event: "run_stale_recovery",
                key: format!("stale_recovery:{}:{}", run.id, run.updated_at),
                age_seconds: Some(age),
                threshold_seconds: progress::RECOVERY_STALE_SECONDS,
                severity: "critical",
            });
        }
    }
    None
}

fn watchdog_notified(ledger: &Ledger, run_id: &str, key: &str) -> Result<bool> {
    Ok(ledger
        .events(run_id)?
        .iter()
        .any(|event| event.kind == "watchdog:stale_notified" && event.data.as_deref() == Some(key)))
}

fn run_one(root: &Path, run_id: &str) {
    let result = (|| -> Result<()> {
        let plane = Plane::load(root)?;
        let mut ledger = Ledger::open(&plane.db_path())?;
        let run = dispatch::dispatch_run(&plane, &mut ledger, run_id)?;
        if cfg!(debug_assertions)
            && std::env::var("BB_TEST_PANIC_AFTER_RUN_ID").as_deref() == Ok(run_id)
        {
            panic!("BB_TEST_PANIC_AFTER_RUN_ID");
        }
        eprintln!("run {} {} (task={})", run.id, run.state, run.task);
        Ok(())
    })();
    if let Err(e) = result {
        let msg = format!("{e:#}");
        eprintln!("run {run_id}: {msg}");
        canary::report_error("bb.run.dispatch", &format!("run {run_id}: {msg}"));
    }
}

fn report_runtime_error(scope: &str, class: &str, error: &anyhow::Error) {
    let detail = format!("{scope}: {error:#}");
    eprintln!("{detail}");
    canary::report_error(class, &detail);
}

fn http_loop(root: &Path, bind: &str) -> Result<()> {
    let server = tiny_http::Server::http(bind).map_err(|e| anyhow::anyhow!("bind {bind}: {e}"))?;
    eprintln!("bb serve listening on {bind}");

    for mut request in server.incoming_requests() {
        let response = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
            handle_request(root, &mut request)
        }));
        let (status, body) = match response {
            Ok(Ok(r)) => r,
            Ok(Err(e)) => {
                report_runtime_error("http request", "bb.http.request", &e);
                (500, format!("{{\"error\":\"{e}\"}}"))
            }
            Err(panic) => {
                let payload = panic
                    .downcast_ref::<&str>()
                    .copied()
                    .or_else(|| panic.downcast_ref::<String>().map(String::as_str))
                    .unwrap_or("non-string panic payload");
                let detail = format!("http request panic: {payload}");
                eprintln!("{detail}");
                canary::report_error("bb.http.panic", &detail);
                (500, "{\"error\":\"internal server error\"}".to_string())
            }
        };
        let content_type: &[u8] = if body.starts_with("<!doctype") {
            b"text/html; charset=utf-8"
        } else {
            b"application/json"
        };
        let _ = request.respond(
            tiny_http::Response::from_string(body)
                .with_status_code(status)
                .with_header(
                    tiny_http::Header::from_bytes(&b"Content-Type"[..], content_type)
                        .expect("static header"),
                ),
        );
    }
    Ok(())
}

fn ingress_bind(configured: &str) -> String {
    std::env::var(INGRESS_BIND_ENV).unwrap_or_else(|_| configured.to_string())
}

fn enforce_public_bind_token(bind: &str) -> Result<()> {
    if bind_is_loopback(bind) || api_token().is_some() {
        return Ok(());
    }
    anyhow::bail!("{API_TOKEN_ENV} must be set before binding non-loopback address {bind}");
}

fn bind_is_loopback(bind: &str) -> bool {
    if let Ok(addr) = bind.parse::<std::net::SocketAddr>() {
        return addr.ip().is_loopback();
    }
    bind.rsplit_once(':')
        .map(|(host, _)| host == "localhost")
        .unwrap_or(bind == "localhost")
}

fn api_token() -> Option<String> {
    std::env::var(API_TOKEN_ENV).ok().filter(|t| !t.is_empty())
}
fn read_authorized(request: &tiny_http::Request) -> bool {
    let Some(token) = api_token() else {
        return true;
    };
    request.headers().iter().any(|h| {
        h.field
            .as_str()
            .as_str()
            .eq_ignore_ascii_case("authorization")
            && bearer_matches(h.value.as_str(), &token)
    })
}

fn bearer_matches(value: &str, token: &str) -> bool {
    let Some(found) = value.strip_prefix("Bearer ") else {
        return false;
    };
    found.len() == token.len() && found.as_bytes().ct_eq(token.as_bytes()).into()
}

fn query_param(url: &str, name: &str) -> Option<String> {
    let (_, query) = url.split_once('?')?;
    query.split('&').find_map(|kv| {
        let (k, v) = kv.split_once('=')?;
        (k == name && !v.is_empty()).then(|| v.to_string())
    })
}

fn json_error(message: String) -> String {
    serde_json::json!({ "error": message }).to_string()
}

fn read_capped_body(
    request: &mut tiny_http::Request,
    max_body_bytes: usize,
) -> Result<std::result::Result<String, usize>> {
    let mut body = String::new();
    request
        .as_reader()
        .take(max_body_bytes.saturating_add(1) as u64)
        .read_to_string(&mut body)
        .context("read body")?;
    if body.len() > max_body_bytes {
        return Ok(Err(body.len()));
    }
    Ok(Ok(body))
}

fn body_too_large(bytes: usize, max_body_bytes: usize) -> (u16, String) {
    (
        413,
        serde_json::json!({
            "error": format!("request body {bytes} bytes exceeds max_body_bytes {max_body_bytes}")
        })
        .to_string(),
    )
}

#[derive(serde::Deserialize)]
struct ExternalRunPatch {
    status: String,
    completed_at: Option<String>,
}

fn handle_request(root: &Path, request: &mut tiny_http::Request) -> Result<(u16, String)> {
    let method = request.method().to_string();
    let url = request.url().to_string();

    if method == "GET" && url == "/favicon.ico" {
        return Ok((204, String::new()));
    }

    if method == "GET" && (url == "/" || url.starts_with("/?")) {
        return Ok((200, include_str!("operator.html").into()));
    }

    if method == "GET" && url.starts_with("/api/") {
        if !read_authorized(request) {
            return Ok((401, "{\"error\":\"missing or bad bearer token\"}".into()));
        }
        let plane = Plane::load(root)?;
        let ledger = Ledger::open(&plane.db_path())?;
        let path = url.split('?').next().unwrap_or(&url);
        return match path {
            "/api/runs" => {
                let task = query_param(&url, "task");
                let state = query_param(&url, "state");
                Ok((
                    200,
                    serde_json::to_string(&runs_view(&ledger, task.as_deref(), state.as_deref())?)?,
                ))
            }
            "/api/status" => Ok((
                200,
                serde_json::to_string(&crate::health::status_view(&plane, &ledger)?)?,
            )),
            "/api/external-runs" => {
                let limit = query_param(&url, "limit")
                    .and_then(|s| s.parse::<i64>().ok())
                    .unwrap_or(50);
                Ok((
                    200,
                    serde_json::to_string(&ledger.list_external_runs(limit)?)?,
                ))
            }
            "/api/dlq" => Ok((200, serde_json::to_string(&ledger.list_dead_letters()?)?)),
            "/api/notify" => {
                let limit = query_param(&url, "limit")
                    .and_then(|s| s.parse::<i64>().ok())
                    .unwrap_or(50);
                Ok((
                    200,
                    serde_json::to_string(&ledger.list_notification_outbox(limit)?)?,
                ))
            }
            "/api/leases" => Ok((200, serde_json::to_string(&ledger.list_host_leases()?)?)),
            "/api/ingress" => {
                let limit = query_param(&url, "limit")
                    .and_then(|s| s.parse::<i64>().ok())
                    .unwrap_or(50);
                Ok((
                    200,
                    serde_json::to_string(&ledger.latest_ingress_events(limit)?)?,
                ))
            }
            "/api/export" => {
                let lines = crate::telemetry::export_all(&plane, &ledger)?
                    .into_iter()
                    .map(|line| line.to_string())
                    .collect::<Vec<_>>()
                    .join("\n");
                Ok((200, format!("{lines}\n")))
            }
            "/api/submissions" => {
                let limit = query_param(&url, "limit")
                    .and_then(|s| s.parse::<i64>().ok())
                    .map(crate::submit::clamp_submission_list_limit)
                    .unwrap_or(50);
                Ok((
                    200,
                    serde_json::to_string(&ledger.list_submissions(limit)?)?,
                ))
            }
            "/api/gate" => {
                let id = match (query_param(&url, "submission"), query_param(&url, "change")) {
                    (Some(id), _) => id,
                    (None, Some(change)) => match ledger.latest_submission(&change)? {
                        Some(sub) => sub.id,
                        None => return Ok((404, "{\"error\":\"no submissions\"}".into())),
                    },
                    (None, None) => {
                        return Ok((400, "{\"error\":\"pass submission= or change=\"}".into()))
                    }
                };
                Ok((
                    200,
                    serde_json::to_string(&gate_view(&plane, &ledger, Some(&id), None)?)?,
                ))
            }
            "/api/tasks" => Ok((200, serde_json::to_string(&tasks_view(&plane, &ledger)?)?)),
            _ => {
                if let Some(id) = path.strip_prefix("/api/runs/") {
                    Ok((200, run_view(&ledger, id)?.to_string()))
                } else if let Some(id) = path.strip_prefix("/api/external-runs/") {
                    Ok((200, serde_json::to_string(&ledger.external_run(id)?)?))
                } else {
                    Ok((404, "{\"error\":\"not found\"}".into()))
                }
            }
        };
    }

    if (method == "POST" && url.split('?').next() == Some("/api/external-runs"))
        || (method == "PATCH"
            && url
                .split('?')
                .next()
                .unwrap_or("")
                .starts_with("/api/external-runs/"))
    {
        if !read_authorized(request) {
            return Ok((401, "{\"error\":\"missing or bad bearer token\"}".into()));
        }
        let plane = Plane::load(root)?;
        let ledger = Ledger::open(&plane.db_path())?;
        let path = url.split('?').next().unwrap_or(&url);
        let body = match read_capped_body(request, plane.spec.ingress.max_body_bytes)? {
            Ok(body) => body,
            Err(bytes) => return Ok(body_too_large(bytes, plane.spec.ingress.max_body_bytes)),
        };
        if method == "POST" {
            let input: ExternalRunCreate = match serde_json::from_str(&body) {
                Ok(input) => input,
                Err(err) => return Ok((400, json_error(format!("invalid json: {err}")))),
            };
            return match ledger.create_external_run(input) {
                Ok(row) => Ok((201, serde_json::to_string(&row)?)),
                Err(err) => Ok((400, json_error(err.to_string()))),
            };
        }
        let Some(id) = path
            .strip_prefix("/api/external-runs/")
            .filter(|s| !s.is_empty())
        else {
            return Ok((404, "{\"error\":\"not found\"}".into()));
        };
        let patch: ExternalRunPatch = match serde_json::from_str(&body) {
            Ok(patch) => patch,
            Err(err) => return Ok((400, json_error(format!("invalid json: {err}")))),
        };
        return match ledger.update_external_run(id, &patch.status, patch.completed_at.as_deref()) {
            Ok(row) => Ok((200, serde_json::to_string(&row)?)),
            Err(err) => {
                let status = if err.to_string().contains("not found") {
                    404
                } else {
                    400
                };
                Ok((status, json_error(err.to_string())))
            }
        };
    }

    if method == "GET" && url == "/health" {
        let plane = Plane::load(root)?;
        let ledger = Ledger::open(&plane.db_path())?;
        let pending = ledger.runs_in_state("pending")?;
        let running = ledger.runs_in_state("running")?;
        let oldest_pending = pending.last().map(|r| r.created_at.clone());
        return Ok((
            200,
            serde_json::json!({
                "pending": pending.len(),
                "running": running.len(),
                "oldest_pending": oldest_pending,
            })
            .to_string(),
        ));
    }

    if method == "POST" {
        if let Some(route) = url.strip_prefix("/hooks/") {
            let route = route.trim_end_matches('/').to_string();
            let headers: Vec<(String, String)> = request
                .headers()
                .iter()
                .map(|h| {
                    (
                        h.field.as_str().as_str().to_string(),
                        h.value.as_str().to_string(),
                    )
                })
                .collect();
            let plane = Plane::load(root)?;
            if ingress::webhook_target(&plane, &route).is_none() {
                return Ok((404, format!("{{\"error\":\"no webhook route '{route}'\"}}")));
            }
            let body = match read_capped_body(request, plane.spec.ingress.max_body_bytes)? {
                Ok(body) => body,
                Err(bytes) => {
                    let ledger = Ledger::open(&plane.db_path())?;
                    if let Some((task, _)) = ingress::webhook_target(&plane, &route) {
                        ledger.record_guard_event(
                            "ingress_oversized",
                            Some(&task.name),
                            &format!(
                                "route={route} bytes={bytes} max={}",
                                plane.spec.ingress.max_body_bytes
                            ),
                            1,
                        )?;
                    }
                    return Ok(body_too_large(bytes, plane.spec.ingress.max_body_bytes));
                }
            };
            let mut ledger = Ledger::open(&plane.db_path())?;
            let resp = ingress::handle_webhook(&plane, &mut ledger, &route, &headers, &body)?;
            return Ok((resp.status, resp.body));
        }
    }

    Ok((404, "{\"error\":\"not found\"}".to_string()))
}

pub fn runs_view(
    ledger: &Ledger,
    task: Option<&str>,
    state: Option<&str>,
) -> Result<serde_json::Value> {
    Ok(serde_json::to_value(ledger.list_runs(task, state)?)?)
}

pub fn run_view(ledger: &Ledger, run_id: &str) -> Result<serde_json::Value> {
    let run = ledger.run(run_id)?;
    let attempts = ledger.attempts(run_id)?;
    let events = ledger.events(run_id)?;
    let progress = progress::from_ledger(ledger, &run, OffsetDateTime::now_utc())?;
    Ok(serde_json::json!({
        "run": run,
        "attempts": attempts,
        "events": events,
        "progress": progress,
    }))
}

pub fn gate_view(
    plane: &Plane,
    ledger: &Ledger,
    submission: Option<&str>,
    change: Option<&str>,
) -> Result<crate::submit::GateReport> {
    let id = match (submission, change) {
        (Some(id), _) => id.to_string(),
        (None, Some(change)) => {
            ledger
                .latest_submission(change)?
                .ok_or_else(|| anyhow::anyhow!("no submissions for change '{change}'"))?
                .id
        }
        (None, None) => bail!("pass submission or change"),
    };
    crate::submit::evaluate(plane, ledger, &id)
}

pub fn tasks_view(plane: &Plane, ledger: &Ledger) -> Result<Vec<serde_json::Value>> {
    let mut out = Vec::new();
    for task in plane.tasks.values() {
        out.push(serde_json::json!({
            "task": task.name,
            "agent": format!("{}@v{}", task.agent_name, task.agent.version),
            "agent_role": task.agent.role,
            "agent_skills": task.agent.skills,
            "harness": task.agent.harness,
            "model": task.agent.model,
            "substrate": task.spec.substrate,
            "triggers": task.spec.triggers.len(),
            "trigger_details": task.spec.triggers.iter().map(trigger_view).collect::<Vec<_>>(),
            "verdict": task.spec.verdict,
            "source": task.source,
            "roster": task.roster,
            "parked": ledger.parked_reason(&task.name)?,
            "runs_today": ledger.runs_today(&task.name)?,
            "max_runs_per_day": task.spec.budget.max_runs_per_day,
            "max_cost_per_run_usd": task.spec.budget.max_cost_per_run_usd,
            "timeout_minutes": task.spec.budget.timeout_minutes,
            "turn_cap": task.spec.budget.turn_cap,
            "tool_action_cap": task.spec.budget.tool_action_cap,
            "output_bytes_cap": task.spec.budget.output_bytes_cap,
            "policy": serde_json::to_value(&task.agent.policy)?,
            "provider_key": crate::provider_keys::local_status_for_task(plane, task)?,
        }));
    }
    Ok(out)
}
fn trigger_view(trigger: &TriggerSpec) -> serde_json::Value {
    match trigger {
        TriggerSpec::Manual => serde_json::json!({"kind": "manual"}),
        TriggerSpec::Cron { schedule } => serde_json::json!({
            "kind": "cron",
            "schedule": schedule,
        }),
        TriggerSpec::Webhook {
            route,
            secret_env,
            dedupe_key,
            action,
            filter,
        } => serde_json::json!({
            "kind": "webhook",
            "route": route,
            "secret_env": secret_env,
            "dedupe_key": dedupe_key,
            "action": action.as_ref().map(webhook_action_view),
            "filters": filter.iter().map(filter_view).collect::<Vec<_>>(),
        }),
    }
}

fn webhook_action_view(action: &crate::spec::WebhookActionSpec) -> serde_json::Value {
    match action {
        crate::spec::WebhookActionSpec::SubmissionStorm {
            change,
            rev,
            repo,
            version,
        } => serde_json::json!({
            "kind": "submission_storm",
            "change": change,
            "rev": rev,
            "repo": repo,
            "version": version,
        }),
    }
}

fn filter_view(filter: &crate::spec::FilterSpec) -> serde_json::Value {
    serde_json::json!({
        "pointer": &filter.pointer,
        "equals": &filter.equals,
        "any_of": &filter.any_of,
        "not_any_of": &filter.not_any_of,
        "max": filter.max,
    })
}

/// Config-surface snapshot shared by `bb check --json`, the MCP `bb_check`
/// tool, and future API routes. Read-only; same shape everywhere so MCP
/// never builds its own check/status shapes (backlog 078 oracle).
pub fn check_view(plane: &Plane, ledger: &Ledger) -> Result<serde_json::Value> {
    let mut agent_policy = serde_json::Map::new();
    for (name, agent) in &plane.agents {
        agent_policy.insert(name.clone(), serde_json::to_value(&agent.policy)?);
    }
    Ok(serde_json::json!({
        "root": plane.root,
        "db_path": plane.db_path(),
        "ledger": {
            "schema_version": ledger.schema_version()?,
            "supported_schema_version": LEDGER_SCHEMA_VERSION,
        },
        "backup": &plane.spec.backup,
        "agents": plane.agents.keys().collect::<Vec<_>>(),
        "agent_policy": agent_policy,
        "provider_keys": crate::provider_keys::local_statuses(plane)?,
        "tasks": plane.tasks.keys().collect::<Vec<_>>(),
        "task_details": tasks_view(plane, ledger)?,
    }))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::ledger::IngressRequest;
    use chrono::TimeZone;
    use std::fs;
    use time::{format_description::well_known::Rfc3339, Duration as TimeDuration};

    static NOTIFY_ENV_LOCK: std::sync::Mutex<()> = std::sync::Mutex::new(());

    fn guard_total(ledger: &Ledger, kind: &str) -> i64 {
        ledger
            .guard_event_counts()
            .unwrap()
            .into_iter()
            .find(|c| c.kind == kind)
            .map(|c| c.total)
            .unwrap_or(0)
    }

    fn watchdog_plane(root: &Path) -> Plane {
        fs::create_dir_all(root.join("agents")).unwrap();
        fs::create_dir_all(root.join("tasks/demo")).unwrap();
        fs::write(
            root.join("plane.toml"),
            "dev = true\n[notify]\nwebhook_url = \"http://example.invalid/hook\"\n",
        )
        .unwrap();
        fs::write(
            root.join("agents/a.toml"),
            "version = 1\nharness = \"command\"\nmodel = \"\"\nbin = \"true\"\n",
        )
        .unwrap();
        fs::write(root.join("tasks/demo/card.md"), "card\n").unwrap();
        fs::write(
            root.join("tasks/demo/task.toml"),
            "agent = \"a\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
        )
        .unwrap();
        Plane::load(root).unwrap()
    }

    fn stale_executing_run(ledger: &mut Ledger, plane: &Plane) -> String {
        let run_id = ledger
            .ingest(IngressRequest {
                task: "demo",
                trigger_kind: "manual",
                idempotency_key: None,
                source_event_id: None,
                payload: Some("{}"),
                parent_run_id: None,
            })
            .unwrap()
            .run_id;
        ledger.transition(&run_id, "running", None).unwrap();
        let attempt = ledger
            .create_attempt(&run_id, 1, "a", 1, "command", "")
            .unwrap();
        ledger.set_attempt_phase(attempt, "executing").unwrap();
        ledger.record_progress(&run_id, "phase:executing").unwrap();
        let stale_at = (OffsetDateTime::now_utc()
            - TimeDuration::seconds(progress::PROGRESS_STALE_SECONDS + 60))
        .format(&Rfc3339)
        .unwrap();
        rusqlite::Connection::open(plane.db_path())
            .unwrap()
            .execute(
                "UPDATE run_events SET at = ?1 WHERE run_id = ?2 AND kind = 'progress'",
                rusqlite::params![stale_at, run_id],
            )
            .unwrap();
        run_id
    }

    #[test]
    fn reflex_dispatch_pause_check_fails_closed_on_ledger_error() {
        let dir = tempfile::tempdir().unwrap();
        let db = dir.path().join("ledger.sqlite");
        let ledger = Ledger::open(&db).unwrap();
        rusqlite::Connection::open(&db)
            .unwrap()
            .execute("DROP TABLE plane_pause", [])
            .unwrap();

        assert!(reflex_dispatch_paused(&ledger));
    }

    #[test]
    fn cron_tick_does_not_rewind_successful_tasks_when_peer_fails() {
        let dir = tempfile::tempdir().unwrap();
        let plane = watchdog_plane(dir.path());
        let db = dir.path().join("ledger.sqlite");
        let mut ledger = Ledger::open(&db).unwrap();
        rusqlite::Connection::open(&db)
            .unwrap()
            .execute_batch(
                "CREATE TRIGGER fail_bad_ingress
                 BEFORE INSERT ON ingress_events
                 WHEN NEW.task = 'bad'
                 BEGIN
                   SELECT RAISE(FAIL, 'simulated bad-task cron failure');
                 END;",
            )
            .unwrap();

        let schedules = vec![
            (
                "good".to_string(),
                ingress::parse_schedule("* * * * *").unwrap(),
            ),
            (
                "bad".to_string(),
                ingress::parse_schedule("* * * * *").unwrap(),
            ),
        ];
        let mut last_by_task = HashMap::new();
        let mut default_last = Utc.with_ymd_and_hms(2026, 6, 10, 12, 0, 0).unwrap();
        let first = Utc.with_ymd_and_hms(2026, 6, 10, 12, 6, 0).unwrap();
        let second = Utc.with_ymd_and_hms(2026, 6, 10, 12, 7, 0).unwrap();

        run_cron_tick(
            &plane,
            &mut ledger,
            &schedules,
            &mut last_by_task,
            &mut default_last,
            first,
            2,
        );
        assert_eq!(ledger.ingress_event_count("good").unwrap(), 2);
        assert_eq!(ledger.ingress_event_count("bad").unwrap(), 0);
        assert_eq!(guard_total(&ledger, "cron_collapse"), 4);

        run_cron_tick(
            &plane,
            &mut ledger,
            &schedules,
            &mut last_by_task,
            &mut default_last,
            second,
            2,
        );
        assert_eq!(ledger.ingress_event_count("good").unwrap(), 3);
        assert_eq!(ledger.ingress_event_count("bad").unwrap(), 0);
        assert_eq!(guard_total(&ledger, "cron_collapse"), 4);
    }

    #[test]
    fn watchdog_escalates_stale_executing_once_through_notification_outbox() {
        let _guard = NOTIFY_ENV_LOCK.lock().unwrap();
        std::env::set_var("BB_NOTIFY_BIN", "true");
        let dir = tempfile::tempdir().unwrap();
        let plane = watchdog_plane(dir.path());
        let mut ledger = Ledger::open(&plane.db_path()).unwrap();
        let run_id = stale_executing_run(&mut ledger, &plane);

        assert_eq!(watchdog_scan(&plane, &ledger).unwrap(), 1);
        let rows = ledger.list_notification_outbox(10).unwrap();
        assert_eq!(rows.len(), 1);
        assert_eq!(rows[0].event, "run_stale_executing");
        assert_eq!(rows[0].status, "delivered");
        assert_eq!(rows[0].attempts, 1);
        assert_eq!(guard_total(&ledger, "run_stale_executing"), 1);
        assert!(ledger.events(&run_id).unwrap().iter().any(|e| {
            e.kind == "watchdog:stale_notified"
                && e.data
                    .as_deref()
                    .is_some_and(|data| data.starts_with("stale_executing:"))
        }));

        assert_eq!(watchdog_scan(&plane, &ledger).unwrap(), 0);
        assert_eq!(ledger.list_notification_outbox(10).unwrap().len(), 1);
        assert_eq!(guard_total(&ledger, "run_stale_executing"), 1);
        std::env::remove_var("BB_NOTIFY_BIN");
    }
}
