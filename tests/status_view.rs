use std::fs;

use bitterblossom::health;
use bitterblossom::ledger::{ExternalRunCreate, IngressRequest, Ledger};
use bitterblossom::spec::Plane;
use rusqlite::params;
use time::{format_description::well_known::Rfc3339, Duration, OffsetDateTime};

fn write_plane(root: &std::path::Path) {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::write(
        root.join("agents/a.toml"),
        "version = 3\nharness = \"command\"\nmodel = \"\"\nbin = \"true\"\n",
    )
    .unwrap();
    fs::write(
        root.join("plane.toml"),
        "dev = true\n[gate]\nrequired = [\"security\"]\nquorum = 1\narm_timeout_seconds = 77\n",
    )
    .unwrap();
    for task in [
        "review",
        "security",
        "verify",
        "product",
        "correctness",
        "recovery",
        "fresh-recovery",
    ] {
        let dir = root.join("tasks").join(task);
        fs::create_dir_all(&dir).unwrap();
        fs::write(dir.join("card.md"), "card\n").unwrap();
        let verdict = if task == "security" {
            "verdict = \"security\"\n"
        } else {
            ""
        };
        fs::write(
            dir.join("task.toml"),
            format!(
                "agent = \"a\"\nsubstrate = \"local\"\n{verdict}[[trigger]]\nkind = \"manual\"\n"
            ),
        )
        .unwrap();
    }
}

#[test]
fn status_view_surfaces_rollout_authority_and_scorecard() {
    let dir = tempfile::tempdir().unwrap();
    fs::create_dir_all(dir.path().join("agents")).unwrap();
    fs::create_dir_all(dir.path().join("tasks/backlog-chewer-dry-run")).unwrap();
    fs::write(dir.path().join("plane.toml"), "dev = true\n").unwrap();
    fs::write(
        dir.path().join("agents/a.toml"),
        "version = 1\nharness = \"command\"\nmodel = \"\"\nbin = \"true\"\n",
    )
    .unwrap();
    fs::write(
        dir.path().join("tasks/backlog-chewer-dry-run/card.md"),
        "card\n",
    )
    .unwrap();
    fs::write(
        dir.path().join("tasks/backlog-chewer-dry-run/task.toml"),
        r##"agent = "a"
substrate = "local"

[rollout]
authority = "dry-run"
scorecard = "docs/rollout-scorecards.md#backlog-chewer-dry-run-dry-run-backlog-082"

[[trigger]]
kind = "manual"
"##,
    )
    .unwrap();
    let plane = Plane::load(dir.path()).unwrap();
    let ledger = Ledger::open(&plane.db_path()).unwrap();

    let doc = health::status_view(&plane, &ledger).unwrap();
    let task = doc["tasks"]
        .as_array()
        .unwrap()
        .iter()
        .find(|task| task["task"] == "backlog-chewer-dry-run")
        .unwrap();
    assert_eq!(task["rollout"]["authority"], "dry-run");
    assert_eq!(
        task["rollout"]["scorecard"],
        "docs/rollout-scorecards.md#backlog-chewer-dry-run-dry-run-backlog-082"
    );
}

fn ingest(ledger: &mut Ledger, task: &str) -> String {
    ledger
        .ingest(IngressRequest {
            task,
            trigger_kind: "manual",
            idempotency_key: None,
            source_event_id: None,
            payload: None,
            parent_run_id: None,
        })
        .unwrap()
        .run_id
}

fn finish(ledger: &mut Ledger, task: &str, state: &str, reason: Option<&str>, cost: Option<f64>) {
    let run = ingest(ledger, task);
    ledger.transition(&run, "running", None).unwrap();
    ledger.set_run_agent(&run, "a", 3).unwrap();
    ledger.finalize_run(&run, cost, 1234).unwrap();
    ledger.transition(&run, state, reason).unwrap();
}

fn awaiting_recovery(ledger: &mut Ledger, task: &str) -> String {
    let run = ingest(ledger, task);
    ledger.transition(&run, "running", None).unwrap();
    ledger
        .transition(&run, "awaiting_recovery", Some("probe: unknown"))
        .unwrap();
    run
}

#[test]
fn status_view_covers_operator_truth_fixtures() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let plane = Plane::load(dir.path()).unwrap();
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    finish(
        &mut ledger,
        "review",
        "failure",
        Some("unparseable harness output"),
        None,
    );
    finish(&mut ledger, "verify", "success", None, Some(0.01));
    finish(
        &mut ledger,
        "correctness",
        "failure",
        Some("expensive model timeout"),
        Some(0.42),
    );
    ingest(&mut ledger, "correctness");
    let product = ingest(&mut ledger, "product");
    ledger
        .record_dead_letter(&product, "product", Some("{}"), "prepare failed")
        .unwrap();
    ledger.park_task("security", "cost cap").unwrap();
    ingest(&mut ledger, "security");
    let stale_recovery = awaiting_recovery(&mut ledger, "recovery");
    awaiting_recovery(&mut ledger, "fresh-recovery");
    let stale_at = (OffsetDateTime::now_utc() - Duration::hours(2))
        .format(&Rfc3339)
        .unwrap();
    rusqlite::Connection::open(plane.db_path())
        .unwrap()
        .execute(
            "UPDATE runs SET updated_at = ?1 WHERE id = ?2",
            params![stale_at, stale_recovery],
        )
        .unwrap();
    let notification = ledger
        .enqueue_notification("status_probe", r#"{"event":"status_probe"}"#)
        .unwrap();
    ledger
        .mark_notification_failed(notification, "webhook down", None, None)
        .unwrap();
    let external = ledger
        .create_external_run(ExternalRunCreate {
            agent: "codex-bb-909".into(),
            role: "implementer".into(),
            repo: "misty-step/bitterblossom".into(),
            brief_hash: "sha256:test".into(),
            plane: "local".into(),
            status_url: Some("https://example.test/status".into()),
            receipt_path: Some("/tmp/bb-909.md".into()),
            started_at: "2026-07-04T12:00:00Z".into(),
        })
        .unwrap();
    ledger
        .update_external_run(&external.id, "done", Some("2026-07-04T12:05:00Z"))
        .unwrap();

    let doc = health::status_view(&plane, &ledger).unwrap();
    assert_eq!(
        doc["ledger"]["schema_version"],
        bitterblossom::ledger::LEDGER_SCHEMA_VERSION
    );
    assert_eq!(
        doc["ledger"]["supported_schema_version"],
        bitterblossom::ledger::LEDGER_SCHEMA_VERSION
    );
    let tasks = doc["tasks"].as_array().unwrap();
    let by_task = |name: &str| tasks.iter().find(|t| t["task"] == name).unwrap();

    assert_eq!(by_task("review")["runs"]["by_state"]["failure"], 1);
    assert_eq!(
        by_task("review")["safe_next_actions"][0]["kind"],
        "inspect_artifact"
    );
    assert_eq!(by_task("security")["parked"], "cost cap");
    assert_eq!(by_task("security")["runs"]["by_state"]["blocked_budget"], 1);
    assert_eq!(by_task("verify")["runs"]["by_state"]["success"], 1);
    assert_eq!(by_task("verify")["runs"]["cost_usd"], 0.01);
    assert_eq!(by_task("correctness")["runs"]["by_state"]["failure"], 1);
    assert_eq!(by_task("correctness")["queue"]["pending"], 1);
    assert!(by_task("correctness")["queue"]["oldest_pending_age_seconds"].is_number());
    assert_eq!(by_task("product")["dlq"]["open"], 1);
    assert_eq!(
        by_task("product")["safe_next_actions"][0]["kind"],
        "replay_pre_execute_dlq"
    );
    assert_eq!(
        by_task("fresh-recovery")["safe_next_actions"][0]["kind"],
        "resolve_after_side_effect_inspection"
    );
    let recovery_action = &by_task("recovery")["safe_next_actions"][0];
    assert_eq!(recovery_action["kind"], "escalate_stale_recovery");
    assert!(recovery_action["age_seconds"].as_i64().unwrap() >= 3600);
    assert_eq!(recovery_action["stale_after_seconds"], 3600);
    assert_eq!(doc["guards"]["notify"]["outbox"]["failed"], 1);
    assert_eq!(
        doc["guards"]["notify"]["recent_outbox"][0]["event"],
        "status_probe"
    );
    assert_eq!(doc["guards"]["gate"]["required"][0], "security");
    assert_eq!(doc["guards"]["gate"]["quorum"], 1);
    assert_eq!(doc["guards"]["gate"]["arm_timeout_seconds"], 77);
    assert_eq!(doc["guards"]["attention_debt"]["blocking"], true);
    assert_eq!(doc["guards"]["attention_debt"]["open_dlq"], 1);
    assert_eq!(doc["guards"]["attention_debt"]["parked_tasks"], 1);
    assert_eq!(doc["guards"]["attention_debt"]["awaiting_recovery"], 2);
    assert_eq!(doc["guards"]["attention_debt"]["notification_failed"], 1);
    assert_eq!(doc["summary"]["external_runs"], 1);
    assert_eq!(doc["summary"]["external_running"], 0);
    assert_eq!(doc["external_runs"]["by_status"]["done"], 1);
    assert_eq!(doc["external_runs"]["recent"][0]["source"], "external");
    assert_eq!(doc["external_runs"]["recent"][0]["agent"], "codex-bb-909");
    let contracts = doc["freshness_contracts"].as_array().unwrap();
    let recovery_contract = contracts
        .iter()
        .find(|c| c["subject"] == "run.awaiting_recovery")
        .unwrap();
    assert_eq!(recovery_contract["threshold_seconds"], 3600);
    assert_eq!(recovery_contract["notification_severity"], "critical");
}

#[test]
fn status_view_reports_active_lease_even_when_run_is_outside_recent_window() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let plane = Plane::load(dir.path()).unwrap();
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let leased = ingest(&mut ledger, "review");
    ledger.transition(&leased, "running", None).unwrap();
    ledger.try_acquire_host_lease("lane-old", &leased).unwrap();
    for _ in 0..205 {
        ingest(&mut ledger, "review");
    }

    let doc = health::status_view(&plane, &ledger).unwrap();
    let tasks = doc["tasks"].as_array().unwrap();
    let review = tasks.iter().find(|t| t["task"] == "review").unwrap();

    assert_eq!(review["lease"]["host"], "lane-old");
    assert_eq!(review["lease"]["run_id"], leased);
}

#[test]
fn status_view_reports_backup_freshness_from_heartbeat() {
    let dir = tempfile::tempdir().unwrap();
    fs::create_dir_all(dir.path().join(".bb")).unwrap();
    fs::write(
        dir.path().join("plane.toml"),
        r#"dev = true

[backup]
enabled = true
provider = "litestream"
replica_env = "LITESTREAM_REPLICA_URL"
last_success_path = ".bb/backup-last-success"
rpo_seconds = 300
rto_seconds = 1800
"#,
    )
    .unwrap();
    let fresh = (OffsetDateTime::now_utc() - Duration::seconds(60))
        .format(&Rfc3339)
        .unwrap();
    fs::write(dir.path().join(".bb/backup-last-success"), fresh).unwrap();

    let plane = Plane::load(dir.path()).unwrap();
    let ledger = Ledger::open(&plane.db_path()).unwrap();
    let doc = health::status_view(&plane, &ledger).unwrap();
    assert_eq!(doc["backup"]["enabled"], true);
    assert_eq!(doc["backup"]["provider"], "litestream");
    assert_eq!(doc["backup"]["replica_env"], "LITESTREAM_REPLICA_URL");
    assert_eq!(doc["backup"]["rpo_seconds"], 300);
    assert_eq!(doc["backup"]["rto_seconds"], 1800);
    assert_eq!(doc["backup"]["status"], "fresh");
    assert_eq!(doc["backup"]["healthy"], true);
    assert!(doc["backup"]["last_success_age_seconds"].as_i64().unwrap() <= 300);

    let stale = (OffsetDateTime::now_utc() - Duration::seconds(600))
        .format(&Rfc3339)
        .unwrap();
    fs::write(dir.path().join(".bb/backup-last-success"), stale).unwrap();
    let doc = health::status_view(&plane, &ledger).unwrap();
    assert_eq!(doc["backup"]["status"], "stale");
    assert_eq!(doc["backup"]["healthy"], false);
}

#[test]
fn external_run_accepts_a_campaign_plane_label() {
    // bitterblossom-922: register-through's `plane` field is a descriptive
    // label (which logical/campaign plane the externally-owned run belongs to),
    // not a substrate lease. The documented campaign plane value must be
    // accepted, not rejected -- that rejection blocked campaign lanes from
    // registering at all. Empty is still rejected (the field is required).
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let plane = Plane::load(dir.path()).unwrap();
    let ledger = Ledger::open(&plane.db_path()).unwrap();

    let mk = |plane_label: &str| ExternalRunCreate {
        agent: "bb-everything-2026-07-07".into(),
        role: "interactive-lead".into(),
        repo: "bitterblossom".into(),
        brief_hash: "focus-2026-07-07".into(),
        plane: plane_label.into(),
        status_url: None,
        receipt_path: None,
        started_at: "2026-07-07T12:00:00Z".into(),
    };

    // The exact value the 2026-07-04 campaign contract documented and that the
    // endpoint used to reject with HTTP 400.
    let row = ledger
        .create_external_run(mk("campaign-2026-07-07-focus"))
        .expect("campaign plane label must be accepted");
    assert_eq!(row.plane, "campaign-2026-07-07-focus");
    assert_eq!(row.source, "external");

    // "local" still works, and an empty label is still a hard error.
    assert!(ledger.create_external_run(mk("local")).is_ok());
    assert!(ledger.create_external_run(mk("   ")).is_err());
}
