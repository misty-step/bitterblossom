use std::fs;

fn repo_file(path: &str) -> String {
    fs::read_to_string(path).unwrap_or_else(|err| panic!("read {path}: {err}"))
}

#[test]
fn noir_ledger_design_contract_names_the_operator_surface() {
    let design = repo_file("DESIGN.md");
    let contract = repo_file("docs/design-contract.md");

    assert!(design.contains("noir-ledger"));
    assert!(design.contains("src/operator.html"));
    assert!(design.contains("hard square panels"));
    assert!(design.contains("proof strips"));
    assert!(design.contains("caption bands"));
    assert!(contract.contains("@misty-step/aesthetic"));
    assert!(contract.contains("9bbe0f9"));
    assert!(contract.contains("runtime import deferred"));
}

#[test]
fn operator_dashboard_marks_and_renders_noir_ledger_proof_strip() {
    let html = repo_file("src/operator.html");

    assert!(html.contains(r#"data-aesthetic="noir-ledger""#));
    assert!(html.contains(r#"class="proof-strip""#));
    assert!(html.contains(r#"id="schemaVersion""#));
    assert!(html.contains(r#"id="outboxState""#));
    assert!(html.contains(r#"id="freshnessState""#));
    assert!(html.contains("function renderProofStrip()"));
    assert!(html.contains("renderProofStrip();"));
}
