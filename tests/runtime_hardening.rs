use std::fs;

use bitterblossom::ledger::{IngressRequest, Ledger};
use bitterblossom::spec::Plane;
use bitterblossom::workflow::WorkflowDoc;
use bitterblossom::workflow_runtime::{
    accept, resolve_workflow_run, TriggerEnvelope, TriggerSource,
};
use chrono::{TimeZone, Utc};
use rusqlite::params;

fn doc(name: &str, route: &str, policy: &str) -> WorkflowDoc {
    WorkflowDoc::from_toml(&format!(
        r#"
name = "{name}"
goal = "exercise runtime admission"

[[trigger]]
kind = "webhook"
route = "{route}"
secret_env = "BB_TEST_RUNTIME_SECRET"
dedupe_key = "json:/id"

[[trigger]]
kind = "manual"

[policies]
{policy}

[[step]]
name = "work"
goal = "do the bounded work"
[step.agent]
name = "stub"
version = 1
harness = "command"
model = "stub"
"#
    ))
    .unwrap()
}

fn store() -> (tempfile::TempDir, Plane, Ledger) {
    let dir = tempfile::tempdir().unwrap();
    fs::write(dir.path().join("plane.toml"), "dev = true\n").unwrap();
    let plane = Plane::load(dir.path()).unwrap();
    let ledger = Ledger::open(&dir.path().join("plane.db")).unwrap();
    (dir, plane, ledger)
}

#[test]
fn legacy_resolve_closes_open_attempt_and_releases_lease() {
    let (_dir, _plane, mut ledger) = store();
    let accepted = ledger
        .ingest(IngressRequest {
            task: "legacy",
            trigger_kind: "manual",
            idempotency_key: Some("legacy-1"),
            source_event_id: None,
            payload: Some("{}"),
            parent_run_id: None,
        })
        .unwrap();
    let run_id = accepted.run_id.to_string();
    ledger
        .create_attempt(&run_id, 1, "agent", 1, "harness", "model")
        .unwrap();
    let attempt_id = ledger.attempts(&run_id).unwrap()[0].id;
    ledger.set_attempt_phase(attempt_id, "executing").unwrap();
    assert!(ledger
        .try_acquire_host_lease("legacy-host", &run_id)
        .unwrap());

    ledger
        .resolve_run(&run_id, "failure", "operator resolved after restart")
        .unwrap();

    let attempt = &ledger.attempts(&run_id).unwrap()[0];
    assert_eq!(attempt.phase, "released");
    assert_eq!(attempt.outcome.as_deref(), Some("failure"));
    assert!(attempt.ended_at.is_some());
    assert!(ledger.lease_holder("legacy-host").unwrap().is_none());
    assert_eq!(ledger.run(&run_id).unwrap().state, "failure");
}

#[test]
fn workflow_admission_denial_is_atomic_and_audited() {
    let (_dir, plane, ledger) = store();
    let workflow = doc(
        "bounded",
        "bounded-hook",
        "max_runs_per_day = 1
concurrency = 1",
    );
    let (row, revision) = ledger.create_workflow(&workflow, "test", None).unwrap();
    ledger.activate_workflow(&row.name, Some(revision)).unwrap();
    let accepted = accept(
        &plane,
        &ledger,
        &TriggerEnvelope {
            workflow: row.name.clone(),
            source: TriggerSource::Manual,
            payload: Some(r#"{"id":"one"}"#.into()),
            dedupe_key: Some("manual:one".into()),
        },
    )
    .unwrap();
    let accepted_id = match &accepted {
        bitterblossom::workflow::AcceptOutcome::Accepted { run } => run.id.clone(),
        other => panic!("expected acceptance, got {other:?}"),
    };
    let run_events = ledger.workflow_run_events(&accepted_id).unwrap();
    assert!(run_events.iter().any(|event| event.kind == "run_accepted"));
    let denied = accept(
        &plane,
        &ledger,
        &TriggerEnvelope {
            workflow: row.name.clone(),
            source: TriggerSource::Manual,
            payload: Some(r#"{"id":"two"}"#.into()),
            dedupe_key: Some("manual:two".into()),
        },
    )
    .unwrap();
    match denied {
        bitterblossom::workflow::AcceptOutcome::Denied { kind, .. } => {
            assert_eq!(kind, "workflow_max_runs_per_day")
        }
        other => panic!("expected denial, got {other:?}"),
    }
    assert_eq!(ledger.workflow_runs(&row.name).unwrap().len(), 1);
    assert!(ledger
        .workflow_events(&row.name)
        .unwrap()
        .iter()
        .any(|e| e.kind == "run_denied"));
}

#[test]
fn legacy_workflow_event_table_backfills_run_id_for_readback() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("legacy-events.db");
    let (workflow_id, workflow_name) = {
        let ledger = Ledger::open(&path).unwrap();
        let workflow = doc("legacy-events", "legacy-events-hook", "");
        let (row, _) = ledger.create_workflow(&workflow, "test", None).unwrap();
        (row.id, row.name)
    };
    {
        let conn = rusqlite::Connection::open(&path).unwrap();
        conn.execute_batch(
            "DROP TABLE workflow_events;
            CREATE TABLE workflow_events (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                workflow_id TEXT NOT NULL,
                kind TEXT NOT NULL,
                data TEXT,
                at TEXT NOT NULL
            );",
        )
        .unwrap();
        conn.execute(
            "INSERT INTO workflow_events (workflow_id, kind, data, at) VALUES (?1, 'legacy', 'old', '2026-01-01T00:00:00Z')",
            params![workflow_id],
        )
        .unwrap();
    }
    let ledger = Ledger::open(&path).unwrap();
    let events = ledger.workflow_events(&workflow_name).unwrap();
    assert_eq!(events.len(), 1);
    assert_eq!(events[0].kind, "legacy");
    assert!(events[0].run_id.is_none());
}

#[test]
fn invalid_dedupe_is_refused_before_activation() {
    let (_dir, _plane, ledger) = store();
    let mut workflow = doc("bad-dedupe", "bad-dedupe-hook", "");
    workflow.triggers[0].dedupe_key = Some("missing-separator".into());
    let created = ledger.create_workflow(&workflow, "test", None);
    assert!(created.is_err());
}

#[test]
fn active_workflow_route_collision_fails_closed() {
    let (_dir, _plane, ledger) = store();
    let first = doc("first", "same-hook", "");
    let (first_row, first_revision) = ledger.create_workflow(&first, "test", None).unwrap();
    ledger
        .activate_workflow(&first_row.name, Some(first_revision))
        .unwrap();
    let second = doc("second", "same-hook", "");
    let (second_row, second_revision) = ledger.create_workflow(&second, "test", None).unwrap();
    let error = ledger
        .activate_workflow(&second_row.name, Some(second_revision))
        .unwrap_err();
    assert!(
        error
            .to_string()
            .contains("already owned by active workflow"),
        "{error:#}"
    );
}

#[test]
fn duplicate_workflow_routes_are_rejected_on_activation() {
    let (_dir, _plane, ledger) = store();
    let mut workflow = doc("duplicate-routes", "same", "");
    workflow.triggers.push(workflow.triggers[0].clone());
    let (row, revision) = ledger.create_workflow(&workflow, "test", None).unwrap();
    let error = ledger
        .activate_workflow(&row.name, Some(revision))
        .unwrap_err();
    assert!(
        error
            .to_string()
            .contains("duplicate or empty webhook route"),
        "{error:#}"
    );
}

#[test]
fn paused_workflow_redelivery_is_duplicate_not_suppressed() {
    let (_dir, plane, ledger) = store();
    let workflow = doc("paused-dedupe", "paused", "");
    let (row, revision) = ledger.create_workflow(&workflow, "test", None).unwrap();
    ledger.activate_workflow(&row.name, Some(revision)).unwrap();
    let first = accept(
        &plane,
        &ledger,
        &TriggerEnvelope {
            workflow: row.name.clone(),
            source: TriggerSource::Manual,
            payload: Some(r#"{"id":"same"}"#.into()),
            dedupe_key: Some("same".into()),
        },
    )
    .unwrap();
    assert!(matches!(
        first,
        bitterblossom::workflow::AcceptOutcome::Accepted { .. }
    ));
    ledger.pause_workflow(&row.name, "operator test").unwrap();
    let replay = accept(
        &plane,
        &ledger,
        &TriggerEnvelope {
            workflow: row.name.clone(),
            source: TriggerSource::Manual,
            payload: Some(r#"{"id":"same"}"#.into()),
            dedupe_key: Some("same".into()),
        },
    )
    .unwrap();
    assert!(matches!(
        replay,
        bitterblossom::workflow::AcceptOutcome::Duplicate { .. }
    ));
}

#[test]
fn workflow_terminal_transitions_and_stop_requests_are_fail_closed() {
    let (_dir, plane, ledger) = store();
    let workflow = doc("transitions", "transitions", "");
    let (row, revision) = ledger.create_workflow(&workflow, "test", None).unwrap();
    ledger.activate_workflow(&row.name, Some(revision)).unwrap();
    let accepted = accept(
        &plane,
        &ledger,
        &TriggerEnvelope {
            workflow: row.name.clone(),
            source: TriggerSource::Manual,
            payload: Some(r#"{"id":"transition"}"#.into()),
            dedupe_key: Some("transition".into()),
        },
    )
    .unwrap();
    let run_id = match accepted {
        bitterblossom::workflow::AcceptOutcome::Accepted { run } => run.id,
        other => panic!("{other:?}"),
    };
    assert!(ledger
        .set_workflow_run_state(&run_id, "stopped", Some("bad shortcut"))
        .is_err());
    ledger
        .request_workflow_run_stop(&run_id, "operator stop")
        .unwrap();
    assert!(ledger.claim_workflow_run(&run_id).unwrap());
    ledger
        .set_workflow_run_state(&run_id, "stopped", Some("stopped"))
        .unwrap();
    assert!(ledger
        .request_workflow_run_stop(&run_id, "late stop")
        .is_err());
    assert!(ledger
        .set_workflow_run_state(&run_id, "succeeded", Some("invalid"))
        .is_err());
}

#[test]
fn workflow_freshness_reports_queued_running_and_attention() {
    let (_dir, plane, ledger) = store();
    let workflow = doc("freshness", "freshness", "");
    let (row, revision) = ledger.create_workflow(&workflow, "test", None).unwrap();
    ledger.activate_workflow(&row.name, Some(revision)).unwrap();
    let accepted = accept(
        &plane,
        &ledger,
        &TriggerEnvelope {
            workflow: row.name.clone(),
            source: TriggerSource::Manual,
            payload: Some(r#"{"id":"fresh"}"#.into()),
            dedupe_key: Some("fresh".into()),
        },
    )
    .unwrap();
    let run_id = match accepted {
        bitterblossom::workflow::AcceptOutcome::Accepted { run } => run.id,
        other => panic!("{other:?}"),
    };
    let queued = ledger.workflow_freshness_summary(300).unwrap();
    assert_eq!(queued["queued"], 1);
    assert_eq!(queued["running"], 0);
    assert_eq!(queued["needs_attention"], 0);
    assert!(ledger.claim_workflow_run(&run_id).unwrap());
    ledger
        .set_workflow_run_state(&run_id, "needs_attention", Some("probe unknown"))
        .unwrap();
    let attention = ledger.workflow_freshness_summary(300).unwrap();
    assert_eq!(attention["needs_attention"], 1);
}

#[test]
fn dead_letter_open_count_excludes_replayed_rows_and_list_is_complete() {
    let (_dir, _plane, ledger) = store();
    let mut ids = Vec::new();
    for n in 0..205 {
        ids.push(
            ledger
                .record_dead_letter(&format!("run-{n}"), "demo", Some("{}"), "test")
                .unwrap(),
        );
    }
    assert!(ledger
        .mark_dead_letter_replayed(ids[0], "replay-run")
        .unwrap());
    assert_eq!(ledger.open_dead_letter_count(None).unwrap(), 204);
    assert_eq!(ledger.list_dead_letters().unwrap().len(), 205);
    assert_eq!(ledger.list_dead_letters_page(200).unwrap().len(), 200);
}

#[test]
fn task_workflow_route_collision_fails_at_activation() {
    let (_dir, _plane, ledger) = store();
    let workflow = doc("workflow-owner", " Shared-Route ", "");
    let (row, revision) = ledger.create_workflow(&workflow, "test", None).unwrap();
    let error = ledger
        .activate_workflow_with_reserved_routes(
            &row.name,
            Some(revision),
            &["/shared-route/".to_string()],
        )
        .unwrap_err();
    assert!(
        error.to_string().contains("already owned by a task"),
        "{error:#}"
    );
}

#[test]
fn paused_workflow_keeps_normalized_route_reserved() {
    let (_dir, _plane, ledger) = store();
    let first = doc("paused-owner", "retained-route", "");
    let (first_row, first_revision) = ledger.create_workflow(&first, "test", None).unwrap();
    ledger
        .activate_workflow(&first_row.name, Some(first_revision))
        .unwrap();
    ledger
        .pause_workflow(&first_row.name, "operator maintenance")
        .unwrap();

    let second = doc("new-owner", " RETAINED-ROUTE ", "");
    let (second_row, second_revision) = ledger.create_workflow(&second, "test", None).unwrap();
    let error = ledger
        .activate_workflow(&second_row.name, Some(second_revision))
        .unwrap_err();
    assert!(error.to_string().contains("paused workflow"), "{error:#}");
}

#[test]
fn workflow_cron_collapse_accumulates_each_trigger_before_cursor_advance() {
    let (_dir, plane, ledger) = store();
    let workflow = WorkflowDoc::from_toml(
        r#"
name = "multi-cron"
goal = "record every discarded catch-up fire"

[[trigger]]
kind = "cron"
schedule = "*/5 * * * *"

[[trigger]]
kind = "cron"
schedule = "*/7 * * * *"

[[step]]
name = "work"
goal = "ack"
[step.agent]
name = "stub"
version = 1
harness = "command"
model = "stub"
"#,
    )
    .unwrap();
    let (row, revision) = ledger.create_workflow(&workflow, "test", None).unwrap();
    ledger.activate_workflow(&row.name, Some(revision)).unwrap();
    let last = Utc.with_ymd_and_hms(2026, 7, 21, 12, 0, 0).unwrap();
    let now = Utc.with_ymd_and_hms(2026, 7, 21, 12, 30, 0).unwrap();
    let mut cursors = std::collections::HashMap::new();
    let accepted = bitterblossom::workflow_runtime::workflow_cron_tick(
        &plane,
        &ledger,
        &mut cursors,
        last,
        now,
        2,
    )
    .unwrap();
    assert_eq!(accepted.len(), 2, "{accepted:?}");
    let collapse = ledger
        .guard_event_counts()
        .unwrap()
        .into_iter()
        .find(|event| event.kind == "workflow_cron_collapse")
        .expect("workflow cron collapse event");
    // Each trigger discarded four fires and the workflow-level cap discarded
    // two of the four retained candidates.
    assert_eq!(collapse.total, 8);
    assert_eq!(cursors.get(&row.name), Some(&now));
}

#[test]
fn resolving_recovered_run_closes_alive_step_before_terminal_state() {
    let (dir, plane, ledger) = store();
    let workflow = doc("resolve-alive", "resolve-alive", "");
    let (row, revision) = ledger.create_workflow(&workflow, "test", None).unwrap();
    ledger.activate_workflow(&row.name, Some(revision)).unwrap();
    let accepted = accept(
        &plane,
        &ledger,
        &TriggerEnvelope {
            workflow: row.name.clone(),
            source: TriggerSource::Manual,
            payload: Some(r#"{"id":"alive"}"#.into()),
            dedupe_key: Some("alive".into()),
        },
    )
    .unwrap();
    let run_id = match accepted {
        bitterblossom::workflow::AcceptOutcome::Accepted { run } => run.id,
        other => panic!("{other:?}"),
    };
    {
        let conn = rusqlite::Connection::open(dir.path().join("plane.db")).unwrap();
        conn.execute(
            "INSERT INTO workflow_step_runs
             (id, run_id, step, attempt, agent_json, goal, state, artifact_dir, authority_json, started_at)
             VALUES (?1, ?2, 'work', 1, '{}', 'ack', 'running', NULL, '[]', ?3)",
            params!["step-alive", run_id, "2026-07-21T00:00:00Z"],
        ).unwrap();
        conn.execute(
            "UPDATE workflow_run_status SET state = 'needs_attention', detail = 'probe alive', updated_at = ?2 WHERE run_id = ?1",
            params![run_id, "2026-07-21T00:00:01Z"],
        ).unwrap();
    }
    let status =
        resolve_workflow_run(&ledger, &run_id, "succeeded", "operator verified effect").unwrap();
    assert_eq!(status.state, "succeeded");
    let steps = ledger.workflow_step_runs(&run_id).unwrap();
    assert_eq!(steps.len(), 1);
    assert_eq!(steps[0].state, "failed");
    assert!(steps[0]
        .error
        .as_deref()
        .unwrap()
        .contains("operator resolved recovered step"));
}
