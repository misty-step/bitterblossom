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
fn anthropic_or_openai_api_keys_are_forbidden_as_optional_secrets_too() {
    let dir = tempfile::tempdir().unwrap();
    let err = plane_with(
        dir.path(),
        "harness = \"pi\"\nmodel = \"m\"\noptional_secrets = [\"ANTHROPIC_API_KEY\"]\n",
        MANUAL,
    )
    .unwrap_err();
    assert!(err.to_string().contains("forbidden"), "{err}");
}

#[test]
fn a_secret_cannot_be_declared_both_required_and_optional() {
    let dir = tempfile::tempdir().unwrap();
    let err = plane_with(
        dir.path(),
        "harness = \"pi\"\nmodel = \"m\"\nsecrets = [\"GH_TOKEN\"]\noptional_secrets = [\"GH_TOKEN\"]\n",
        MANUAL,
    )
    .unwrap_err();
    assert!(
        err.to_string().contains("both required and optional"),
        "{err}"
    );
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
fn open_harness_agents_default_to_api_auth_and_openrouter_and_may_run_reflex() {
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

    let dir2 = tempfile::tempdir().unwrap();
    let plane = plane_with(
        dir2.path(),
        "harness = \"omp\"\nmodel = \"z-ai/glm-5.2\"\nsecrets = [\"OPENROUTER_API_KEY\"]\n",
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
fn open_harness_agents_cannot_claim_subscription_auth() {
    let dir = tempfile::tempdir().unwrap();
    let err = plane_with(
        dir.path(),
        "harness = \"pi\"\nmodel = \"m\"\nauth = \"subscription\"\n",
        MANUAL,
    )
    .unwrap_err();
    assert!(err.to_string().contains("no subscription auth"), "{err}");

    let dir2 = tempfile::tempdir().unwrap();
    let err = plane_with(
        dir2.path(),
        "harness = \"omp\"\nmodel = \"m\"\nauth = \"subscription\"\n",
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
fn required_artifacts_must_stay_inside_attempt_artifacts() {
    let cases = [
        ("required_artifacts = [\"\"]\n", "non-empty relative path"),
        (
            "required_artifacts = [\"/tmp/REPORT.json\"]\n",
            "non-empty relative path",
        ),
        (
            "required_artifacts = [\"../REPORT.json\"]\n",
            "non-empty relative path",
        ),
        (
            "required_artifacts = [\"nested/../REPORT.json\"]\n",
            "non-empty relative path",
        ),
        (
            "required_artifacts = [\"ANALYSIS.md\"]\n",
            "only REPORT.json is supported",
        ),
    ];

    for (artifact, want) in cases {
        let dir = tempfile::tempdir().unwrap();
        let err = plane_with(
            dir.path(),
            "harness = \"pi\"\nmodel = \"m\"\n",
            &format!("{artifact}{MANUAL}"),
        )
        .unwrap_err();
        assert!(err.to_string().contains(want), "{err}");
    }
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

const POLICY_AGENT: &str = r#"version = 2
harness = "omp"
model = "z-ai/glm-5.2"
auth = "api"
secrets = ["OPENROUTER_API_KEY"]
[policy]
authority = "edit"
provider_key_name = "openrouter-builder"
provider_spend_cap_usd = 25.0
model_allowlist = ["z-ai/glm-5.2", "z-ai/glm-5.2-mini"]
trigger_bindings = ["manual"]
iteration_cap = 1
turn_cap = 120
tool_action_cap = 200
output_bytes_cap = 262144
wall_clock_minutes = 45
side_effect_policy = "log"
"#;

#[test]
fn policy_table_loads_validates_and_projects() {
    let dir = tempfile::tempdir().unwrap();
    let plane = plane_with(dir.path(), POLICY_AGENT, MANUAL).unwrap();

    let p = &plane.agents["a"].policy;
    assert_eq!(p.authority.as_deref(), Some("edit"));
    assert_eq!(p.provider_key_name.as_deref(), Some("openrouter-builder"));
    assert_eq!(p.provider_spend_cap_usd, Some(25.0));
    assert_eq!(p.model_allowlist.len(), 2);
    assert_eq!(p.trigger_bindings, vec!["manual".to_string()]);
    assert_eq!(p.iteration_cap, Some(1));
    assert_eq!(p.turn_cap, Some(120));
    assert_eq!(p.tool_action_cap, Some(200));
    assert_eq!(p.output_bytes_cap, Some(262_144));
    assert_eq!(p.wall_clock_minutes, Some(45));
    assert_eq!(p.side_effect_policy.as_deref(), Some("log"));

    let ledger = Ledger::open(&plane.db_path()).unwrap();
    let rows = bitterblossom::serve::tasks_view(&plane, &ledger).unwrap();
    assert_eq!(rows[0]["policy"]["authority"], "edit");
    assert_eq!(rows[0]["policy"]["provider_spend_cap_usd"], 25.0);
    assert_eq!(rows[0]["policy"]["model_allowlist"][0], "z-ai/glm-5.2");
    assert_eq!(rows[0]["policy"]["side_effect_policy"], "log");

    let check = bitterblossom::serve::check_view(&plane, &ledger).unwrap();
    assert_eq!(check["agent_policy"]["a"]["authority"], "edit");
    assert_eq!(check["task_details"][0]["policy"]["authority"], "edit");
}

#[test]
fn policy_absent_defaults_to_empty_and_still_projects() {
    let dir = tempfile::tempdir().unwrap();
    let plane = plane_with(
        dir.path(),
        "version = 2\nharness = \"omp\"\nmodel = \"z-ai/glm-5.2\"\nauth = \"api\"\n",
        MANUAL,
    )
    .unwrap();
    let p = &plane.agents["a"].policy;
    assert!(p.authority.is_none());
    assert!(p.model_allowlist.is_empty());

    let ledger = Ledger::open(&plane.db_path()).unwrap();
    let rows = bitterblossom::serve::tasks_view(&plane, &ledger).unwrap();
    assert!(rows[0]["policy"]["authority"].is_null());
    assert!(rows[0]["policy"]["model_allowlist"].is_array());
}

#[test]
fn rollout_authority_loads_validates_and_projects() {
    let dir = tempfile::tempdir().unwrap();
    let plane = plane_with(
        dir.path(),
        "version = 2\nharness = \"command\"\nmodel = \"\"\nbin = \"true\"\n",
        "agent = \"a\"\n[rollout]\nauthority = \"report-only\"\nscorecard = \"docs/rollout-scorecards.md#canary-triage-report-only-backlog-080\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();

    let rollout = &plane.tasks["t"].spec.rollout;
    assert_eq!(rollout.authority.as_deref(), Some("report-only"));
    assert_eq!(
        rollout.scorecard.as_deref(),
        Some("docs/rollout-scorecards.md#canary-triage-report-only-backlog-080")
    );

    let ledger = Ledger::open(&plane.db_path()).unwrap();
    let rows = bitterblossom::serve::tasks_view(&plane, &ledger).unwrap();
    assert_eq!(rows[0]["rollout"]["authority"], "report-only");
    assert_eq!(
        rows[0]["rollout"]["scorecard"],
        "docs/rollout-scorecards.md#canary-triage-report-only-backlog-080"
    );

    let status = bitterblossom::health::status_view(&plane, &ledger).unwrap();
    assert_eq!(status["tasks"][0]["rollout"]["authority"], "report-only");
    assert_eq!(
        status["tasks"][0]["rollout"]["scorecard"],
        "docs/rollout-scorecards.md#canary-triage-report-only-backlog-080"
    );
}

#[test]
fn rollout_authority_requires_known_level_and_scorecard() {
    let dir = tempfile::tempdir().unwrap();
    let err = plane_with(
        dir.path(),
        "version = 2\nharness = \"command\"\nmodel = \"\"\nbin = \"true\"\n",
        "agent = \"a\"\n[rollout]\nauthority = \"full-send\"\nscorecard = \"docs/rollout-scorecards.md\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap_err();
    let msg = format!("{err:#}");
    assert!(msg.contains("rollout.authority"), "{msg}");

    let dir = tempfile::tempdir().unwrap();
    let err = plane_with(
        dir.path(),
        "version = 2\nharness = \"command\"\nmodel = \"\"\nbin = \"true\"\n",
        "agent = \"a\"\n[rollout]\nauthority = \"report-only\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap_err();
    let msg = format!("{err:#}");
    assert!(msg.contains("rollout.scorecard"), "{msg}");
}

#[test]
fn policy_rejects_model_allowlist_mismatch() {
    let dir = tempfile::tempdir().unwrap();
    // Replace only the `model = "..."` line (first occurrence), leaving the
    // allowlist listing glm variants so the model is not a member.
    let agent = POLICY_AGENT.replacen(
        "model = \"z-ai/glm-5.2\"",
        "model = \"anthropic/claude-opus-4\"",
        1,
    );
    let err = plane_with(dir.path(), &agent, MANUAL).unwrap_err();
    let msg = format!("{err:#}");
    assert!(
        msg.contains("not in policy.model_allowlist") && msg.contains("agent 'a'"),
        "unexpected error: {msg}"
    );
}

#[test]
fn policy_rejects_unknown_authority_and_side_effect() {
    // authority's original value is "edit"; side_effect_policy's is "log".
    for (field, original_val, bad) in [
        ("authority", "edit", "merge-and-delete"),
        ("side_effect_policy", "log", "explode"),
    ] {
        let dir = tempfile::tempdir().unwrap();
        let from = format!("{field} = \"{original_val}\"");
        let to = format!("{field} = \"{bad}\"");
        let agent = POLICY_AGENT.replacen(&from, &to, 1);
        let err = plane_with(dir.path(), &agent, MANUAL).unwrap_err();
        let msg = format!("{err:#}");
        assert!(
            msg.contains(&format!("policy.{field}")) && msg.contains("is unknown"),
            "unexpected error for {field}={bad}: {msg}"
        );
    }
}

#[test]
fn policy_rejects_zero_cap_and_negative_spend() {
    // zero iteration_cap
    let dir = tempfile::tempdir().unwrap();
    let agent = POLICY_AGENT.replace("iteration_cap = 1", "iteration_cap = 0");
    let err = plane_with(dir.path(), &agent, MANUAL).unwrap_err();
    assert!(format!("{err:#}").contains("policy.iteration_cap must be greater than zero"));

    // negative spend cap
    let dir = tempfile::tempdir().unwrap();
    let agent = POLICY_AGENT.replace(
        "provider_spend_cap_usd = 25.0",
        "provider_spend_cap_usd = -1.0",
    );
    let err = plane_with(dir.path(), &agent, MANUAL).unwrap_err();
    assert!(format!("{err:#}").contains("provider_spend_cap_usd must be non-negative"));
}

#[test]
fn policy_rejects_unknown_and_duplicate_trigger_bindings() {
    // unknown binding kind
    let dir = tempfile::tempdir().unwrap();
    let agent = POLICY_AGENT.replace(
        "trigger_bindings = [\"manual\"]",
        "trigger_bindings = [\"manual\", \"slack\"]",
    );
    let err = plane_with(dir.path(), &agent, MANUAL).unwrap_err();
    assert!(format!("{err:#}").contains("policy.trigger_bindings entry 'slack' is unknown"));

    // duplicate binding
    let dir = tempfile::tempdir().unwrap();
    let agent = POLICY_AGENT.replace(
        "trigger_bindings = [\"manual\"]",
        "trigger_bindings = [\"manual\", \"manual\"]",
    );
    let err = plane_with(dir.path(), &agent, MANUAL).unwrap_err();
    assert!(format!("{err:#}").contains("is duplicated"));
}
