use bitterblossom::ledger::{phase_reached, IngressOutcome, IngressRequest, Ledger};

fn open_ledger() -> (tempfile::TempDir, Ledger) {
    let dir = tempfile::tempdir().unwrap();
    let ledger = Ledger::open(&dir.path().join("plane.db")).unwrap();
    (dir, ledger)
}

fn ingest_manual(ledger: &mut Ledger, task: &str, key: Option<&str>) -> IngressOutcome {
    ledger
        .ingest(IngressRequest {
            task,
            trigger_kind: "manual",
            idempotency_key: key,
            source_event_id: None,
            payload: Some("{\"x\":1}"),
            parent_run_id: None,
        })
        .unwrap()
}

#[test]
fn duplicate_idempotency_key_creates_one_run_two_ingress_events() {
    let (_dir, mut ledger) = open_ledger();
    let first = ingest_manual(&mut ledger, "demo", Some("X"));
    let second = ingest_manual(&mut ledger, "demo", Some("X"));
    assert!(!first.duplicate);
    assert!(second.duplicate);
    assert_eq!(first.run_id, second.run_id);
    assert_eq!(ledger.list_runs(Some("demo"), None).unwrap().len(), 1);
    assert_eq!(ledger.ingress_event_count("demo").unwrap(), 2);
}

#[test]
fn same_key_different_task_is_not_a_duplicate() {
    let (_dir, mut ledger) = open_ledger();
    let a = ingest_manual(&mut ledger, "demo", Some("X"));
    let b = ingest_manual(&mut ledger, "other", Some("X"));
    assert!(!b.duplicate);
    assert_ne!(a.run_id, b.run_id);
}

#[test]
fn keyless_ingress_always_creates_a_run() {
    let (_dir, mut ledger) = open_ledger();
    let a = ingest_manual(&mut ledger, "demo", None);
    let b = ingest_manual(&mut ledger, "demo", None);
    assert_ne!(a.run_id, b.run_id);
    assert_eq!(ledger.list_runs(Some("demo"), None).unwrap().len(), 2);
}

#[test]
fn legal_lifecycle_transitions() {
    let (_dir, mut ledger) = open_ledger();
    let run = ingest_manual(&mut ledger, "demo", None).run_id;
    assert_eq!(ledger.run_state(&run).unwrap(), "pending");
    ledger.transition(&run, "running", None).unwrap();
    ledger.transition(&run, "success", None).unwrap();
    assert_eq!(ledger.run_state(&run).unwrap(), "success");
}

#[test]
fn terminal_states_reject_transitions() {
    let (_dir, mut ledger) = open_ledger();
    let run = ingest_manual(&mut ledger, "demo", None).run_id;
    ledger.transition(&run, "running", None).unwrap();
    ledger.transition(&run, "failure", Some("boom")).unwrap();
    for to in ["running", "success", "pending"] {
        assert!(
            ledger.transition(&run, to, None).is_err(),
            "failure -> {to} must be illegal"
        );
    }
}

#[test]
fn pending_cannot_jump_to_success() {
    let (_dir, mut ledger) = open_ledger();
    let run = ingest_manual(&mut ledger, "demo", None).run_id;
    assert!(ledger.transition(&run, "success", None).is_err());
}

#[test]
fn awaiting_recovery_resolves_by_operator() {
    let (_dir, mut ledger) = open_ledger();
    let run = ingest_manual(&mut ledger, "demo", None).run_id;
    ledger.transition(&run, "running", None).unwrap();
    ledger
        .transition(&run, "awaiting_recovery", Some("orphaned"))
        .unwrap();
    ledger
        .transition(&run, "success", Some("verified by operator"))
        .unwrap();
}

#[test]
fn parked_task_ingress_is_blocked_and_unpark_releases() {
    let (_dir, mut ledger) = open_ledger();
    ledger.park_task("demo", "budget breach").unwrap();
    let outcome = ingest_manual(&mut ledger, "demo", None);
    assert_eq!(outcome.state, "blocked_budget");
    let released = ledger.unpark_task("demo").unwrap();
    assert_eq!(released, vec![outcome.run_id.clone()]);
    assert_eq!(ledger.run_state(&outcome.run_id).unwrap(), "pending");
}

#[test]
fn host_lease_is_exclusive_until_released() {
    let (_dir, ledger) = open_ledger();
    assert!(ledger.try_acquire_host_lease("host-1", "run-a").unwrap());
    assert!(!ledger.try_acquire_host_lease("host-1", "run-b").unwrap());
    assert_eq!(
        ledger.lease_holder("host-1").unwrap().as_deref(),
        Some("run-a")
    );
    ledger.release_host_lease("host-1", "run-a").unwrap();
    assert!(ledger.try_acquire_host_lease("host-1", "run-b").unwrap());
}

#[test]
fn dead_letter_replay_lineage() {
    let (_dir, mut ledger) = open_ledger();
    let run = ingest_manual(&mut ledger, "demo", None).run_id;
    let dl = ledger
        .record_dead_letter(&run, "demo", Some("{}"), "host unreachable")
        .unwrap();
    let replay = ledger
        .ingest(IngressRequest {
            task: "demo",
            trigger_kind: "replay",
            idempotency_key: Some("replay:1:abc"),
            source_event_id: None,
            payload: Some("{}"),
            parent_run_id: Some(&run),
        })
        .unwrap();
    ledger
        .mark_dead_letter_replayed(dl, &replay.run_id)
        .unwrap();
    let row = ledger.dead_letter(dl).unwrap();
    assert_eq!(row.replayed_run_id.as_deref(), Some(replay.run_id.as_str()));
    let replay_row = ledger.run(&replay.run_id).unwrap();
    assert_eq!(replay_row.parent_run_id.as_deref(), Some(run.as_str()));
}

#[test]
fn claim_is_atomic_second_claimer_loses() {
    let (_dir, mut ledger) = open_ledger();
    let run = ingest_manual(&mut ledger, "demo", None).run_id;
    assert!(ledger.try_transition(&run, "running", None).unwrap());
    assert!(!ledger.try_transition(&run, "running", None).unwrap());
    assert_eq!(ledger.run_state(&run).unwrap(), "running");
}

#[test]
fn dead_letter_replay_claim_is_atomic() {
    let (_dir, mut ledger) = open_ledger();
    let run = ingest_manual(&mut ledger, "demo", None).run_id;
    let dl = ledger
        .record_dead_letter(&run, "demo", None, "boom")
        .unwrap();
    assert!(ledger.mark_dead_letter_replayed(dl, "replay-a").unwrap());
    assert!(ledger.mark_dead_letter_replayed(dl, "replay-a").unwrap());
    assert!(!ledger.mark_dead_letter_replayed(dl, "replay-b").unwrap());
}

#[test]
fn cross_handle_ingest_dedupes_on_disk() {
    let dir = tempfile::tempdir().unwrap();
    let db = dir.path().join("plane.db");
    let mut a = Ledger::open(&db).unwrap();
    let mut b = Ledger::open(&db).unwrap();
    let first = ingest_manual(&mut a, "demo", Some("K"));
    let second = ingest_manual(&mut b, "demo", Some("K"));
    assert!(!first.duplicate);
    assert!(second.duplicate);
    assert_eq!(first.run_id, second.run_id);
    assert_eq!(a.ingress_event_count("demo").unwrap(), 2);
}

#[test]
fn attempt_phase_ordering() {
    assert!(phase_reached("executing", "executing"));
    assert!(phase_reached("released", "executing"));
    assert!(!phase_reached("prepared", "executing"));
    assert!(!phase_reached("bogus", "executing"));
}
