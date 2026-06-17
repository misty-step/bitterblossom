//! Lifecycle reflex packet contracts. These tests intentionally exercise the
//! checked-in plane files instead of a synthetic-only fixture so lifecycle
//! slices stay data-owned and visible to agents.

use std::fs;
use std::path::{Path, PathBuf};

use bitterblossom::ingress::{handle_webhook, sign_hmac};
use bitterblossom::ledger::Ledger;
use bitterblossom::spec::{AuthClass, Plane, TriggerSpec, WebhookActionSpec};

fn repo_root() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
}

#[test]
fn ci_diagnose_task_is_api_auth_reflex_contract() {
    let plane = Plane::load(&repo_root().join("plane")).unwrap();
    let task = plane.task("ci-diagnose").unwrap();

    assert_eq!(task.agent_name, "ci-diagnoser");
    assert_eq!(task.agent.harness, "pi");
    assert_eq!(task.agent.model, "deepseek/deepseek-v4-flash");
    assert_eq!(task.agent.auth_class().unwrap(), AuthClass::Api);
    assert_eq!(task.agent.role.as_deref(), Some("diagnoser"));
    assert!(task
        .agent
        .skills
        .contains(&"harness-kit/diagnose#ci-failure".to_string()));

    let webhook = task
        .spec
        .triggers
        .iter()
        .find_map(|trigger| match trigger {
            TriggerSpec::Webhook {
                route,
                dedupe_key,
                filter,
                ..
            } => Some((route, dedupe_key, filter)),
            TriggerSpec::Manual | TriggerSpec::Cron { .. } => None,
        })
        .expect("ci-diagnose webhook trigger");
    assert_eq!(webhook.0, "ci-diagnose");
    assert_eq!(webhook.1.as_deref(), Some("json:/check_suite/head_sha"));
    assert!(webhook
        .2
        .iter()
        .any(|f| f.pointer == "/check_suite/conclusion"));

    for required in [
        "check_suite.failed",
        "RUN.json",
        "report `task` from `RUN.json`",
        "\"event\"",
        "\"task\"",
        "\"repo\"",
        "\"rev\"",
        "\"claim\"",
        "\"evidence\"",
        "\"suggested_next_run\"",
        "\"cost_usd\"",
        "\"artifact_paths\"",
        r#""artifact_paths": ["REPORT.json"]"#,
        "\"residual_risk\"",
    ] {
        assert!(task.card.contains(required), "card missing {required}");
    }
}

#[test]
fn ci_diagnose_webhook_filters_failed_bitterblossom_check_suites() {
    let dir = tempfile::tempdir().unwrap();
    let plane = temp_ci_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    std::env::set_var("BB_HOOK_CI_DIAGNOSE", "s3cret");

    let deliver = |ledger: &mut Ledger, body: &str, delivery: &str| {
        let sig = sign_hmac("s3cret", body.as_bytes());
        handle_webhook(
            &plane,
            ledger,
            "ci-diagnose",
            &[
                ("X-Hub-Signature-256".to_string(), sig),
                ("X-GitHub-Delivery".to_string(), delivery.to_string()),
            ],
            body,
        )
        .unwrap()
    };

    let in_scope = r#"{"action":"completed","repository":{"full_name":"misty-step/bitterblossom"},"check_suite":{"head_sha":"deadbeef","status":"completed","conclusion":"failure","app":{"slug":"github-actions"}}}"#;
    assert_eq!(deliver(&mut ledger, in_scope, "d1").status, 202);

    for (body, delivery) in [
        (
            r#"{"action":"completed","repository":{"full_name":"elsewhere/repo"},"check_suite":{"head_sha":"a","status":"completed","conclusion":"failure","app":{"slug":"github-actions"}}}"#,
            "d2",
        ),
        (
            r#"{"action":"completed","repository":{"full_name":"misty-step/bitterblossom"},"check_suite":{"head_sha":"b","status":"completed","conclusion":"success","app":{"slug":"github-actions"}}}"#,
            "d3",
        ),
        (
            r#"{"action":"requested","repository":{"full_name":"misty-step/bitterblossom"},"check_suite":{"head_sha":"c","status":"queued","conclusion":"failure","app":{"slug":"github-actions"}}}"#,
            "d4",
        ),
        (
            r#"{"action":"completed","repository":{"full_name":"misty-step/bitterblossom"},"check_suite":{"head_sha":"d","status":"completed","conclusion":"failure","app":{"slug":"coderabbitai"}}}"#,
            "d5",
        ),
    ] {
        let resp = deliver(&mut ledger, body, delivery);
        assert_eq!(resp.status, 200, "{body} -> {}", resp.body);
        assert!(resp.body.contains("filtered"), "{}", resp.body);
    }
    assert_eq!(
        ledger.list_runs(Some("ci-diagnose"), None).unwrap().len(),
        1
    );
}

#[test]
fn review_webhook_is_submission_storm_reflex_without_additions_cap() {
    let plane = Plane::load(&repo_root().join("plane")).unwrap();
    let task = plane.task("review").unwrap();
    assert_eq!(task.spec.budget.max_cost_per_run_usd, None);
    let webhook = task
        .spec
        .triggers
        .iter()
        .find_map(|trigger| match trigger {
            TriggerSpec::Webhook {
                route,
                dedupe_key,
                filter,
                action,
                ..
            } => Some((route, dedupe_key, filter, action)),
            TriggerSpec::Manual | TriggerSpec::Cron { .. } => None,
        })
        .expect("review webhook trigger");

    assert_eq!(webhook.0, "review");
    assert_eq!(
        webhook.1.as_deref(),
        Some("json:/pull_request/html_url|json:/pull_request/head/sha")
    );
    assert!(!webhook
        .2
        .iter()
        .any(|f| f.pointer == "/pull_request/additions"));
    assert!(webhook.2.iter().any(|f| f.pointer == "/pull_request/draft"));

    match webhook.3.as_ref().expect("submission storm action") {
        WebhookActionSpec::SubmissionStorm {
            change,
            rev,
            repo,
            version,
        } => {
            assert_eq!(change, "json:/pull_request/html_url");
            assert_eq!(rev, "json:/pull_request/head/sha");
            assert_eq!(repo.as_deref(), Some("json:/repository/full_name"));
            assert_eq!(version.as_deref(), Some("json:/pull_request/updated_at"));
        }
    }
}

fn temp_ci_plane(root: &Path) -> Plane {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/ci-diagnose")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();

    let repo = repo_root();
    fs::write(
        root.join("agents/ci-diagnoser.toml"),
        fs::read_to_string(repo.join("plane/agents/ci-diagnoser.toml")).unwrap(),
    )
    .unwrap();
    fs::write(
        root.join("tasks/ci-diagnose/task.toml"),
        fs::read_to_string(repo.join("plane/tasks/ci-diagnose/task.toml")).unwrap(),
    )
    .unwrap();
    fs::write(
        root.join("tasks/ci-diagnose/card.md"),
        fs::read_to_string(repo.join("plane/tasks/ci-diagnose/card.md")).unwrap(),
    )
    .unwrap();

    Plane::load(root).unwrap()
}
