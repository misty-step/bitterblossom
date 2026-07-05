//! bitterblossom-120: docs-sync (report-only) + docs-sync-pr (PR-only)
//! authority separation, artifact schema, and duplicate-PR-suppression
//! prose, following the canary-triage/canary-remediate precedent documented
//! in docs/rollout-scorecards.md.

use std::fs;
use std::path::Path;

use bitterblossom::spec::Plane;
use serde_json::Value;

fn docs_sync_plane_root() -> std::path::PathBuf {
    Path::new(env!("CARGO_MANIFEST_DIR")).join("examples/docs-sync-plane")
}

#[test]
fn docs_sync_plane_loads_with_separated_rollout_authority() {
    let plane = Plane::load(&docs_sync_plane_root()).unwrap();

    let docs_sync = plane.tasks.get("docs-sync").expect("docs-sync task");
    assert_eq!(
        docs_sync.spec.rollout.authority.as_deref(),
        Some("report-only")
    );

    let docs_sync_pr = plane.tasks.get("docs-sync-pr").expect("docs-sync-pr task");
    assert_eq!(
        docs_sync_pr.spec.rollout.authority.as_deref(),
        Some("PR-only")
    );

    // Never the same authority level doing both jobs.
    assert_ne!(
        docs_sync.spec.rollout.authority,
        docs_sync_pr.spec.rollout.authority
    );

    // PR-only is scoped to a narrower repo allowlist than report-only.
    assert!(docs_sync.spec.workspace.repos.len() > docs_sync_pr.spec.workspace.repos.len());

    // PR-only is manual-dispatch only; report-only also accepts cron/webhook.
    assert_eq!(docs_sync_pr.spec.triggers.len(), 1);
    assert!(docs_sync.spec.triggers.len() > 1);
}

#[test]
fn docs_sync_pr_card_names_duplicate_check_and_forbidden_actions() {
    let card =
        fs::read_to_string(docs_sync_plane_root().join("tasks/docs-sync-pr/card.md")).unwrap();

    // Duplicate-active-PR suppression: agent-verified prose, matching the
    // canary-remediate precedent (no plane-level mechanism backs a
    // manual-only task here).
    assert!(card.contains("existing open PR"));
    assert!(card.contains("gh pr list"));

    // Authority separation and boundaries, in prose the agent reads.
    assert!(card.contains("must never"));
    assert!(card.contains("merge"));
    assert!(card.contains("exactly one pull request") || card.contains("one pull request"));
    assert!(card.contains("Manual dispatch only"));
}

#[test]
fn docs_sync_report_sample_matches_declared_schema() {
    let raw = fs::read_to_string(docs_sync_plane_root().join("samples/REPORT.json")).unwrap();
    let value: Value = serde_json::from_str(&raw).unwrap();

    assert_eq!(value["schema"], "bb.docs_sync.report.v2");
    for field in [
        "repo",
        "trigger",
        "changed_files",
        "docs_targets",
        "drift_findings",
        "recommended_changes",
        "skipped_mutations",
        "artifacts",
        "cost_usd",
        "residual_risk",
    ] {
        assert!(
            value.get(field).is_some(),
            "docs-sync REPORT.json sample missing field '{field}'"
        );
    }
    assert!(value["trigger"]["source_ref"].is_string());
    assert!(value["artifacts"]
        .as_array()
        .unwrap()
        .contains(&Value::String("REPORT.json".into())));
}

#[test]
fn docs_sync_pr_report_sample_matches_declared_schema() {
    let raw = fs::read_to_string(docs_sync_plane_root().join("samples/REPORT-pr.json")).unwrap();
    let value: Value = serde_json::from_str(&raw).unwrap();

    assert_eq!(value["schema"], "bb.docs_sync_pr.report.v1");
    for field in [
        "repo",
        "source_report",
        "duplicate_check",
        "pr",
        "changed_files",
        "forbidden_actions_confirmed",
        "artifacts",
        "cost_usd",
        "residual_risk",
    ] {
        assert!(
            value.get(field).is_some(),
            "docs-sync-pr REPORT-pr.json sample missing field '{field}'"
        );
    }
    assert!(value["duplicate_check"]["existing_open_pr"].is_boolean());
    assert!(value["pr"]["url"].is_string());
}
