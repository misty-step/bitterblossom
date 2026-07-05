//! bitterblossom-121: ci-audit (report-only) + ci-audit-pr (PR-only)
//! authority separation, manual payload validation, artifact schema, and
//! duplicate-active-work refusal, following the same canary-triage/
//! canary-remediate and docs-sync/docs-sync-pr precedent (bitterblossom-120).

use std::fs;
use std::path::Path;

use bitterblossom::spec::Plane;
use serde_json::Value;

fn ci_audit_plane_root() -> std::path::PathBuf {
    Path::new(env!("CARGO_MANIFEST_DIR")).join("examples/ci-audit-plane")
}

#[test]
fn ci_audit_plane_loads_with_separated_rollout_authority() {
    let plane = Plane::load(&ci_audit_plane_root()).unwrap();

    let audit = plane.tasks.get("ci-audit").expect("ci-audit task");
    assert_eq!(audit.spec.rollout.authority.as_deref(), Some("report-only"));

    let audit_pr = plane.tasks.get("ci-audit-pr").expect("ci-audit-pr task");
    assert_eq!(audit_pr.spec.rollout.authority.as_deref(), Some("PR-only"));

    // Never the same authority level doing both jobs.
    assert_ne!(
        audit.spec.rollout.authority,
        audit_pr.spec.rollout.authority
    );

    // PR-only is scoped to a narrower repo allowlist than report-only.
    assert!(audit.spec.workspace.repos.len() > audit_pr.spec.workspace.repos.len());

    // PR-only is manual-dispatch only; report-only also accepts cron.
    assert_eq!(audit_pr.spec.triggers.len(), 1);
    assert!(audit.spec.triggers.len() > 1);

    // ci-audit is proactive (cron-scheduled), never webhook-reactive --
    // that job belongs to the separate ci-diagnose reflex.
    assert!(!audit
        .spec
        .triggers
        .iter()
        .any(|t| matches!(t, bitterblossom::spec::TriggerSpec::Webhook { .. })));
}

#[test]
fn ci_audit_card_documents_and_validates_the_manual_repo_payload() {
    let card = fs::read_to_string(ci_audit_plane_root().join("tasks/ci-audit/card.md")).unwrap();

    // The manual payload contract: names a repo, must be in the allowlist,
    // a mismatched repo is refused before any audit runs.
    assert!(card.contains("\"repo\""));
    assert!(card.contains("workspace.repos"));
    assert!(card.contains("refused"));

    let sample: Value = serde_json::from_str(
        &fs::read_to_string(ci_audit_plane_root().join("samples/manual-audit-payload.json"))
            .unwrap(),
    )
    .unwrap();
    let payload_repo = sample["repo"].as_str().unwrap();

    let plane = Plane::load(&ci_audit_plane_root()).unwrap();
    let audit = plane.tasks.get("ci-audit").unwrap();
    assert!(
        audit
            .spec
            .workspace
            .repos
            .iter()
            .any(|r| r.url.contains(payload_repo)),
        "sample manual payload names a repo not in ci-audit's own allowlist"
    );
}

#[test]
fn ci_audit_pr_card_names_duplicate_check_and_never_weakens_gates() {
    let card = fs::read_to_string(ci_audit_plane_root().join("tasks/ci-audit-pr/card.md")).unwrap();

    // Duplicate-active-work (open PR) refusal: agent-verified prose, same
    // contract as canary-remediate and docs-sync-pr.
    assert!(card.contains("existing open PR"));
    assert!(card.contains("gh pr list"));

    // The one absolute red line for this task family: never weaken a gate.
    assert!(card.contains("must never"));
    assert!(card.contains("weaken"));
    assert!(card.contains("gates_weakened"));
    assert!(card.contains("Manual dispatch only"));
}

#[test]
fn ci_audit_report_sample_matches_declared_schema() {
    let raw = fs::read_to_string(ci_audit_plane_root().join("samples/REPORT.json")).unwrap();
    let value: Value = serde_json::from_str(&raw).unwrap();

    assert_eq!(value["schema"], "bb.ci_audit.report.v1");
    for field in [
        "repo",
        "trigger",
        "current_gates",
        "missing_or_weak_gates",
        "proposed_checks",
        "risk",
        "reproduction_commands",
        "artifacts",
        "cost_usd",
        "residual_risk",
    ] {
        assert!(
            value.get(field).is_some(),
            "ci-audit REPORT.json sample missing field '{field}'"
        );
    }
    assert!(!value["reproduction_commands"]
        .as_array()
        .unwrap()
        .is_empty());
}

#[test]
fn ci_audit_pr_report_sample_matches_declared_schema_and_never_weakens_a_gate() {
    let raw = fs::read_to_string(ci_audit_plane_root().join("samples/REPORT-pr.json")).unwrap();
    let value: Value = serde_json::from_str(&raw).unwrap();

    assert_eq!(value["schema"], "bb.ci_audit_pr.report.v1");
    for field in [
        "repo",
        "source_report",
        "duplicate_check",
        "pr",
        "gates_added",
        "gates_weakened",
        "artifacts",
        "cost_usd",
        "residual_risk",
    ] {
        assert!(
            value.get(field).is_some(),
            "ci-audit-pr REPORT-pr.json sample missing field '{field}'"
        );
    }
    // The one mechanically-checkable invariant this schema exists to prove:
    // a PR-only hardening run must never weaken an existing gate.
    assert!(
        value["gates_weakened"].as_array().unwrap().is_empty(),
        "sample ci-audit-pr report must demonstrate gates_weakened is empty"
    );
    assert!(value["duplicate_check"]["existing_open_pr"].is_boolean());
}
