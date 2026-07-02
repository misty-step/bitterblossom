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
fn review_factory_template_task_cards_have_agent_contract_sections() {
    let root = Path::new(env!("CARGO_MANIFEST_DIR")).join("examples/review-factory-plane/tasks");
    let checked = assert_contract_cards(&root);
    assert!(
        checked >= 1,
        "expected review-factory template cards, checked {checked}"
    );
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
        assert!(!card.contains("TODO"), "{name} contains TODO");
        checked += 1;
    }
    checked
}
