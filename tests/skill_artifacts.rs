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
    assert!(skill.contains("skills/bitterblossom/"));
    assert!(skill.contains("payload has no 'submission' field"));
    assert!(!skill.contains("TODO"));

    assert!(recipes.contains("GH_TOKEN=$(gh auth token) bb --config <plane> run review"));
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
    let root = Path::new(env!("CARGO_MANIFEST_DIR")).join("skills/bitterblossom-dogfood");
    let skill = fs::read_to_string(root.join("SKILL.md")).unwrap();
    let notes = fs::read_to_string(root.join("references/session-notes-template.md")).unwrap();
    let openai = fs::read_to_string(root.join("agents/openai.yaml")).unwrap();

    assert!(skill.starts_with("---\n"));
    assert!(skill.contains("name: bitterblossom-dogfood"));
    assert!(skill.contains("description: |"));
    assert!(skill.contains("sprite use -o misty-step lane-1"));
    assert!(skill.contains("./target/debug/bb --config plane task list --json"));
    assert!(skill.contains("submit open"));
    assert!(skill.contains("payload '{\"submission\":\"<submission>\""));
    assert!(skill.contains("Do not unpark a task just to make a gate run"));
    assert!(!skill.contains("TODO"));

    assert!(notes.contains("## UX Notes"));
    assert!(notes.contains("### Friction"));
    assert!(notes.contains("### Delight"));
    assert!(notes.contains("Next best pickup"));

    assert!(openai.contains("display_name: \"Bitterblossom Dogfood\""));
    assert!(openai.contains("default_prompt: \"Use $bitterblossom-dogfood"));
}

#[test]
fn canary_responder_skill_matches_template_contract() {
    let root = Path::new(env!("CARGO_MANIFEST_DIR")).join("skills/canary-responder");
    let skill = fs::read_to_string(root.join("SKILL.md")).unwrap();

    assert!(skill.starts_with("---\n"));
    assert!(skill.contains("name: canary-responder"));
    assert!(skill.contains("description: |"));
    assert!(skill.contains("webhooks as wake-up hints"));
    assert!(skill.contains("canary-services.toml"));
    assert!(skill.contains("REPORT.json"));
    assert!(skill.contains("examples/canary-responder-plane/"));
    assert!(!skill.contains("TODO"));
}
