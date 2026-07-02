//! Boot recovery: inherited `running` runs are classified — never blindly
//! orphaned — and only pre-execute attempts become mechanically replayable
//! dead letters.

use std::fs;
use std::path::Path;
use std::process::Command;

use bitterblossom::dispatch::{attempt_dir, attempt_marker};
use bitterblossom::ledger::{IngressRequest, Ledger};
use bitterblossom::recovery::recover_inherited_runs;
use bitterblossom::spec::Plane;

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

fn bb(args: &[&str]) -> std::process::Output {
    Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(args)
        .output()
        .unwrap()
}

#[test]
fn pre_execute_inherited_run_dead_letters_for_replay() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let run_id = running_run(&mut ledger, "demo");
    let attempt = ledger
        .create_attempt(&run_id, 1, "a", 1, "claude", "m")
        .unwrap();
    ledger.set_attempt_phase(attempt, "prepared").unwrap();

    let reports = recover_inherited_runs(&plane, &mut ledger).unwrap();
    assert_eq!(reports.len(), 1);
    assert!(reports[0].disposition.starts_with("dead_letter:"));
    assert_eq!(ledger.run_state(&run_id).unwrap(), "failure");
    assert_eq!(ledger.list_dead_letters().unwrap().len(), 1);
}

#[test]
fn executing_inherited_run_with_dead_process_awaits_recovery() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let run_id = running_run(&mut ledger, "demo");
    let attempt = ledger
        .create_attempt(&run_id, 1, "a", 1, "claude", "m")
        .unwrap();
    ledger.set_attempt_phase(attempt, "executing").unwrap();
    // A pidfile pointing at a long-gone pid: probe says Dead.
    let adir = attempt_dir(&plane, &run_id, 1);
    fs::create_dir_all(&adir).unwrap();
    fs::write(adir.join("harness.pid"), "999999").unwrap();

    let reports = recover_inherited_runs(&plane, &mut ledger).unwrap();
    assert_eq!(reports[0].disposition, "awaiting_recovery");
    assert_eq!(reports[0].probe.as_deref(), Some("dead"));
    assert_eq!(reports[0].probe_state.as_deref(), Some("dead"));
    assert_eq!(reports[0].lease_disposition.as_deref(), Some("released"));
    assert_eq!(ledger.run_state(&run_id).unwrap(), "awaiting_recovery");
    // No mechanical replay path: executing attempts never dead-letter.
    assert!(ledger.list_dead_letters().unwrap().is_empty());

    // Operator resolves after checking side effects.
    ledger
        .transition(&run_id, "success", Some("verified externally"))
        .unwrap();
    let _ = attempt_marker(attempt);
}

#[test]
fn unknown_probe_evidence_is_visible_for_operator_resolution() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let run_id = running_run(&mut ledger, "demo");
    let attempt = ledger
        .create_attempt(&run_id, 1, "a", 1, "claude", "m")
        .unwrap();
    ledger.set_attempt_phase(attempt, "executing").unwrap();
    let adir = attempt_dir(&plane, &run_id, 1);
    fs::create_dir_all(&adir).unwrap();
    fs::write(adir.join("harness.pid"), "not-a-pid").unwrap();
    assert!(ledger.try_acquire_host_lease("local", &run_id).unwrap());
    drop(ledger);

    let root = dir.path().to_str().unwrap();
    let recovered = bb(&["--config", root, "recover", "--json"]);
    assert!(
        recovered.status.success(),
        "{}",
        String::from_utf8_lossy(&recovered.stderr)
    );
    let reports: serde_json::Value = serde_json::from_slice(&recovered.stdout).unwrap();
    assert_eq!(reports[0]["disposition"], "awaiting_recovery");
    assert_eq!(reports[0]["probe_state"], "unknown");
    assert_eq!(reports[0]["probe_reason"], "unparseable pidfile");
    assert_eq!(reports[0]["lease_disposition"], "retained");
    assert_eq!(
        reports[0]["operator_action"],
        "inspect_side_effects_then_resolve"
    );
    assert!(reports[0]["probe"]
        .as_str()
        .unwrap()
        .starts_with("unknown: unparseable pidfile"));

    let shown = bb(&["--config", root, "runs", "show", &run_id, "--json"]);
    assert!(shown.status.success());
    let doc: serde_json::Value = serde_json::from_slice(&shown.stdout).unwrap();
    assert_eq!(doc["run"]["state"], "awaiting_recovery");
    assert!(doc["run"]["state_reason"]
        .as_str()
        .unwrap()
        .contains("probe: unknown: unparseable pidfile"));
    assert!(doc["events"].as_array().unwrap().iter().any(|e| {
        e["kind"] == "boot_probe"
            && e["data"]
                .as_str()
                .unwrap()
                .starts_with("unknown: unparseable pidfile")
    }));

    let ledger = Ledger::open(&plane.db_path()).unwrap();
    assert_eq!(
        ledger.lease_holder("local").unwrap().as_deref(),
        Some(run_id.as_str()),
        "unknown probes must not release a possibly live host lease"
    );
}

#[test]
fn missing_probe_marker_is_unknown_and_retains_host_lease() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let run_id = running_run(&mut ledger, "demo");
    let attempt = ledger
        .create_attempt(&run_id, 1, "a", 1, "claude", "m")
        .unwrap();
    ledger.set_attempt_phase(attempt, "executing").unwrap();
    fs::create_dir_all(attempt_dir(&plane, &run_id, 1)).unwrap();
    assert!(ledger.try_acquire_host_lease("local", &run_id).unwrap());

    let reports = recover_inherited_runs(&plane, &mut ledger).unwrap();
    assert_eq!(reports[0].disposition, "awaiting_recovery");
    assert_eq!(reports[0].probe_state.as_deref(), Some("unknown"));
    assert!(reports[0]
        .probe_reason
        .as_deref()
        .unwrap()
        .contains("no pidfile"));
    assert_eq!(reports[0].lease_disposition.as_deref(), Some("retained"));
    assert_eq!(
        ledger.lease_holder("local").unwrap().as_deref(),
        Some(run_id.as_str())
    );
}

#[test]
fn executing_inherited_run_with_live_process_stays_running() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let run_id = running_run(&mut ledger, "demo");
    let attempt = ledger
        .create_attempt(&run_id, 1, "a", 1, "claude", "m")
        .unwrap();
    ledger.set_attempt_phase(attempt, "executing").unwrap();
    // Our own pid is definitely alive.
    let adir = attempt_dir(&plane, &run_id, 1);
    fs::create_dir_all(&adir).unwrap();
    fs::write(adir.join("harness.pid"), std::process::id().to_string()).unwrap();
    assert!(ledger.try_acquire_host_lease("local", &run_id).unwrap());

    let reports = recover_inherited_runs(&plane, &mut ledger).unwrap();
    assert_eq!(reports[0].disposition, "still_running");
    assert_eq!(reports[0].probe_state.as_deref(), Some("alive"));
    assert_eq!(reports[0].lease_disposition.as_deref(), Some("retained"));
    assert_eq!(ledger.run_state(&run_id).unwrap(), "running");
    // The host is genuinely busy: the lease must survive recovery.
    assert_eq!(
        ledger.lease_holder("local").unwrap().as_deref(),
        Some(run_id.as_str())
    );
}

#[test]
fn recovery_releases_stale_host_leases() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let run_id = running_run(&mut ledger, "demo");
    assert!(ledger.try_acquire_host_lease("local", &run_id).unwrap());

    recover_inherited_runs(&plane, &mut ledger).unwrap();
    assert_eq!(ledger.lease_holder("local").unwrap(), None);
}
