//! Contract test: the simplification storm member's card must invoke the
//! Thermo-Nuclear maintainability lens with provenance to Harness Kit's
//! synced skill (backlog 088). This is a content presence test — it
//! verifies the lens is projected into the storm gate, not that a model
//! applies it correctly (that is live QA territory).
//!
//! If the card and the Harness Kit skill diverge, the skill is canonical;
//! this test guards the projection, not the source.

use std::fs;

const CARD_PATH: &str = "plane/tasks/simplification/card.md";

fn simplification_card() -> String {
    fs::read_to_string(CARD_PATH).unwrap_or_else(|e| panic!("read {CARD_PATH}: {e}"))
}

fn contains_phrase(haystack: &str, needle: &str) -> bool {
    let normalized_haystack = haystack.split_whitespace().collect::<Vec<_>>().join(" ");
    let normalized_needle = needle.split_whitespace().collect::<Vec<_>>().join(" ");
    normalized_haystack.contains(&normalized_needle)
}

#[test]
fn simplification_card_invokes_thermo_nuclear_lens() {
    let card = simplification_card();

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

    assert_card_contains_projected_lens_sections(&card);
    assert_card_preserves_gate_blocking_contract(&card);
}

#[test]
fn simplification_card_projects_all_non_negotiable_rules() {
    let card = simplification_card();

    let rules = [
        "Be ambitious about structural simplification",
        "Do not let a PR push a file from under 1k lines to over 1k lines",
        "Do not allow random spaghetti growth in existing code",
        "Bias toward cleaning the design, not just accepting working code",
        "Prefer direct, boring, maintainable code over hacky or magical code",
        "Push hard on type and boundary cleanliness",
        "Keep logic in the canonical layer and reuse existing helpers",
        "Treat unnecessary sequential orchestration and non-atomic updates as design smells",
    ];

    for rule in rules {
        assert!(
            contains_phrase(&card, rule),
            "card missing projected rule: {rule}"
        );
    }
}

#[test]
fn simplification_card_makes_structural_regressions_gate_blocking() {
    let card = simplification_card();

    for required in [
        "structural maintainability regression is also blocking",
        "approval bar above",
        "unjustified file-size explosion",
        "spaghetti growth",
        "wrong-layer workload logic",
        "gate-weakening",
        "code-judo simplification",
        "vague taste, naming, or style feedback is not blocking",
    ] {
        assert!(
            contains_phrase(&card, required),
            "card must make structural maintainability gate semantics explicit: missing {required}"
        );
    }
}

fn assert_card_contains_projected_lens_sections(card: &str) {
    for section in [
        "### Non-negotiable review rules",
        "### Primary review questions",
        "### What to flag aggressively",
        "### Approval bar",
    ] {
        assert!(
            contains_phrase(card, section),
            "card missing lens section: {section}"
        );
    }

    for concept in ["code judo", "1000 lines", "spaghetti", "approval bar"] {
        assert!(
            contains_phrase(card, concept),
            "card must include load-bearing Thermo-Nuclear concept: {concept}"
        );
    }
}

fn assert_card_preserves_gate_blocking_contract(card: &str) {
    assert!(
        card.contains("gate-weakening") || card.contains("gate-weaken"),
        "card must retain the gate-weakening blocking rule"
    );
    assert!(
        contains_phrase(
            card,
            "If the skill content and this card diverge, the skill is canonical"
        ),
        "card must name the external skill as canonical when projection drifts"
    );
}
