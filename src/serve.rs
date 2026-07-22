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
use crate::ledger::{ExternalRunCreate, IngressRequest, Ledger, LEDGER_SCHEMA_VERSION};
use crate::recovery;
use crate::spec::{AuthClass, Plane, TriggerSpec};
use crate::{canary, dispatch, notify, progress, workflow};

const DISPATCH_POLL: Duration = Duration::from_millis(500);
const CRON_POLL: Duration = Duration::from_secs(10);
const NOTIFY_RETRY_POLL: Duration = Duration::from_secs(60);
const WATCHDOG_POLL: Duration = Duration::from_secs(60);
const INGRESS_BIND_ENV: &str = "BB_INGRESS_BIND";
const API_TOKEN_ENV: &str = "BB_API_TOKEN";
/// Backlog bitterblossom-926: test-only escape hatch that reports the real
/// bound port after `tiny_http` binds it, so a test caller can request an
/// OS-assigned port (`BB_INGRESS_BIND=127.0.0.1:0`) and read back which one
/// it actually got, instead of pre-choosing a port on a throwaway listener
/// and hoping nothing else claims it before this process rebinds it -- the
/// exact TOCTOU window that caused the port-binding race flake.
const REPORT_PORT_FILE_ENV: &str = "BB_INGRESS_REPORT_PORT_FILE";

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
    // Workflow runs inherited mid-execution are classified, never blindly
    // re-executed: a step attempt may already have external side effects.
    for r in &crate::workflow_runtime::recover_inherited_workflow_runs(&plane, &ledger)? {
        eprintln!(
            "recovered workflow run {} workflow={} step={} -> {}",
            r.run_id,
            r.workflow,
            r.step.as_deref().unwrap_or("-"),
            r.disposition,
        );
        notify::notify(
            &plane,
            &ledger,
            "workflow_run_recovered_at_boot",
            &serde_json::json!({
                "workflow_run_id": r.run_id,
                "workflow": r.workflow,
                "step": r.step,
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
            .name("bb-workflow".into())
            .spawn(move || workflow_runner_loop(&root))?;
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
    let mut wf_default_last = Utc::now();
    let mut wf_last_by = HashMap::new();
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
        {
            let now = Utc::now();
            match crate::workflow_runtime::workflow_cron_tick(
                &plane,
                &ledger,
                &mut wf_last_by,
                wf_default_last,
                now,
                plane.spec.ingress.max_cron_catchup_fires,
            ) {
                Ok(outcomes) => {
                    for outcome in &outcomes {
                        match outcome.disposition {
                            crate::workflow_runtime::CronDisposition::Accepted => eprintln!(
                                "cron: workflow {} accepted schedule fire {}",
                                outcome.workflow, outcome.scheduled
                            ),
                            crate::workflow_runtime::CronDisposition::Denied => eprintln!(
                                "cron: workflow {} denied schedule fire {} ({})",
                                outcome.workflow,
                                outcome.scheduled,
                                outcome.detail.as_deref().unwrap_or("admission denied")
                            ),
                            crate::workflow_runtime::CronDisposition::Suppressed => eprintln!(
                                "cron: workflow {} suppressed schedule fire {} ({})",
                                outcome.workflow,
                                outcome.scheduled,
                                outcome.detail.as_deref().unwrap_or("workflow suppressed")
                            ),
                            crate::workflow_runtime::CronDisposition::Duplicate => {}
                        }
                    }
                    let denied = outcomes
                        .iter()
                        .filter(|outcome| {
                            outcome.disposition == crate::workflow_runtime::CronDisposition::Denied
                        })
                        .count();
                    if denied > 0 {
                        eprintln!("cron: denied {denied} workflow schedule fire(s)");
                    }
                    wf_default_last = now;
                }
                Err(e) => {
                    eprintln!("cron: workflows: {e:#}");
                    canary::report_error("bb.cron.workflows", &format!("{e:#}"));
                }
            }
        }
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
    let mut grouped: HashMap<String, Vec<&Schedule>> = HashMap::new();
    for (task, schedule) in schedules {
        grouped.entry(task.clone()).or_default().push(schedule);
    }
    let mut seen = HashSet::new();
    // Catch up all triggers for one task before advancing its one cursor.
    // Advancing once per trigger silently drops later trigger windows.
    for (task, task_schedules) in grouped {
        seen.insert(task.clone());
        let last = *last_by_task.entry(task.clone()).or_insert(*default_last);
        match ingress::cron_catchup_guarded_multi(
            plane,
            ledger,
            &task,
            &task_schedules,
            last,
            now,
            max_fires,
        ) {
            Ok(o) => {
                if o.skipped > 0 {
                    eprintln!(
                        "cron: task {task} refused/collapsed {} skipped fires",
                        o.skipped
                    );
                }
                last_by_task.insert(task, now);
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

/// Drain queued workflow runs to terminal states. Sequential by design in
/// v1: the queued->running claim is a CAS, so this loop and an operator's
/// `bb workflow execute` can never both execute one run group. A paused
/// plane halts this loop exactly like reflex dispatch.
///
/// The thread must outlive any panic: the executor already converts its own
/// panics to `failed` runs, and this shield keeps anything else (plane load,
/// ledger, notify) from silently killing the only workflow runner.
fn workflow_runner_loop(root: &Path) {
    loop {
        std::thread::sleep(DISPATCH_POLL);
        let tick =
            std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| workflow_runner_tick(root)));
        if tick.is_err() {
            eprintln!("workflow runner: tick panicked; runner thread survives");
            canary::report_error("bb.workflow.panic", "workflow runner tick panicked");
        }
    }
}

fn workflow_runner_tick(root: &Path) {
    let plane = match Plane::load(root) {
        Ok(plane) => plane,
        Err(e) => {
            report_runtime_error("workflow runner: plane load", "bb.plane.load", &e);
            return;
        }
    };
    let ledger = match Ledger::open(&plane.db_path()) {
        Ok(ledger) => ledger,
        Err(e) => {
            report_runtime_error("workflow runner: ledger open", "bb.ledger.open", &e);
            return;
        }
    };
    if reflex_dispatch_paused(&ledger) {
        return;
    }
    let ids = match ledger.queued_workflow_run_ids() {
        Ok(ids) => ids,
        Err(e) => {
            eprintln!("workflow runner: {e:#}");
            canary::report_error("bb.workflow.queued", &format!("{e:#}"));
            return;
        }
    };
    use crate::workflow_runtime::ExecutionDisposition;
    for id in ids {
        match crate::workflow_runtime::execute_if_queued(&plane, &ledger, &id) {
            Ok(ExecutionDisposition::Executed(view)) => eprintln!(
                "workflow run {id} {}",
                view["status"]["state"].as_str().unwrap_or("-")
            ),
            // claimed elsewhere between listing and CAS
            Ok(ExecutionDisposition::ClaimLost) => {}
            Ok(ExecutionDisposition::Deferred { reason }) => {
                eprintln!("workflow run {id} deferred: {reason}");
            }
            Err(e) => {
                eprintln!("workflow run {id}: {e:#}");
                canary::report_error("bb.workflow.execute", &format!("run {id}: {e:#}"));
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
    report_bound_port(&server);
    install_graceful_shutdown_handler();

    // Poll with a bounded timeout rather than blocking forever on
    // `incoming_requests()` so a SIGTERM (recorded by the signal handler
    // above, itself async-signal-safe -- only an atomic store) is noticed
    // and the loop returns Ok(()) instead of the process dying mid-signal.
    // That matters beyond graceful operator restarts: an instrumented-
    // coverage build only flushes its .profraw on a normal process exit, not
    // on an unhandled-signal termination (verified empirically), so a test
    // harness that sends SIGTERM instead of SIGKILL now gets real coverage
    // credit for whatever this loop executed.
    while !shutdown_requested() {
        let request = match server.recv_timeout(SHUTDOWN_POLL_INTERVAL) {
            Ok(Some(request)) => request,
            Ok(None) => continue,
            Err(e) => {
                report_runtime_error("http accept", "bb.http.accept", &anyhow::anyhow!(e));
                continue;
            }
        };
        handle_one_request(root, request);
    }
    eprintln!("bb serve: SIGTERM received, shutting down");
    Ok(())
}

fn handle_one_request(root: &Path, mut request: tiny_http::Request) {
    let response = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
        handle_request(root, &mut request)
    }));
    let (status, body) = match response {
        Ok(Ok(r)) => r,
        Ok(Err(e)) => {
            if crate::ledger::is_queue_backpressure(&e) {
                (429, json_error(e.to_string()))
            } else if let Some(client) = e.downcast_ref::<ingress::IngressClientError>() {
                (400, json_error(client.to_string()))
            } else {
                report_runtime_error("http request", "bb.http.request", &e);
                (500, json_error(e.to_string()))
            }
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

const SHUTDOWN_POLL_INTERVAL: Duration = Duration::from_millis(200);
static SHUTDOWN_REQUESTED: std::sync::atomic::AtomicBool =
    std::sync::atomic::AtomicBool::new(false);

fn shutdown_requested() -> bool {
    SHUTDOWN_REQUESTED.load(std::sync::atomic::Ordering::SeqCst)
}

extern "C" fn on_sigterm(_signal: libc::c_int) {
    // Async-signal-safe: an atomic store is the only thing done here.
    SHUTDOWN_REQUESTED.store(true, std::sync::atomic::Ordering::SeqCst);
}

/// Graceful shutdown, not just an operator nicety: `bb serve` previously had
/// no way to exit other than a bind/panic error or an unhandled signal, and
/// coverage instrumentation only flushes on a normal exit -- see
/// docs/coverage-ratchet.md and bitterblossom-930's commit for the full
/// finding (an unhandled SIGTERM behaved identically to SIGKILL for that
/// purpose, empirically verified, until this handler existed).
fn install_graceful_shutdown_handler() {
    unsafe {
        libc::signal(libc::SIGTERM, on_sigterm as *const () as libc::sighandler_t);
    }
}

fn ingress_bind(configured: &str) -> String {
    std::env::var(INGRESS_BIND_ENV).unwrap_or_else(|_| configured.to_string())
}

/// Write the real bound port to `BB_INGRESS_REPORT_PORT_FILE` when set (test
/// callers only). Written to a sibling `.tmp` path then renamed into place so
/// a concurrent reader polling for the file never observes a partial write.
/// Silently a no-op for a non-IP listener (unix socket) or when the env var
/// is unset -- production binds a fixed configured address and never sets it.
fn report_bound_port(server: &tiny_http::Server) {
    let Ok(path) = std::env::var(REPORT_PORT_FILE_ENV) else {
        return;
    };
    let tiny_http::ListenAddr::IP(addr) = server.server_addr() else {
        return;
    };
    let tmp_path = format!("{path}.tmp");
    if std::fs::write(&tmp_path, addr.port().to_string()).is_ok() {
        let _ = std::fs::rename(&tmp_path, &path);
    }
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

/// Extract a presented bearer value regardless of what it should match --
/// used by the ask/answer routes, whose valid token is per-run, not the
/// plane-wide `BB_API_TOKEN` `read_authorized` checks against.
fn presented_bearer(request: &tiny_http::Request) -> Option<String> {
    request.headers().iter().find_map(|h| {
        h.field
            .as_str()
            .as_str()
            .eq_ignore_ascii_case("authorization")
            .then(|| h.value.as_str().strip_prefix("Bearer ").map(str::to_string))
            .flatten()
    })
}

fn ask_token_authorized(request: &tiny_http::Request, expected: Option<&str>) -> bool {
    match (presented_bearer(request), expected) {
        (Some(presented), Some(expected)) => {
            presented.len() == expected.len()
                && presented.as_bytes().ct_eq(expected.as_bytes()).into()
        }
        _ => false,
    }
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

#[derive(serde::Deserialize)]
struct AskRaiseRequest {
    run_id: String,
    task: String,
    kind: String,
    question: String,
    #[serde(default)]
    context: Option<String>,
    blocking: bool,
    window_seconds: i64,
}

#[derive(serde::Deserialize)]
struct AskAnswerRequest {
    answer: String,
    answered_by: String,
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

    // bitterblossom-930: the HITL ask/answer surface. Raise and poll are
    // authenticated by the ask's own per-run capability token (the
    // dispatched agent's declared BB_API_TOKEN-equivalent for its own run
    // only -- it never receives the operator's global token), so this must
    // run before the generic `/api/*` GET handler below, which requires the
    // plane-wide `BB_API_TOKEN` and would otherwise 401 every ask poll.
    // Answer is operator-facing and uses that same plane-wide token.
    let path_no_query = url.split('?').next().unwrap_or(&url).to_string();
    if method == "POST" && path_no_query == "/api/asks" {
        let plane = Plane::load(root)?;
        let ledger = Ledger::open(&plane.db_path())?;
        let body = match read_capped_body(request, plane.spec.ingress.max_body_bytes) {
            Err(error) => {
                return Ok((
                    400,
                    json_error(format!("request body must be valid UTF-8: {error}")),
                ))
            }
            Ok(Ok(body)) => body,
            Ok(Err(bytes)) => return Ok(body_too_large(bytes, plane.spec.ingress.max_body_bytes)),
        };
        let req: AskRaiseRequest = match serde_json::from_str(&body) {
            Ok(req) => req,
            Err(err) => return Ok((400, json_error(format!("invalid json: {err}")))),
        };
        if !matches!(req.kind.as_str(), "question" | "decision" | "approval") {
            return Ok((
                400,
                json_error(format!(
                    "kind '{}' must be question, decision, or approval",
                    req.kind
                )),
            ));
        }
        let stored_token = ledger.run_ask_token(&req.run_id)?;
        if !ask_token_authorized(request, stored_token.as_deref()) {
            return Ok((401, json_error("bad ask_token for run_id".into())));
        }
        let id = format!("ask-{}", crate::ledger::new_id());
        let ask = ledger.raise_ask(
            &id,
            &req.run_id,
            &req.task,
            &req.kind,
            &req.question,
            req.context.as_deref(),
            req.blocking,
            req.window_seconds,
        )?;
        if let Ok(task) = plane.task(&req.task) {
            crate::glass::post_asked(
                &plane,
                &ledger,
                &req.run_id,
                &req.task,
                &task.agent_name,
                &id,
                &req.kind,
                &req.question,
            );
        }
        return Ok((201, serde_json::to_string(&ask)?));
    }
    if let Some(rest) = path_no_query.strip_prefix("/api/asks/") {
        let plane = Plane::load(root)?;
        let mut ledger = Ledger::open(&plane.db_path())?;
        if method == "GET" {
            let id = rest.to_string();
            let ask = match ledger.ask(&id) {
                Ok(ask) => ask,
                Err(_) => return Ok((404, json_error(format!("ask {id} not found")))),
            };
            let stored_token = ledger.run_ask_token(&ask.run_id)?;
            if !ask_token_authorized(request, stored_token.as_deref()) {
                return Ok((401, json_error("bad ask_token".into())));
            }
            let ask = ledger.park_ask_if_expired(&id)?;
            return Ok((200, serde_json::to_string(&ask)?));
        }
        if method == "POST" {
            let Some(id) = rest.strip_suffix("/answer") else {
                return Ok((404, "{\"error\":\"not found\"}".into()));
            };
            if !read_authorized(request) {
                return Ok((401, "{\"error\":\"missing or bad bearer token\"}".into()));
            }
            let body = match read_capped_body(request, plane.spec.ingress.max_body_bytes) {
                Err(error) => {
                    return Ok((
                        400,
                        json_error(format!("request body must be valid UTF-8: {error}")),
                    ))
                }
                Ok(Ok(body)) => body,
                Ok(Err(bytes)) => {
                    return Ok(body_too_large(bytes, plane.spec.ingress.max_body_bytes))
                }
            };
            let req: AskAnswerRequest = match serde_json::from_str(&body) {
                Ok(req) => req,
                Err(err) => return Ok((400, json_error(format!("invalid json: {err}")))),
            };
            let ask = match ledger.ask(id) {
                Ok(ask) => ask,
                Err(_) => return Ok((404, json_error(format!("ask {id} not found")))),
            };
            let run = ledger.run(&ask.run_id)?;
            let (ask, resumed_run_id) = if run.state == "parked_on_ask" {
                let packet = match crate::artifacts::read(
                    &ledger,
                    &ask.run_id,
                    dispatch::ASK_PACKET_FILENAME,
                )? {
                    crate::artifacts::ReadOutcome::Text { content, .. } => Some(content),
                    _ => None,
                };
                let resume_payload = serde_json::json!({
                    "ask": {"id": ask.id, "kind": ask.kind, "question": ask.question, "context": ask.context},
                    "answer": req.answer,
                    "answered_by": req.answered_by,
                    "packet": packet,
                })
                .to_string();
                let resume_key = format!("resume:{}", ask.id);
                let (ask, outcome) = match ledger.answer_ask_and_resume(
                    id,
                    &req.answer,
                    &req.answered_by,
                    IngressRequest {
                        task: &ask.task,
                        trigger_kind: "resume",
                        idempotency_key: Some(&resume_key),
                        source_event_id: None,
                        payload: Some(&resume_payload),
                        parent_run_id: Some(&ask.run_id),
                    },
                ) {
                    Ok(result) => result,
                    Err(err) if crate::ledger::is_queue_backpressure(&err) => return Err(err),
                    Err(err) => return Ok((409, json_error(err.to_string()))),
                };
                if let Ok(task) = plane.task(&ask.task) {
                    crate::glass::post_resumed(
                        &plane,
                        &ledger,
                        &ask.run_id,
                        &outcome.run_id,
                        &ask.task,
                        &task.agent_name,
                        &ask.id,
                    );
                }
                (ask, Some(outcome.run_id))
            } else {
                let ask = match ledger.answer_ask(id, &req.answer, &req.answered_by) {
                    Ok(ask) => ask,
                    Err(err) => return Ok((409, json_error(err.to_string()))),
                };
                (ask, None)
            };
            return Ok((
                200,
                serde_json::json!({"ask": ask, "resumed_run_id": resumed_run_id}).to_string(),
            ));
        }
    }

    if method == "GET" && url.starts_with("/api/") {
        if !read_authorized(request) {
            return Ok((401, "{\"error\":\"missing or bad bearer token\"}".into()));
        }
        let plane = Plane::load(root)?;
        let ledger = Ledger::open(&plane.db_path())?;
        let path = url.split('?').next().unwrap_or(&url);
        if let Some(response) = workflow_get(&ledger, path, &url)? {
            return Ok(response);
        }
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
            "/api/agents" => Ok((200, serde_json::to_string(&agents_view(&plane)?)?)),
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
        let body = match read_capped_body(request, plane.spec.ingress.max_body_bytes) {
            Err(error) => {
                return Ok((
                    400,
                    json_error(format!("request body must be valid UTF-8: {error}")),
                ))
            }
            Ok(Ok(body)) => body,
            Ok(Err(bytes)) => return Ok(body_too_large(bytes, plane.spec.ingress.max_body_bytes)),
        };
        if method == "POST" {
            let input: ExternalRunCreate = match serde_json::from_str(&body) {
                Ok(input) => input,
                Err(err) => return Ok((400, json_error(format!("invalid json: {err}")))),
            };
            return match ledger.create_external_run(input) {
                Ok(row) => {
                    // bitterblossom-956: an interactive/register-through run is
                    // now visible on the glass live stage the moment it
                    // registers -- same lifecycle floor a dispatched run gets,
                    // zero extra agent cooperation. Best-effort by construction.
                    crate::glass::post_external_registered(&plane, &ledger, &row);
                    Ok((201, serde_json::to_string(&row)?))
                }
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
            Ok(row) => {
                // bitterblossom-956: close the external run's glass session on
                // done/failed, reusing the session opened at registration so
                // the interactive lane reads as one continuous feed. A bare
                // `running` heartbeat patch re-announces liveness on the same
                // session. Best-effort; never fails the request.
                if row.status == "done" || row.status == "failed" {
                    crate::glass::post_external_completed(&plane, &ledger, &row);
                } else {
                    crate::glass::post_external_registered(&plane, &ledger, &row);
                }
                Ok((200, serde_json::to_string(&row)?))
            }
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

    // bitterblossom-workflow-store: the workflow configuration store's
    // mutating surface. Same plane token as the rest of `/api/*`; bodies
    // are capped like every other ingress read. All arithmetic lives in
    // src/workflow.rs so CLI and HTTP mutate the same immutable revisions.
    // `/api/workflow-runs/<id>/stop` (runtime-v1) rides the same branch.
    if method == "POST"
        && (path_no_query.starts_with("/api/workflows")
            || path_no_query.starts_with("/api/workflow-runs/"))
    {
        if !read_authorized(request) {
            return Ok((401, "{\"error\":\"missing or bad bearer token\"}".into()));
        }
        let plane = Plane::load(root)?;
        let ledger = Ledger::open(&plane.db_path())?;
        let body = match read_capped_body(request, plane.spec.ingress.max_body_bytes) {
            Err(error) => {
                return Ok((
                    400,
                    json_error(format!("request body must be valid UTF-8: {error}")),
                ))
            }
            Ok(Ok(body)) => body,
            Ok(Err(bytes)) => return Ok(body_too_large(bytes, plane.spec.ingress.max_body_bytes)),
        };
        return workflow_post(&plane, &ledger, &path_no_query, &body);
    }

    if method == "GET" && url == "/health" {
        let plane = Plane::load(root)?;
        let ledger = Ledger::open(&plane.db_path())?;
        let pending = ledger.runs_in_state("pending")?;
        let running = ledger.runs_in_state("running")?;
        let oldest_pending = pending.last().map(|r| r.created_at.clone());
        let workflow_runtime = ledger.workflow_freshness_summary(300)?;
        return Ok((
            200,
            serde_json::json!({
                "pending": pending.len(),
                "running": running.len(),
                "oldest_pending": oldest_pending,
                "workflow_runtime": workflow_runtime,
            })
            .to_string(),
        ));
    }

    if method == "POST" {
        if let Some(route) = url.strip_prefix("/hooks/") {
            let route = crate::ingress::normalize_route(route);
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
            let task_target = match ingress::webhook_target(&plane, &route) {
                Ok(target) => target,
                Err(_) => {
                    return Ok((
                        409,
                        serde_json::json!({
                            "error": "webhook route is ambiguous",
                            "route": route,
                        })
                        .to_string(),
                    ))
                }
            };
            let ledger = Ledger::open(&plane.db_path())?;
            let workflow_target =
                match crate::workflow_runtime::webhook_workflow_target(&ledger, &route) {
                    Ok(target) => target,
                    Err(error) => {
                        // Never choose one owner when active workflow routes are
                        // ambiguous. Refuse before reading or accepting the body.
                        return Ok((
                            409,
                            serde_json::json!({
                                "error": "webhook route is ambiguous",
                                "route": route,
                                "detail": error.to_string(),
                            })
                            .to_string(),
                        ));
                    }
                };
            if task_target.is_some() && workflow_target.is_some() {
                return Ok((
                    409,
                    serde_json::json!({
                        "error": "webhook route is claimed by both task and workflow",
                        "route": route,
                    })
                    .to_string(),
                ));
            }
            if task_target.is_none() && workflow_target.is_none() {
                return Ok((404, format!("{{\"error\":\"no webhook route '{route}'\"}}")));
            }
            let body = match read_capped_body(request, plane.spec.ingress.max_body_bytes) {
                Err(error) => {
                    return Ok((
                        400,
                        json_error(format!("request body must be valid UTF-8: {error}")),
                    ))
                }
                Ok(Ok(body)) => body,
                Ok(Err(bytes)) => {
                    return Ok(body_too_large(bytes, plane.spec.ingress.max_body_bytes));
                }
            };
            let mut ledger = ledger;
            let resp = if let Some((workflow, trigger)) = &workflow_target {
                ingress::handle_workflow_webhook(
                    &plane, &ledger, workflow, trigger, &headers, &body,
                )?
            } else {
                ingress::handle_webhook(&plane, &mut ledger, &route, &headers, &body)?
            };
            return Ok((resp.status, resp.body));
        }
    }

    Ok((404, "{\"error\":\"not found\"}".to_string()))
}

/// Read routes for the workflow store. Returns `None` when the path is not
/// a workflow route so the generic `/api/*` match keeps handling it.
fn workflow_get(ledger: &Ledger, path: &str, url: &str) -> Result<Option<(u16, String)>> {
    let respond = |result: Result<serde_json::Value>| -> Result<Option<(u16, String)>> {
        Ok(Some(match result {
            Ok(value) => (200, value.to_string()),
            Err(err) => (workflow_error_status(&err), json_error(format!("{err:#}"))),
        }))
    };
    if path == "/api/workflows" {
        // Route through respond() so a store error maps to a client status
        // instead of propagating as a 500 + runtime-error report.
        return respond((|| Ok(serde_json::to_value(ledger.list_workflows()?)?))());
    }
    if let Some(id) = path.strip_prefix("/api/workflow-runs/") {
        return respond(crate::workflow_runtime::run_detail_view(ledger, id));
    }
    let Some(rest) = path.strip_prefix("/api/workflows/") else {
        return Ok(None);
    };
    let (name, action) = match rest.split_once('/') {
        Some((name, action)) => (name, action),
        None => return respond(workflow::workflow_view(ledger, rest)),
    };
    if let Some(revision) = action.strip_prefix("revisions/") {
        let revision: i64 = match revision.parse() {
            Ok(revision) => revision,
            Err(_) => return Ok(Some((400, json_error("bad revision number".into())))),
        };
        return respond(workflow::revision_view(ledger, name, revision));
    }
    match action {
        "diff" => {
            let (from, to) = match (
                query_param(url, "from").and_then(|s| s.parse::<i64>().ok()),
                query_param(url, "to").and_then(|s| s.parse::<i64>().ok()),
            ) {
                (Some(from), Some(to)) => (from, to),
                _ => {
                    return Ok(Some((
                        400,
                        json_error("pass from= and to= revision numbers".into()),
                    )))
                }
            };
            respond(workflow::diff_view(ledger, name, from, to))
        }
        "export" => {
            // A malformed revision= is a 400, like the diff arm — never a
            // silent fall-through to the default revision.
            let revision = match query_param(url, "revision") {
                None => None,
                Some(raw) => match raw.parse::<i64>() {
                    Ok(revision) => Some(revision),
                    Err(_) => return Ok(Some((400, json_error("bad revision number".into())))),
                },
            };
            respond(export_workflow_toml(ledger, name, revision))
        }
        // Unknown names are 404s via respond(), not propagated 500s.
        "runs" => respond((|| Ok(serde_json::to_value(ledger.workflow_runs(name)?)?))()),
        "events" => respond((|| {
            Ok(serde_json::to_value(ledger.workflow_events(name)?)?)
        })()),
        _ => Ok(Some((404, "{\"error\":\"not found\"}".into()))),
    }
}

#[derive(serde::Deserialize)]
struct WorkflowDocRequest {
    document: workflow::WorkflowDoc,
    #[serde(default)]
    note: Option<String>,
}

fn workflow_post(plane: &Plane, ledger: &Ledger, path: &str, body: &str) -> Result<(u16, String)> {
    let respond = |result: Result<(u16, serde_json::Value)>| -> Result<(u16, String)> {
        Ok(match result {
            Ok((status, value)) => (status, value.to_string()),
            Err(err) => (workflow_error_status(&err), json_error(format!("{err:#}"))),
        })
    };
    let doc_request = |body: &str| -> Result<WorkflowDocRequest> {
        serde_json::from_str(body).map_err(|err| anyhow::anyhow!("invalid json: {err}"))
    };
    if path == "/api/workflows" {
        return respond((|| {
            let req = doc_request(body)?;
            let (wf, revision) =
                ledger.create_workflow(&req.document, "http", req.note.as_deref())?;
            Ok((
                201,
                serde_json::json!({ "workflow": wf, "revision": revision }),
            ))
        })());
    }
    if path == "/api/workflows/import" {
        return respond((|| {
            let req = doc_request(body)?;
            let (wf, revision, outcome) =
                ledger.import_workflow(&req.document, "http", req.note.as_deref())?;
            Ok((
                200,
                serde_json::json!({ "workflow": wf, "revision": revision, "outcome": outcome }),
            ))
        })());
    }
    if let Some(rest) = path.strip_prefix("/api/workflow-runs/") {
        let Some(id) = rest.strip_suffix("/stop").filter(|id| !id.is_empty()) else {
            return Ok((404, "{\"error\":\"not found\"}".into()));
        };
        return respond((|| {
            let args: serde_json::Value = if body.trim().is_empty() {
                serde_json::json!({})
            } else {
                serde_json::from_str(body).map_err(|err| anyhow::anyhow!("invalid json: {err}"))?
            };
            let reason = args
                .get("reason")
                .and_then(|v| v.as_str())
                .unwrap_or("stopped by operator");
            let status = ledger.request_workflow_run_stop(id, reason)?;
            Ok((200, serde_json::to_value(status)?))
        })());
    }
    let Some((name, action)) = path
        .strip_prefix("/api/workflows/")
        .and_then(|rest| rest.split_once('/'))
    else {
        return Ok((404, "{\"error\":\"not found\"}".into()));
    };
    let args: serde_json::Value = if body.trim().is_empty() {
        serde_json::json!({})
    } else {
        match serde_json::from_str(body) {
            Ok(value) => value,
            Err(err) => return Ok((400, json_error(format!("invalid json: {err}")))),
        }
    };
    respond((|| match action {
        "revisions" => {
            let req = doc_request(body)?;
            let (wf, revision) =
                ledger.revise_workflow(name, &req.document, "http", req.note.as_deref())?;
            Ok((
                201,
                serde_json::json!({ "workflow": wf, "revision": revision }),
            ))
        }
        "activate" => {
            let revision = match args.get("revision") {
                None => None,
                Some(serde_json::Value::Number(value)) => {
                    Some(value.as_i64().context("revision must be an integer")?)
                }
                Some(_) => bail!("revision must be an integer"),
            };
            let workflow = ledger.activate_workflow_with_reserved_routes(
                name,
                revision,
                &ingress::task_webhook_routes(plane),
            )?;
            let snapshots = workflow
                .active_revision
                .map(|revision| ledger.launch_snapshots_for_revision(&workflow.id, revision))
                .transpose()?
                .unwrap_or_default();
            let mut value = serde_json::to_value(workflow)?;
            value["launch_snapshots"] = serde_json::to_value(snapshots)?;
            Ok((200, value))
        }
        "pause" => {
            let reason = args
                .get("reason")
                .and_then(|v| v.as_str())
                .unwrap_or("paused by operator");
            Ok((
                200,
                serde_json::to_value(ledger.pause_workflow(name, reason)?)?,
            ))
        }
        "resume" => Ok((
            200,
            serde_json::to_value(ledger.resume_workflow_with_reserved_routes(
                name,
                &ingress::task_webhook_routes(plane),
            )?)?,
        )),
        "archive" => Ok((200, serde_json::to_value(ledger.archive_workflow(name)?)?)),
        "rollback" => {
            let to = args
                .get("to")
                .and_then(|v| v.as_i64())
                .context("pass {\"to\": <revision>}")?;
            let (wf, revision) = ledger.rollback_workflow_with_reserved_routes(
                name,
                to,
                &ingress::task_webhook_routes(plane),
            )?;
            Ok((
                200,
                serde_json::json!({ "workflow": wf, "revision": revision }),
            ))
        }
        "runs" => {
            let trigger_kind = args
                .get("trigger_kind")
                .and_then(|v| v.as_str())
                .unwrap_or("manual");
            let payload = match args.get("payload") {
                None | Some(serde_json::Value::Null) => None,
                Some(value) => Some(value.to_string()),
            };
            let dedupe_key = args
                .get("dedupe_key")
                .and_then(|v| v.as_str())
                .map(str::to_string);
            let outcome = crate::workflow_runtime::accept(
                plane,
                ledger,
                &crate::workflow_runtime::TriggerEnvelope {
                    workflow: name.to_string(),
                    source: crate::workflow_runtime::TriggerSource::from_kind(trigger_kind)?,
                    payload,
                    dedupe_key,
                },
            )?;
            let status = match &outcome {
                workflow::AcceptOutcome::Accepted { .. } => 201,
                workflow::AcceptOutcome::Duplicate { .. } => 200,
                workflow::AcceptOutcome::Denied { .. } => 429,
                workflow::AcceptOutcome::Suppressed { .. } => 202,
            };
            Ok((status, serde_json::to_value(outcome)?))
        }
        _ => bail!("unknown workflow action '{action}' not found"),
    })())
}

/// TOML export wrapped in a JSON envelope so the HTTP surface stays one
/// content type; the CLI prints the raw TOML.
fn export_workflow_toml(
    ledger: &Ledger,
    name: &str,
    revision: Option<i64>,
) -> Result<serde_json::Value> {
    let (revision, toml) = workflow::export_toml(ledger, name, revision)?;
    Ok(serde_json::json!({ "workflow": name, "revision": revision, "toml": toml }))
}

fn workflow_error_status(err: &anyhow::Error) -> u16 {
    if format!("{err:#}").contains("not found") {
        404
    } else {
        400
    }
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
            "admission": serde_json::to_value(&task.spec.admission)?,
            "rollout": serde_json::to_value(&task.spec.rollout)?,
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
            "archived": task.spec.archived,
        }));
    }
    Ok(out)
}

/// Every configured agent's declared contract (bitterblossom-934): harness,
/// model, secrets, skills, policy caps, where it came from (roster-
/// materialized vs inline agents/<name>.toml, with the vendored roster
/// commit sha when applicable), and which tasks bind it. Read-only
/// projection over already-loaded config, same shape for `/api/agents`,
/// `bb task list`, and any future MCP tool -- no new judgment, only exposing
/// what `Plane::load` already resolved.
pub fn agents_view(plane: &Plane) -> Result<Vec<serde_json::Value>> {
    let mut bound_tasks: std::collections::BTreeMap<&str, Vec<&str>> =
        std::collections::BTreeMap::new();
    for task in plane.tasks.values() {
        bound_tasks
            .entry(task.agent_name.as_str())
            .or_default()
            .push(task.name.as_str());
    }
    let mut out = Vec::new();
    for (name, agent) in &plane.agents {
        out.push(serde_json::json!({
            "agent": name,
            "version": agent.version,
            "role": agent.role,
            "harness": agent.harness,
            "model": agent.model,
            "provider": agent.provider(),
            "auth": agent.auth_class().ok().map(|a| match a {
                AuthClass::Subscription => "subscription",
                AuthClass::Api => "api",
            }),
            "secrets": agent.secrets,
            "checkout_secrets": agent.checkout_secrets,
            "optional_secrets": agent.optional_secrets,
            "skills": agent.skills,
            "policy": serde_json::to_value(&agent.policy)?,
            "roster": agent.roster.as_ref().map(|source| roster_provenance_view(plane, source)),
            "bound_tasks": bound_tasks.get(name.as_str()).cloned().unwrap_or_default(),
        }));
    }
    out.sort_by(|a, b| a["agent"].as_str().cmp(&b["agent"].as_str()));
    Ok(out)
}

/// The vendored roster commit sha (`vendor/roster/SOURCE`'s `Commit:` line)
/// alongside the declared roster agent identity -- the operator's own ask:
/// "from which roster identity (vendored provenance sha)".
fn roster_provenance_view(plane: &Plane, source: &crate::spec::RosterSource) -> serde_json::Value {
    let sha = std::fs::read_to_string(plane.root.join(&source.root).join("SOURCE"))
        .ok()
        .and_then(|text| {
            text.lines()
                .find_map(|line| line.strip_prefix("Commit:").map(|s| s.trim().to_string()))
        });
    serde_json::json!({
        "root": source.root,
        "agent": source.agent,
        "sha": sha,
    })
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
