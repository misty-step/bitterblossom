//! Progress/stale classification for long-running attempts (backlog 087).
//!
//! Mechanism only: the classifier reads ledger evidence (progress markers, the
//! boot probe recorded by recovery, and the latest attempt phase) and produces a
//! machine-readable classification plus an explicit operator `safe_next_action`.
//! It never decides to kill or retry executing work with side effects.

use std::fs;
use std::path::Path;

use bitterblossom::ledger::{IngressRequest, Ledger};
use bitterblossom::progress;
use bitterblossom::spec::Plane;
use time::{format_description::well_known::Rfc3339, Duration, OffsetDateTime};

fn make_plane(root: &Path) -> Plane {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
    fs::write(
        root.join("agents/a.toml"),
        "harness = \"claude\"\nmodel = \"m\"\n",
    )
    .unwrap();
    fs::write(root.join("tasks/demo/card.md"), "card\n").unwrap();
    fs::write(
        root.join("tasks/demo/task.toml"),
        "agent = \"a\"\nsubstrate = \"local\"\n\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
    Plane::load(root).unwrap()
}

fn running_run(ledger: &mut Ledger, task: &str) -> String {
    let run_id = ledger
        .ingest(IngressRequest {
            task,
            trigger_kind: "manual",
            idempotency_key: None,
            source_event_id: None,
            payload: Some("{\"p\":1}"),
            parent_run_id: None,
        })
        .unwrap()
        .run_id;
    ledger.transition(&run_id, "running", None).unwrap();
    run_id
}

fn backdate_progress(db_path: &Path, run_id: &str, ago: Duration) {
    let at = (OffsetDateTime::now_utc() - ago).format(&Rfc3339).unwrap();
    rusqlite::Connection::open(db_path)
        .unwrap()
        .execute(
            "UPDATE run_events SET at = ?1 \
             WHERE id = (SELECT id FROM run_events WHERE run_id = ?2 AND kind = 'progress' \
                         ORDER BY id DESC LIMIT 1)",
            rusqlite::params![at, run_id],
        )
        .unwrap();
}

fn run_progress_view(ledger: &Ledger, run_id: &str) -> progress::ProgressView {
    let run = ledger.run(run_id).unwrap();
    progress::from_ledger(ledger, &run, OffsetDateTime::now_utc()).unwrap()
}

#[test]
fn fresh_running_has_recent_progress() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let run_id = running_run(&mut ledger, "demo");
    ledger.record_progress(&run_id, "phase:executing").unwrap();

    let view = run_progress_view(&ledger, &run_id);
    assert_eq!(view.classification, "fresh");
    assert!(view.last_progress_at.is_some());
    assert!(
        view.age_seconds.unwrap() < progress::PROGRESS_STALE_SECONDS,
        "fresh run age must be under threshold"
    );
    assert_eq!(view.safe_next_action.kind, "monitor");
}

#[test]
fn alive_but_no_recent_progress_is_stale_executing() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let run_id = running_run(&mut ledger, "demo");
    let attempt = ledger
        .create_attempt(&run_id, 1, "a", 1, "claude", "m")
        .unwrap();
    ledger.set_attempt_phase(attempt, "executing").unwrap();
    ledger.record_progress(&run_id, "phase:executing").unwrap();
    // Backdate the only progress marker well past the threshold.
    backdate_progress(
        &plane.db_path(),
        &run_id,
        Duration::seconds(progress::PROGRESS_STALE_SECONDS + 60),
    );

    let view = run_progress_view(&ledger, &run_id);
    assert_eq!(view.classification, "stale_executing");
    assert_eq!(view.attempt_phase.as_deref(), Some("executing"));
    assert!(view.age_seconds.unwrap() >= progress::PROGRESS_STALE_SECONDS);
    // Stale executing must NEVER auto-kill: it points the operator at a probe.
    assert_eq!(view.safe_next_action.kind, "probe_before_action");
    assert!(
        view.safe_next_action.reason.contains("do not auto-kill"),
        "stale_executing must forbid automatic kill"
    );
}

#[test]
fn dead_pre_attempt_classifies_for_replay() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let run_id = running_run(&mut ledger, "demo");
    let attempt = ledger
        .create_attempt(&run_id, 1, "a", 1, "claude", "m")
        .unwrap();
    ledger.set_attempt_phase(attempt, "prepared").unwrap();
    ledger
        .record_event(&run_id, "boot_probe", Some("dead"))
        .unwrap();
    ledger
        .transition(&run_id, "awaiting_recovery", Some("probe: dead"))
        .unwrap();

    let view = run_progress_view(&ledger, &run_id);
    assert_eq!(view.classification, "dead_pre_attempt");
    assert_eq!(view.probe.as_deref(), Some("dead"));
    assert_eq!(view.attempt_phase.as_deref(), Some("prepared"));
    assert_eq!(view.safe_next_action.kind, "replay_or_resolve");
}

#[test]
fn dead_executing_awaits_side_effect_inspection() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let run_id = running_run(&mut ledger, "demo");
    let attempt = ledger
        .create_attempt(&run_id, 1, "a", 1, "claude", "m")
        .unwrap();
    ledger.set_attempt_phase(attempt, "executing").unwrap();
    ledger
        .record_event(&run_id, "boot_probe", Some("dead"))
        .unwrap();
    ledger
        .transition(&run_id, "awaiting_recovery", Some("probe: dead"))
        .unwrap();

    let view = run_progress_view(&ledger, &run_id);
    assert_eq!(view.classification, "dead_executing");
    assert_eq!(view.probe.as_deref(), Some("dead"));
    assert_eq!(view.attempt_phase.as_deref(), Some("executing"));
    // Executing attempts have possible side effects: no mechanical replay.
    assert_eq!(
        view.safe_next_action.kind,
        "resolve_after_side_effect_inspection"
    );
}

#[test]
fn unknown_probe_is_operator_blocker() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let run_id = running_run(&mut ledger, "demo");
    let attempt = ledger
        .create_attempt(&run_id, 1, "a", 1, "claude", "m")
        .unwrap();
    ledger.set_attempt_phase(attempt, "executing").unwrap();
    ledger
        .record_event(&run_id, "boot_probe", Some("unknown: unparseable pidfile"))
        .unwrap();
    ledger
        .transition(
            &run_id,
            "awaiting_recovery",
            Some("probe: unknown: unparseable pidfile"),
        )
        .unwrap();

    let view = run_progress_view(&ledger, &run_id);
    assert_eq!(view.classification, "unknown_probe");
    assert!(view.probe.as_deref().unwrap().starts_with("unknown"));
    assert_eq!(view.safe_next_action.kind, "inspect_manually");
}

#[test]
fn terminal_runs_are_done() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let run_id = running_run(&mut ledger, "demo");
    ledger.transition(&run_id, "success", None).unwrap();

    let view = run_progress_view(&ledger, &run_id);
    assert_eq!(view.classification, "done");
    assert_eq!(view.safe_next_action.kind, "none");
}

#[test]
fn runs_show_json_exposes_progress_classification() {
    use std::process::Command;
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let run_id = running_run(&mut ledger, "demo");
    let attempt = ledger
        .create_attempt(&run_id, 1, "a", 1, "claude", "m")
        .unwrap();
    ledger.set_attempt_phase(attempt, "executing").unwrap();
    ledger.record_progress(&run_id, "phase:executing").unwrap();
    backdate_progress(
        &plane.db_path(),
        &run_id,
        Duration::seconds(progress::PROGRESS_STALE_SECONDS + 60),
    );
    drop(ledger);

    let shown = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", root, "runs", "show", &run_id, "--json"])
        .output()
        .unwrap();
    assert!(
        shown.status.success(),
        "{}",
        String::from_utf8_lossy(&shown.stderr)
    );
    let doc: serde_json::Value = serde_json::from_slice(&shown.stdout).unwrap();
    assert_eq!(doc["progress"]["classification"], "stale_executing");
    assert_eq!(doc["progress"]["attempt_phase"], "executing");
    assert_eq!(
        doc["progress"]["safe_next_action"]["kind"],
        "probe_before_action"
    );
    assert_eq!(
        doc["progress"]["threshold_seconds"],
        progress::PROGRESS_STALE_SECONDS
    );
}

#[test]
fn status_json_exposes_progress_for_running_attempts() {
    use std::process::Command;
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let run_id = running_run(&mut ledger, "demo");
    let attempt = ledger
        .create_attempt(&run_id, 1, "a", 1, "claude", "m")
        .unwrap();
    ledger.set_attempt_phase(attempt, "executing").unwrap();
    ledger.record_progress(&run_id, "phase:executing").unwrap();
    backdate_progress(
        &plane.db_path(),
        &run_id,
        Duration::seconds(progress::PROGRESS_STALE_SECONDS + 60),
    );
    drop(ledger);

    let out = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", root, "status", "--json"])
        .output()
        .unwrap();
    assert!(
        out.status.success(),
        "{}",
        String::from_utf8_lossy(&out.stderr)
    );
    let doc: serde_json::Value = serde_json::from_slice(&out.stdout).unwrap();
    let demo = doc["tasks"]
        .as_array()
        .unwrap()
        .iter()
        .find(|t| t["task"] == "demo")
        .unwrap();
    let running = demo["progress"]["running"].as_array().unwrap();
    assert_eq!(running.len(), 1);
    assert_eq!(running[0]["run_id"], run_id);
    assert_eq!(running[0]["classification"], "stale_executing");
    assert_eq!(
        running[0]["safe_next_action"]["kind"],
        "probe_before_action"
    );
}
