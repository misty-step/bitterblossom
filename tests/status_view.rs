use std::fs;

use bitterblossom::health;
use bitterblossom::ledger::{IngressRequest, Ledger};
use bitterblossom::spec::Plane;

fn write_plane(root: &std::path::Path) {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::write(
        root.join("agents/a.toml"),
        "version = 3\nharness = \"command\"\nmodel = \"\"\nbin = \"true\"\n",
    )
    .unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
    for task in ["review", "security", "verify", "product", "correctness"] {
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

    let doc = health::status_view(&plane, &ledger).unwrap();
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
}
