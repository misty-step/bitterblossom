//! Lifecycle reflex packet contracts. These tests intentionally exercise the
//! checked-in plane files instead of a synthetic-only fixture so lifecycle
//! slices stay data-owned and visible to agents.

use std::fs;
use std::path::{Path, PathBuf};

use bitterblossom::ingress::{handle_webhook, sign_hmac};
use bitterblossom::ledger::Ledger;
use bitterblossom::spec::{AuthClass, Plane, TriggerSpec};

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
fn canary_triage_task_is_report_only_sprite_reflex_contract() {
    let plane = Plane::load(&repo_root().join("plane")).unwrap();
    let task = plane.task("canary-triage").unwrap();

    assert_eq!(task.agent_name, "canary-triager");
    assert_eq!(task.agent.harness, "pi");
    assert_eq!(task.agent.model, "deepseek/deepseek-v4-flash");
    assert_eq!(task.agent.auth_class().unwrap(), AuthClass::Api);
    assert_eq!(task.agent.role.as_deref(), Some("diagnoser"));
    assert!(task
        .agent
        .secrets
        .contains(&"OPENROUTER_API_KEY".to_string()));
    assert!(task.agent.secrets.contains(&"GH_TOKEN".to_string()));
    assert!(task.agent.secrets.contains(&"CANARY_ENDPOINT".to_string()));
    assert!(task.agent.secrets.contains(&"CANARY_API_KEY".to_string()));
    assert_eq!(task.spec.substrate, "sprites");
    assert_eq!(task.spec.required_artifacts, vec!["REPORT.json"]);
    assert_eq!(task.spec.budget.max_runs_per_day, Some(8));
    assert_eq!(task.spec.budget.max_cost_per_run_usd, Some(0.75));

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
        .expect("canary-triage webhook trigger");
    assert_eq!(webhook.0, "canary-triage");
    assert_eq!(webhook.1.as_deref(), Some("header:X-Delivery-Id"));
    assert!(webhook.2.iter().any(|f| {
        f.pointer == "/event"
            && f.any_of.as_ref().is_some_and(|values| {
                values.contains(&serde_json::json!("incident.opened"))
                    && values.contains(&serde_json::json!("incident.updated"))
            })
    }));
    assert!(webhook.2.iter().any(|f| f.pointer == "/incident/service"
        && f.any_of
            .as_ref()
            .is_some_and(|values| values.contains(&serde_json::json!("canary")))));
    assert!(
        webhook.3.is_none(),
        "canary triage must create only the report-only task run"
    );

    for required in [
        "report_only",
        "No code edits",
        "No branches",
        "No PRs",
        "No deploys",
        "Read RUN.json first",
        "Read EVENT.json",
        "query Canary",
        "remediation claim",
        "REPORT.json",
        "\"canary_subject\"",
        "\"delivery_id\"",
        "\"bb_run_id\"",
        "\"service\"",
        "\"repo\"",
        "\"evidence\"",
        "\"hypotheses\"",
        "\"residual_uncertainty\"",
    ] {
        assert!(task.card.contains(required), "card missing {required}");
    }
}

#[test]
fn canary_triage_webhook_filters_and_dedupes_canary_events() {
    let dir = tempfile::tempdir().unwrap();
    let plane = temp_canary_triage_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    std::env::set_var("BB_HOOK_CANARY_TRIAGE", "s3cret");

    let deliver = |ledger: &mut Ledger, body: &str, delivery: &str| {
        let timestamp = "2026-07-01T17:00:00Z";
        let sig = sign_hmac(
            "s3cret",
            format!("{timestamp}.{delivery}.{body}").as_bytes(),
        );
        handle_webhook(
            &plane,
            ledger,
            "canary-triage",
            &[
                ("X-Canary-Signature".to_string(), sig),
                ("X-Timestamp".to_string(), timestamp.to_string()),
                ("X-Delivery-Id".to_string(), delivery.to_string()),
            ],
            body,
        )
        .unwrap()
    };

    let in_scope = r#"{"event":"incident.opened","incident":{"id":"INC-factory","service":"canary","severity":"error","opened_at":"2026-07-01T17:00:00Z","signals":[{"signal_type":"error_group","signal_ref":"grp-factory"}]},"tenant_id":"default","project_id":"default"}"#;
    assert_eq!(deliver(&mut ledger, in_scope, "DLV-1").status, 202);

    let duplicate = deliver(&mut ledger, in_scope, "DLV-1");
    assert_eq!(duplicate.status, 202);
    assert!(duplicate.body.contains("\"duplicate\":true"));

    for (body, delivery) in [
        (
            r#"{"event":"incident.opened","incident":{"id":"INC-linejam","service":"linejam"}}"#,
            "DLV-2",
        ),
        (
            r#"{"event":"annotation.added","incident":{"id":"INC-factory","service":"canary"}}"#,
            "DLV-3",
        ),
        (
            r#"{"event":"incident.opened","signal":{"kind":"error_group"}}"#,
            "DLV-4",
        ),
    ] {
        let resp = deliver(&mut ledger, body, delivery);
        assert_eq!(resp.status, 200, "{body} -> {}", resp.body);
        assert!(resp.body.contains("filtered"), "{}", resp.body);
    }
    assert_eq!(
        ledger.list_runs(Some("canary-triage"), None).unwrap().len(),
        1
    );
}

#[test]
fn review_webhook_is_cerberus_reflex_with_org_and_noise_controls() {
    let plane = Plane::load(&repo_root().join("plane")).unwrap();
    let task = plane.task("review").unwrap();
    assert_eq!(task.agent_name, "cerberus-reviewer");
    assert_eq!(task.agent.harness, "command");
    assert_eq!(task.agent.auth_class().unwrap(), AuthClass::Api);
    assert!(task.agent.secrets.contains(&"GH_TOKEN".to_string()));
    assert!(task
        .agent
        .secrets
        .contains(&"OPENROUTER_API_KEY".to_string()));
    assert!(task
        .spec
        .workspace
        .repos
        .iter()
        .any(|repo| repo.url == "https://github.com/misty-step/cerberus.git"));
    assert!(task
        .spec
        .workspace
        .repos
        .iter()
        .any(|repo| repo.url == "https://github.com/misty-step/bitterblossom.git"));
    assert_eq!(task.spec.budget.max_runs_per_day, Some(20));
    assert_eq!(task.spec.budget.max_cost_per_run_usd, Some(1.25));
    assert!(task.card.contains("cerberus review-pr"));

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
    assert!(webhook.2.iter().any(|f| {
        f.pointer == "/repository/owner/login"
            && f.any_of.as_ref().is_some_and(|values| {
                values.contains(&serde_json::json!("misty-step"))
                    && values.contains(&serde_json::json!("phrazzld"))
            })
    }));
    assert!(webhook.2.iter().any(|f| {
        f.pointer == "/sender/login"
            && f.not_any_of.as_ref().is_some_and(|values| {
                values.contains(&serde_json::json!("dependabot[bot]"))
                    && values.contains(&serde_json::json!("renovate[bot]"))
            })
    }));
    assert!(webhook
        .2
        .iter()
        .any(|f| f.pointer == "/pull_request/additions" && f.max == Some(2500.0)));
    assert!(webhook
        .2
        .iter()
        .any(|f| f.pointer == "/pull_request/changed_files" && f.max == Some(50.0)));
    assert!(webhook.2.iter().any(|f| f.pointer == "/pull_request/draft"));

    assert!(
        webhook.3.is_none(),
        "org review reflex must not fan out submission-storm member runs"
    );
}

#[test]
fn review_webhook_filters_org_rollout_without_storm_fanout() {
    let dir = tempfile::tempdir().unwrap();
    let plane = temp_review_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    std::env::set_var("BB_HOOK_REVIEW", "s3cret");

    let deliver = |ledger: &mut Ledger, body: &str, delivery: &str| {
        let sig = sign_hmac("s3cret", body.as_bytes());
        handle_webhook(
            &plane,
            ledger,
            "review",
            &[
                ("X-Hub-Signature-256".to_string(), sig),
                ("X-GitHub-Delivery".to_string(), delivery.to_string()),
            ],
            body,
        )
        .unwrap()
    };

    let in_scope = r#"{"action":"opened","sender":{"login":"allie"},"repository":{"full_name":"phrazzld/vanity","owner":{"login":"phrazzld"}},"pull_request":{"number":121,"draft":false,"html_url":"https://github.com/phrazzld/vanity/pull/121","updated_at":"2026-06-25T10:00:00Z","additions":42,"changed_files":3,"head":{"sha":"abc123"}}}"#;
    assert_eq!(deliver(&mut ledger, in_scope, "d1").status, 202);
    assert_eq!(ledger.list_runs(Some("review"), None).unwrap().len(), 1);
    assert_eq!(ledger.list_runs(None, None).unwrap().len(), 1);

    let duplicate = deliver(&mut ledger, in_scope, "d2");
    assert_eq!(duplicate.status, 202);
    assert!(duplicate.body.contains("\"duplicate\":true"));
    assert_eq!(ledger.list_runs(None, None).unwrap().len(), 1);

    for (body, delivery) in [
        (
            r#"{"action":"opened","sender":{"login":"dependabot[bot]"},"repository":{"full_name":"phrazzld/vanity","owner":{"login":"phrazzld"}},"pull_request":{"number":122,"draft":false,"html_url":"https://github.com/phrazzld/vanity/pull/122","updated_at":"2026-06-25T10:00:00Z","additions":1,"changed_files":1,"head":{"sha":"botsha"}}}"#,
            "d3",
        ),
        (
            r#"{"action":"opened","repository":{"full_name":"phrazzld/vanity","owner":{"login":"phrazzld"}},"pull_request":{"number":123,"draft":false,"html_url":"https://github.com/phrazzld/vanity/pull/123","updated_at":"2026-06-25T10:00:00Z","additions":1,"changed_files":1,"head":{"sha":"nosender"}}}"#,
            "d4",
        ),
        (
            r#"{"action":"opened","sender":{"login":"allie"},"repository":{"full_name":"other/repo","owner":{"login":"other"}},"pull_request":{"number":1,"draft":false,"html_url":"https://github.com/other/repo/pull/1","updated_at":"2026-06-25T10:00:00Z","additions":1,"changed_files":1,"head":{"sha":"outsider"}}}"#,
            "d5",
        ),
        (
            r#"{"action":"opened","sender":{"login":"allie"},"repository":{"full_name":"misty-step/big","owner":{"login":"misty-step"}},"pull_request":{"number":9,"draft":false,"html_url":"https://github.com/misty-step/big/pull/9","updated_at":"2026-06-25T10:00:00Z","additions":2501,"changed_files":1,"head":{"sha":"bigsha"}}}"#,
            "d6",
        ),
    ] {
        let resp = deliver(&mut ledger, body, delivery);
        assert_eq!(resp.status, 200, "{body} -> {}", resp.body);
        assert!(resp.body.contains("filtered"), "{}", resp.body);
    }
    assert_eq!(ledger.list_runs(None, None).unwrap().len(), 1);
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

fn temp_canary_triage_plane(root: &Path) -> Plane {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/canary-triage")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();

    let repo = repo_root();
    fs::write(
        root.join("agents/canary-triager.toml"),
        fs::read_to_string(repo.join("plane/agents/canary-triager.toml")).unwrap(),
    )
    .unwrap();
    fs::write(
        root.join("tasks/canary-triage/task.toml"),
        fs::read_to_string(repo.join("plane/tasks/canary-triage/task.toml")).unwrap(),
    )
    .unwrap();
    fs::write(
        root.join("tasks/canary-triage/card.md"),
        fs::read_to_string(repo.join("plane/tasks/canary-triage/card.md")).unwrap(),
    )
    .unwrap();

    Plane::load(root).unwrap()
}

fn temp_review_plane(root: &Path) -> Plane {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/review")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();

    let repo = repo_root();
    fs::write(
        root.join("agents/cerberus-reviewer.toml"),
        fs::read_to_string(repo.join("plane/agents/cerberus-reviewer.toml")).unwrap(),
    )
    .unwrap();
    fs::write(
        root.join("tasks/review/task.toml"),
        fs::read_to_string(repo.join("plane/tasks/review/task.toml")).unwrap(),
    )
    .unwrap();
    fs::write(
        root.join("tasks/review/card.md"),
        fs::read_to_string(repo.join("plane/tasks/review/card.md")).unwrap(),
    )
    .unwrap();

    Plane::load(root).unwrap()
}
