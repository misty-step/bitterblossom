//! `bb serve` — the resident plane: boot recovery, webhook ingress, cron
//! scheduler, and the dispatch loop. Plumbing only; every decision lives
//! in `ingress`/`dispatch`/`recovery`, which run identically from the CLI.

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

pub fn serve(root: &Path) -> Result<()> {
    let plane = Plane::load(root)?;
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

    http_loop(&root_buf, &plane)
}

fn cron_loop(root: &Path) {
    let Ok(plane) = Plane::load(root) else { return };
    let Ok(mut ledger) = Ledger::open(&plane.db_path()) else {
        return;
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
    let mut last = Utc::now();
    loop {
        std::thread::sleep(CRON_POLL);
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
                // Per-task FIFO: one run per task in flight; older runs in
                // the same task block newer ones.
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
                    run_one(&root, &run_id);
                    in_flight_worker
                        .lock()
                        .expect("in_flight lock")
                        .remove(&task_name);
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
        // dispatch_run owns state-transition notifications (dead letters,
        // budget parks), so the CLI path notifies identically.
        let run = dispatch::dispatch_run(&plane, &mut ledger, run_id)?;
        eprintln!("run {} {} (task={})", run.id, run.state, run.task);
        Ok(())
    })();
    if let Err(e) = result {
        eprintln!("run {run_id}: {e:#}");
    }
}

fn http_loop(root: &Path, plane: &Plane) -> Result<()> {
    let bind = plane.spec.ingress.bind.clone();
    let server = tiny_http::Server::http(&bind).map_err(|e| anyhow::anyhow!("bind {bind}: {e}"))?;
    eprintln!("bb serve listening on {bind}");

    for mut request in server.incoming_requests() {
        let response = handle_request(root, &mut request);
        let (status, body) = match response {
            Ok(r) => r,
            Err(e) => (500, format!("{{\"error\":\"{e}\"}}")),
        };
        let _ = request.respond(
            tiny_http::Response::from_string(body)
                .with_status_code(status)
                .with_header(
                    tiny_http::Header::from_bytes(&b"Content-Type"[..], &b"application/json"[..])
                        .expect("static header"),
                ),
        );
    }
    Ok(())
}

fn handle_request(root: &Path, request: &mut tiny_http::Request) -> Result<(u16, String)> {
    let method = request.method().to_string();
    let url = request.url().to_string();

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
