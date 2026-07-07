use std::fs;
use std::path::Path;

/// Backlog 088: Cerberus must invoke the Thermo-Nuclear maintainability lens
/// for ship-bound implementation diffs, sourced from a pinned/provenance-
/// tracked copy rather than retyped, drift-prone prose.
#[test]
fn thermo_nuclear_lens_is_vendored_with_provenance_and_wired_into_cerberus() {
    let root = Path::new(env!("CARGO_MANIFEST_DIR"));
    let lens_dir = root.join("vendor/skills/thermo-nuclear-code-quality-review");
    let skill = fs::read_to_string(lens_dir.join("SKILL.md")).unwrap();
    let meta: serde_json::Value =
        serde_json::from_str(&fs::read_to_string(lens_dir.join(".sync-meta.json")).unwrap())
            .unwrap();

    // The vendored copy is byte-identical to the hash recorded at vendoring
    // time — proof this file is a mechanical copy, not a hand-retyped
    // paraphrase, and that nobody has silently hand-edited it since. This
    // does NOT detect the upstream Harness Kit skill changing later; keeping
    // this copy current with upstream is a separate, manual re-vendor step.
    use sha2::{Digest, Sha256};
    let mut hasher = Sha256::new();
    hasher.update(skill.as_bytes());
    let actual_sha256 = format!("{:x}", hasher.finalize());
    assert_eq!(
        meta["vendored_from_sha256"].as_str().unwrap(),
        actual_sha256,
        "vendored SKILL.md no longer matches the hash recorded at vendoring time"
    );
    assert!(meta["upstream"]["repo"]
        .as_str()
        .unwrap()
        .contains("cursor/plugins"));
    assert!(meta["upstream"]["sha"].as_str().unwrap().len() >= 7);
    assert!(skill.contains("1000 lines"));
    assert!(skill.contains("code judo"));

    // roster-921: the lens now ships natively upstream, wired to roster's
    // own vendored copy of the same skill — the vendored role/instructions
    // here are a clean mechanical copy, not a bb-only hand patch.
    let role = fs::read_to_string(root.join("vendor/roster/agents/cerberus/role.yaml")).unwrap();
    assert!(role.contains("name: thermo-nuclear-maintainability"));
    assert!(role.contains(
        "primitives/skills/.external/cursor-thermo-nuclear-code-quality-review/SKILL.md"
    ));

    let instructions =
        fs::read_to_string(root.join("vendor/roster/agents/cerberus/instructions.md")).unwrap();
    assert!(instructions.contains("Maintainability lens"));
    assert!(instructions.contains("severity: \"blocking\""));
    assert!(instructions.contains("risk tier"));

    let roster_card =
        fs::read_to_string(root.join("examples/roster-cerberus-plane/tasks/review/card.md"))
            .unwrap();
    assert!(roster_card.contains("Thermo-Nuclear maintainability lens"));

    let factory_agent =
        fs::read_to_string(root.join("examples/review-factory-plane/agents/reviewer.toml"))
            .unwrap();
    assert!(factory_agent.contains("cursor-thermo-nuclear-code-quality-review"));
    // The waiver mechanics (`bb submit waive`) are bb's own workflow surface
    // (per roster-921's composition-free ruling), never part of the
    // upstream identity — asserted only against bb's own task card.
    let factory_card =
        fs::read_to_string(root.join("examples/review-factory-plane/tasks/review/card.md"))
            .unwrap();
    assert!(factory_card.contains("Thermo-Nuclear maintainability lens"));
    assert!(factory_card.contains("risk-tier:<tier>"));
    assert!(factory_card.contains("bb submit waive"));
}

#[test]
fn bitterblossom_skill_is_exportable_agent_interface() {
    let root = Path::new(env!("CARGO_MANIFEST_DIR")).join("skills/bitterblossom");
    let skill = fs::read_to_string(root.join("SKILL.md")).unwrap();
    let recipes = fs::read_to_string(root.join("references/operator-recipes.md")).unwrap();
    let openai = fs::read_to_string(root.join("agents/openai.yaml")).unwrap();

    assert!(skill.starts_with("---\n"));
    assert!(skill.contains("name: bitterblossom"));
    assert!(skill.contains("description: |"));
    assert!(skill.contains("bb --config <plane> check"));
    assert!(skill.contains("bb --config <plane> status --json"));
    assert!(skill.contains("bb --config <plane> task list --json"));
    assert!(skill.contains("bb --config <plane> run <task> --payload '<json>' --json"));
    assert!(skill.contains("bb --config <plane> run build"));
    assert!(skill.contains("bb --config <plane> run ci-diagnose"));
    assert!(skill.contains("bb --config <plane> run model-eval"));
    assert!(skill.contains("## Authority And Readiness"));
    assert!(skill.contains("| Supervised dispatch |"));
    assert!(skill.contains("| Unsupervised reflex |"));
    assert!(skill.contains("| Read-only inspection |"));
    assert!(skill.contains("bb --config <plane> preflight <task> --json"));
    assert!(skill.contains("bb_artifacts_list`/`bb_artifact_read"));
    assert!(skill.contains("A closeout receipt is incomplete"));
    assert!(skill.contains("skills/bitterblossom/"));
    assert!(skill.contains("payload has no 'submission' field"));
    assert!(!skill.contains("TODO"));

    assert!(recipes.contains(
        "CERBERUS_REVIEW_GH_TOKEN=\"$CERBERUS_REVIEW_GH_TOKEN\" bb --config <plane> run review"
    ));
    assert!(recipes.contains("GH_TOKEN=$(gh auth token) bb --config <plane> run ci-diagnose"));
    assert!(recipes.contains("bb --config <plane> run model-eval"));
    assert!(recipes.contains("bb --config <plane> run build"));
    assert!(recipes.contains("bb --config <plane> gate --submission <submission> --json"));
    assert!(recipes.contains("bb --config <plane> dlq list --json"));
    assert!(recipes.contains("bb --config <plane> task unpark <task>"));
    assert!(!recipes.contains("TODO"));

    assert!(openai.contains("display_name: \"Bitterblossom\""));
    assert!(openai.contains("default_prompt: \"Use $bitterblossom"));
}

#[test]
fn bitterblossom_dogfood_skill_is_exportable_agent_interface() {
    let root = Path::new(env!("CARGO_MANIFEST_DIR")).join(".agents/skills/bb-dogfood");
    let skill = fs::read_to_string(root.join("SKILL.md")).unwrap();
    let notes = fs::read_to_string(root.join("references/session-notes-template.md")).unwrap();
    let ux_card = fs::read_to_string(root.join("references/ux-review-card.md")).unwrap();
    let openai = fs::read_to_string(root.join("agents/openai.yaml")).unwrap();

    assert!(skill.starts_with("---\n"));
    assert!(skill.contains("name: bb-dogfood"));
    assert!(skill.contains("description: |"));
    assert!(skill.contains("bb-dogfood"));
    assert!(skill.contains("../../../skills/bitterblossom/SKILL.md"));
    assert!(skill.contains("sprite use -o misty-step lane-1"));
    assert!(skill.contains("./target/debug/bb --config \"$BB_RUNTIME_PLANE\" task list --json"));
    assert!(skill.contains("./target/debug/bb --config \"$BB_RUNTIME_PLANE\" run build"));
    assert!(skill.contains("gh pr create --draft"));
    assert!(skill.contains("submit open"));
    assert!(skill.contains("payload '{\"submission\":\"<submission>\""));
    assert!(skill.contains("Do not unpark a task just to make a gate run"));
    assert!(skill.contains("Reflect into backlog"));
    assert!(!skill.contains("TODO"));

    assert!(notes.contains("## UX Notes"));
    assert!(notes.contains("### Good"));
    assert!(notes.contains("### Bad"));
    assert!(notes.contains("### Ugly"));
    assert!(notes.contains("### Friction"));
    assert!(notes.contains("### Delight"));
    assert!(notes.contains("## Reflection"));
    assert!(notes.contains("Next best pickup"));

    assert!(ux_card.contains("Does it work?"));
    assert!(ux_card.contains("Does it produce useful results?"));
    assert!(ux_card.contains("Backlog-worthy"));

    assert!(openai.contains("display_name: \"Bitterblossom Dogfood\""));
    assert!(openai.contains("default_prompt: \"Use $bb-dogfood"));
}

#[test]
fn bb_dogfood_has_no_duplicate_skill_alias() {
    let root = Path::new(env!("CARGO_MANIFEST_DIR"));

    assert!(!root.join("skills/bb-dogfood/SKILL.md").exists());
    assert!(!root.join("skills/bitterblossom-dogfood/SKILL.md").exists());
    assert!(root.join(".agents/skills/bb-dogfood/SKILL.md").exists());
}

#[test]
fn bitterblossom_skill_projection_has_single_source_of_truth() {
    let root = Path::new(env!("CARGO_MANIFEST_DIR"));
    let adr = fs::read_to_string(root.join("docs/adr/006-skill-projection.md")).unwrap();
    let skill = fs::read_to_string(root.join("skills/bitterblossom/SKILL.md")).unwrap();
    let dogfood = fs::read_to_string(root.join(".agents/skills/bb-dogfood/SKILL.md")).unwrap();

    assert!(adr.contains("Status:** Accepted"));
    assert!(adr.contains("`skills/bitterblossom/` in this repository is the source of truth"));
    assert!(adr.contains("Manual copied skill folders are not an accepted projection path"));
    assert!(adr.contains(".agents/skills/bb-dogfood/"));
    assert!(skill.contains("docs/adr/006-skill-projection.md"));
    assert!(dogfood.contains("../../../skills/bitterblossom/SKILL.md"));

    let aliases: Vec<_> = [
        root.join("skills/bitterblossom/SKILL.md"),
        root.join("skills/bb-dogfood/SKILL.md"),
        root.join("skills/bitterblossom-dogfood/SKILL.md"),
        root.join(".agents/skills/bb-dogfood/SKILL.md"),
    ]
    .into_iter()
    .filter(|path| path.exists())
    .collect();
    assert_eq!(
        aliases,
        vec![
            root.join("skills/bitterblossom/SKILL.md"),
            root.join(".agents/skills/bb-dogfood/SKILL.md")
        ]
    );
}
