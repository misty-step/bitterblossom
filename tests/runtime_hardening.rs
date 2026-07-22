use bitterblossom::ledger::Ledger;
use bitterblossom::workflow::WorkflowDoc;
use bitterblossom::workflow_runtime::{accept, TriggerEnvelope, TriggerSource};

fn doc(name: &str, route: &str, policy: &str) -> WorkflowDoc {
    WorkflowDoc::from_toml(&format!(r#"
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
"#)).unwrap()
}

fn store() -> (tempfile::TempDir, Ledger) {
    let dir = tempfile::tempdir().unwrap();
    let ledger = Ledger::open(&dir.path().join("plane.db")).unwrap();
    (dir, ledger)
}

#[test]
fn workflow_admission_denial_is_atomic_and_audited() {
    let (_dir, ledger) = store();
    let workflow = doc("bounded", "bounded-hook", "max_runs_per_day = 1
concurrency = 1");
    let (row, revision) = ledger.create_workflow(&workflow, "test", None).unwrap();
    ledger.activate_workflow(&row.name, Some(revision)).unwrap();
    let accepted = accept(&ledger, &TriggerEnvelope {
        workflow: row.name.clone(),
        source: TriggerSource::Manual,
        payload: Some(r#"{"id":"one"}"#.into()),
        dedupe_key: Some("manual:one".into()),
    }).unwrap();
    assert!(matches!(accepted, bitterblossom::workflow::AcceptOutcome::Accepted { .. }));
    let denied = accept(&ledger, &TriggerEnvelope {
        workflow: row.name.clone(),
        source: TriggerSource::Manual,
        payload: Some(r#"{"id":"two"}"#.into()),
        dedupe_key: Some("manual:two".into()),
    }).unwrap();
    match denied {
        bitterblossom::workflow::AcceptOutcome::Denied { kind, .. } => assert_eq!(kind, "max_runs_per_day"),
        other => panic!("expected denial, got {other:?}"),
    }
    assert_eq!(ledger.workflow_runs(&row.name).unwrap().len(), 1);
    assert!(ledger.workflow_events(&row.name).unwrap().iter().any(|e| e.kind == "run_denied"));
}

#[test]
fn invalid_dedupe_is_refused_before_activation() {
    let (_dir, ledger) = store();
    let mut workflow = doc("bad-dedupe", "bad-dedupe-hook", "");
    workflow.triggers[0].dedupe_key = Some("missing-separator".into());
    let created = ledger.create_workflow(&workflow, "test", None);
    assert!(created.is_err());
}

#[test]
fn active_workflow_route_collision_fails_closed() {
    let (_dir, ledger) = store();
    let first = doc("first", "same-hook", "");
    let (first_row, first_revision) = ledger.create_workflow(&first, "test", None).unwrap();
    ledger.activate_workflow(&first_row.name, Some(first_revision)).unwrap();
    let second = doc("second", "same-hook", "");
    let (second_row, second_revision) = ledger.create_workflow(&second, "test", None).unwrap();
    let error = ledger.activate_workflow(&second_row.name, Some(second_revision)).unwrap_err();
    assert!(error.to_string().contains("already owned by active workflow"), "{error:#}");
}


#[test]
fn duplicate_workflow_routes_are_rejected_on_activation() {
    let (_dir, ledger) = store();
    let mut workflow = doc("duplicate-routes", "same", "");
    workflow.triggers.push(workflow.triggers[0].clone());
    let (row, revision) = ledger.create_workflow(&workflow, "test", None).unwrap();
    let error = ledger.activate_workflow(&row.name, Some(revision)).unwrap_err();
    assert!(error.to_string().contains("duplicate or empty webhook route"), "{error:#}");
}

#[test]
fn paused_workflow_redelivery_is_duplicate_not_suppressed() {
    let (_dir, ledger) = store();
    let workflow = doc("paused-dedupe", "paused", "");
    let (row, revision) = ledger.create_workflow(&workflow, "test", None).unwrap();
    ledger.activate_workflow(&row.name, Some(revision)).unwrap();
    let first = accept(&ledger, &TriggerEnvelope {
        workflow: row.name.clone(), source: TriggerSource::Manual,
        payload: Some(r#"{"id":"same"}"#.into()), dedupe_key: Some("same".into()),
    }).unwrap();
    assert!(matches!(first, bitterblossom::workflow::AcceptOutcome::Accepted { .. }));
    ledger.pause_workflow(&row.name, "operator test").unwrap();
    let replay = accept(&ledger, &TriggerEnvelope {
        workflow: row.name.clone(), source: TriggerSource::Manual,
        payload: Some(r#"{"id":"same"}"#.into()), dedupe_key: Some("same".into()),
    }).unwrap();
    assert!(matches!(replay, bitterblossom::workflow::AcceptOutcome::Duplicate { .. }));
}

#[test]
fn workflow_terminal_transitions_and_stop_requests_are_fail_closed() {
    let (_dir, ledger) = store();
    let workflow = doc("transitions", "transitions", "");
    let (row, revision) = ledger.create_workflow(&workflow, "test", None).unwrap();
    ledger.activate_workflow(&row.name, Some(revision)).unwrap();
    let accepted = accept(&ledger, &TriggerEnvelope {
        workflow: row.name.clone(), source: TriggerSource::Manual,
        payload: Some(r#"{"id":"transition"}"#.into()), dedupe_key: Some("transition".into()),
    }).unwrap();
    let run_id = match accepted { bitterblossom::workflow::AcceptOutcome::Accepted { run } => run.id, other => panic!("{other:?}") };
    assert!(ledger.set_workflow_run_state(&run_id, "stopped", Some("bad shortcut")).is_err());
    ledger.request_workflow_run_stop(&run_id, "operator stop").unwrap();
    assert!(ledger.claim_workflow_run(&run_id).unwrap());
    ledger.set_workflow_run_state(&run_id, "stopped", Some("stopped")).unwrap();
    assert!(ledger.request_workflow_run_stop(&run_id, "late stop").is_err());
    assert!(ledger.set_workflow_run_state(&run_id, "succeeded", Some("invalid")).is_err());
}

#[test]
fn workflow_freshness_reports_queued_running_and_attention() {
    let (_dir, ledger) = store();
    let workflow = doc("freshness", "freshness", "");
    let (row, revision) = ledger.create_workflow(&workflow, "test", None).unwrap();
    ledger.activate_workflow(&row.name, Some(revision)).unwrap();
    let accepted = accept(&ledger, &TriggerEnvelope {
        workflow: row.name.clone(), source: TriggerSource::Manual,
        payload: Some(r#"{"id":"fresh"}"#.into()), dedupe_key: Some("fresh".into()),
    }).unwrap();
    let run_id = match accepted { bitterblossom::workflow::AcceptOutcome::Accepted { run } => run.id, other => panic!("{other:?}") };
    let queued = ledger.workflow_freshness_summary(300).unwrap();
    assert_eq!(queued["queued"], 1);
    assert_eq!(queued["running"], 0);
    assert_eq!(queued["needs_attention"], 0);
    assert!(ledger.claim_workflow_run(&run_id).unwrap());
    ledger.set_workflow_run_state(&run_id, "needs_attention", Some("probe unknown")).unwrap();
    let attention = ledger.workflow_freshness_summary(300).unwrap();
    assert_eq!(attention["needs_attention"], 1);
}
