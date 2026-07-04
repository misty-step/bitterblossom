use roster_core::Roster;
use std::{fs, path::PathBuf};

fn workspace_root() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .parent()
        .and_then(|path| path.parent())
        .expect("workspace root")
        .to_path_buf()
}

#[test]
fn loads_seed_agents_from_repo() {
    let roster = Roster::load(workspace_root()).expect("seed roster loads");
    let names: Vec<_> = roster
        .agents()
        .iter()
        .map(|agent| agent.role.name.as_str())
        .collect();

    assert_eq!(names, ["cerberus", "lead", "sweep"]);

    let cerberus = roster.agent("cerberus").expect("cerberus exists");
    assert!(cerberus.role.description.contains("Code-review master"));
    assert_eq!(cerberus.role.model_policy.preferred, "codex-class");
    assert_eq!(cerberus.role.model_policy.reasoning, "xhigh");
    assert!(
        cerberus
            .instructions
            .contains("Cerberus code-review master")
    );
}

#[test]
fn unknown_role_fields_are_rejected() {
    let temp = tempfile::tempdir().expect("tempdir");
    let agent_dir = temp.path().join("agents/bad");
    fs::create_dir_all(&agent_dir).expect("agent dir");
    fs::write(agent_dir.join("instructions.md"), "# Bad\n").expect("instructions");
    fs::write(
        agent_dir.join("role.yaml"),
        r#"schema_version: roster.role.v1
name: bad
description: Bad fixture
model_policy:
  preferred: codex-class
  fallbacks: []
  reasoning: high
permissions:
  filesystem: read-only
  commands: read-only
  network: none
  secrets: none
  mutations: none
skills: []
mcps: []
subagent_rights:
  may_dispatch: false
  may_spawn_subagents: false
  may_use_peer_harnesses: false
evidence_expectations: []
surprise: should fail
"#,
    )
    .expect("role");

    let error = Roster::load(temp.path()).expect_err("unknown field rejected");
    assert!(error.to_string().contains("unknown field"), "{error}");
}

#[test]
fn missing_instructions_are_rejected() {
    let temp = tempfile::tempdir().expect("tempdir");
    let agent_dir = temp.path().join("agents/bad");
    fs::create_dir_all(&agent_dir).expect("agent dir");
    fs::write(
        agent_dir.join("role.yaml"),
        r#"schema_version: roster.role.v1
name: bad
description: Bad fixture
model_policy:
  preferred: codex-class
  fallbacks: []
  reasoning: high
permissions:
  filesystem: read-only
  commands: read-only
  network: none
  secrets: none
  mutations: none
skills: []
mcps: []
subagent_rights:
  may_dispatch: false
  may_spawn_subagents: false
  may_use_peer_harnesses: false
evidence_expectations: []
"#,
    )
    .expect("role");

    let error = Roster::load(temp.path()).expect_err("missing instructions rejected");
    assert!(error.to_string().contains("instructions.md"), "{error}");
}
