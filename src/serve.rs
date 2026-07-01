use std::collections::HashSet;
use std::path::Path;
use std::sync::{Arc, Mutex};
use std::time::Duration;

use anyhow::{Context, Result};
use chrono::Utc;

use crate::ingress;
use crate::ledger::Ledger;
use crate::recovery;
use crate::spec::{Plane, TriggerSpec};
use crate::{dispatch, notify};

const DISPATCH_POLL: Duration = Duration::from_millis(500);
const CRON_POLL: Duration = Duration::from_secs(10);
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

    http_loop(&root_buf, &bind)
}

fn cron_loop(root: &Path) {
    let mut last = Utc::now();
    loop {
        std::thread::sleep(CRON_POLL);
        let Ok(plane) = Plane::load(root) else {
            continue;
        };
        let Ok(mut ledger) = Ledger::open(&plane.db_path()) else {
            continue;
        };
        let mut schedules = Vec::new();
        for task in plane.tasks.values() {
            for trigger in &task.spec.triggers {
                if let TriggerSpec::Cron { schedule } = trigger {
                    match ingress::parse_schedule(schedule) {
                        Ok(s) => schedules.push((task.name.clone(), s)),
                        Err(e) => eprintln!("cron: task {}: {e:#}", task.name),
                    }
                }
            }
        }
        let now = Utc::now();
        for (task, schedule) in &schedules {
            for fire in ingress::due_fires(schedule, last, now) {
                match ingress::ingest_cron_fire(&mut ledger, task, fire) {
                    Ok(o) if !o.duplicate => {
                        eprintln!("cron: task {task} fire {fire} -> run {}", o.run_id)
                    }
                    Ok(_) => {}
                    Err(e) => eprintln!("cron: task {task}: {e:#}"),
                }
            }
        }
        last = now;
    }
}

fn dispatch_loop(root: &Path) {
    let in_flight: Arc<Mutex<HashSet<String>>> = Arc::default();
    let Ok(plane) = Plane::load(root) else { return };
    let Ok(ledger) = Ledger::open(&plane.db_path()) else {
        return;
    };
    loop {
        std::thread::sleep(DISPATCH_POLL);
        let pending = match ledger.pending_runs_oldest_first() {
            Ok(p) => p,
            Err(e) => {
                eprintln!("dispatch: {e:#}");
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
                    panic.unwrap_or_else(|panic| std::panic::resume_unwind(panic));
                });
            if let Err(e) = spawned {
                eprintln!("dispatch: spawn failed: {e}");
                in_flight.lock().expect("in_flight lock").remove(&run.task);
            }
        }
    }
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
        eprintln!("run {run_id}: {e:#}");
    }
}

fn http_loop(root: &Path, bind: &str) -> Result<()> {
    let server = tiny_http::Server::http(bind).map_err(|e| anyhow::anyhow!("bind {bind}: {e}"))?;
    eprintln!("bb serve listening on {bind}");

    for mut request in server.incoming_requests() {
        let response = handle_request(root, &mut request);
        let (status, body) = match response {
            Ok(r) => r,
            Err(e) => (500, format!("{{\"error\":\"{e}\"}}")),
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
            && h.value.as_str() == format!("Bearer {token}")
    })
}

fn query_param(url: &str, name: &str) -> Option<String> {
    let (_, query) = url.split_once('?')?;
    query.split('&').find_map(|kv| {
        let (k, v) = kv.split_once('=')?;
        (k == name && !v.is_empty()).then(|| v.to_string())
    })
}

fn handle_request(root: &Path, request: &mut tiny_http::Request) -> Result<(u16, String)> {
    let method = request.method().to_string();
    let url = request.url().to_string();

    if method == "GET" && url == "/favicon.ico" {
        return Ok((204, String::new()));
    }

    if method == "GET" && (url.starts_with("/api/") || url == "/" || url.starts_with("/?")) {
        if !read_authorized(request) {
            return Ok((401, "{\"error\":\"missing or bad bearer token\"}".into()));
        }
        let plane = Plane::load(root)?;
        let ledger = Ledger::open(&plane.db_path())?;
        let path = url.split('?').next().unwrap_or(&url);
        return match path {
            "/" => Ok((200, include_str!("operator.html").into())),
            "/api/runs" => {
                let task = query_param(&url, "task");
                let state = query_param(&url, "state");
                let runs = ledger.list_runs(task.as_deref(), state.as_deref())?;
                Ok((200, serde_json::to_string(&runs)?))
            }
            "/api/status" => Ok((
                200,
                serde_json::to_string(&crate::health::status_view(&plane, &ledger)?)?,
            )),
            "/api/dlq" => Ok((200, serde_json::to_string(&ledger.list_dead_letters()?)?)),
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
                let report = crate::submit::evaluate(&plane, &ledger, &id)?;
                Ok((200, serde_json::to_string(&report)?))
            }
            "/api/tasks" => Ok((200, serde_json::to_string(&tasks_view(&plane, &ledger)?)?)),
            _ => {
                if let Some(id) = path.strip_prefix("/api/runs/") {
                    let run = ledger.run(id)?;
                    let body = serde_json::json!({
                        "run": run,
                        "attempts": ledger.attempts(id)?,
                        "events": ledger.events(id)?,
                    });
                    Ok((200, body.to_string()))
                } else {
                    Ok((404, "{\"error\":\"not found\"}".into()))
                }
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
            let mut body = String::new();
            request
                .as_reader()
                .read_to_string(&mut body)
                .context("read body")?;
            let plane = Plane::load(root)?;
            let mut ledger = Ledger::open(&plane.db_path())?;
            let resp = ingress::handle_webhook(&plane, &mut ledger, &route, &headers, &body)?;
            return Ok((resp.status, resp.body));
        }
    }

    Ok((404, "{\"error\":\"not found\"}".to_string()))
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
            "parked": ledger.parked_reason(&task.name)?,
            "runs_today": ledger.runs_today(&task.name)?,
            "max_runs_per_day": task.spec.budget.max_runs_per_day,
            "max_cost_per_run_usd": task.spec.budget.max_cost_per_run_usd,
            "timeout_minutes": task.spec.budget.timeout_minutes,
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
    Ok(serde_json::json!({
        "root": plane.root,
        "db_path": plane.db_path(),
        "agents": plane.agents.keys().collect::<Vec<_>>(),
        "tasks": plane.tasks.keys().collect::<Vec<_>>(),
        "task_details": tasks_view(plane, ledger)?,
    }))
}
