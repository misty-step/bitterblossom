use serde_json::Value;

#[test]
fn dry_run_report_fixture_selects_ready_and_shapes_vague_without_mutation() {
    let report: Value = serde_json::from_str(include_str!(
        "fixtures/contracts/bb.backlog_chewer_dry_run_report.v1.valid.json"
    ))
    .unwrap();

    assert_eq!(
        report["schema_version"],
        "bb.backlog_chewer_dry_run_report.v1"
    );
    assert_eq!(report["mode"], "dry_run");
    assert_eq!(report["authority"]["current"], "dry-run");
    assert_eq!(report["authority"]["no_side_effects"], true);
    assert_eq!(report["artifact_paths"], serde_json::json!(["REPORT.json"]));

    let forbidden = report["authority"]["forbidden_actions"].as_array().unwrap();
    for action in ["branch", "pr", "merge", "deploy", "code_edit"] {
        assert!(
            forbidden.iter().any(|value| value == action),
            "forbidden action missing {action}"
        );
    }

    let selected = &report["selected_ticket"];
    assert_eq!(selected["id"], "001-ready");
    assert_eq!(selected["readiness"], "ready");
    assert!(selected["goal"].as_str().unwrap().contains("Add a receipt"));
    assert!(selected["oracle"].as_array().unwrap().len() >= 2);
    assert!(selected["verifier"]
        .as_str()
        .unwrap()
        .contains("cargo test"));
    assert!(selected["branch_name"]
        .as_str()
        .unwrap()
        .contains("bb/build/001-ready"));
    assert!(!selected["expected_changed_paths"]
        .as_array()
        .unwrap()
        .is_empty());
    assert!(!selected["stop_conditions"].as_array().unwrap().is_empty());

    let shaping = report["shaping_packets"].as_array().unwrap();
    assert!(shaping.iter().any(|ticket| ticket["id"] == "002-vague"
        && ticket["reason"]
            .as_str()
            .unwrap()
            .contains("missing executable oracle")));

    let skipped = report["skipped_tickets"].as_array().unwrap();
    assert!(skipped.iter().any(|ticket| ticket["id"] == "003-blocked"
        && ticket["reason"].as_str().unwrap().contains("blocked")));
    assert!(skipped.iter().any(|ticket| ticket["id"] == "004-dangerous"
        && ticket["reason"].as_str().unwrap().contains("destructive")));

    assert!(report["suggested_next_run"]
        .as_str()
        .unwrap()
        .contains("bb run backlog-chewer-dry-run --payload-file"));
}

#[test]
fn fixture_backlog_contains_ready_vague_blocked_and_dangerous_examples() {
    let ready = include_str!("fixtures/backlog-chewer/backlog.d/001-ready.md");
    let vague = include_str!("fixtures/backlog-chewer/backlog.d/002-vague.md");
    let blocked = include_str!("fixtures/backlog-chewer/backlog.d/003-blocked.md");
    let dangerous = include_str!("fixtures/backlog-chewer/backlog.d/004-dangerous.md");

    assert!(ready.contains("## Goal"));
    assert!(ready.contains("## Oracle"));
    assert!(ready.contains("./scripts/verify.sh"));
    assert!(vague.contains("needs brainstorming"));
    assert!(!vague.contains("## Oracle"));
    assert!(blocked.contains("Status: blocked"));
    assert!(dangerous.contains("destructive"));
}
