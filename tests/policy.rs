//! Model & auth policy is enforced at config load (`bb check`), not at
//! dispatch: subscription harnesses never take API keys, reflex triggers
//! never bind subscription agents.

use std::fs;
use std::path::Path;

use bitterblossom::ledger::Ledger;
use bitterblossom::spec::Plane;

fn plane_with(root: &Path, agent_toml: &str, task_toml: &str) -> anyhow::Result<Plane> {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/t")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
    fs::write(root.join("agents/a.toml"), agent_toml).unwrap();
    fs::write(root.join("tasks/t/card.md"), "card\n").unwrap();
    fs::write(root.join("tasks/t/task.toml"), task_toml).unwrap();
    Plane::load(root)
}

const MANUAL: &str = "agent = \"a\"\n[[trigger]]\nkind = \"manual\"\n";
const CRON: &str = "agent = \"a\"\n[[trigger]]\nkind = \"cron\"\nschedule = \"0 * * * *\"\n";

#[test]
fn claude_with_api_auth_is_rejected_at_load() {
    let dir = tempfile::tempdir().unwrap();
    let err = plane_with(
        dir.path(),
        "harness = \"claude\"\nmodel = \"m\"\nauth = \"api\"\n",
        MANUAL,
    )
    .unwrap_err();
    assert!(err.to_string().contains("subscription auth only"), "{err}");
}

#[test]
fn anthropic_or_openai_api_keys_are_forbidden_secrets() {
    let dir = tempfile::tempdir().unwrap();
    let err = plane_with(
        dir.path(),
        "harness = \"pi\"\nmodel = \"m\"\nsecrets = [\"OPENAI_API_KEY\"]\n",
        MANUAL,
    )
    .unwrap_err();
    assert!(err.to_string().contains("forbidden"), "{err}");
}

#[test]
fn reflex_triggers_require_api_auth_agents() {
    let dir = tempfile::tempdir().unwrap();
    // claude defaults to subscription; a cron (reflex) trigger must fail.
    let err = plane_with(dir.path(), "harness = \"claude\"\nmodel = \"m\"\n", CRON).unwrap_err();
    assert!(err.to_string().contains("reflex"), "{err}");

    // The same agent on a manual-only (dispatch) task is fine.
    let dir2 = tempfile::tempdir().unwrap();
    plane_with(dir2.path(), "harness = \"claude\"\nmodel = \"m\"\n", MANUAL).unwrap();
}

#[test]
fn pi_agents_default_to_api_auth_and_openrouter_and_may_run_reflex() {
    let dir = tempfile::tempdir().unwrap();
    let plane = plane_with(
        dir.path(),
        "harness = \"pi\"\nmodel = \"moonshotai/kimi-k2.6\"\nsecrets = [\"OPENROUTER_API_KEY\"]\n",
        CRON,
    )
    .unwrap();
    let agent = &plane.tasks["t"].agent;
    assert_eq!(
        agent.auth_class().unwrap(),
        bitterblossom::spec::AuthClass::Api
    );
    assert_eq!(agent.provider(), "openrouter");
}

#[test]
fn pi_cannot_claim_subscription_auth() {
    let dir = tempfile::tempdir().unwrap();
    let err = plane_with(
        dir.path(),
        "harness = \"pi\"\nmodel = \"m\"\nauth = \"subscription\"\n",
        MANUAL,
    )
    .unwrap_err();
    assert!(err.to_string().contains("no subscription auth"), "{err}");
}

#[test]
fn local_substrate_is_rejected_outside_dev_planes() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/t")).unwrap();
    // No dev flag: this is a production plane.
    fs::write(root.join("plane.toml"), "").unwrap();
    fs::write(
        root.join("agents/a.toml"),
        "harness = \"pi\"\nmodel = \"m\"\n",
    )
    .unwrap();
    fs::write(root.join("tasks/t/card.md"), "card\n").unwrap();
    fs::write(
        root.join("tasks/t/task.toml"),
        "agent = \"a\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
    let err = Plane::load(root).unwrap_err();
    assert!(err.to_string().contains("dev/test machinery"), "{err}");
}

#[test]
fn non_local_substrates_require_explicit_host_generically() {
    let dir = tempfile::tempdir().unwrap();
    let err = plane_with(
        dir.path(),
        "harness = \"pi\"\nmodel = \"m\"\n",
        "agent = \"a\"\nsubstrate = \"ssh\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap_err();
    let msg = err.to_string();
    assert!(
        msg.contains("substrate 'ssh' requires workspace.host"),
        "{msg}"
    );
    assert!(!msg.contains("sprites"), "{msg}");
}

#[test]
fn agent_role_and_skill_contract_are_loaded_and_exposed() {
    let dir = tempfile::tempdir().unwrap();
    let plane = plane_with(
        dir.path(),
        r#"version = 4
harness = "codex"
model = "gpt-5.5"
role = "builder"
skills = ["harness-kit/deliver#builder", "bitterblossom/operator-min"]
"#,
        MANUAL,
    )
    .unwrap();

    let agent = &plane.agents["a"];
    assert_eq!(agent.role.as_deref(), Some("builder"));
    assert_eq!(agent.skills.len(), 2);
    assert_eq!(agent.skills[0], "harness-kit/deliver#builder");

    let ledger = Ledger::open(&plane.db_path()).unwrap();
    let rows = bitterblossom::serve::tasks_view(&plane, &ledger).unwrap();
    assert_eq!(rows[0]["agent_role"], "builder");
    assert_eq!(rows[0]["agent_skills"][0], "harness-kit/deliver#builder");
}
