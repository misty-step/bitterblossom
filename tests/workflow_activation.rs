use std::fs;

use bitterblossom::ledger::Ledger;
use bitterblossom::spec::Plane;
use bitterblossom::workflow::{AcceptOutcome, LaunchSnapshot, WorkflowAction, WorkflowDoc};

const EFFECT_DOC: &str = r#"
name = "typed-effects"
goal = "Compile one bounded effect declaration."

[grant]
operations = ["claim"]

[[trigger]]
kind = "test"

[[step]]
name = "claim"
kind = "effect"

[step.effect]
adapter = "powder"
operation = "claim"
repository = "org/repo"
branch = "feature/typed"
enforcement = "enforced"

[step.effect.bindings.card]
type = "string"
value = "card-1"
"#;

fn ledger_with_plane() -> (tempfile::TempDir, Ledger, Plane) {
    let root = tempfile::tempdir().unwrap();
    fs::write(
        root.path().join("plane.toml"),
        "dev = true\n[ingress]\nbind = \"127.0.0.1:0\"\n",
    )
    .unwrap();
    let plane = Plane::load(root.path()).unwrap();
    let ledger = Ledger::open(&root.path().join(".bb/plane.db")).unwrap();
    (root, ledger, plane)
}

#[test]
fn typed_effect_round_trip_compiles_and_pins_every_run() {
    let (root, ledger, plane) = ledger_with_plane();
    let doc = WorkflowDoc::from_toml(EFFECT_DOC).unwrap();
    assert!(matches!(doc.steps[0].action, WorkflowAction::Effect { .. }));
    let exported = doc.to_toml().unwrap();
    let restored = WorkflowDoc::from_toml(&exported).unwrap();
    assert_eq!(restored, doc);
    let (workflow, revision) = ledger.create_workflow(&doc, "test", None).unwrap();
    let plan = ledger
        .plan_activation(&workflow.name, Some(revision))
        .unwrap();
    assert_eq!(plan.compiled.steps[0].kind, "effect");
    assert_eq!(
        plan.compiled.steps[0].adapter.as_ref().unwrap().adapter,
        "powder"
    );
    assert!(plan.preflight.secret_free);
    let activated = ledger.activate_plan(&plan).unwrap();
    assert_eq!(
        activated.active_activation_id.as_deref(),
        Some(plan.activation_id.as_str())
    );
    let snapshot = ledger
        .activation_snapshot(&workflow.name, revision)
        .unwrap();
    snapshot.verify_digest().unwrap();
    assert_eq!(snapshot.compiled.digest, plan.compiled.digest);
    let launch_rows = ledger
        .launch_snapshots_for_revision(&workflow.id, revision)
        .unwrap();
    assert_eq!(launch_rows.len(), 1);
    let launch: LaunchSnapshot = serde_json::from_value(launch_rows[0].snapshot.clone()).unwrap();
    assert_eq!(launch.grant, plan.compiled.steps[0].grant);
    assert_eq!(launch.adapter, plan.compiled.steps[0].adapter);
    assert_eq!(snapshot.activation_id, plan.activation_id);
    assert_eq!(snapshot.digest.len(), 64);
    let accepted = ledger
        .accept_workflow_run(&plane, &workflow.name, "test", Some("{}"), None)
        .unwrap();
    let run = match accepted {
        AcceptOutcome::Accepted { run } => run,
        other => panic!("{other:?}"),
    };
    assert_eq!(run.activation_id, plan.activation_id);
    let _ = root;
}

#[test]
fn malformed_and_unsupported_effect_declarations_fail_before_activation() {
    let cases = [
        (
            "default branch",
            EFFECT_DOC.replace("feature/typed", "master"),
            "default branch",
        ),
        (
            "secret env",
            EFFECT_DOC.replace(
                "enforcement = \"enforced\"",
                "enforcement = \"enforced\"\nsecret_env = \"TOKEN\"",
            ),
            "secret_env",
        ),
        (
            "shell plugin",
            EFFECT_DOC.replace("adapter = \"powder\"", "adapter = \"shell\""),
            "shell/plugin",
        ),
        (
            "unsupported enforcement",
            EFFECT_DOC.replace("enforcement = \"enforced\"", "enforcement = \"advisory\""),
            "unsupported enforcement",
        ),
        (
            "widening operation",
            EFFECT_DOC.replace("operations = [\"claim\"]", "operations = [\"work_log\"]"),
            "widens",
        ),
    ];
    for (label, text, expected) in cases {
        let (_root, ledger, _plane) = ledger_with_plane();
        let error = match WorkflowDoc::from_toml(&text) {
            Err(error) => error.to_string(),
            Ok(doc) => match ledger.create_workflow(&doc, "test", None) {
                Err(error) => error.to_string(),
                Ok((workflow, revision)) => ledger
                    .activate_workflow(&workflow.name, Some(revision))
                    .unwrap_err()
                    .to_string(),
            },
        };
        assert!(error.contains(expected), "{label}: {error}");
    }
    let unknown = EFFECT_DOC.replace("kind = \"effect\"", "kind = \"unknown\"");
    let unknown_error = WorkflowDoc::from_toml(&unknown).unwrap_err().to_string();
    assert!(!unknown_error.is_empty(), "{unknown_error}");
}

#[test]
fn approval_action_is_typed_and_activation_snapshot_is_immutable() {
    let text = r#"
name = "typed-approval"
goal = "Require an operator decision."
[[step]]
name = "approve"
kind = "approval"
[step.approval]
principal = "operator"
question = "Proceed?"
timeout_seconds = 30
"#;
    let (_root, ledger, _plane) = ledger_with_plane();
    let doc = WorkflowDoc::from_toml(text).unwrap();
    assert!(matches!(
        doc.steps[0].action,
        WorkflowAction::Approval { .. }
    ));
    let (workflow, revision) = ledger.create_workflow(&doc, "test", None).unwrap();

    let mut document_tampered = ledger
        .plan_activation(&workflow.name, Some(revision))
        .unwrap();
    document_tampered.document_digest = "tampered".into();
    let error = ledger
        .activate_plan(&document_tampered)
        .unwrap_err()
        .to_string();
    assert!(error.contains("document digest changed"), "{error}");
    assert!(ledger
        .activation_snapshot(&workflow.name, revision)
        .is_err());
    assert!(ledger
        .launch_snapshots_for_revision(&workflow.id, revision)
        .unwrap()
        .is_empty());

    let mut compiled_tampered = ledger
        .plan_activation(&workflow.name, Some(revision))
        .unwrap();
    compiled_tampered.compiled.steps[0]
        .grant
        .operations
        .insert("approve".into());
    let error = ledger
        .activate_plan(&compiled_tampered)
        .unwrap_err()
        .to_string();
    assert!(error.contains("activation plan payload changed"), "{error}");
    assert!(ledger
        .activation_snapshot(&workflow.name, revision)
        .is_err());
    assert!(ledger
        .launch_snapshots_for_revision(&workflow.id, revision)
        .unwrap()
        .is_empty());
    let row = ledger.workflow_by_name(&workflow.name).unwrap();
    assert_eq!(row.state, "draft");
    assert_eq!(row.active_revision, None);
    assert_eq!(row.active_activation_id, None);
}

#[test]
fn rollback_mints_fresh_activation_and_pins_runs_without_resuming_pause() {
    let (root, ledger, plane) = ledger_with_plane();
    let doc = WorkflowDoc::from_toml(EFFECT_DOC).unwrap();
    let (workflow, revision) = ledger.create_workflow(&doc, "test", None).unwrap();
    let first = ledger
        .activate_workflow(&workflow.name, Some(revision))
        .unwrap();
    let first_activation = first.active_activation_id.clone().unwrap();
    ledger.pause_workflow(&workflow.name, "hold").unwrap();

    let (rolled, rollback_revision) = ledger.rollback_workflow(&workflow.name, revision).unwrap();
    assert_eq!(rolled.state, "paused");
    assert_eq!(rolled.active_revision, Some(rollback_revision));
    assert_ne!(
        rolled.active_activation_id.as_deref(),
        Some(first_activation.as_str())
    );
    let snapshot = ledger
        .activation_snapshot(&workflow.name, rollback_revision)
        .unwrap();
    assert_eq!(
        snapshot.activation_id,
        rolled.active_activation_id.clone().unwrap()
    );
    assert_eq!(
        ledger
            .launch_snapshots_for_revision(&workflow.id, rollback_revision)
            .unwrap()
            .len(),
        1
    );

    ledger.resume_workflow(&workflow.name).unwrap();
    let accepted = ledger
        .accept_workflow_run(&plane, &workflow.name, "test", Some("{}"), None)
        .unwrap();
    let run = match accepted {
        AcceptOutcome::Accepted { run } => run,
        other => panic!("{other:?}"),
    };
    assert_eq!(run.revision, rollback_revision);
    assert_eq!(run.activation_id, rolled.active_activation_id.unwrap());
    let _ = root;
}
