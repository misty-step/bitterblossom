use bitterblossom::harness::{build_command, parse_output};

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
    assert!(joined.contains("grep -v -F"), "{joined}");
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
    assert!(joined.contains("grep -v -F"), "{joined}");
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
