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
    assert!(run.contains("--payload-file <PAYLOAD_FILE>"));
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
        ".agents/skills/bb-dogfood/SKILL.md",
    ];
    for rel in current_contracts {
        let text = read(rel);
        assert!(!text.contains("--var"), "{rel} documents stale --var");
        assert!(!text.contains("--since"), "{rel} documents stale --since");
    }

    let spine = read("docs/spine.md");
    assert!(spine.contains(
        "bb run <task> [--idempotency-key K] [--payload JSON | --payload-file PATH] [--json]"
    ));
    assert!(spine.contains("bb task list [--json]"));
    assert!(spine.contains("bb runs export"));
    assert!(spine.contains("bb gate --change K | --submission ID [--json]"));

    let skill = read("skills/bitterblossom/SKILL.md");
    assert!(skill.contains("bb --config <plane> run <task> --payload '<json>' --json"));
    assert!(skill.contains("bb --config <plane> runs export"));
    assert!(skill.contains("bb --config <plane> gate --submission <submission> --json"));

    let recipes = read("skills/bitterblossom/references/operator-recipes.md");
    assert!(recipes.contains("bb --config <plane> runs export"));
    assert!(recipes.contains("bb --config <plane> dlq replay <id> --json"));
    assert!(recipes.contains("curl --config -"));
    assert!(!recipes.contains("curl -H \"Authorization: Bearer $BB_API_TOKEN\""));

    let dogfood = read(".agents/skills/bb-dogfood/SKILL.md");
    assert!(dogfood.contains("./target/debug/bb --config \"$BB_RUNTIME_PLANE\" status --json"));
    assert!(dogfood.contains(
        "./target/debug/bb --config \"$BB_RUNTIME_PLANE\" gate --submission <submission> --json"
    ));
}

#[test]
fn historical_adrs_are_explicitly_superseded() {
    for rel in [
        "docs/adr/001-claude-code-canonical-harness.md",
        "docs/adr/002-architecture-minimalism.md",
        "docs/adr/003-conductor-control-plane.md",
        "docs/adr/004-bounded-review-governance.md",
        "docs/adr/004-elixir-conductor-architecture.md",
    ] {
        let text = read(rel);
        assert!(
            text.contains("Superseded for current Bitterblossom operation by"),
            "{rel} must warn readers that it is historical"
        );
        assert!(text.contains("005-rust-event-plane.md"));
        assert!(text.contains("../spine.md"));
    }
}

#[test]
fn operations_runbook_and_drill_are_wired_into_the_gate() {
    let ops = read("docs/operations/README.md");
    assert!(ops.contains("scripts/production-ops-drill.sh --remote"));
    assert!(ops.contains("scripts/production-ops-drill.sh --local"));
    assert!(ops.contains("flyctl releases rollback"));
    assert!(ops.contains("BB_PLANE_DIR=${BB_PLANE_DIR:-/app/plane} bb recover --json"));
    assert!(ops.contains("bb dlq replay <id> --json"));
    assert!(ops.contains("bb dlq ack <id> --reason <text>"));
    assert!(!ops.contains("there is no first-class acknowledge"));
    assert!(!ops.contains("?token=$BB_API_TOKEN"));

    let script = read("scripts/production-ops-drill.sh");
    assert!(script.contains("backup_restore_check"));
    assert!(script.contains("expect_bearer_code remote-tasks"));
    assert!(script.contains("curl --config -"));
    assert!(!script.contains("-H \"Authorization: Bearer $BB_API_TOKEN\""));
    assert!(!script.contains("?token="));

    let verify = read("scripts/verify.sh");
    assert!(verify.contains("scripts/production-ops-drill.sh --local"));
}
