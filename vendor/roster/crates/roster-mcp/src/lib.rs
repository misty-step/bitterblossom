#![forbid(unsafe_code)]

use roster_core::{
    Models, Roster, render_bb_agent, render_brief, render_claude_agent, render_show,
};
use serde_json::{Value, json};
use std::path::{Path, PathBuf};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct ToolDef {
    pub name: &'static str,
    pub description: &'static str,
    pub input_schema: &'static str,
}

// `sync` (the CLI's other mutating verb) intentionally has no MCP tool. It
// writes managed files across the CALLER'S `$HOME` (`.codex/agents/`,
// `.claude/agents/`, `.pi/agents/`, `.roster/orchestrator/`) with an install/disable
// lifecycle tied to that specific filesystem. An MCP call has no reliable
// notion of "the caller's home" the way a locally-run CLI invocation does,
// and remote/arbitrary MCP callers writing harness config files on whatever
// host runs this server is a materially different risk than rendering text.
// `materialize` has no such issue: like `show`/`brief`, it only reads the
// roster and returns a rendered string, so it gets full MCP parity below.
pub const TOOLS: &[ToolDef] = &[
    ToolDef {
        name: "list",
        description: "List roster agents from role.yaml declarations.",
        input_schema: r#"{"type":"object","properties":{"root":{"type":"string"}}}"#,
    },
    ToolDef {
        name: "show",
        description: "Show one roster agent declaration as prompt-native text.",
        input_schema: r#"{"type":"object","required":["agent"],"properties":{"root":{"type":"string"},"agent":{"type":"string"}}}"#,
    },
    ToolDef {
        name: "brief",
        description: "Render a prompt-native dispatch brief for one roster agent.",
        input_schema: r#"{"type":"object","required":["agent"],"properties":{"root":{"type":"string"},"agent":{"type":"string"},"add_skills":{"type":"array","items":{"type":"string"}},"add_mcps":{"type":"array","items":{"type":"string"}}}}"#,
    },
    ToolDef {
        name: "materialize",
        description: "Render one roster agent declaration for a specific harness (claude, codex, or bb).",
        input_schema: r#"{"type":"object","required":["agent","harness"],"properties":{"root":{"type":"string"},"agent":{"type":"string"},"harness":{"type":"string","enum":["claude","codex","bb"]}}}"#,
    },
];

pub fn tool_defs_json() -> Value {
    Value::Array(
        TOOLS
            .iter()
            .map(|tool| {
                json!({
                    "name": tool.name,
                    "description": tool.description,
                    "inputSchema": serde_json::from_str::<Value>(tool.input_schema)
                        .expect("tool schema is valid json"),
                })
            })
            .collect(),
    )
}

pub fn handle_json_rpc(request: &Value) -> Option<Value> {
    let id = request.get("id").cloned();
    let method = request.get("method").and_then(Value::as_str).unwrap_or("");

    let result = match method {
        "initialize" => Ok(json!({
            "protocolVersion": request["params"]["protocolVersion"]
                .as_str()
                .unwrap_or("2024-11-05"),
            "serverInfo": {"name": "roster", "version": env!("CARGO_PKG_VERSION")},
            "capabilities": {"tools": {"listChanged": false}},
        })),
        "tools/list" => Ok(json!({ "tools": tool_defs_json() })),
        "tools/call" => {
            let params = &request["params"];
            let name = params["name"].as_str().unwrap_or("");
            Ok(match call_tool(name, &params["arguments"]) {
                Ok(value) => value,
                Err(message) => tool_error(message),
            })
        }
        "ping" => Ok(json!({})),
        other => Err(format!("method not found: {other}")),
    };

    id.map(|id| match result {
        Ok(value) => json!({"jsonrpc": "2.0", "id": id, "result": value}),
        Err(message) => json!({
            "jsonrpc": "2.0",
            "id": id,
            "error": {"code": -32603, "message": message},
        }),
    })
}

pub fn call_tool(name: &str, args: &Value) -> Result<Value, String> {
    match name {
        "list" => list_agents(args),
        "show" => show_agent(args),
        "brief" => brief_agent(args),
        "materialize" => materialize_agent(args),
        other => Err(format!("unknown tool: {other}")),
    }
}

fn list_agents(args: &Value) -> Result<Value, String> {
    let roster = load_roster(args)?;
    let agents = roster
        .agents()
        .iter()
        .map(|agent| {
            json!({
                "name": agent.role.name,
                "preferred_model": agent.role.model_policy.preferred.model,
                "reasoning": agent.role.model_policy.preferred.reasoning,
                "description": agent.role.description,
            })
        })
        .collect::<Vec<_>>();
    let text = roster
        .agents()
        .iter()
        .map(|agent| {
            format!(
                "{}\t{}\t{}\t{}",
                agent.role.name,
                agent.role.model_policy.preferred.model,
                agent.role.model_policy.preferred.reasoning,
                agent.role.description
            )
        })
        .collect::<Vec<_>>()
        .join("\n");

    Ok(tool_result(text, json!({ "agents": agents })))
}

fn show_agent(args: &Value) -> Result<Value, String> {
    let roster = load_roster(args)?;
    let agent_name = required_str(args, "agent")?;
    let agent = roster
        .agent(agent_name)
        .ok_or_else(|| format!("unknown agent {agent_name:?}"))?;
    Ok(tool_result(
        render_show(agent),
        json!({ "agent": agent.role.name }),
    ))
}

fn brief_agent(args: &Value) -> Result<Value, String> {
    let roster = load_roster(args)?;
    let agent_name = required_str(args, "agent")?;
    let agent = roster
        .agent(agent_name)
        .ok_or_else(|| format!("unknown agent {agent_name:?}"))?;
    let add_skills = string_array(args, "add_skills")?;
    let add_mcps = string_array(args, "add_mcps")?;
    Ok(tool_result(
        render_brief(agent, &add_skills, &add_mcps, None),
        json!({
            "agent": agent.role.name,
            "added_skills": add_skills,
            "added_mcps": add_mcps,
        }),
    ))
}

fn materialize_agent(args: &Value) -> Result<Value, String> {
    let roster = load_roster(args)?;
    let agent_name = required_str(args, "agent")?;
    let agent = roster
        .agent(agent_name)
        .ok_or_else(|| format!("unknown agent {agent_name:?}"))?;
    let harness = required_str(args, "harness")?;
    let text = match harness {
        "claude" => render_claude_agent(agent, &load_models(args)?),
        "codex" => render_brief(agent, &[], &[], None),
        "bb" => render_bb_agent(agent, &load_models(args)?)?,
        other => {
            return Err(format!(
                "unknown harness {other:?}; expected claude, codex, or bb"
            ));
        }
    };
    Ok(tool_result(
        text,
        json!({ "agent": agent.role.name, "harness": harness }),
    ))
}

fn load_roster(args: &Value) -> Result<Roster, String> {
    Roster::load(root_path(args)).map_err(|error| error.to_string())
}

fn load_models(args: &Value) -> Result<Models, String> {
    Models::load(root_path(args)).map_err(|error| error.to_string())
}

fn root_path(args: &Value) -> PathBuf {
    args["root"]
        .as_str()
        .map(PathBuf::from)
        .or_else(|| std::env::var("ROSTER_ROOT").ok().map(PathBuf::from))
        .unwrap_or_else(|| Path::new(".").to_path_buf())
}

fn required_str<'a>(args: &'a Value, field: &str) -> Result<&'a str, String> {
    args[field]
        .as_str()
        .filter(|value| !value.trim().is_empty())
        .ok_or_else(|| format!("{field} is required"))
}

fn string_array(args: &Value, field: &str) -> Result<Vec<String>, String> {
    match args.get(field).and_then(Value::as_array) {
        Some(items) => items
            .iter()
            .map(|item| {
                item.as_str()
                    .map(ToOwned::to_owned)
                    .ok_or_else(|| format!("{field} entries must be strings"))
            })
            .collect(),
        None => Ok(Vec::new()),
    }
}

fn tool_result(text: String, structured_content: Value) -> Value {
    json!({
        "content": [{"type": "text", "text": text}],
        "structuredContent": structured_content,
        "isError": false,
    })
}

fn tool_error(message: String) -> Value {
    json!({
        "content": [{"type": "text", "text": message}],
        "isError": true,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;

    fn workspace_root() -> PathBuf {
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .parent()
            .and_then(|path| path.parent())
            .expect("workspace root")
            .to_path_buf()
    }

    fn text(response: &Value) -> &str {
        response["content"][0]["text"]
            .as_str()
            .expect("text content")
    }

    #[test]
    fn tool_list_exposes_roster_faces() {
        let tools = tool_defs_json();
        let names = tools
            .as_array()
            .unwrap()
            .iter()
            .map(|tool| tool["name"].as_str().unwrap())
            .collect::<Vec<_>>();

        assert_eq!(names, ["list", "show", "brief", "materialize"]);
        assert_eq!(tools[2]["inputSchema"]["required"][0], "agent");
        assert_eq!(
            tools[3]["inputSchema"]["required"],
            json!(["agent", "harness"])
        );
    }

    #[test]
    fn mcp_tools_render_list_show_and_brief() {
        let root = workspace_root();
        let list = call_tool("list", &json!({"root": root})).unwrap();
        assert!(text(&list).contains("orchestrator\tclaude-fable-5\thigh"));
        // Roster::load sorts agents by name; `ai-scout` sorts first.
        assert_eq!(list["structuredContent"]["agents"][0]["name"], "ai-scout");

        let show = call_tool(
            "show",
            &json!({"root": workspace_root(), "agent": "orchestrator"}),
        )
        .unwrap();
        assert!(text(&show).contains("# orchestrator"));
        assert_eq!(show["structuredContent"]["agent"], "orchestrator");

        let brief = call_tool(
            "brief",
            &json!({
                "root": workspace_root(),
                "agent": "sweep",
                "add_skills": ["extra-skill"],
                "add_mcps": ["extra-mcp"],
            }),
        )
        .unwrap();
        assert!(text(&brief).contains("# Roster Brief: sweep"));
        assert!(text(&brief).contains("- override: extra-skill"));
        assert_eq!(brief["structuredContent"]["added_mcps"][0], "extra-mcp");
    }

    #[test]
    fn mcp_materialize_matches_cli_output_per_harness() {
        let root = workspace_root();

        // Same substrings the CLI's own `materialize --harness codex/bb` tests
        // assert on the same agent, proving MCP/CLI parity rather than just
        // "materialize renders something."
        let codex = call_tool(
            "materialize",
            &json!({"root": root, "agent": "cerberus", "harness": "codex"}),
        )
        .unwrap();
        assert!(text(&codex).contains("# Roster Brief: cerberus"));
        assert!(text(&codex).contains("Read:"));
        assert!(text(&codex).contains("Code-review master"));
        assert_eq!(codex["structuredContent"]["harness"], "codex");

        let bb = call_tool(
            "materialize",
            &json!({"root": root, "agent": "cerberus", "harness": "bb"}),
        )
        .unwrap();
        assert!(text(&bb).contains("# Generated from roster agent cerberus"));
        assert!(text(&bb).contains("harness = \"pi\""));
        assert!(text(&bb).contains("role = \"cerberus\""));

        let claude = call_tool(
            "materialize",
            &json!({"root": root, "agent": "orchestrator", "harness": "claude"}),
        )
        .unwrap();
        assert!(text(&claude).contains("name: orchestrator"));
        assert!(text(&claude).contains("tools:"));
        assert_eq!(claude["structuredContent"]["agent"], "orchestrator");

        let bad_harness = call_tool(
            "materialize",
            &json!({"root": root, "agent": "orchestrator", "harness": "unknown"}),
        )
        .unwrap_err();
        assert!(bad_harness.contains("unknown harness"), "{bad_harness}");
    }

    #[test]
    fn invalid_role_yaml_fails_through_core_validation() {
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
  preferred:
    model: gpt-5.5
    reasoning: high
  fallbacks: []
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

        let error = call_tool("list", &json!({"root": temp.path()})).expect_err("invalid role");
        assert!(error.contains("unknown field"), "{error}");
    }

    #[test]
    fn json_rpc_wraps_success_and_error_outputs() {
        let success = handle_json_rpc(&json!({
            "jsonrpc": "2.0",
            "id": 1,
            "method": "tools/call",
            "params": {"name": "show", "arguments": {"root": workspace_root(), "agent": "orchestrator"}},
        }))
        .expect("success response");
        assert_eq!(success["jsonrpc"], "2.0");
        assert_eq!(success["id"], 1);
        assert!(
            success["result"]["content"][0]["text"]
                .as_str()
                .unwrap()
                .contains("# orchestrator")
        );

        let tool_error = handle_json_rpc(&json!({
            "jsonrpc": "2.0",
            "id": 2,
            "method": "tools/call",
            "params": {"name": "show", "arguments": {"root": workspace_root(), "agent": "missing"}},
        }))
        .expect("tool error response");
        assert_eq!(tool_error["jsonrpc"], "2.0");
        assert_eq!(tool_error["id"], 2);
        assert_eq!(tool_error["result"]["isError"], true);
        assert!(
            tool_error["result"]["content"][0]["text"]
                .as_str()
                .unwrap()
                .contains("unknown agent")
        );

        let protocol_error = handle_json_rpc(&json!({
            "jsonrpc": "2.0",
            "id": 3,
            "method": "missing/method",
        }))
        .expect("protocol error response");
        assert_eq!(protocol_error["error"]["code"], -32603);
        assert!(
            protocol_error["error"]["message"]
                .as_str()
                .unwrap()
                .contains("method not found")
        );
    }
}
