use std::fs;
use std::path::Path;
use std::process::Command;

fn help(args: &[&str]) -> String {
    let output = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(args)
        .output()
        .unwrap();
    assert!(
        output.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );
    format!(
        "{}\n{}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    )
}

fn read(rel: &str) -> String {
    let root = Path::new(env!("CARGO_MANIFEST_DIR"));
    fs::read_to_string(root.join(rel)).unwrap()
}

#[test]
fn live_help_exposes_current_agent_cli_contract() {
    let run = help(&["run", "--help"]);
    assert!(run.contains("Usage: bb run [OPTIONS] <TASK>"));
    assert!(run.contains("--payload <PAYLOAD>"));
    assert!(run.contains("--json"));
    assert!(!run.contains("--var"));

    let export = help(&["runs", "export", "--help"]);
    assert!(export.contains("Usage: bb runs export [OPTIONS]"));
    assert!(!export.contains("--since"));

    let gate = help(&["gate", "--help"]);
    assert!(gate.contains("--submission <SUBMISSION>"));
    assert!(gate.contains("--change <CHANGE>"));
    assert!(gate.contains("--json"));
}

#[test]
fn current_docs_and_skills_match_live_cli_contract() {
    let current_contracts = [
        "README.md",
        "docs/spine.md",
        "skills/bitterblossom/SKILL.md",
        "skills/bitterblossom/references/operator-recipes.md",
        "skills/bitterblossom-dogfood/SKILL.md",
    ];
    for rel in current_contracts {
        let text = read(rel);
        assert!(!text.contains("--var"), "{rel} documents stale --var");
        assert!(!text.contains("--since"), "{rel} documents stale --since");
    }

    let spine = read("docs/spine.md");
    assert!(spine.contains("bb run <task> [--idempotency-key K] [--payload JSON] [--json]"));
    assert!(spine.contains("bb runs export"));
    assert!(spine.contains("bb gate --change K | --submission ID [--json]"));

    let skill = read("skills/bitterblossom/SKILL.md");
    assert!(skill.contains("bb --config <plane> run <task> --payload '<json>' --json"));
    assert!(skill.contains("bb --config <plane> runs export"));
    assert!(skill.contains("bb --config <plane> gate --submission <submission> --json"));

    let recipes = read("skills/bitterblossom/references/operator-recipes.md");
    assert!(recipes.contains("bb --config <plane> runs export"));
    assert!(recipes.contains("bb --config <plane> dlq replay <id> --json"));
    assert!(recipes.contains("curl -H \"Authorization: Bearer $BB_API_TOKEN\""));

    let dogfood = read("skills/bitterblossom-dogfood/SKILL.md");
    assert!(dogfood.contains("./target/debug/bb --config plane status --json"));
    assert!(
        dogfood.contains("./target/debug/bb --config plane gate --submission <submission> --json")
    );
}
