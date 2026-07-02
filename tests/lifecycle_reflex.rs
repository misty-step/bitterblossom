//! Lifecycle reflex packet contracts. These tests intentionally exercise a
//! public fixture plane so lifecycle slices stay data-owned without tracking the
//! operator's production instance config.

use std::fs;
use std::path::{Path, PathBuf};

use bitterblossom::ingress::{handle_webhook, sign_hmac};
use bitterblossom::ledger::Ledger;
use bitterblossom::spec::{AuthClass, Plane, TriggerSpec};

fn repo_root() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
}

fn public_plane_root() -> PathBuf {
    repo_root().join("tests/fixtures/public-plane")
}

#[test]
fn ci_diagnose_task_is_api_auth_reflex_contract() {
    let plane = Plane::load(&public_plane_root()).unwrap();
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

    let in_scope = r#"{"action":"completed","repository":{"full_name":"example-org/bitterblossom-example"},"check_suite":{"head_sha":"deadbeef","status":"completed","conclusion":"failure","app":{"slug":"github-actions"}}}"#;
    assert_eq!(deliver(&mut ledger, in_scope, "d1").status, 202);

    for (body, delivery) in [
        (
            r#"{"action":"completed","repository":{"full_name":"elsewhere/repo"},"check_suite":{"head_sha":"a","status":"completed","conclusion":"failure","app":{"slug":"github-actions"}}}"#,
            "d2",
        ),
        (
            r#"{"action":"completed","repository":{"full_name":"example-org/bitterblossom-example"},"check_suite":{"head_sha":"b","status":"completed","conclusion":"success","app":{"slug":"github-actions"}}}"#,
            "d3",
        ),
        (
            r#"{"action":"requested","repository":{"full_name":"example-org/bitterblossom-example"},"check_suite":{"head_sha":"c","status":"queued","conclusion":"failure","app":{"slug":"github-actions"}}}"#,
            "d4",
        ),
        (
            r#"{"action":"completed","repository":{"full_name":"example-org/bitterblossom-example"},"check_suite":{"head_sha":"d","status":"completed","conclusion":"failure","app":{"slug":"coderabbitai"}}}"#,
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
    let plane = Plane::load(&public_plane_root()).unwrap();
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
        "No merges",
        "Do not create",
        "remediation claims",
        "Do not annotate",
        "Do not park or unpark tasks",
        "Do not resolve BB runs",
        "Read RUN.json first",
        "Read EVENT.json",
        "query Canary",
        "REPORT.json",
        "bb.canary_incident_response.report.v1",
        "\"canary_subject\"",
        "\"delivery_id\"",
        "\"bb_run_id\"",
        "\"service\"",
        "\"repo\"",
        "\"evidence\"",
        "\"hypotheses\"",
        "\"recommended_actions\"",
        "\"constraints\"",
        "mutations_performed",
        "\"residual_uncertainty\"",
    ] {
        assert!(task.card.contains(required), "card missing {required}");
    }
}

#[test]
fn deploy_prod_verify_task_is_report_only_reflex_contract() {
    let plane = Plane::load(&public_plane_root()).unwrap();
    let task = plane.task("deploy-prod-verify").unwrap();

    assert_eq!(task.agent_name, "prod-verifier");
    assert_eq!(task.agent.harness, "pi");
    assert_eq!(task.agent.model, "deepseek/deepseek-v4-flash");
    assert_eq!(task.agent.auth_class().unwrap(), AuthClass::Api);
    assert_eq!(task.agent.role.as_deref(), Some("verifier"));
    assert!(task
        .agent
        .secrets
        .contains(&"OPENROUTER_API_KEY".to_string()));
    assert!(task.agent.secrets.contains(&"PROD_READ_TOKEN".to_string()));
    assert_eq!(task.spec.substrate, "sprites");
    assert_eq!(task.spec.required_artifacts, vec!["REPORT.json"]);
    assert_eq!(task.spec.budget.max_runs_per_day, Some(12));
    assert_eq!(task.spec.budget.max_cost_per_run_usd, Some(0.60));

    let has_manual = task
        .spec
        .triggers
        .iter()
        .any(|trigger| matches!(trigger, TriggerSpec::Manual));
    assert!(has_manual);

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
        .expect("deploy-prod-verify webhook trigger");
    assert_eq!(webhook.0, "deploy-prod-verify");
    assert_eq!(
        webhook.1.as_deref(),
        Some("json:/event|json:/subject/service|json:/subject/revision")
    );
    assert!(webhook.2.iter().any(|f| f.pointer == "/schema_version"
        && f.equals.as_ref() == Some(&serde_json::json!("bb.deploy_prod_verifier_event.v1"))));
    assert!(webhook.2.iter().any(|f| {
        f.pointer == "/event"
            && f.any_of.as_ref().is_some_and(|values| {
                values.contains(&serde_json::json!("deploy_smoke.failed"))
                    && values.contains(&serde_json::json!("production_incident.opened"))
            })
    }));
    assert!(webhook.2.iter().any(|f| f.pointer == "/subject/service"
        && f.any_of
            .as_ref()
            .is_some_and(|values| values.contains(&serde_json::json!("canary")))));
    assert!(
        webhook.3.is_none(),
        "deploy/prod verifier must create only the report-only task run"
    );

    for required in [
        "report_only",
        "deploy-smoke failure",
        "production incident",
        "browser/API evidence",
        "Read `RUN.json` first",
        "read `EVENT.json` next",
        "REPORT.json",
        "\"api_evidence\"",
        "\"browser_evidence\"",
        "\"suggested_next_run\"",
        "No code edits",
        "No branches",
        "No PRs",
        "No deploys",
    ] {
        assert!(task.card.contains(required), "card missing {required}");
    }
}

#[test]
fn fix_prompt_task_is_report_only_gate_blocked_contract() {
    let plane = Plane::load(&public_plane_root()).unwrap();
    let task = plane.task("fix-prompt").unwrap();

    assert_eq!(task.agent_name, "fix-prompt-generator");
    assert_eq!(task.agent.harness, "pi");
    assert_eq!(task.agent.model, "deepseek/deepseek-v4-flash");
    assert_eq!(task.agent.auth_class().unwrap(), AuthClass::Api);
    assert_eq!(task.agent.role.as_deref(), Some("fix-prompt-generator"));
    assert!(task
        .agent
        .secrets
        .contains(&"OPENROUTER_API_KEY".to_string()));
    assert_eq!(task.spec.substrate, "sprites");
    assert_eq!(task.spec.required_artifacts, vec!["REPORT.json"]);
    assert_eq!(task.spec.budget.timeout_minutes, Some(15));
    assert_eq!(task.spec.budget.max_runs_per_day, Some(20));
    assert_eq!(task.spec.budget.max_cost_per_run_usd, Some(0.20));

    let has_manual = task
        .spec
        .triggers
        .iter()
        .any(|trigger| matches!(trigger, TriggerSpec::Manual));
    assert!(has_manual);

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
        .expect("fix-prompt webhook trigger");
    assert_eq!(webhook.0, "fix-prompt");
    assert_eq!(webhook.1.as_deref(), Some("json:/submission|json:/rev"));
    assert!(webhook
        .2
        .iter()
        .any(|f| f.pointer == "/event"
            && f.equals.as_ref() == Some(&serde_json::json!("gate.blocked"))));
    assert!(
        webhook.3.is_none(),
        "fix-prompt must create only the report-only task run"
    );

    for required in [
        "gate.blocked",
        "Read `RUN.json`",
        "EVENT.json",
        "every blocking fingerprint",
        "bb run build --payload-file",
        "REPORT.json",
        "\"blocking_fingerprints\"",
        "\"builder_packet\"",
        "\"suggested_next_run\"",
        "\"no_side_effects\": true",
        "No code edits",
        "No branches",
        "No PRs",
        "No deploys",
        "No task parking or",
        "No run resolution",
    ] {
        assert!(task.card.contains(required), "card missing {required}");
    }
}

#[test]
fn fix_prompt_report_fixture_preserves_every_gate_blocked_fingerprint() {
    let event: serde_json::Value = serde_json::from_str(include_str!(
        "fixtures/contracts/bb.gate_blocked_event.v1.valid.json"
    ))
    .unwrap();
    let report: serde_json::Value = serde_json::from_str(include_str!(
        "fixtures/contracts/bb.fix_prompt_report.v1.valid.json"
    ))
    .unwrap();

    assert_eq!(event["event"], "gate.blocked");
    assert_eq!(report["schema_version"], "bb.fix_prompt_report.v1");
    assert_eq!(report["event"], event["event"]);
    assert_eq!(report["submission"], event["submission"]);
    assert_eq!(report["change"], event["change"]);
    assert_eq!(report["rev"], event["rev"]);
    assert_eq!(report["no_side_effects"], true);
    assert_eq!(report["artifact_paths"], serde_json::json!(["REPORT.json"]));
    assert!(report["suggested_next_run"]
        .as_str()
        .unwrap()
        .contains("bb run build --payload-file"));

    let report_fingerprints = report["blocking_fingerprints"].as_array().unwrap();
    let packet_fingerprints = report["builder_packet"]["must_include_fingerprints"]
        .as_array()
        .unwrap();
    for finding in event["blocking"].as_array().unwrap() {
        let fp = &finding["fingerprint"];
        assert!(report_fingerprints.contains(fp), "report missing {fp}");
        assert!(packet_fingerprints.contains(fp), "packet missing {fp}");
        let report_finding = report["findings"]
            .as_array()
            .unwrap()
            .iter()
            .find(|candidate| candidate["fingerprint"] == *fp)
            .unwrap_or_else(|| panic!("finding missing {fp}"));
        assert_eq!(report_finding["file"], finding["file"]);
        assert_eq!(report_finding["line"], finding["line"]);
        assert_eq!(report_finding["claim"], finding["claim"]);
        assert_eq!(report_finding["evidence"], finding["evidence"]);
    }
}

#[test]
fn self_drill_task_is_weekly_sprite_reflex_contract() {
    let plane = Plane::load(&public_plane_root()).unwrap();
    let task = plane.task("self-drill").unwrap();

    assert_eq!(task.agent_name, "self-drill-runner");
    assert_eq!(task.agent.harness, "command");
    assert_eq!(task.agent.auth_class().unwrap(), AuthClass::Api);
    assert_eq!(task.agent.role.as_deref(), Some("self-drill"));
    assert_eq!(task.spec.substrate, "sprites");
    assert_eq!(task.host(), "example-org/lane-1");
    assert_eq!(task.spec.required_artifacts, vec!["REPORT.json"]);
    assert_eq!(task.spec.budget.timeout_minutes, Some(20));
    assert_eq!(task.spec.budget.max_runs_per_day, Some(1));
    assert_eq!(task.spec.budget.max_cost_per_run_usd, Some(0.01));
    assert!(task
        .spec
        .workspace
        .repos
        .iter()
        .any(|repo| repo.url == "https://github.com/example-org/bitterblossom-example.git"));
    assert!(task
        .agent
        .args
        .iter()
        .any(|arg| arg.contains("scripts/self-drill-chaos.sh")));

    let has_manual = task
        .spec
        .triggers
        .iter()
        .any(|trigger| matches!(trigger, TriggerSpec::Manual));
    let cron = task
        .spec
        .triggers
        .iter()
        .find_map(|trigger| match trigger {
            TriggerSpec::Cron { schedule } => Some(schedule.as_str()),
            TriggerSpec::Manual | TriggerSpec::Webhook { .. } => None,
        })
        .expect("self-drill cron trigger");
    assert!(has_manual);
    assert_eq!(cron, "0 16 * * 1");

    for required in [
        "isolated temporary dev plane",
        "stale submission-storm member",
        "`bb gate`",
        "notification outbox",
        "REPORT.json",
        "Do not touch the production ledger directly",
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
fn deploy_prod_verify_webhook_filters_and_dedupes_events() {
    let dir = tempfile::tempdir().unwrap();
    let plane = temp_deploy_prod_verify_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    std::env::set_var("BB_HOOK_DEPLOY_PROD_VERIFY", "s3cret");

    let deliver = |ledger: &mut Ledger, body: &str, delivery: &str| {
        let sig = sign_hmac("s3cret", body.as_bytes());
        handle_webhook(
            &plane,
            ledger,
            "deploy-prod-verify",
            &[
                ("X-Hub-Signature-256".to_string(), sig),
                ("X-GitHub-Delivery".to_string(), delivery.to_string()),
            ],
            body,
        )
        .unwrap()
    };

    let in_scope = include_str!("fixtures/contracts/bb.deploy_prod_verifier_event.v1.valid.json");
    assert_eq!(deliver(&mut ledger, in_scope, "DLV-DEPLOY-1").status, 202);

    let duplicate = deliver(&mut ledger, in_scope, "DLV-DEPLOY-2");
    assert_eq!(duplicate.status, 202);
    assert!(duplicate.body.contains("\"duplicate\":true"));

    for (body, delivery) in [
        (
            r#"{"schema_version":"bb.deploy_prod_verifier_event.v2","event":"deploy_smoke.failed","subject":{"service":"canary","revision":"abc123"}}"#,
            "DLV-DEPLOY-3",
        ),
        (
            r#"{"schema_version":"bb.deploy_prod_verifier_event.v1","event":"deploy_smoke.passed","subject":{"service":"canary","revision":"abc123"}}"#,
            "DLV-DEPLOY-4",
        ),
        (
            r#"{"schema_version":"bb.deploy_prod_verifier_event.v1","event":"deploy_smoke.failed","subject":{"service":"other","revision":"abc123"}}"#,
            "DLV-DEPLOY-5",
        ),
    ] {
        let resp = deliver(&mut ledger, body, delivery);
        assert_eq!(resp.status, 200, "{body} -> {}", resp.body);
        assert!(resp.body.contains("filtered"), "{}", resp.body);
    }
    assert_eq!(
        ledger
            .list_runs(Some("deploy-prod-verify"), None)
            .unwrap()
            .len(),
        1
    );
}

#[test]
fn fix_prompt_webhook_filters_and_dedupes_gate_blocked_events() {
    let dir = tempfile::tempdir().unwrap();
    let plane = temp_fix_prompt_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    std::env::set_var("BB_HOOK_FIX_PROMPT", "s3cret");

    let deliver = |ledger: &mut Ledger, body: &str, delivery: &str| {
        let sig = sign_hmac("s3cret", body.as_bytes());
        handle_webhook(
            &plane,
            ledger,
            "fix-prompt",
            &[
                ("X-Hub-Signature-256".to_string(), sig),
                ("X-GitHub-Delivery".to_string(), delivery.to_string()),
            ],
            body,
        )
        .unwrap()
    };

    let in_scope = include_str!("fixtures/contracts/bb.gate_blocked_event.v1.valid.json");
    assert_eq!(deliver(&mut ledger, in_scope, "FIX-1").status, 202);

    let duplicate = deliver(&mut ledger, in_scope, "FIX-2");
    assert_eq!(duplicate.status, 202);
    assert!(duplicate.body.contains("\"duplicate\":true"));

    for (body, delivery) in [
        (
            r#"{"event":"gate.clear","submission":"sub-fix-prompt","rev":"abc123def456","blocking":[]}"#,
            "FIX-3",
        ),
        (
            r#"{"submission":"sub-fix-prompt","rev":"abc123def456","blocking":[]}"#,
            "FIX-4",
        ),
    ] {
        let resp = deliver(&mut ledger, body, delivery);
        assert_eq!(resp.status, 200, "{body} -> {}", resp.body);
        assert!(resp.body.contains("filtered"), "{}", resp.body);
    }
    assert_eq!(ledger.list_runs(Some("fix-prompt"), None).unwrap().len(), 1);
}

#[test]
fn review_webhook_is_cerberus_reflex_with_org_and_noise_controls() {
    let plane = Plane::load(&public_plane_root()).unwrap();
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
        .any(|repo| repo.url == "https://github.com/example-org/cerberus-example.git"));
    assert!(task
        .spec
        .workspace
        .repos
        .iter()
        .any(|repo| repo.url == "https://github.com/example-org/bitterblossom-example.git"));
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
                values.contains(&serde_json::json!("example-org"))
                    && values.contains(&serde_json::json!("external-example"))
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

    let in_scope = r#"{"action":"opened","sender":{"login":"allie"},"repository":{"full_name":"external-example/review-target","owner":{"login":"external-example"}},"pull_request":{"number":121,"draft":false,"html_url":"https://github.com/external-example/review-target/pull/121","updated_at":"2026-06-25T10:00:00Z","additions":42,"changed_files":3,"head":{"sha":"abc123"}}}"#;
    assert_eq!(deliver(&mut ledger, in_scope, "d1").status, 202);
    assert_eq!(ledger.list_runs(Some("review"), None).unwrap().len(), 1);
    assert_eq!(ledger.list_runs(None, None).unwrap().len(), 1);

    let duplicate = deliver(&mut ledger, in_scope, "d2");
    assert_eq!(duplicate.status, 202);
    assert!(duplicate.body.contains("\"duplicate\":true"));
    assert_eq!(ledger.list_runs(None, None).unwrap().len(), 1);

    for (body, delivery) in [
        (
            r#"{"action":"opened","sender":{"login":"dependabot[bot]"},"repository":{"full_name":"external-example/review-target","owner":{"login":"external-example"}},"pull_request":{"number":122,"draft":false,"html_url":"https://github.com/external-example/review-target/pull/122","updated_at":"2026-06-25T10:00:00Z","additions":1,"changed_files":1,"head":{"sha":"botsha"}}}"#,
            "d3",
        ),
        (
            r#"{"action":"opened","repository":{"full_name":"external-example/review-target","owner":{"login":"external-example"}},"pull_request":{"number":123,"draft":false,"html_url":"https://github.com/external-example/review-target/pull/123","updated_at":"2026-06-25T10:00:00Z","additions":1,"changed_files":1,"head":{"sha":"nosender"}}}"#,
            "d4",
        ),
        (
            r#"{"action":"opened","sender":{"login":"allie"},"repository":{"full_name":"other/repo","owner":{"login":"other"}},"pull_request":{"number":1,"draft":false,"html_url":"https://github.com/other/repo/pull/1","updated_at":"2026-06-25T10:00:00Z","additions":1,"changed_files":1,"head":{"sha":"outsider"}}}"#,
            "d5",
        ),
        (
            r#"{"action":"opened","sender":{"login":"allie"},"repository":{"full_name":"example-org/big","owner":{"login":"example-org"}},"pull_request":{"number":9,"draft":false,"html_url":"https://github.com/example-org/big/pull/9","updated_at":"2026-06-25T10:00:00Z","additions":2501,"changed_files":1,"head":{"sha":"bigsha"}}}"#,
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

    let repo = public_plane_root();
    fs::write(
        root.join("agents/ci-diagnoser.toml"),
        fs::read_to_string(repo.join("agents/ci-diagnoser.toml")).unwrap(),
    )
    .unwrap();
    fs::write(
        root.join("tasks/ci-diagnose/task.toml"),
        fs::read_to_string(repo.join("tasks/ci-diagnose/task.toml")).unwrap(),
    )
    .unwrap();
    fs::write(
        root.join("tasks/ci-diagnose/card.md"),
        fs::read_to_string(repo.join("tasks/ci-diagnose/card.md")).unwrap(),
    )
    .unwrap();

    Plane::load(root).unwrap()
}

fn temp_canary_triage_plane(root: &Path) -> Plane {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/canary-triage")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();

    let repo = public_plane_root();
    fs::write(
        root.join("agents/canary-triager.toml"),
        fs::read_to_string(repo.join("agents/canary-triager.toml")).unwrap(),
    )
    .unwrap();
    fs::write(
        root.join("tasks/canary-triage/task.toml"),
        fs::read_to_string(repo.join("tasks/canary-triage/task.toml")).unwrap(),
    )
    .unwrap();
    fs::write(
        root.join("tasks/canary-triage/card.md"),
        fs::read_to_string(repo.join("tasks/canary-triage/card.md")).unwrap(),
    )
    .unwrap();

    Plane::load(root).unwrap()
}

fn temp_deploy_prod_verify_plane(root: &Path) -> Plane {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/deploy-prod-verify")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();

    let repo = public_plane_root();
    fs::write(
        root.join("agents/prod-verifier.toml"),
        fs::read_to_string(repo.join("agents/prod-verifier.toml")).unwrap(),
    )
    .unwrap();
    fs::write(
        root.join("tasks/deploy-prod-verify/task.toml"),
        fs::read_to_string(repo.join("tasks/deploy-prod-verify/task.toml")).unwrap(),
    )
    .unwrap();
    fs::write(
        root.join("tasks/deploy-prod-verify/card.md"),
        fs::read_to_string(repo.join("tasks/deploy-prod-verify/card.md")).unwrap(),
    )
    .unwrap();

    Plane::load(root).unwrap()
}

fn temp_fix_prompt_plane(root: &Path) -> Plane {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/fix-prompt")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();

    let repo = public_plane_root();
    fs::write(
        root.join("agents/fix-prompt-generator.toml"),
        fs::read_to_string(repo.join("agents/fix-prompt-generator.toml")).unwrap(),
    )
    .unwrap();
    fs::write(
        root.join("tasks/fix-prompt/task.toml"),
        fs::read_to_string(repo.join("tasks/fix-prompt/task.toml")).unwrap(),
    )
    .unwrap();
    fs::write(
        root.join("tasks/fix-prompt/card.md"),
        fs::read_to_string(repo.join("tasks/fix-prompt/card.md")).unwrap(),
    )
    .unwrap();

    Plane::load(root).unwrap()
}

fn temp_review_plane(root: &Path) -> Plane {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/review")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();

    let repo = public_plane_root();
    fs::write(
        root.join("agents/cerberus-reviewer.toml"),
        fs::read_to_string(repo.join("agents/cerberus-reviewer.toml")).unwrap(),
    )
    .unwrap();
    fs::write(
        root.join("tasks/review/task.toml"),
        fs::read_to_string(repo.join("tasks/review/task.toml")).unwrap(),
    )
    .unwrap();
    fs::write(
        root.join("tasks/review/card.md"),
        fs::read_to_string(repo.join("tasks/review/card.md")).unwrap(),
    )
    .unwrap();

    Plane::load(root).unwrap()
}
