use roster_core::{
    Agent, ModelEntry, ModelPolicy, Models, Permissions, Role, Roster, SubagentPool,
    SubagentRights, render_bb_agent, render_claude_agent,
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
fn orchestrator_claude_render_resolves_claude_fable_5_to_inherit_not_sonnet() {
    let roster = Roster::load(workspace_root()).expect("roster loads");
    let models = Models::load(workspace_root()).expect("models.yaml loads");
    let orchestrator = roster.agent("orchestrator").expect("orchestrator exists");

    let rendered = render_claude_agent(orchestrator, &models);

    assert!(rendered.contains("model: inherit"), "{rendered}");
    assert!(!rendered.contains("model: sonnet"), "{rendered}");
}

#[test]
fn orchestrator_bb_render_resolves_claude_fable_5_to_kimi_not_bare_id() {
    let roster = Roster::load(workspace_root()).expect("roster loads");
    let models = Models::load(workspace_root()).expect("models.yaml loads");
    let orchestrator = roster.agent("orchestrator").expect("orchestrator exists");

    let rendered =
        render_bb_agent(orchestrator, &models).expect("claude-fable-5 resolves via the table");

    assert!(
        rendered.contains("model = \"moonshotai/kimi-k2.7-code\""),
        "{rendered}"
    );
    assert!(!rendered.contains("model = \"claude-fable-5\""));
}

#[test]
fn cerberus_claude_render_resolves_gpt_5_5_to_sonnet() {
    let roster = Roster::load(workspace_root()).expect("roster loads");
    let models = Models::load(workspace_root()).expect("models.yaml loads");
    let cerberus = roster.agent("cerberus").expect("cerberus exists");

    let rendered = render_claude_agent(cerberus, &models);

    assert!(rendered.contains("model: sonnet"), "{rendered}");
}

#[test]
fn incident_hound_bb_render_resolves_gpt_5_5_via_models_table() {
    // incident-hound's only fallback (claude-sonnet-5) is not openrouter/-
    // prefixed, so this is the one seed agent whose bb render depends on
    // primitives/models.yaml resolving its concrete `preferred` (gpt-5.5)
    // rather than short-circuiting through an openrouter-prefixed fallback.
    let roster = Roster::load(workspace_root()).expect("roster loads");
    let models = Models::load(workspace_root()).expect("models.yaml loads");
    let incident_hound = roster
        .agent("incident-hound")
        .expect("incident-hound exists");

    let rendered = render_bb_agent(incident_hound, &models)
        .expect("gpt-5.5 resolves via the models.yaml table");

    assert!(
        rendered.contains("model = \"moonshotai/kimi-k2.7-code\""),
        "{rendered}"
    );
}

#[test]
fn sweep_bb_render_is_unchanged_by_the_models_table() {
    let roster = Roster::load(workspace_root()).expect("roster loads");
    let models = Models::load(workspace_root()).expect("models.yaml loads");
    let sweep = roster.agent("sweep").expect("sweep exists");

    let rendered = render_bb_agent(sweep, &models)
        .expect("sweep resolves via its own openrouter-prefixed preferred id, same as before");

    // sweep's preferred id is already openrouter/-prefixed, so the models
    // table is never consulted for it -- this pins that this card did not
    // change its output.
    assert!(
        rendered.contains("model = \"deepseek/deepseek-v4-flash\""),
        "{rendered}"
    );
}

#[test]
fn bb_render_fails_loudly_instead_of_emitting_an_unresolvable_bare_id() {
    let roster = Roster::load(workspace_root()).expect("roster loads");
    let models = Models::load(workspace_root()).expect("models.yaml loads");
    let agent = unresolvable_model_agent();

    let error = render_bb_agent(&agent, &models).expect_err(
        "a concrete id absent from models.yaml and with no openrouter fallback must fail",
    );

    assert!(error.contains("cannot resolve bb model"), "{error}");
    assert!(error.contains("made-up-model"), "{error}");
    assert!(!error.contains("model = \"made-up-model\""));
    let _ = roster; // keep the loaded roster in scope for parity with other tests
}

#[test]
fn subagent_pool_loads_and_carries_the_operator_list() {
    let pool = SubagentPool::load(workspace_root()).expect("subagent-pool.yaml loads");

    assert_eq!(pool.schema_version, "roster.subagent-pool.v1");
    let names = pool
        .pool
        .iter()
        .map(|entry| entry.model.as_str())
        .collect::<Vec<_>>();
    assert_eq!(
        names,
        [
            "claude-sonnet-5",
            "gpt-5.5",
            "glm-5.2",
            "openrouter/moonshotai/kimi-k2.7-code",
            "minimax-3",
            "gemini-3.5-flash",
        ]
    );

    let gpt = pool
        .pool
        .iter()
        .find(|entry| entry.model == "gpt-5.5")
        .expect("gpt-5.5 in pool");
    assert_eq!(gpt.reasoning.as_deref(), Some("low"));

    let sonnet = pool
        .pool
        .iter()
        .find(|entry| entry.model == "claude-sonnet-5")
        .expect("claude-sonnet-5 in pool");
    assert_eq!(sonnet.reasoning, None);
}

fn unresolvable_model_agent() -> Agent {
    Agent {
        directory: PathBuf::from("agents/fixture-unresolvable"),
        role: Role {
            schema_version: "roster.role.v1".to_string(),
            name: "fixture-unresolvable".to_string(),
            description: "Fixture agent with a concrete model absent from models.yaml.".to_string(),
            model_policy: ModelPolicy {
                preferred: ModelEntry {
                    model: "made-up-model".to_string(),
                    reasoning: "medium".to_string(),
                },
                fallbacks: vec![ModelEntry {
                    model: "also-not-a-real-model".to_string(),
                    reasoning: "medium".to_string(),
                }],
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
