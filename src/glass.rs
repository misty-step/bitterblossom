//! bitterblossom-933: the run plane's own glass emitter -- lifecycle posts
//! (dispatched, completed/failed/parked, asked, resumed) fire automatically
//! at existing dispatch/serve seams, with zero agent cooperation required.
//! Agent-authored posts remain the rich layer on top; this is the floor.
//!
//! Mirrors notify.rs's shape deliberately: shells to curl (no HTTP client
//! dependency, matching canary.rs/notify.rs/ask.rs), a missing
//! `[glass].base_url` is a no-op, and delivery is best-effort -- a glass
//! outage must never affect dispatch. Unlike `[notify]`, there is no durable
//! outbox/retry: this is observability, not a delivery guarantee.
//!
//! Session ids are glass-assigned, not bb-invented (proven live against the
//! real instance: `POST /api/posts` with an unrecognized `session_id` is a
//! 404, not an auto-create -- only omitting it creates one). So the first
//! post in a lineage omits `session_id` and persists whatever glass returns
//! (`runs.glass_session_id`, keyed on the lineage root via `parent_run_id`);
//! every later post in that lineage reuses the stored id, which is what
//! makes a parked run and its resume cohere into one glass session.

use std::io::Write;
use std::process::{Command, Stdio};

use crate::ledger::Ledger;
use crate::spec::Plane;

const REQUEST_TIMEOUT_SECONDS: u64 = 10;

/// The root run_id of a lineage chain (walking `parent_run_id` back to the
/// run that started it) -- the key `runs.glass_session_id` is stored under,
/// so a parked run and its resume share one glass session. A run with no
/// parent is its own root. Bounded walk so a data anomaly can never loop.
fn lineage_root(ledger: &Ledger, run_id: &str) -> String {
    let mut current = run_id.to_string();
    for _ in 0..20 {
        let Ok(run) = ledger.run(&current) else {
            return current;
        };
        match run.parent_run_id {
            Some(parent) => current = parent,
            None => return current,
        }
    }
    current
}

pub fn post_dispatched(
    plane: &Plane,
    ledger: &Ledger,
    run_id: &str,
    task: &str,
    agent: &str,
    harness: &str,
    model: &str,
) {
    publish(
        plane,
        ledger,
        run_id,
        agent,
        &format!("{task} dispatched"),
        serde_json::json!([
            {"kind": "markdown", "markdown": format!(
                "**{task}** dispatched\n\n- run: `{run_id}`\n- agent: `{agent}`\n- harness: `{harness}`\n- model: `{model}`"
            )},
            {"kind": "metric", "label": "state", "value": "running"},
        ]),
    );
}

#[allow(clippy::too_many_arguments)]
pub fn post_completed(
    plane: &Plane,
    ledger: &Ledger,
    run_id: &str,
    task: &str,
    agent: &str,
    outcome: &str,
    cost_usd: Option<f64>,
    duration_ms: Option<i64>,
) {
    let cost = cost_usd.map_or_else(|| "-".to_string(), |c| format!("${c:.4}"));
    let duration = duration_ms.map_or_else(|| "-".to_string(), |d| format!("{d}ms"));
    publish(
        plane,
        ledger,
        run_id,
        agent,
        &format!("{task} {outcome}"),
        serde_json::json!([
            {"kind": "markdown", "markdown": format!(
                "**{task}** {outcome}\n\n- run: `{run_id}`\n- cost: {cost}\n- duration: {duration}"
            )},
            {"kind": "metric", "label": "state", "value": outcome},
        ]),
    );
}

#[allow(clippy::too_many_arguments)]
pub fn post_asked(
    plane: &Plane,
    ledger: &Ledger,
    run_id: &str,
    task: &str,
    agent: &str,
    ask_id: &str,
    kind: &str,
    question: &str,
) {
    publish(
        plane,
        ledger,
        run_id,
        agent,
        &format!("{task} asked ({kind})"),
        serde_json::json!([
            {"kind": "markdown", "markdown": format!(
                "**{task}** raised a {kind}\n\n- ask: `{ask_id}`\n- run: `{run_id}`\n\n> {question}"
            )},
            {"kind": "metric", "label": "state", "value": "asked"},
        ]),
    );
}

pub fn post_resumed(
    plane: &Plane,
    ledger: &Ledger,
    parked_run_id: &str,
    resumed_run_id: &str,
    task: &str,
    agent: &str,
    ask_id: &str,
) {
    publish(
        plane,
        ledger,
        parked_run_id,
        agent,
        &format!("{task} resumed"),
        serde_json::json!([
            {"kind": "markdown", "markdown": format!(
                "**{task}** resumed after an answered ask\n\n- ask: `{ask_id}`\n- parked run: `{parked_run_id}`\n- resume run: `{resumed_run_id}`"
            )},
            {"kind": "metric", "label": "state", "value": "resumed"},
        ]),
    );
}

/// Resolve (or create) the glass session for `run_id`'s lineage, publish one
/// post into it, and persist a freshly created session id so later posts in
/// the same lineage reuse it. Entirely best-effort: any failure logs to
/// stderr and returns without propagating -- dispatch must never depend on
/// glass being reachable.
fn publish(
    plane: &Plane,
    ledger: &Ledger,
    run_id: &str,
    agent: &str,
    title: &str,
    surfaces: serde_json::Value,
) {
    let Some(base_url) = &plane.spec.glass.base_url else {
        return;
    };
    let root = lineage_root(ledger, run_id);
    let existing_session = ledger.run_glass_session(&root).ok().flatten();

    let mut body = serde_json::json!({
        "agent": agent,
        "title": title,
        "surfaces": surfaces,
    });
    if let Some(session) = &existing_session {
        body["session_id"] = serde_json::Value::String(session.clone());
    }

    match deliver(base_url, &body.to_string()) {
        Ok(response) => {
            if existing_session.is_none() {
                if let Some(session) = response
                    .get("post")
                    .and_then(|p| p.get("session_id"))
                    .and_then(serde_json::Value::as_str)
                {
                    let _ = ledger.set_run_glass_session(&root, session);
                }
            }
        }
        Err(e) => eprintln!("glass: post failed: {e}"),
    }
}

fn deliver(base_url: &str, body: &str) -> std::io::Result<serde_json::Value> {
    let bin = std::env::var("BB_GLASS_BIN").unwrap_or_else(|_| "curl".into());
    let url = format!("{}/api/posts", base_url.trim_end_matches('/'));
    let mut child = Command::new(bin)
        .args([
            "-sS",
            "-f",
            "-m",
            &REQUEST_TIMEOUT_SECONDS.to_string(),
            "-XPOST",
            "-HContent-Type: application/json",
            "-d@-",
        ])
        .arg(&url)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()?;
    if let Some(mut stdin) = child.stdin.take() {
        stdin.write_all(body.as_bytes())?;
    }
    let output = child.wait_with_output()?;
    if !output.status.success() {
        return Err(std::io::Error::other(format!(
            "curl exit {}: {}",
            output.status,
            String::from_utf8_lossy(&output.stderr).trim()
        )));
    }
    serde_json::from_slice(&output.stdout).map_err(|e| {
        std::io::Error::other(format!(
            "non-JSON response: {e} ({})",
            String::from_utf8_lossy(&output.stdout)
        ))
    })
}
