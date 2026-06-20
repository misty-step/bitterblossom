//! Model-evaluation workflow contracts. These tests keep model diversity and
//! evaluator synthesis as checked-in plane shape, not chat doctrine.

use std::collections::BTreeSet;
use std::fs;
use std::path::PathBuf;

use bitterblossom::spec::{AuthClass, Plane, TriggerSpec};

struct Candidate<'a> {
    task: &'a str,
    harness: &'a str,
    model: &'a str,
    diversity_key: &'a str,
    auth: AuthClass,
    verdict: Option<&'a str>,
}

struct Cohort<'a> {
    flow: &'a str,
    candidates: &'a [Candidate<'a>],
}

fn root() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
}

const COHORTS: &[Cohort<'_>] = &[
    Cohort {
        flow: "build",
        candidates: &[
            Candidate {
                task: "build",
                harness: "omp",
                model: "z-ai/glm-5.2",
                diversity_key: "omp-glm",
                auth: AuthClass::Api,
                verdict: None,
            },
            Candidate {
                task: "build-kimi",
                harness: "pi",
                model: "moonshotai/kimi-k2.7-code",
                diversity_key: "kimi",
                auth: AuthClass::Api,
                verdict: None,
            },
            Candidate {
                task: "build-glm",
                harness: "pi",
                model: "z-ai/glm-5.2",
                diversity_key: "pi-glm",
                auth: AuthClass::Api,
                verdict: None,
            },
        ],
    },
    Cohort {
        flow: "review",
        candidates: &[
            Candidate {
                task: "review",
                harness: "pi",
                model: "moonshotai/kimi-k2.6:minimal",
                diversity_key: "kimi",
                auth: AuthClass::Api,
                verdict: None,
            },
            Candidate {
                task: "review-deepseek",
                harness: "pi",
                model: "deepseek/deepseek-v4-pro",
                diversity_key: "deepseek",
                auth: AuthClass::Api,
                verdict: None,
            },
            Candidate {
                task: "review-glm",
                harness: "pi",
                model: "z-ai/glm-5.2",
                diversity_key: "glm",
                auth: AuthClass::Api,
                verdict: None,
            },
        ],
    },
    Cohort {
        flow: "gardener",
        candidates: &[
            Candidate {
                task: "gardener",
                harness: "pi",
                model: "deepseek/deepseek-v4-flash",
                diversity_key: "deepseek",
                auth: AuthClass::Api,
                verdict: None,
            },
            Candidate {
                task: "gardener-kimi",
                harness: "pi",
                model: "moonshotai/kimi-k2.7-code",
                diversity_key: "kimi",
                auth: AuthClass::Api,
                verdict: None,
            },
            Candidate {
                task: "gardener-glm",
                harness: "pi",
                model: "z-ai/glm-5.2",
                diversity_key: "glm",
                auth: AuthClass::Api,
                verdict: None,
            },
        ],
    },
    Cohort {
        flow: "ci-diagnose",
        candidates: &[
            Candidate {
                task: "ci-diagnose",
                harness: "pi",
                model: "deepseek/deepseek-v4-flash",
                diversity_key: "deepseek",
                auth: AuthClass::Api,
                verdict: None,
            },
            Candidate {
                task: "ci-diagnose-kimi",
                harness: "pi",
                model: "moonshotai/kimi-k2.7-code",
                diversity_key: "kimi",
                auth: AuthClass::Api,
                verdict: None,
            },
            Candidate {
                task: "ci-diagnose-glm",
                harness: "pi",
                model: "z-ai/glm-5.2",
                diversity_key: "glm",
                auth: AuthClass::Api,
                verdict: None,
            },
        ],
    },
    Cohort {
        flow: "correctness",
        candidates: &[
            Candidate {
                task: "correctness",
                harness: "pi",
                model: "deepseek/deepseek-v4-pro",
                diversity_key: "deepseek",
                auth: AuthClass::Api,
                verdict: Some("correctness"),
            },
            Candidate {
                task: "correctness-kimi",
                harness: "pi",
                model: "moonshotai/kimi-k2.7-code",
                diversity_key: "kimi",
                auth: AuthClass::Api,
                verdict: Some("correctness-kimi"),
            },
            Candidate {
                task: "correctness-glm",
                harness: "pi",
                model: "z-ai/glm-5.2",
                diversity_key: "glm",
                auth: AuthClass::Api,
                verdict: Some("correctness-glm"),
            },
        ],
    },
    Cohort {
        flow: "security",
        candidates: &[
            Candidate {
                task: "security",
                harness: "pi",
                model: "deepseek/deepseek-v4-pro",
                diversity_key: "deepseek",
                auth: AuthClass::Api,
                verdict: Some("security"),
            },
            Candidate {
                task: "security-kimi",
                harness: "pi",
                model: "moonshotai/kimi-k2.7-code",
                diversity_key: "kimi",
                auth: AuthClass::Api,
                verdict: Some("security-kimi"),
            },
            Candidate {
                task: "security-glm",
                harness: "pi",
                model: "z-ai/glm-5.2",
                diversity_key: "glm",
                auth: AuthClass::Api,
                verdict: Some("security-glm"),
            },
        ],
    },
    Cohort {
        flow: "simplification",
        candidates: &[
            Candidate {
                task: "simplification",
                harness: "pi",
                model: "deepseek/deepseek-v4-flash",
                diversity_key: "deepseek",
                auth: AuthClass::Api,
                verdict: Some("simplification"),
            },
            Candidate {
                task: "simplification-kimi",
                harness: "pi",
                model: "moonshotai/kimi-k2.7-code",
                diversity_key: "kimi",
                auth: AuthClass::Api,
                verdict: Some("simplification-kimi"),
            },
            Candidate {
                task: "simplification-glm",
                harness: "pi",
                model: "z-ai/glm-5.2",
                diversity_key: "glm",
                auth: AuthClass::Api,
                verdict: Some("simplification-glm"),
            },
        ],
    },
    Cohort {
        flow: "product",
        candidates: &[
            Candidate {
                task: "product",
                harness: "pi",
                model: "x-ai/grok-4.3",
                diversity_key: "grok",
                auth: AuthClass::Api,
                verdict: Some("product"),
            },
            Candidate {
                task: "product-kimi",
                harness: "pi",
                model: "moonshotai/kimi-k2.7-code",
                diversity_key: "kimi",
                auth: AuthClass::Api,
                verdict: Some("product-kimi"),
            },
            Candidate {
                task: "product-glm",
                harness: "pi",
                model: "z-ai/glm-5.2",
                diversity_key: "glm",
                auth: AuthClass::Api,
                verdict: Some("product-glm"),
            },
        ],
    },
];

fn assert_manual_only(task: &bitterblossom::spec::Task) {
    assert!(!task.spec.triggers.is_empty());
    assert!(task
        .spec
        .triggers
        .iter()
        .all(|trigger| matches!(trigger, TriggerSpec::Manual)));
}

#[test]
fn evaluated_flows_have_three_diverse_candidate_configs() {
    let repo = root();
    let plane = Plane::load(&repo.join("plane")).unwrap();

    for cohort in COHORTS {
        let baseline = plane.task(cohort.flow).unwrap();
        let mut diversity_keys = BTreeSet::new();
        assert_eq!(cohort.candidates.len(), 3);

        for candidate in cohort.candidates {
            let task = plane.task(candidate.task).unwrap();
            assert_eq!(task.agent.harness, candidate.harness, "{}", candidate.task);
            assert_eq!(task.agent.model, candidate.model, "{}", candidate.task);
            assert_eq!(task.agent.auth_class().unwrap(), candidate.auth);
            assert_eq!(task.spec.verdict.as_deref(), candidate.verdict);
            if candidate.task == cohort.flow {
                assert!(!task.spec.triggers.is_empty());
                assert!(task
                    .spec
                    .triggers
                    .iter()
                    .any(|trigger| matches!(trigger, TriggerSpec::Manual)));
            } else {
                assert_manual_only(task);
                assert_eq!(
                    task.card, baseline.card,
                    "{} must share the {} card",
                    candidate.task, cohort.flow
                );
            }
            diversity_keys.insert(candidate.diversity_key);
        }
        assert_eq!(diversity_keys.len(), 3, "{}", cohort.flow);
    }
}

#[test]
fn evaluator_task_and_cards_preserve_model_eval_contracts() {
    let repo = root();
    let plane = Plane::load(&repo.join("plane")).unwrap();
    let evaluator = plane.task("model-eval").unwrap();
    assert_eq!(evaluator.agent.role.as_deref(), Some("evaluator"));
    assert_eq!(evaluator.agent.model, "openai/gpt-5.5");
    assert_manual_only(evaluator);
    for required in [
        "\"candidates\"",
        "\"scorecard\"",
        "\"winner\"",
        "\"reference_context\"",
        "\"residual_risk\"",
        "at least three",
        "materially different",
        "blocked_reason",
        "winner: null",
        "integer from 1 to 5",
        "cost_usd` field as the source of truth",
        "when present, matches",
    ] {
        assert!(evaluator.card.contains(required), "missing {required}");
    }

    for (task, phrase) in [
        ("build", "dry_run = true"),
        ("review", "force measurement mode"),
        ("gardener", "force `dry_run = true`"),
    ] {
        assert!(plane.task(task).unwrap().card.contains(phrase), "{task}");
    }
    for task in ["correctness", "security", "simplification", "product"] {
        let card = &plane.task(task).unwrap().card;
        assert!(card.contains("not"), "{task}");
        assert!(card.contains("canonical"), "{task}");
        assert!(card.contains("gate"), "{task}");
        assert!(card.contains("members"), "{task}");
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
    assert!(readme.contains("51f3f03980a6"));
    assert!(readme.contains("June 16, 2026"));
    assert!(!readme.contains("not a runnable OpenRouter API model"));
    for cohort in COHORTS {
        assert!(readme.contains(&format!("]({}/README.md)", cohort.flow)));
        let flow_readme = fs::read_to_string(
            repo.join("docs")
                .join("model-evals")
                .join(cohort.flow)
                .join("README.md"),
        )
        .unwrap();
        for candidate in cohort.candidates {
            assert!(flow_readme.contains(candidate.task), "{}", candidate.task);
            assert!(flow_readme.contains(candidate.model), "{}", candidate.model);
        }
    }
}

#[test]
fn build_cost_calibration_record_matches_default_cap() {
    let repo = root();
    let plane = Plane::load(&repo.join("plane")).unwrap();
    let build = plane.task("build").unwrap();
    assert_eq!(build.spec.budget.max_cost_per_run_usd, Some(4.0));

    let build_readme = fs::read_to_string(repo.join("docs/model-evals/build/README.md")).unwrap();
    assert!(build_readme.contains("2026-06-20-builder-cost-calibration.md"));
    assert!(build_readme.contains("a6d019b66cda"));

    let record = fs::read_to_string(
        repo.join("docs/model-evals/build/2026-06-20-builder-cost-calibration.md"),
    )
    .unwrap();
    for required in [
        "f6f2d75b2c3a",
        "87421671ebd5",
        "5bccae0c1d4a",
        "a6d019b66cda",
        "$4.00",
        "d19d71f1eeae",
    ] {
        assert!(record.contains(required), "missing {required}");
    }
}

#[test]
fn ci_diagnose_real_failure_record_has_complete_receipts() {
    let repo = root();
    let readme = fs::read_to_string(repo.join("docs/model-evals/ci-diagnose/README.md")).unwrap();
    let record_name = "2026-06-16-real-failure-diagnosis.md";
    assert!(readme.contains(record_name));

    let record = fs::read_to_string(
        repo.join("docs")
            .join("model-evals")
            .join("ci-diagnose")
            .join(record_name),
    )
    .unwrap();

    for required in [
        "24208282343",
        "2b7e1b2b2b9a9694bfcbfff1950681d10c4e9be4",
        "Hook Tests",
        "Accepted Candidate Runs",
        "ci-diagnose`",
        "ci-diagnose-kimi`",
        "ci-diagnose-glm`",
        "Accepted Evaluator Run",
        "model-eval`",
        "Winner",
        "Reference Context",
        "Dogfood Notes",
        "Residual Risk",
    ] {
        assert!(record.contains(required), "missing {required}");
    }
    assert!(
        !record.contains("PENDING"),
        "real-failure record must be completed before merge"
    );
}

#[test]
fn canonical_gate_verdict_kinds_stay_single_lane() {
    let repo = root();
    let plane = Plane::load(&repo.join("plane")).unwrap();

    for kind in [
        "verify",
        "correctness",
        "security",
        "simplification",
        "product",
    ] {
        let tasks: Vec<_> = plane
            .tasks
            .values()
            .filter(|task| task.spec.verdict.as_deref() == Some(kind))
            .map(|task| task.name.as_str())
            .collect();
        assert_eq!(tasks.len(), 1, "{kind}: {tasks:?}");
    }

    for task_name in [
        "correctness-kimi",
        "correctness-glm",
        "security-kimi",
        "security-glm",
        "simplification-kimi",
        "simplification-glm",
        "product-kimi",
        "product-glm",
    ] {
        let task = plane.task(task_name).unwrap();
        assert_eq!(task.spec.verdict.as_deref(), Some(task_name));
    }
}
