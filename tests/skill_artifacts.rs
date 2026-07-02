use std::fs;
use std::path::Path;

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
    assert!(skill.contains("skills/bitterblossom/"));
    assert!(skill.contains("payload has no 'submission' field"));
    assert!(!skill.contains("TODO"));

    assert!(recipes.contains("GH_TOKEN=$(gh auth token) bb --config <plane> run review"));
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
