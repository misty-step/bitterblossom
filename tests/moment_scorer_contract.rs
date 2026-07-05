use std::path::Path;
use std::process::Command;

use bitterblossom::ledger::{IngressRequest, Ledger};

fn repo_root() -> std::path::PathBuf {
    std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR"))
}

fn open_ledger(dir: &Path) -> Ledger {
    Ledger::open(&dir.join("plane.db")).unwrap()
}

/// Seeds `count` unremarkable, successful runs for `task` with fixed
/// cost/duration, so later anomaly detection has a real trailing baseline to
/// compare against (never a fabricated one).
fn seed_normal_runs(ledger: &mut Ledger, task: &str, count: u32, cost_usd: f64, duration_ms: i64) {
    for i in 0..count {
        let run = ledger
            .ingest(IngressRequest {
                task,
                trigger_kind: "cron",
                idempotency_key: None,
                source_event_id: None,
                payload: None,
                parent_run_id: None,
            })
            .unwrap()
            .run_id;
        ledger.transition(&run, "running", None).unwrap();
        let attempt = ledger
            .create_attempt(&run, 1, "agent", 1, "pi", "test/model")
            .unwrap();
        ledger
            .finish_attempt(
                attempt,
                "success",
                None,
                Some(0),
                &bitterblossom::ledger::AttemptStats {
                    tokens_in: Some(10),
                    tokens_out: Some(10),
                    turns: Some(1),
                    cost_usd: Some(cost_usd),
                },
                None,
            )
            .unwrap();
        ledger.transition(&run, "success", None).unwrap();
        ledger
            .finalize_run(&run, Some(cost_usd), duration_ms)
            .unwrap();
        let _ = i;
    }
}

fn run_scorer(db: &Path, moments_db: &Path) -> serde_json::Value {
    let output = Command::new("python3")
        .arg(repo_root().join("scripts/moment-scorer.py"))
        .arg("scan")
        .arg("--db")
        .arg(db)
        .arg("--moments-db")
        .arg(moments_db)
        .output()
        .unwrap();
    assert!(
        output.status.success(),
        "stdout={}\nstderr={}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );
    let stdout = String::from_utf8(output.stdout).unwrap();
    let last_line = stdout.lines().last().expect("at least one output line");
    let result: serde_json::Value = serde_json::from_str(last_line).unwrap();
    assert_eq!(result["schema_version"], "bb.command_result.v1");
    assert_eq!(result["cost_usd"], 0.0, "scorer makes no model calls");
    result
}

fn list_moments(moments_db: &Path, all: bool) -> Vec<serde_json::Value> {
    let mut cmd = Command::new("python3");
    cmd.arg(repo_root().join("scripts/moment-scorer.py"))
        .arg("list")
        .arg("--moments-db")
        .arg(moments_db)
        .arg("--json");
    if all {
        cmd.arg("--all");
    }
    let output = cmd.output().unwrap();
    assert!(
        output.status.success(),
        "stdout={}\nstderr={}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );
    serde_json::from_slice(&output.stdout).unwrap()
}

#[test]
fn below_threshold_run_produces_no_moment() {
    let dir = tempfile::tempdir().unwrap();
    let mut ledger = open_ledger(dir.path());
    seed_normal_runs(&mut ledger, "quiet-task", 6, 0.01, 5_000);
    drop(ledger);

    let moments_db = dir.path().join("moments.db");
    let result = run_scorer(&dir.path().join("plane.db"), &moments_db);
    assert!(
        result["result"].as_str().unwrap().contains("0 new moment"),
        "{result}"
    );
    let moments = list_moments(&moments_db, true);
    assert_eq!(moments.len(), 0, "{moments:?}");
}

#[test]
fn a_failed_run_produces_a_failure_moment() {
    let dir = tempfile::tempdir().unwrap();
    let mut ledger = open_ledger(dir.path());
    seed_normal_runs(&mut ledger, "flaky-task", 6, 0.01, 5_000);
    let run = ledger
        .ingest(IngressRequest {
            task: "flaky-task",
            trigger_kind: "cron",
            idempotency_key: None,
            source_event_id: None,
            payload: None,
            parent_run_id: None,
        })
        .unwrap()
        .run_id;
    ledger.transition(&run, "running", None).unwrap();
    let attempt = ledger
        .create_attempt(&run, 1, "agent", 1, "pi", "test/model")
        .unwrap();
    ledger
        .finish_attempt(
            attempt,
            "failed",
            Some("connection reset by peer"),
            Some(1),
            &bitterblossom::ledger::AttemptStats::default(),
            None,
        )
        .unwrap();
    ledger.transition(&run, "failure", None).unwrap();
    ledger.finalize_run(&run, Some(0.01), 5_000).unwrap();
    drop(ledger);

    let moments_db = dir.path().join("moments.db");
    let result = run_scorer(&dir.path().join("plane.db"), &moments_db);
    assert!(
        result["result"].as_str().unwrap().contains("1 new moment"),
        "{result}"
    );
    let moments = list_moments(&moments_db, true);
    assert_eq!(moments.len(), 1);
    assert_eq!(moments[0]["run_id"], run);
    assert_eq!(moments[0]["class"], "failure");
    assert!(
        moments[0]["excerpt"]
            .as_str()
            .unwrap()
            .contains("connection reset"),
        "{moments:?}"
    );
    assert!(
        moments[0]["run_link"].as_str().unwrap().contains(&run),
        "{moments:?}"
    );
}

#[test]
fn a_retried_then_succeeded_run_produces_a_recovery_moment() {
    let dir = tempfile::tempdir().unwrap();
    let mut ledger = open_ledger(dir.path());
    seed_normal_runs(&mut ledger, "retry-task", 6, 0.01, 5_000);
    let run = ledger
        .ingest(IngressRequest {
            task: "retry-task",
            trigger_kind: "cron",
            idempotency_key: None,
            source_event_id: None,
            payload: None,
            parent_run_id: None,
        })
        .unwrap()
        .run_id;
    ledger.transition(&run, "running", None).unwrap();
    let attempt1 = ledger
        .create_attempt(&run, 1, "agent", 1, "pi", "test/model")
        .unwrap();
    ledger
        .finish_attempt(
            attempt1,
            "failed",
            Some("timeout"),
            Some(1),
            &bitterblossom::ledger::AttemptStats::default(),
            None,
        )
        .unwrap();
    let attempt2 = ledger
        .create_attempt(&run, 2, "agent", 1, "pi", "test/model")
        .unwrap();
    ledger
        .finish_attempt(
            attempt2,
            "success",
            None,
            Some(0),
            &bitterblossom::ledger::AttemptStats {
                tokens_in: Some(10),
                tokens_out: Some(10),
                turns: Some(1),
                cost_usd: Some(0.01),
            },
            None,
        )
        .unwrap();
    ledger.transition(&run, "success", None).unwrap();
    ledger.finalize_run(&run, Some(0.01), 5_000).unwrap();
    drop(ledger);

    let moments_db = dir.path().join("moments.db");
    run_scorer(&dir.path().join("plane.db"), &moments_db);
    let moments = list_moments(&moments_db, true);
    assert_eq!(moments.len(), 1);
    assert_eq!(moments[0]["run_id"], run);
    assert_eq!(moments[0]["class"], "recovery");
    assert!(
        moments[0]["excerpt"].as_str().unwrap().contains('2'),
        "{moments:?}"
    );
}

#[test]
fn a_cost_outlier_run_produces_a_cost_anomaly_moment() {
    let dir = tempfile::tempdir().unwrap();
    let mut ledger = open_ledger(dir.path());
    seed_normal_runs(&mut ledger, "pricey-task", 8, 0.02, 5_000);
    seed_normal_runs(&mut ledger, "pricey-task", 1, 5.00, 5_000);
    drop(ledger);

    let moments_db = dir.path().join("moments.db");
    run_scorer(&dir.path().join("plane.db"), &moments_db);
    let moments = list_moments(&moments_db, true);
    assert_eq!(moments.len(), 1, "{moments:?}");
    assert_eq!(moments[0]["class"], "cost_anomaly");
    assert!(
        moments[0]["excerpt"].as_str().unwrap().contains('5'),
        "{moments:?}"
    );
}

#[test]
fn a_guard_event_in_the_run_window_produces_a_surprise_moment() {
    let dir = tempfile::tempdir().unwrap();
    let mut ledger = open_ledger(dir.path());
    seed_normal_runs(&mut ledger, "guarded-task", 6, 0.01, 5_000);
    let run = ledger
        .ingest(IngressRequest {
            task: "guarded-task",
            trigger_kind: "cron",
            idempotency_key: None,
            source_event_id: None,
            payload: None,
            parent_run_id: None,
        })
        .unwrap()
        .run_id;
    ledger.transition(&run, "running", None).unwrap();
    ledger
        .record_guard_event(
            "attention_debt_brake",
            Some("guarded-task"),
            "serve-mode cron catch-up collapsed 4 fires",
            4,
        )
        .unwrap();
    let attempt = ledger
        .create_attempt(&run, 1, "agent", 1, "pi", "test/model")
        .unwrap();
    ledger
        .finish_attempt(
            attempt,
            "success",
            None,
            Some(0),
            &bitterblossom::ledger::AttemptStats {
                tokens_in: Some(10),
                tokens_out: Some(10),
                turns: Some(1),
                cost_usd: Some(0.01),
            },
            None,
        )
        .unwrap();
    ledger.transition(&run, "success", None).unwrap();
    ledger.finalize_run(&run, Some(0.01), 5_000).unwrap();
    drop(ledger);

    let moments_db = dir.path().join("moments.db");
    run_scorer(&dir.path().join("plane.db"), &moments_db);
    let moments = list_moments(&moments_db, true);
    assert_eq!(moments.len(), 1, "{moments:?}");
    assert_eq!(moments[0]["run_id"], run);
    assert_eq!(moments[0]["class"], "surprise");
    assert!(
        moments[0]["excerpt"]
            .as_str()
            .unwrap()
            .contains("attention_debt_brake"),
        "{moments:?}"
    );
}

#[test]
fn fleet_wide_daily_cap_publishes_at_most_three_and_never_silently_drops_the_rest() {
    let dir = tempfile::tempdir().unwrap();
    let mut ledger = open_ledger(dir.path());
    // Five independent failing tasks in one day -- five real Failure-class
    // signals, more than the ≤3/day fleet-wide cap.
    for n in 0..5 {
        let task = format!("cap-task-{n}");
        seed_normal_runs(&mut ledger, &task, 5, 0.01, 5_000);
        let run = ledger
            .ingest(IngressRequest {
                task: &task,
                trigger_kind: "cron",
                idempotency_key: None,
                source_event_id: None,
                payload: None,
                parent_run_id: None,
            })
            .unwrap()
            .run_id;
        ledger.transition(&run, "running", None).unwrap();
        let attempt = ledger
            .create_attempt(&run, 1, "agent", 1, "pi", "test/model")
            .unwrap();
        ledger
            .finish_attempt(
                attempt,
                "failed",
                Some("boom"),
                Some(1),
                &bitterblossom::ledger::AttemptStats::default(),
                None,
            )
            .unwrap();
        ledger.transition(&run, "failure", None).unwrap();
        ledger.finalize_run(&run, Some(0.01), 5_000).unwrap();
    }
    drop(ledger);

    let moments_db = dir.path().join("moments.db");
    run_scorer(&dir.path().join("plane.db"), &moments_db);

    let published = list_moments(&moments_db, false);
    assert_eq!(
        published.len(),
        3,
        "fleet-wide cap must publish at most 3/day: {published:?}"
    );
    let all = list_moments(&moments_db, true);
    assert_eq!(
        all.len(),
        5,
        "the cap must never silently drop a real signal -- the other 2 stay \
         recorded as unpublished, not discarded: {all:?}"
    );
    assert_eq!(
        all.iter().filter(|m| m["published"] == false).count(),
        2,
        "{all:?}"
    );
}

#[test]
fn scanning_twice_never_reprocesses_or_duplicates_an_already_scored_run() {
    let dir = tempfile::tempdir().unwrap();
    let mut ledger = open_ledger(dir.path());
    seed_normal_runs(&mut ledger, "idempotent-task", 6, 0.01, 5_000);
    let run = ledger
        .ingest(IngressRequest {
            task: "idempotent-task",
            trigger_kind: "cron",
            idempotency_key: None,
            source_event_id: None,
            payload: None,
            parent_run_id: None,
        })
        .unwrap()
        .run_id;
    ledger.transition(&run, "running", None).unwrap();
    let attempt = ledger
        .create_attempt(&run, 1, "agent", 1, "pi", "test/model")
        .unwrap();
    ledger
        .finish_attempt(
            attempt,
            "failed",
            Some("boom"),
            Some(1),
            &bitterblossom::ledger::AttemptStats::default(),
            None,
        )
        .unwrap();
    ledger.transition(&run, "failure", None).unwrap();
    ledger.finalize_run(&run, Some(0.01), 5_000).unwrap();
    drop(ledger);

    let db = dir.path().join("plane.db");
    let moments_db = dir.path().join("moments.db");
    let first = run_scorer(&db, &moments_db);
    assert!(first["result"].as_str().unwrap().contains("1 new moment"));
    let second = run_scorer(&db, &moments_db);
    assert!(
        second["result"].as_str().unwrap().contains("0 new moment"),
        "{second}"
    );

    let moments = list_moments(&moments_db, true);
    assert_eq!(moments.len(), 1, "no duplicate card on rescan: {moments:?}");
}
