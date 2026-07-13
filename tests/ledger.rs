use bitterblossom::ledger::{
    phase_reached, IngressOutcome, IngressRequest, Ledger, LEDGER_SCHEMA_VERSION,
};

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
fn dead_letter_replay_and_acknowledge_are_mutually_exclusive_in_ledger() {
    let (_dir, mut ledger) = open_ledger();
    let run = ingest_manual(&mut ledger, "demo", None).run_id;
    let acked = ledger
        .record_dead_letter(&run, "demo", None, "missing secret")
        .unwrap();
    ledger
        .acknowledge_dead_letter(acked, " superseded by replacement ")
        .unwrap();
    let row = ledger.dead_letter(acked).unwrap();
    assert_eq!(row.status, "acknowledged");
    assert_eq!(
        row.acknowledged_reason.as_deref(),
        Some("superseded by replacement")
    );
    let events = ledger.events(&run).unwrap();
    assert!(events.iter().any(|e| {
        e.kind == "dlq:acknowledged" && e.data.as_deref() == Some("superseded by replacement")
    }));
    assert!(
        !ledger.mark_dead_letter_replayed(acked, "replay-a").unwrap(),
        "acknowledged DLQs cannot later be claimed for replay"
    );

    let replayed = ledger
        .record_dead_letter(&run, "demo", None, "bad command")
        .unwrap();
    assert!(ledger
        .mark_dead_letter_replayed(replayed, "replay-b")
        .unwrap());
    let err = match ledger.acknowledge_dead_letter(replayed, "superseded") {
        Ok(_) => panic!("replayed DLQs cannot be acknowledged"),
        Err(err) => err.to_string(),
    };
    assert!(err.contains("already replayed"), "{err}");
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

#[test]
fn open_stamps_current_schema_version() {
    let (_dir, ledger) = open_ledger();
    assert_eq!(ledger.schema_version().unwrap(), LEDGER_SCHEMA_VERSION);
}

#[test]
fn open_refuses_newer_schema_before_running_migrations() {
    let dir = tempfile::tempdir().unwrap();
    let db = dir.path().join("plane.db");
    let conn = rusqlite::Connection::open(&db).unwrap();
    conn.pragma_update(None, "user_version", LEDGER_SCHEMA_VERSION + 1)
        .unwrap();
    drop(conn);

    let err = match Ledger::open(&db) {
        Ok(_) => panic!("newer schema should be refused"),
        Err(err) => err.to_string(),
    };
    assert!(err.contains("newer than this bb binary supports"), "{err}");
    assert!(
        err.contains("roll forward or restore a compatible backup"),
        "{err}"
    );
}

// bitterblossom-930: the asks ledger primitive, exercised directly (no HTTP,
// no subprocess) so it counts as unit coverage of the state machine itself.
#[test]
fn ask_lifecycle_open_answer_park_and_reject_double_answer() {
    let (_dir, mut ledger) = open_ledger();
    let run_id = ingest_manual(&mut ledger, "demo", None).run_id;
    ledger.transition(&run_id, "running", None).unwrap();
    ledger.set_run_ask_token(&run_id, "secret-token").unwrap();
    assert_eq!(
        ledger.run_ask_token(&run_id).unwrap().as_deref(),
        Some("secret-token")
    );

    let ask = ledger
        .raise_ask(
            "ask-1",
            &run_id,
            "demo",
            "question",
            "may I proceed?",
            Some("{\"evidence\":[]}"),
            true,
            600,
        )
        .unwrap();
    assert_eq!(ask.state, "open");
    assert!(ask.answer.is_none());
    assert_eq!(ledger.asks_for_run(&run_id).unwrap().len(), 1);

    // Well within the window: polling for expiry is a no-op.
    let still_open = ledger.park_ask_if_expired("ask-1").unwrap();
    assert_eq!(still_open.state, "open");

    let answered = ledger.answer_ask("ask-1", "go ahead", "operator").unwrap();
    assert_eq!(answered.state, "answered");
    assert_eq!(answered.answer.as_deref(), Some("go ahead"));
    assert_eq!(answered.answered_by.as_deref(), Some("operator"));
    assert!(answered.answered_at.is_some());

    let err = ledger
        .answer_ask("ask-1", "different", "operator")
        .unwrap_err()
        .to_string();
    assert!(err.contains("already answered"), "{err}");
}

#[test]
fn ask_parks_once_its_window_has_elapsed() {
    let (_dir, mut ledger) = open_ledger();
    let run_id = ingest_manual(&mut ledger, "demo", None).run_id;
    ledger.transition(&run_id, "running", None).unwrap();
    ledger.set_run_ask_token(&run_id, "secret-token").unwrap();

    ledger
        .raise_ask(
            "ask-2", &run_id, "demo", "approval", "deploy?", None, true, 0,
        )
        .unwrap();
    // window_seconds = 0: any elapsed time parks it.
    let parked = ledger.park_ask_if_expired("ask-2").unwrap();
    assert_eq!(parked.state, "parked");

    // Answering a parked ask still records the answer -- the caller (the
    // serve.rs route) is responsible for deciding whether that also creates
    // a resume run, based on the owning run's own state, not the ask row.
    let answered = ledger.answer_ask("ask-2", "approve", "operator").unwrap();
    assert_eq!(answered.state, "answered");
    assert_eq!(answered.answer.as_deref(), Some("approve"));
}

#[test]
fn unanswered_asks_lists_open_and_parked_but_not_answered() {
    let (_dir, mut ledger) = open_ledger();
    let run_id = ingest_manual(&mut ledger, "demo", None).run_id;
    ledger.transition(&run_id, "running", None).unwrap();

    ledger
        .raise_ask(
            "ask-open",
            &run_id,
            "demo",
            "question",
            "still running?",
            None,
            true,
            600,
        )
        .unwrap();
    ledger
        .raise_ask(
            "ask-parked",
            &run_id,
            "demo",
            "decision",
            "resume later?",
            None,
            true,
            0,
        )
        .unwrap();
    ledger.park_ask_if_expired("ask-parked").unwrap();
    ledger
        .raise_ask(
            "ask-answered",
            &run_id,
            "demo",
            "approval",
            "ship?",
            None,
            true,
            600,
        )
        .unwrap();
    ledger
        .answer_ask("ask-answered", "yes", "operator")
        .unwrap();

    let asks = ledger.unanswered_asks().unwrap();
    assert_eq!(
        asks.iter().map(|ask| ask.id.as_str()).collect::<Vec<_>>(),
        vec!["ask-open", "ask-parked"]
    );
}

#[test]
fn answered_parked_ask_stays_actionable_until_resume_child_exists() {
    let (_dir, mut ledger) = open_ledger();
    let run_id = ingest_manual(&mut ledger, "demo", None).run_id;
    ledger.transition(&run_id, "running", None).unwrap();
    ledger
        .raise_ask(
            "ask-recovery",
            &run_id,
            "demo",
            "question",
            "continue?",
            None,
            true,
            0,
        )
        .unwrap();
    ledger.park_ask_if_expired("ask-recovery").unwrap();
    ledger
        .transition(&run_id, "parked_on_ask", Some("waiting"))
        .unwrap();
    ledger
        .answer_ask("ask-recovery", "continue", "operator")
        .unwrap();
    ledger
        .answer_ask("ask-recovery", "continue", "operator")
        .expect("identical retry is idempotent");

    assert_eq!(
        ledger
            .unanswered_asks()
            .unwrap()
            .iter()
            .map(|ask| ask.id.as_str())
            .collect::<Vec<_>>(),
        vec!["ask-recovery"]
    );

    ledger
        .ingest(IngressRequest {
            task: "demo",
            trigger_kind: "replay",
            idempotency_key: Some("unrelated-child"),
            source_event_id: None,
            payload: Some("{}"),
            parent_run_id: Some(&run_id),
        })
        .unwrap();
    assert_eq!(
        ledger
            .unanswered_asks()
            .unwrap()
            .iter()
            .map(|ask| ask.id.as_str())
            .collect::<Vec<_>>(),
        vec!["ask-recovery"],
        "an unrelated child must not hide a failed resume"
    );

    ledger
        .ingest(IngressRequest {
            task: "demo",
            trigger_kind: "resume",
            idempotency_key: Some("resume:ask-recovery"),
            source_event_id: None,
            payload: Some("{}"),
            parent_run_id: Some(&run_id),
        })
        .unwrap();
    assert!(ledger.unanswered_asks().unwrap().is_empty());
}
