use bitterblossom::harness::{build_command, parse_output, parse_partial_progress};
use bitterblossom::spec::TaskBudget;

#[test]
fn parse_claude_bare_object() {
    let out = r#"{"type":"result","result":"done","total_cost_usd":0.01,"num_turns":2,"usage":{"input_tokens":5,"output_tokens":3}}"#;
    let parsed = parse_output("claude", out).unwrap();
    assert_eq!(parsed.result, "done");
    assert_eq!(parsed.stats.cost_usd, Some(0.01));
}

#[test]
fn parse_claude_transcript_array() {
    let out = r#"[{"type":"system","subtype":"init"},{"type":"assistant","message":"..."},{"type":"result","subtype":"success","result":"review posted","total_cost_usd":2.0459,"num_turns":12,"usage":{"input_tokens":9216,"output_tokens":6598}}]"#;
    let parsed = parse_output("claude", out).unwrap();
    assert_eq!(parsed.result, "review posted");
    assert_eq!(parsed.stats.cost_usd, Some(2.0459));
    assert_eq!(parsed.stats.tokens_out, Some(6598));
}

#[test]
fn pi_command_carries_provider_and_model() {
    let agent: bitterblossom::spec::AgentSpec =
        toml::from_str("harness = \"pi\"\nmodel = \"moonshotai/kimi-k2.6\"\n").unwrap();
    let cmd = build_command(&agent, &bitterblossom::spec::TaskBudget::default()).unwrap();
    let joined = cmd.join(" ");
    assert!(joined.contains("--provider' 'openrouter"), "{joined}");
    assert!(joined.contains("moonshotai/kimi-k2.6"), "{joined}");
    assert!(joined.contains("--no-session"), "{joined}");
    assert_eq!(cmd[0], "sh");
    assert!(joined.contains("while IFS= read -r line"), "{joined}");
    assert!(joined.contains("message_update"), "{joined}");
    assert!(joined.contains("tool_execution_end"), "{joined}");
    assert!(joined.contains("toolResult"), "{joined}");
}

/// Regression for bitterblossom-918: every bounded `harness = "pi"` dispatch
/// must disable extension discovery by default. Without it, a non-interactive
/// `--no-session` run can crash after a successful model response when a
/// global pi extension (observed: ops-watchdog) registers a recurring
/// sampler that outlives session teardown and throws the SDK's stale-context
/// guard — reproduced live against this machine's real pi + ops-watchdog
/// install (see bitterblossom-918-report.md). `--no-extensions` is a
/// workaround recorded pending the upstream fix (pi-agent-config#23,
/// deliberately unmerged per the operator's pi-agent-config retirement
/// ruling) — remove this assertion (and the flag) once bb no longer needs it.
#[test]
fn pi_command_disables_extension_discovery_by_default() {
    let agent: bitterblossom::spec::AgentSpec =
        toml::from_str("harness = \"pi\"\nmodel = \"moonshotai/kimi-k2.6\"\n").unwrap();
    let cmd = build_command(&agent, &bitterblossom::spec::TaskBudget::default()).unwrap();
    let joined = cmd.join(" ");
    assert!(
        joined.contains("--no-extensions"),
        "every harness=pi dispatch must disable extension discovery by default \
         (bitterblossom-918: an active pi extension can crash a --no-session run \
         after a successful response): {joined}"
    );
}

#[test]
fn claude_command_uses_strictest_policy_turn_or_iteration_cap() {
    let agent: bitterblossom::spec::AgentSpec = toml::from_str(
        r#"
harness = "claude"
model = "claude-fable-5"
[policy]
iteration_cap = 3
turn_cap = 5
"#,
    )
    .unwrap();
    let budget = TaskBudget {
        turn_cap: Some(10),
        ..TaskBudget::default()
    };
    let cmd = build_command(&agent, &budget).unwrap();
    let max_turns = cmd
        .windows(2)
        .find(|pair| pair[0] == "--max-turns")
        .map(|pair| pair[1].as_str());
    assert_eq!(max_turns, Some("3"), "{cmd:?}");
}

#[test]
fn unsupported_tool_action_cap_fails_before_command_build() {
    let agent: bitterblossom::spec::AgentSpec = toml::from_str(
        r#"
harness = "command"
model = ""
bin = "/bin/true"
[policy]
tool_action_cap = 1
"#,
    )
    .unwrap();
    let err = build_command(&agent, &TaskBudget::default()).unwrap_err();
    assert!(format!("{err:#}").contains("tool_action_cap is not enforceable"));
}

#[test]
fn unsupported_task_tool_action_cap_fails_before_command_build() {
    let agent: bitterblossom::spec::AgentSpec = toml::from_str(
        r#"
harness = "command"
model = ""
bin = "/bin/true"
"#,
    )
    .unwrap();
    let budget = TaskBudget {
        tool_action_cap: Some(1),
        ..TaskBudget::default()
    };
    let err = build_command(&agent, &budget).unwrap_err();
    assert!(format!("{err:#}").contains("tool_action_cap is not enforceable"));
}

#[test]
fn unsupported_iteration_cap_fails_before_command_build() {
    let agent: bitterblossom::spec::AgentSpec = toml::from_str(
        r#"
harness = "command"
model = ""
bin = "/bin/true"
[policy]
iteration_cap = 1
"#,
    )
    .unwrap();
    let err = build_command(&agent, &TaskBudget::default()).unwrap_err();
    assert!(format!("{err:#}").contains("turn_cap/iteration_cap is not enforceable"));
}

#[test]
fn omp_command_reads_lane_card_and_carries_provider_model_and_args() {
    let agent: bitterblossom::spec::AgentSpec = toml::from_str(
        r#"
harness = "omp"
model = "z-ai/glm-5.2"
args = ["--thinking", "high"]
"#,
    )
    .unwrap();
    let cmd = build_command(&agent, &bitterblossom::spec::TaskBudget::default()).unwrap();
    let joined = cmd.join(" ");
    assert_eq!(cmd[0], "sh");
    assert!(joined.contains("--provider' 'openrouter"), "{joined}");
    assert!(joined.contains("z-ai/glm-5.2"), "{joined}");
    assert!(joined.contains("--no-session"), "{joined}");
    assert!(joined.contains("--mode' 'json"), "{joined}");
    assert!(joined.contains("--auto-approve"), "{joined}");
    assert!(joined.contains("--thinking' 'high"), "{joined}");
    assert!(joined.contains("LANE_CARD.md"), "{joined}");
    assert!(joined.contains("while IFS= read -r line"), "{joined}");
    assert!(joined.contains("message_update"), "{joined}");
    assert!(joined.contains("tool_execution_end"), "{joined}");
    assert!(joined.contains("toolResult"), "{joined}");
}

#[test]
fn parse_pi_jsonl_events_sums_usage_across_messages() {
    let out = concat!(
        r#"{"type":"message_update","assistantMessageEvent":{"type":"text_delta"}}"#,
        "\n",
        r#"{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"thinking about it"},{"type":"toolCall","name":"bash"}],"usage":{"input":1000,"output":50,"cost":{"total":0.001}}}}"#,
        "\n",
        r#"{"type":"turn_end","message":{"role":"assistant"}}"#,
        "\n",
        r#"{"type":"message_end","message":{"role":"assistant","content":[{"type":"thinking","thinking":"..."},{"type":"text","text":" BB-PI-OK"}],"usage":{"input":1117,"output":21,"totalTokens":1138,"cost":{"input":0.0007,"output":0.00007,"total":0.0008}}}}"#,
        "\n",
        r#"{"type":"turn_end","message":{"role":"assistant"}}"#,
        "\n",
        r#"{"type":"agent_end","messages":[]}"#,
    );
    let parsed = parse_output("pi", out).unwrap();
    assert_eq!(parsed.result, "BB-PI-OK");
    assert_eq!(parsed.stats.tokens_in, Some(2117));
    assert_eq!(parsed.stats.tokens_out, Some(71));
    assert_eq!(parsed.stats.turns, Some(2));
    assert_eq!(parsed.stats.cost_usd, Some(0.0018));
}

#[test]
fn partial_progress_counts_streamed_tool_actions() {
    let out = concat!(
        r#"{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"thinking"},{"type":"toolCall","name":"bash"}],"usage":{"input":10,"output":5,"cost":{"total":0.001}}}}"#,
        "\n",
        r#"{"type":"turn_end"}"#,
    );
    let progress = parse_partial_progress("pi", out);
    assert_eq!(progress.stats.turns, Some(1));
    assert_eq!(progress.tool_actions, Some(1));
}

#[test]
fn parse_omp_jsonl_events_sums_usage_and_keeps_last_text() {
    let out = concat!(
        r#"{"type":"message_start","message":{"role":"assistant","content":[{"type":"text","text":"draft"}],"usage":{"input":10,"output":2,"cost":{"total":0.001}}}}"#,
        "\n",
        r#"{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"first"}],"usage":{"input":100,"output":10,"cost":{"total":0.002}}}}"#,
        "\n",
        r#"{"type":"turn_end","message":{"role":"assistant"}}"#,
        "\n",
        r#"{"type":"message_update","message":{"role":"assistant","content":[{"type":"text","text":"noise"}]}}"#,
        "\n",
        r#"{"type":"message_end","message":{"role":"assistant","content":[{"type":"thinking","thinking":"..."},{"type":"text","text":"BB-OMP-OK"}],"usage":{"input":200,"output":20,"cacheRead":1024,"cost":{"input":0.1,"output":0.2,"total":0.003}}}}"#,
        "\n",
        r#"{"type":"turn_end","message":{"role":"assistant"}}"#,
        "\n",
        r#"{"type":"agent_end","messages":[]}"#,
    );
    let parsed = parse_output("omp", out).unwrap();
    assert_eq!(parsed.result, "BB-OMP-OK");
    assert_eq!(parsed.stats.tokens_in, Some(300));
    assert_eq!(parsed.stats.tokens_out, Some(30));
    assert_eq!(parsed.stats.turns, Some(2));
    assert_eq!(parsed.stats.cost_usd, Some(0.005));
}

#[test]
fn opencode_command_carries_provider_slash_model_and_reads_lane_card() {
    let agent: bitterblossom::spec::AgentSpec = toml::from_str(
        r#"
harness = "opencode"
model = "deepseek/deepseek-v4-flash"
args = ["--variant", "high"]
"#,
    )
    .unwrap();
    let cmd = build_command(&agent, &bitterblossom::spec::TaskBudget::default()).unwrap();
    let joined = cmd.join(" ");
    assert_eq!(cmd[0], "opencode");
    assert!(joined.contains("run"), "{joined}");
    assert!(joined.contains("LANE_CARD.md"), "{joined}");
    assert!(joined.contains("--format"), "{joined}");
    assert!(joined.contains("json"), "{joined}");
    assert!(joined.contains("--model"), "{joined}");
    assert!(
        joined.contains("openrouter/deepseek/deepseek-v4-flash"),
        "{joined}"
    );
    assert!(
        joined.contains("--variant' 'high") || joined.contains("--variant high"),
        "{joined}"
    );
}

/// opencode has no CLI flag for turn/iteration/tool-action caps today, so a
/// budget or policy that requests one must fail before command construction
/// rather than silently dispatching an unbounded run.
#[test]
fn opencode_command_rejects_turn_cap_before_build() {
    let agent: bitterblossom::spec::AgentSpec = toml::from_str(
        r#"
harness = "opencode"
model = "deepseek/deepseek-v4-flash"
[policy]
turn_cap = 5
"#,
    )
    .unwrap();
    let err = build_command(&agent, &TaskBudget::default()).unwrap_err();
    assert!(format!("{err:#}").contains("turn_cap/iteration_cap is not enforceable"));
}

#[test]
fn parse_opencode_events_sums_usage_and_keeps_last_text() {
    let out = concat!(
        r#"{"type":"step_start","part":{"type":"step-start"}}"#,
        "\n",
        r#"{"type":"tool_use","part":{"type":"tool","tool":"bash","state":{"status":"completed"}}}"#,
        "\n",
        r#"{"type":"step_finish","part":{"type":"step-finish","cost":0.002,"tokens":{"input":100,"output":10}}}"#,
        "\n",
        r#"{"type":"step_start","part":{"type":"step-start"}}"#,
        "\n",
        r#"{"type":"text","part":{"type":"text","text":"BB-OC-OK"}}"#,
        "\n",
        r#"{"type":"step_finish","part":{"type":"step-finish","cost":0.0005,"tokens":{"input":20,"output":3}}}"#
    );
    let parsed = parse_output("opencode", out).unwrap();
    assert_eq!(parsed.result, "BB-OC-OK");
    assert_eq!(parsed.stats.tokens_in, Some(120));
    assert_eq!(parsed.stats.tokens_out, Some(13));
    assert_eq!(parsed.stats.turns, Some(2));
    assert_eq!(parsed.stats.cost_usd, Some(0.0025));
}

#[test]
fn partial_opencode_progress_counts_streamed_tool_actions() {
    let out = concat!(
        r#"{"type":"tool_use","part":{"type":"tool","tool":"bash","state":{"status":"completed"}}}"#,
        "\n",
        r#"{"type":"step_finish","part":{"type":"step-finish","cost":0.001,"tokens":{"input":10,"output":5}}}"#,
    );
    let progress = parse_partial_progress("opencode", out);
    assert_eq!(progress.stats.turns, Some(1));
    assert_eq!(progress.tool_actions, Some(1));
}

#[test]
fn parse_opencode_rejects_incomplete_runs() {
    assert!(parse_output("opencode", "not json").is_err());
    let no_text = r#"{"type":"step_finish","part":{"type":"step-finish","cost":0.0,"tokens":{"input":1,"output":1}}}"#;
    assert!(parse_output("opencode", no_text).is_err());
}

#[test]
fn parse_command_accepts_structured_result_with_usage() {
    let out = concat!(
        "cerberus noisy stdout\n",
        r#"{"schema_version":"bb.command_result.v1","result":"cerberus review complete","tokens_in":1234,"tokens_out":567,"turns":2,"cost_usd":0.42}"#,
        "\n"
    );
    let parsed = parse_output("command", out).unwrap();
    assert_eq!(parsed.result, "cerberus review complete");
    assert_eq!(parsed.stats.tokens_in, Some(1234));
    assert_eq!(parsed.stats.tokens_out, Some(567));
    assert_eq!(parsed.stats.turns, Some(2));
    assert_eq!(parsed.stats.cost_usd, Some(0.42));
}

#[test]
fn parse_pi_rejects_incomplete_runs() {
    assert!(parse_output("pi", "not json").is_err());
    assert!(parse_output("pi", r#"{"type":"message_update"}"#).is_err());
    let no_text = r#"{"type":"message_end","message":{"role":"assistant","content":[{"type":"thinking","thinking":"x"}],"usage":{}}}"#;
    assert!(parse_output("pi", no_text).is_err());
    assert!(parse_output("omp", "not json").is_err());
}

#[test]
fn parse_claude_rejects_empty_and_error() {
    assert!(parse_output("claude", r#"{"type":"result","result":""}"#).is_err());
    assert!(parse_output(
        "claude",
        r#"{"type":"result","result":"x","is_error":true}"#
    )
    .is_err());
    assert!(parse_output("claude", "not json at all").is_err());
}

/// bitterblossom-971: every dispatched lane's commission preamble must carry
/// the refused-credential STOP-and-report rule, so even a card that forgets
/// to state it is covered on every harness (argv-prompt harnesses included).
/// See docs/credential-refusal-doctrine.md.
#[test]
fn commission_prompt_carries_credential_refusal_doctrine() {
    let prompt = bitterblossom::harness::commission_prompt();
    assert!(prompt.contains("LANE_CARD.md"), "{prompt}");
    assert!(prompt.contains("STOP-and-report"), "{prompt}");
    assert!(
        prompt.contains("never locate or use a stronger credential"),
        "{prompt}"
    );

    let agent: bitterblossom::spec::AgentSpec =
        toml::from_str("harness = \"omp\"\nmodel = \"z-ai/glm-5.2\"\n").unwrap();
    let cmd = build_command(&agent, &TaskBudget::default()).unwrap();
    assert!(cmd.join(" ").contains("STOP-and-report"), "{cmd:?}");
}
