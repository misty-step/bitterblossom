use roster_core::{
    Agent, ModelPolicy, Permissions, Providers, Role, Roster, SubagentRights, render_bb_agent,
    render_claude_agent,
};
use std::path::PathBuf;

fn workspace_root() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .parent()
        .and_then(|path| path.parent())
        .expect("workspace root")
        .to_path_buf()
}

#[test]
fn orchestrator_claude_render_resolves_fable_class_to_inherit_not_sonnet() {
    let roster = Roster::load(workspace_root()).expect("roster loads");
    let providers = Providers::load(workspace_root()).expect("tiers.yaml loads");
    let orchestrator = roster.agent("orchestrator").expect("orchestrator exists");

    let rendered = render_claude_agent(orchestrator, &providers);

    assert!(rendered.contains("model: inherit"), "{rendered}");
    assert!(!rendered.contains("model: sonnet"), "{rendered}");
}

#[test]
fn orchestrator_bb_render_resolves_fable_class_to_kimi_not_bare_tier() {
    let roster = Roster::load(workspace_root()).expect("roster loads");
    let providers = Providers::load(workspace_root()).expect("tiers.yaml loads");
    let orchestrator = roster.agent("orchestrator").expect("orchestrator exists");

    let rendered =
        render_bb_agent(orchestrator, &providers).expect("fable-class resolves via the table");

    assert!(
        rendered.contains("model = \"moonshotai/kimi-k2.7-code\""),
        "{rendered}"
    );
    assert!(!rendered.contains("model = \"fable-class\""), "{rendered}");
}

#[test]
fn sweep_bb_render_is_unchanged_by_the_tiers_table() {
    let roster = Roster::load(workspace_root()).expect("roster loads");
    let providers = Providers::load(workspace_root()).expect("tiers.yaml loads");
    let sweep = roster.agent("sweep").expect("sweep exists");

    let rendered = render_bb_agent(sweep, &providers)
        .expect("sweep resolves via its own openrouter-prefixed fallback, same as before");

    // sweep already resolves through its own fallback list (existing
    // literal-openrouter-prefix behavior), so the tiers table is never
    // consulted for it -- this pins that this card did not change its output.
    assert!(
        rendered.contains("model = \"moonshotai/kimi-k2.7-code\""),
        "{rendered}"
    );
}

#[test]
fn bb_render_fails_loudly_instead_of_emitting_an_unresolvable_bare_tier() {
    let roster = Roster::load(workspace_root()).expect("roster loads");
    let providers = Providers::load(workspace_root()).expect("tiers.yaml loads");
    let agent = unresolvable_tier_agent();

    let error = render_bb_agent(&agent, &providers)
        .expect_err("a tier absent from tiers.yaml and with no openrouter fallback must fail");

    assert!(error.contains("cannot resolve bb model"), "{error}");
    assert!(error.contains("made-up-tier"), "{error}");
    assert!(!error.contains("model = \"made-up-tier\""));
    let _ = roster; // keep the loaded roster in scope for parity with other tests
}

fn unresolvable_tier_agent() -> Agent {
    Agent {
        directory: PathBuf::from("agents/fixture-unresolvable"),
        role: Role {
            schema_version: "roster.role.v1".to_string(),
            name: "fixture-unresolvable".to_string(),
            description: "Fixture agent with a tier absent from tiers.yaml.".to_string(),
            model_policy: ModelPolicy {
                preferred: "made-up-tier".to_string(),
                fallbacks: vec!["also-not-a-real-tier".to_string()],
                reasoning: "medium".to_string(),
            },
            permissions: Permissions {
                filesystem: "read-only".to_string(),
                commands: "read-only".to_string(),
                network: "none".to_string(),
                secrets: "none".to_string(),
                mutations: "none".to_string(),
            },
            skills: vec![],
            mcps: vec![],
            mcps_contextual: vec![],
            subagent_rights: SubagentRights {
                may_dispatch: false,
                may_spawn_subagents: false,
                may_use_peer_harnesses: false,
            },
            evidence_expectations: vec![],
        },
        instructions: "# Fixture\n".to_string(),
    }
}
