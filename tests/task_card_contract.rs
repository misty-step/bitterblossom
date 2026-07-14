use std::fs;
use std::path::Path;

#[test]
fn public_plane_task_cards_have_agent_contract_sections() {
    let root = Path::new(env!("CARGO_MANIFEST_DIR")).join("tests/fixtures/public-plane/tasks");
    let checked = assert_contract_cards(&root);
    assert!(
        checked >= 5,
        "expected public-plane cards, checked {checked}"
    );
}

#[test]
fn canary_responder_template_task_cards_have_agent_contract_sections() {
    let root = Path::new(env!("CARGO_MANIFEST_DIR")).join("examples/canary-responder-plane/tasks");
    let checked = assert_contract_cards(&root);
    assert!(
        checked >= 1,
        "expected canary-responder template cards, checked {checked}"
    );
}

#[test]
fn docs_sync_template_task_cards_have_agent_contract_sections() {
    let root = Path::new(env!("CARGO_MANIFEST_DIR")).join("examples/docs-sync-plane/tasks");
    let checked = assert_contract_cards(&root);
    assert!(
        checked >= 1,
        "expected docs-sync template cards, checked {checked}"
    );
}

#[test]
fn ci_audit_template_task_cards_have_agent_contract_sections() {
    let root = Path::new(env!("CARGO_MANIFEST_DIR")).join("examples/ci-audit-plane/tasks");
    let checked = assert_contract_cards(&root);
    assert!(
        checked >= 1,
        "expected ci-audit template cards, checked {checked}"
    );
}

#[test]
fn review_factory_template_task_cards_have_agent_contract_sections() {
    let root = Path::new(env!("CARGO_MANIFEST_DIR")).join("examples/review-factory-plane/tasks");
    let checked = assert_contract_cards(&root);
    assert!(
        checked >= 1,
        "expected review-factory template cards, checked {checked}"
    );
}

/// bitterblossom-971: the refused-credential boundary is not only for the
/// curated contract planes above — EVERY task card in the repo (all example
/// planes and all test-fixture planes, however minimal) must state that a
/// 401/403 on a declared credential is a STOP-and-report condition, never a
/// prompt to locate a stronger credential. The commission prompt seam covers
/// dispatched lanes mechanically; this keeps the copy-paste surface honest
/// too. See docs/credential-refusal-doctrine.md.
#[test]
fn every_plane_card_states_credential_refusal_boundary() {
    let manifest = Path::new(env!("CARGO_MANIFEST_DIR"));
    let mut checked = 0;
    for planes_root in [manifest.join("examples"), manifest.join("tests/fixtures")] {
        for plane in fs::read_dir(&planes_root).unwrap() {
            let tasks = plane.unwrap().path().join("tasks");
            if !tasks.is_dir() {
                continue;
            }
            for task in fs::read_dir(&tasks).unwrap() {
                let card_path = task.unwrap().path().join("card.md");
                if !card_path.is_file() {
                    continue;
                }
                let card = fs::read_to_string(&card_path).unwrap();
                assert!(
                    card.contains("STOP-and-report"),
                    "{} missing refused-credential STOP-and-report doctrine \
                     (docs/credential-refusal-doctrine.md, bitterblossom-971)",
                    card_path.display()
                );
                checked += 1;
            }
        }
    }
    assert!(checked >= 50, "expected all plane cards, checked {checked}");
}

fn assert_contract_cards(root: &Path) -> usize {
    let mut checked = 0;
    for entry in fs::read_dir(root).unwrap() {
        let dir = entry.unwrap().path();
        if !dir.is_dir() {
            continue;
        }
        let name = dir.file_name().unwrap().to_string_lossy();
        let card = fs::read_to_string(dir.join("card.md")).unwrap();
        for heading in [
            "## Goal",
            "## Oracle",
            "## Boundaries",
            "## Output",
            "## Receipt",
        ] {
            assert!(card.contains(heading), "{name} missing {heading}");
        }
        assert!(card.contains("REPORT.json"), "{name} must name REPORT.json");
        // bitterblossom-971: every lane card states the refused-credential
        // boundary (docs/credential-refusal-doctrine.md) so a 401/403 on a
        // scoped credential blocks-and-reports instead of prompting the lane
        // to locate a stronger credential.
        assert!(
            card.contains("STOP-and-report"),
            "{name} missing refused-credential STOP-and-report doctrine \
             (docs/credential-refusal-doctrine.md, bitterblossom-971)"
        );
        assert!(!card.contains("TODO"), "{name} contains TODO");
        checked += 1;
    }
    checked
}
