//! Backlog 083 guardrail status contracts that need controlled ledger state.

use std::fs;
use std::path::Path;

use bitterblossom::health;
use bitterblossom::ledger::{AttemptStats, IngressRequest, Ledger};
use bitterblossom::spec::Plane;

fn write_plane(root: &Path) -> Plane {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
    fs::write(
        root.join("agents/a.toml"),
        "version = 1\nharness = \"command\"\nmodel = \"\"\nbin = \"/usr/bin/true\"\n",
    )
    .unwrap();
    fs::write(root.join("tasks/demo/card.md"), "card\n").unwrap();
    fs::write(
        root.join("tasks/demo/task.toml"),
        "agent = \"a\"\nsubstrate = \"local\"\n[budget]\nmax_cost_per_run_usd = 0.75\n\
         [[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
    Plane::load(root).unwrap()
}

#[test]
fn status_exposes_in_flight_reserved_spend_policy() {
    let dir = tempfile::tempdir().unwrap();
    let plane = write_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let run_id = ledger
        .ingest(IngressRequest {
            task: "demo",
            trigger_kind: "manual",
            idempotency_key: None,
            source_event_id: None,
            payload: None,
            parent_run_id: None,
        })
        .unwrap()
        .run_id;
    ledger.transition(&run_id, "running", None).unwrap();
    let attempt = ledger
        .create_attempt(&run_id, 1, "a", 1, "command", "")
        .unwrap();
    ledger
        .finish_attempt(
            attempt,
            "success",
            None,
            Some(0),
            &AttemptStats {
                cost_usd: Some(0.2),
                ..Default::default()
            },
            None,
        )
        .unwrap();

    let status = health::status_view(&plane, &ledger).unwrap();
    assert_eq!(status["guards"]["in_flight"]["runs"], 1);
    assert_eq!(status["guards"]["in_flight"]["cost_usd"], 0.2);
    assert_eq!(status["guards"]["in_flight"]["reserved_usd"], 0.75);
    assert_eq!(status["guards"]["in_flight"]["spent_today_usd"], 0.2);
    assert!(status["guards"]["in_flight"]["enforcement_mode"]
        .as_str()
        .unwrap()
        .contains("default kill"));
    assert!(status["guards"]["in_flight"]["policy"]
        .as_str()
        .unwrap()
        .contains("max_cost_per_run_usd"));
    let task = status["tasks"]
        .as_array()
        .unwrap()
        .iter()
        .find(|t| t["task"] == "demo")
        .unwrap();
    assert_eq!(task["budget"]["cost_enforcement"]["mode"], "kill");
    assert_eq!(task["budget"]["cost_enforcement"]["in_flight"], true);
}
