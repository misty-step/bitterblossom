//! Contract test: the simplification storm member's card must invoke the
//! Thermo-Nuclear maintainability lens with provenance to Harness Kit's
//! synced skill (backlog 088). This is a content presence test — it
//! verifies the lens is projected into the storm gate, not that a model
//! applies it correctly (that is live QA territory).
//!
//! If the card and the Harness Kit skill diverge, the skill is canonical;
//! this test guards the projection, not the source.

use std::fs;

#[test]
fn simplification_card_invokes_thermo_nuclear_lens() {
    let card_path = "plane/tasks/simplification/card.md";
    let card = fs::read_to_string(card_path).unwrap_or_else(|e| panic!("read {card_path}: {e}"));

    // The lens must be named explicitly, not just implied.
    assert!(
        card.contains("Thermo-Nuclear"),
        "simplification card must name the Thermo-Nuclear lens"
    );

    // Provenance: the card must cite the Harness Kit skill path so
    // reviewers and operators can trace where the lens comes from.
    assert!(
        card.contains("cursor-thermo-nuclear-code-quality-review"),
        "simplification card must cite the Harness Kit skill provenance"
    );
    assert!(
        card.contains("skills/.external/cursor-thermo-nuclear-code-quality-review/SKILL.md"),
        "simplification card must include the skill file path for provenance"
    );

    // Core review rules from the skill must be present. These are the
    // load-bearing concepts that distinguish the Thermo-Nuclear lens
    // from a generic "look for dead code" simplification review.
    assert!(
        card.contains("code judo"),
        "card must include the 'code judo' structural-simplification concept"
    );
    assert!(
        card.contains("1000 lines") || card.contains("1k lines"),
        "card must include the file-size boundary rule"
    );
    assert!(
        card.contains("spaghetti"),
        "card must include the spaghetti-growth rule"
    );
    assert!(
        card.to_lowercase().contains("approval bar"),
        "card must include the approval-bar section so reviewers know the bar"
    );

    // Gate-weakening must remain blocking — this was in the original
    // card and must not be lost when the lens was expanded.
    assert!(
        card.contains("gate-weakening") || card.contains("gate-weaken"),
        "card must retain the gate-weakening blocking rule"
    );
}
