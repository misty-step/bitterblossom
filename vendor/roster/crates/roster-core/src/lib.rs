use serde::{Deserialize, Serialize};
use std::{
    collections::{BTreeMap, BTreeSet},
    fs,
    path::{Path, PathBuf},
};

#[derive(Debug)]
pub struct Roster {
    agents: Vec<Agent>,
}

impl Roster {
    pub fn load(root: impl AsRef<Path>) -> Result<Self, RosterError> {
        let root = root.as_ref();
        let agents_dir = root.join("agents");
        let entries = fs::read_dir(&agents_dir).map_err(|source| RosterError::Io {
            path: agents_dir.clone(),
            source,
        })?;

        let mut agents = Vec::new();
        for entry in entries {
            let entry = entry.map_err(|source| RosterError::Io {
                path: agents_dir.clone(),
                source,
            })?;
            let file_type = entry.file_type().map_err(|source| RosterError::Io {
                path: entry.path(),
                source,
            })?;
            if file_type.is_dir() {
                agents.push(load_agent(entry.path())?);
            }
        }

        agents.sort_by(|left, right| left.role.name.cmp(&right.role.name));
        validate_unique_names(&agents)?;

        Ok(Self { agents })
    }

    pub fn agents(&self) -> &[Agent] {
        &self.agents
    }

    pub fn agent(&self, name: &str) -> Option<&Agent> {
        self.agents.iter().find(|agent| agent.role.name == name)
    }
}

/// Tier -> per-harness invocable model id, loaded from `primitives/
/// tiers.yaml`. Distinct from the pre-existing `primitives/providers.yaml`
/// (a peer-harness-CLI dispatch table migrated from harness-kit's
/// agents.yaml at P0 -- how to invoke codex/claude/pi/etc, not consulted by
/// this struct): two files, two concepts.
#[derive(Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct Providers {
    pub schema_version: String,
    pub tiers: BTreeMap<String, TierBindings>,
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct TierBindings {
    pub claude: String,
    pub codex: String,
    pub bb: String,
}

impl Providers {
    pub fn load(root: impl AsRef<Path>) -> Result<Self, RosterError> {
        let path = root.as_ref().join("primitives/tiers.yaml");
        let text = fs::read_to_string(&path).map_err(|source| RosterError::Io {
            path: path.clone(),
            source,
        })?;
        serde_yaml::from_str(&text).map_err(|source| RosterError::Yaml { path, source })
    }

    fn claude_tier(&self, tier: &str) -> Option<&str> {
        self.tiers
            .get(tier)
            .map(|bindings| bindings.claude.as_str())
    }

    fn bb_tier(&self, tier: &str) -> Option<&str> {
        self.tiers.get(tier).map(|bindings| bindings.bb.as_str())
    }
}

#[derive(Debug)]
pub struct Agent {
    pub directory: PathBuf,
    pub role: Role,
    pub instructions: String,
}

impl Agent {
    pub fn instruction_path(&self) -> PathBuf {
        self.directory.join("instructions.md")
    }
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct Role {
    pub schema_version: String,
    pub name: String,
    pub description: String,
    pub model_policy: ModelPolicy,
    pub permissions: Permissions,
    pub skills: Vec<SkillRef>,
    pub mcps: Vec<String>,
    #[serde(default)]
    pub mcps_contextual: Vec<String>,
    pub subagent_rights: SubagentRights,
    pub evidence_expectations: Vec<String>,
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct ModelPolicy {
    pub preferred: String,
    pub fallbacks: Vec<String>,
    pub reasoning: String,
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct Permissions {
    pub filesystem: String,
    pub commands: String,
    pub network: String,
    pub secrets: String,
    pub mutations: String,
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct SkillRef {
    pub name: String,
    pub path: String,
    pub reason: String,
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct SubagentRights {
    pub may_dispatch: bool,
    pub may_spawn_subagents: bool,
    pub may_use_peer_harnesses: bool,
}

#[derive(Debug)]
pub struct CardContext {
    pub id: String,
    pub title: String,
    pub body: String,
    pub acceptance: Vec<String>,
}

#[derive(Debug, thiserror::Error)]
pub enum RosterError {
    #[error("failed to read {path}: {source}")]
    Io {
        path: PathBuf,
        #[source]
        source: std::io::Error,
    },
    #[error("failed to parse {path}: {source}")]
    Yaml {
        path: PathBuf,
        #[source]
        source: serde_yaml::Error,
    },
    #[error("{0}")]
    Validation(String),
}

pub fn render_claude_agent(agent: &Agent, providers: &Providers) -> String {
    let role = &agent.role;
    let model = claude_model(role, providers);
    let tools = claude_tools(role);

    format!(
        r#"---
name: {name}
description: {description}
model: {model}
tools: {tools}
---

# {name}

{instructions}

## Model Policy

- Preferred: {preferred}
- Fallbacks: {fallbacks}
- Reasoning: {reasoning}

## Skills To Read

{skills}

## MCP Servers

{mcp_servers}

## Permissions

- Filesystem: {filesystem}
- Commands: {commands}
- Network: {network}
- Secrets: {secrets}
- Mutations: {mutations}

## Evidence Expectations

{evidence}
"#,
        name = role.name,
        description = role.description,
        model = model,
        tools = tools,
        preferred = role.model_policy.preferred,
        reasoning = role.model_policy.reasoning,
        skills = render_skills(&role.skills, &[]),
        instructions = agent.instructions.trim(),
        fallbacks = role.model_policy.fallbacks.join(", "),
        mcp_servers = render_mcp_servers(&role.mcps, &role.mcps_contextual, &[]),
        filesystem = role.permissions.filesystem,
        commands = role.permissions.commands,
        network = role.permissions.network,
        secrets = role.permissions.secrets,
        mutations = role.permissions.mutations,
        evidence = bullet_list(&role.evidence_expectations),
    )
}

fn claude_model(role: &Role, providers: &Providers) -> String {
    let preferred = &role.model_policy.preferred;

    if let Some(model) = providers.claude_tier(preferred) {
        return model.to_string();
    }

    // `preferred` isn't a known tier symbol, so treat it as a literal model
    // id. Pass Claude subagent model names straight through; conservatively
    // map the handful of literal Claude ids role.yaml might carry to their
    // subagent-frontmatter short form; anything else (a codex/browser-only
    // id, or an unrecognized string) falls back to `inherit` — the subagent
    // runs on the session's own model — rather than guessing a wrong one.
    match preferred.as_str() {
        "sonnet" | "opus" | "haiku" | "inherit" => preferred.clone(),
        "claude-opus-4-8" => "opus".to_string(),
        "claude-sonnet-5" => "sonnet".to_string(),
        "claude-haiku-4-5" | "claude-haiku-4-5-20251001" => "haiku".to_string(),
        _ => "inherit".to_string(),
    }
}

fn claude_tools(role: &Role) -> String {
    let mut tools = vec!["Read"];

    if role.permissions.filesystem.contains("write") || role.permissions.mutations != "none" {
        tools.push("Write");
        tools.push("Edit");
    }

    tools.push("Grep");
    tools.push("Glob");

    if role.permissions.commands != "none" && role.permissions.commands != "disabled-by-default" {
        tools.push("Bash");
    }

    if role.permissions.network == "allowed" {
        tools.push("WebSearch");
    }

    tools.join(", ")
}

// bb (Bitterblossom) config has no MCP concept: `role.mcps`/`mcps_contextual`
// are not rendered into the generated TOML at all, required or contextual.
pub fn render_bb_agent(agent: &Agent, providers: &Providers) -> Result<String, String> {
    let role = &agent.role;
    let model = bb_model(role, providers)?;
    let skills = toml_array(
        &role
            .skills
            .iter()
            .map(|skill| skill.name.clone())
            .collect::<Vec<_>>(),
    );

    Ok(format!(
        r#"# Generated from roster agent {name}.
# Roster preferred model: {preferred}
# Roster reasoning: {reasoning}
version = 1
harness = "pi"
model = "{model}"
provider = "openrouter"
auth = "api"
role = "{name}"
skills = {skills}
secrets = ["OPENROUTER_API_KEY"]

[policy]
authority = "read"
model_allowlist = ["{model}"]
trigger_bindings = ["manual"]
iteration_cap = 24
turn_cap = 40
tool_action_cap = 80
output_bytes_cap = 120000
wall_clock_minutes = 30
side_effect_policy = "kill"
"#,
        name = toml_escape(&role.name),
        preferred = toml_escape(&role.model_policy.preferred),
        reasoning = toml_escape(&role.model_policy.reasoning),
        model = toml_escape(&model),
        skills = skills,
    ))
}

pub fn render_brief(
    agent: &Agent,
    add_skills: &[String],
    add_mcps: &[String],
    card: Option<&CardContext>,
) -> String {
    let role = &agent.role;
    let mut output = format!(
        r#"# Roster Brief: {name}

## Role

{description}

## Model Policy

- Preferred: {preferred}
- Fallbacks: {fallbacks}
- Reasoning: {reasoning}

## Instructions

Read: {instruction_path}

{instructions}

## Skills To Read

{skills}

## MCP Servers

{mcp_servers}

## Permissions

- Filesystem: {filesystem}
- Commands: {commands}
- Network: {network}
- Secrets: {secrets}
- Mutations: {mutations}

## Subagent Rights

- May dispatch: {may_dispatch}
- May spawn subagents: {may_spawn_subagents}
- May use peer harnesses: {may_use_peer_harnesses}

## Evidence Contract

{evidence}
"#,
        name = role.name,
        description = role.description,
        preferred = role.model_policy.preferred,
        fallbacks = role.model_policy.fallbacks.join(", "),
        reasoning = role.model_policy.reasoning,
        instruction_path = agent.instruction_path().display(),
        instructions = agent.instructions.trim(),
        skills = render_skills(&role.skills, add_skills),
        mcp_servers = render_mcp_servers(&role.mcps, &role.mcps_contextual, add_mcps),
        filesystem = role.permissions.filesystem,
        commands = role.permissions.commands,
        network = role.permissions.network,
        secrets = role.permissions.secrets,
        mutations = role.permissions.mutations,
        may_dispatch = role.subagent_rights.may_dispatch,
        may_spawn_subagents = role.subagent_rights.may_spawn_subagents,
        may_use_peer_harnesses = role.subagent_rights.may_use_peer_harnesses,
        evidence = bullet_list(&role.evidence_expectations),
    );

    if let Some(card) = card {
        output.push_str("\n## Powder Card\n\n");
        output.push_str(&format!("- ID: {}\n", card.id));
        output.push_str(&format!("- Title: {}\n", card.title));
        if !card.acceptance.is_empty() {
            output.push_str("\n### Acceptance\n\n");
            output.push_str(&bullet_list(&card.acceptance));
            output.push('\n');
        }
        if !card.body.trim().is_empty() {
            output.push_str("\n### Body\n\n");
            output.push_str(card.body.trim());
            output.push('\n');
        }
    }

    output
}

fn bb_model(role: &Role, providers: &Providers) -> Result<String, String> {
    if let Some(model) = openrouter_model(&role.model_policy.preferred) {
        return Ok(model.to_string());
    }
    if let Some(model) = role
        .model_policy
        .fallbacks
        .iter()
        .find_map(|fallback| openrouter_model(fallback))
    {
        return Ok(model.to_string());
    }

    // Neither `preferred` nor any fallback is a literal `openrouter/`-prefixed
    // model id. Resolve `preferred` as a tier symbol through providers.yaml
    // instead of ever falling back to the bare tier string (that string is
    // not an invocable model, so a bb config carrying it would silently fail
    // at dispatch time rather than at render time).
    providers
        .bb_tier(&role.model_policy.preferred)
        .map(|model| openrouter_model(model).unwrap_or(model).to_string())
        .ok_or_else(|| {
            format!(
                "cannot resolve bb model for agent {:?}: preferred {:?} is not an \
                 openrouter/-prefixed literal, has no openrouter/-prefixed fallback, \
                 and is not a known tier in primitives/tiers.yaml",
                role.name, role.model_policy.preferred
            )
        })
}

fn openrouter_model(value: &str) -> Option<&str> {
    value.strip_prefix("openrouter/")
}

pub fn render_show(agent: &Agent) -> String {
    let role = &agent.role;
    format!(
        r#"# {name}

{description}

- Directory: {directory}
- Instructions: {instruction_path}
- Preferred model: {preferred}
- Fallbacks: {fallbacks}
- Reasoning: {reasoning}
- Skills: {skill_count}
- MCPs: {mcps}
- Contextual MCPs: {mcps_contextual}

## Evidence Expectations

{evidence}
"#,
        name = role.name,
        description = role.description,
        directory = agent.directory.display(),
        instruction_path = agent.instruction_path().display(),
        preferred = role.model_policy.preferred,
        fallbacks = role.model_policy.fallbacks.join(", "),
        reasoning = role.model_policy.reasoning,
        skill_count = role.skills.len(),
        mcps = if role.mcps.is_empty() {
            "none".to_string()
        } else {
            role.mcps.join(", ")
        },
        mcps_contextual = if role.mcps_contextual.is_empty() {
            "none".to_string()
        } else {
            role.mcps_contextual.join(", ")
        },
        evidence = bullet_list(&role.evidence_expectations),
    )
}

fn load_agent(directory: PathBuf) -> Result<Agent, RosterError> {
    let role_path = directory.join("role.yaml");
    let role_text = fs::read_to_string(&role_path).map_err(|source| RosterError::Io {
        path: role_path.clone(),
        source,
    })?;
    let role: Role = serde_yaml::from_str(&role_text).map_err(|source| RosterError::Yaml {
        path: role_path.clone(),
        source,
    })?;

    let instructions_path = directory.join("instructions.md");
    let instructions =
        fs::read_to_string(&instructions_path).map_err(|source| RosterError::Io {
            path: instructions_path.clone(),
            source,
        })?;

    validate_agent(&directory, &role, &instructions)?;

    Ok(Agent {
        directory,
        role,
        instructions,
    })
}

fn validate_agent(directory: &Path, role: &Role, instructions: &str) -> Result<(), RosterError> {
    if role.schema_version != "roster.role.v1" {
        return Err(RosterError::Validation(format!(
            "{} role.yaml has unsupported schema_version {:?}",
            directory.display(),
            role.schema_version
        )));
    }

    let directory_name = directory
        .file_name()
        .and_then(|name| name.to_str())
        .ok_or_else(|| {
            RosterError::Validation(format!(
                "{} has no valid directory name",
                directory.display()
            ))
        })?;
    if role.name != directory_name {
        return Err(RosterError::Validation(format!(
            "{} role name {:?} does not match directory {:?}",
            directory.display(),
            role.name,
            directory_name
        )));
    }

    require_non_empty(&role.name, "name", directory)?;
    require_non_empty(&role.description, "description", directory)?;
    require_non_empty(
        &role.model_policy.preferred,
        "model_policy.preferred",
        directory,
    )?;
    require_non_empty(
        &role.model_policy.reasoning,
        "model_policy.reasoning",
        directory,
    )?;
    require_non_empty(
        &role.permissions.filesystem,
        "permissions.filesystem",
        directory,
    )?;
    require_non_empty(
        &role.permissions.commands,
        "permissions.commands",
        directory,
    )?;
    require_non_empty(&role.permissions.network, "permissions.network", directory)?;
    require_non_empty(&role.permissions.secrets, "permissions.secrets", directory)?;
    require_non_empty(
        &role.permissions.mutations,
        "permissions.mutations",
        directory,
    )?;
    require_non_empty(instructions, "instructions.md", directory)?;

    for skill in &role.skills {
        require_non_empty(&skill.name, "skills[].name", directory)?;
        require_non_empty(&skill.path, "skills[].path", directory)?;
        require_non_empty(&skill.reason, "skills[].reason", directory)?;
    }

    for mcp in &role.mcps {
        require_non_empty(mcp, "mcps[]", directory)?;
    }

    for mcp in &role.mcps_contextual {
        require_non_empty(mcp, "mcps_contextual[]", directory)?;
    }

    for expectation in &role.evidence_expectations {
        require_non_empty(expectation, "evidence_expectations[]", directory)?;
    }

    Ok(())
}

fn validate_unique_names(agents: &[Agent]) -> Result<(), RosterError> {
    let mut names = BTreeSet::new();
    for agent in agents {
        if !names.insert(&agent.role.name) {
            return Err(RosterError::Validation(format!(
                "duplicate agent name {:?}",
                agent.role.name
            )));
        }
    }
    Ok(())
}

fn require_non_empty(value: &str, field: &str, directory: &Path) -> Result<(), RosterError> {
    if value.trim().is_empty() {
        return Err(RosterError::Validation(format!(
            "{} has empty {}",
            directory.display(),
            field
        )));
    }
    Ok(())
}

fn render_skills(skills: &[SkillRef], add_skills: &[String]) -> String {
    let mut lines = Vec::new();
    for skill in skills {
        lines.push(format!(
            "- {}: {} ({})",
            skill.name, skill.path, skill.reason
        ));
    }
    for skill in add_skills {
        lines.push(format!("- override: {skill}"));
    }
    if lines.is_empty() {
        "- none".to_string()
    } else {
        lines.join("\n")
    }
}

fn render_mcp_servers(mcps: &[String], mcps_contextual: &[String], add_mcps: &[String]) -> String {
    let required = mcp_lines(mcps, &[]);
    let contextual = mcp_lines(mcps_contextual, add_mcps);
    format!("### Required\n\n{required}\n\n### Contextual (bind when present)\n\n{contextual}")
}

fn mcp_lines(mcps: &[String], overrides: &[String]) -> String {
    let mut lines = mcps
        .iter()
        .map(|mcp| format!("- {mcp}"))
        .collect::<Vec<_>>();
    for mcp in overrides {
        lines.push(format!("- override: {mcp}"));
    }
    if lines.is_empty() {
        "- none".to_string()
    } else {
        lines.join("\n")
    }
}

fn bullet_list(items: &[String]) -> String {
    if items.is_empty() {
        "- none".to_string()
    } else {
        items
            .iter()
            .map(|item| format!("- {item}"))
            .collect::<Vec<_>>()
            .join("\n")
    }
}

fn toml_array(items: &[String]) -> String {
    let values = items
        .iter()
        .map(|item| format!("\"{}\"", toml_escape(item)))
        .collect::<Vec<_>>()
        .join(", ");
    format!("[{values}]")
}

fn toml_escape(value: &str) -> String {
    value.replace('\\', "\\\\").replace('"', "\\\"")
}
