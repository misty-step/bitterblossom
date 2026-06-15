//! Model-evaluation workflow contracts. These tests keep model diversity and
//! evaluator synthesis as checked-in plane shape, not chat doctrine.

use std::collections::BTreeSet;
use std::fs;
use std::path::PathBuf;

use bitterblossom::spec::{AuthClass, Plane, TriggerSpec};

fn root() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
}

#[test]
fn ci_diagnose_has_three_diverse_candidate_configs_and_one_evaluator() {
    let repo = root();
    let plane = Plane::load(&repo.join("plane")).unwrap();
    let cohort = [
        ("ci-diagnose", "deepseek/deepseek-v4-flash", "deepseek"),
        ("ci-diagnose-kimi", "moonshotai/kimi-k2.7-code", "kimi"),
        ("ci-diagnose-glm", "z-ai/glm-5.1", "glm"),
    ];
    let mut families = BTreeSet::new();
    let baseline = plane.task("ci-diagnose").unwrap();

    for (task_name, model, family) in cohort {
        let task = plane.task(task_name).unwrap();
        assert_eq!(task.agent.harness, "pi");
        assert_eq!(task.agent.model, model);
        assert_eq!(task.agent.auth_class().unwrap(), AuthClass::Api);
        assert_eq!(task.agent.role.as_deref(), Some("diagnoser"));
        assert!(task
            .agent
            .skills
            .contains(&"harness-kit/diagnose#ci-failure".to_string()));
        assert_eq!(
            task.card, baseline.card,
            "{task_name} must share the CI card"
        );
        assert!(task
            .spec
            .triggers
            .iter()
            .any(|trigger| matches!(trigger, TriggerSpec::Manual)));
        families.insert(family);
    }
    assert_eq!(families.len(), 3);

    let evaluator = plane.task("model-eval").unwrap();
    assert_eq!(evaluator.agent.role.as_deref(), Some("evaluator"));
    assert_eq!(evaluator.agent.model, "openai/gpt-5.5");
    assert!(evaluator
        .spec
        .triggers
        .iter()
        .all(|trigger| matches!(trigger, TriggerSpec::Manual)));
    for required in [
        "\"candidates\"",
        "\"scorecard\"",
        "\"winner\"",
        "\"reference_context\"",
        "\"residual_risk\"",
        "at least three",
        "integer from 1 to 5",
        "cost_usd` field as the source of truth",
    ] {
        assert!(evaluator.card.contains(required), "missing {required}");
    }
}

#[test]
fn model_eval_reference_context_is_documented_for_future_runs() {
    let repo = root();
    let readme = fs::read_to_string(repo.join("docs/model-evals/README.md")).unwrap();
    assert!(readme.contains("at least three"));
    assert!(readme.contains("model-eval"));
    assert!(readme.contains("reference context"));
    assert!(readme.contains("z-ai/glm-5.2"));
    assert!(readme.contains("June 16, 2026"));
}
