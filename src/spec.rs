use std::collections::BTreeMap;
use std::path::{Component, Path, PathBuf};

use anyhow::{bail, Context, Result};
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Deserialize)]
pub struct PlaneSpec {
    #[serde(default = "default_db_path")]
    pub db_path: String,
    #[serde(default)]
    pub dev: bool,
    #[serde(default)]
    pub ingress: IngressSpec,
    #[serde(default)]
    pub notify: NotifySpec,
    #[serde(default)]
    pub backup: BackupSpec,
    #[serde(default)]
    pub budget: GlobalBudget,
    #[serde(default, rename = "workload_repo")]
    pub workload_repos: Vec<WorkloadRepoSpec>,
    pub gate: Option<GateSpec>,
}

impl Default for PlaneSpec {
    fn default() -> Self {
        Self {
            db_path: default_db_path(),
            dev: false,
            ingress: IngressSpec::default(),
            notify: NotifySpec::default(),
            backup: BackupSpec::default(),
            budget: GlobalBudget::default(),
            workload_repos: Vec::new(),
            gate: None,
        }
    }
}

#[derive(Debug, Clone, Deserialize)]
pub struct WorkloadRepoSpec {
    pub name: String,
    pub path: Option<String>,
    pub url: Option<String>,
    pub repo_url: Option<String>,
    #[serde(default = "default_ref")]
    pub r#ref: String,
    pub agent: String,
    #[serde(default = "default_substrate")]
    pub substrate: String,
    #[serde(default)]
    pub workspace: WorkspaceSpec,
    #[serde(default)]
    pub budget_caps: TaskBudget,
}
#[derive(Debug, Clone, Deserialize)]
pub struct GateSpec {
    pub required: Vec<String>,
    #[serde(default)]
    pub quorum: Option<usize>,
    #[serde(default = "default_gate_arm_timeout_seconds")]
    pub arm_timeout_seconds: u64,
    #[serde(default = "default_max_rounds")]
    pub max_rounds: u32,
    #[serde(default = "default_arbiter")]
    pub arbiter: String,
}

impl GateSpec {
    pub fn effective_quorum(&self) -> usize {
        self.quorum.unwrap_or(self.required.len())
    }
}

fn default_max_rounds() -> u32 {
    3
}

fn default_gate_arm_timeout_seconds() -> u64 {
    3600
}

fn default_arbiter() -> String {
    "arbiter".to_string()
}

fn default_db_path() -> String {
    ".bb/plane.db".to_string()
}

#[derive(Debug, Clone, Deserialize)]
pub struct IngressSpec {
    #[serde(default = "default_bind")]
    pub bind: String,
    /// Maximum accepted webhook request body size in bytes. Oversized
    /// deliveries are rejected (413) before ingest, so they grow no ledger
    /// row. Defaults to a generous webhook ceiling (backlog 083).
    #[serde(default = "default_max_body_bytes")]
    pub max_body_bytes: usize,
    /// Maximum cron fires ingested per tick; older catch-up fires beyond
    /// this are collapsed to the latest and counted as skipped. Bounds
    /// unattended catch-up after downtime (backlog 083).
    #[serde(default = "default_max_cron_catchup_fires")]
    pub max_cron_catchup_fires: u32,
}
fn default_bind() -> String {
    "127.0.0.1:7077".to_string()
}
fn default_max_body_bytes() -> usize {
    1_048_576
}
fn default_max_cron_catchup_fires() -> u32 {
    1
}
impl Default for IngressSpec {
    fn default() -> Self {
        Self {
            bind: default_bind(),
            max_body_bytes: default_max_body_bytes(),
            max_cron_catchup_fires: default_max_cron_catchup_fires(),
        }
    }
}

#[derive(Debug, Clone, Deserialize, Default)]
pub struct NotifySpec {
    pub webhook_url: Option<String>,
}

#[derive(Debug, Clone, Deserialize, Serialize, Default)]
pub struct BackupSpec {
    #[serde(default)]
    pub enabled: bool,
    pub provider: Option<String>,
    pub replica_env: Option<String>,
    pub last_success_path: Option<String>,
    pub rpo_seconds: Option<u64>,
    pub rto_seconds: Option<u64>,
}

#[derive(Debug, Clone, Deserialize, Default)]
pub struct GlobalBudget {
    pub max_cost_per_day_usd: Option<f64>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct AgentSpec {
    #[serde(default = "default_version")]
    pub version: u32,
    pub harness: String,
    #[serde(default)]
    pub model: String,
    pub role: Option<String>,
    #[serde(default)]
    pub skills: Vec<String>,
    pub provider: Option<String>,
    pub auth: Option<String>,
    pub bin: Option<String>,
    #[serde(default)]
    pub args: Vec<String>,
    #[serde(default)]
    pub secrets: Vec<String>,
    /// Optional per-agent governance boundary (backlog 091): validated at
    /// load, projected read-only via check/task-list/api-tasks JSON.
    #[serde(default)]
    pub policy: PolicySpec,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum AuthClass {
    Subscription,
    Api,
}

impl AgentSpec {
    pub fn auth_class(&self) -> Result<AuthClass> {
        match self.auth.as_deref() {
            Some("subscription") => Ok(AuthClass::Subscription),
            Some("api") => Ok(AuthClass::Api),
            Some(other) => bail!("unknown auth '{other}' (known: subscription, api)"),
            None => Ok(match self.harness.as_str() {
                "claude" | "codex" => AuthClass::Subscription,
                _ => AuthClass::Api,
            }),
        }
    }

    pub fn provider(&self) -> &str {
        self.provider.as_deref().unwrap_or("openrouter")
    }
}
/// Optional per-agent governance boundary (backlog 091): authority, provider
/// key, spend cap, model allowlist, trigger bindings, caps, side-effect policy.
#[derive(Debug, Clone, Default, Deserialize, Serialize)]
pub struct PolicySpec {
    pub authority: Option<String>,
    pub provider_key_name: Option<String>,
    pub provider_spend_cap_usd: Option<f64>,
    #[serde(default)]
    pub model_allowlist: Vec<String>,
    #[serde(default)]
    pub trigger_bindings: Vec<String>,
    pub iteration_cap: Option<u32>,
    pub turn_cap: Option<u32>,
    pub tool_action_cap: Option<u32>,
    pub output_bytes_cap: Option<u64>,
    pub wall_clock_minutes: Option<u64>,
    pub side_effect_policy: Option<String>,
}

impl PolicySpec {
    /// Reject malformed values and model-allowlist mismatches at load.
    pub fn validate(&self, agent: &str, model: &str) -> Result<()> {
        if let Some(a) = &self.authority {
            if !matches!(a.as_str(), "read" | "edit" | "merge") {
                bail!("agent '{agent}': policy.authority '{a}' is unknown (read/edit/merge)");
            }
        }
        if let Some(s) = &self.side_effect_policy {
            if !matches!(s.as_str(), "log" | "quarantine" | "kill") {
                bail!("agent '{agent}': policy.side_effect_policy '{s}' is unknown");
            }
        }
        if let Some(c) = self.provider_spend_cap_usd {
            if c < 0.0 {
                bail!("agent '{agent}': policy.provider_spend_cap_usd must be non-negative");
            }
        }
        for (field, n) in [
            ("iteration_cap", self.iteration_cap.map(u64::from)),
            ("turn_cap", self.turn_cap.map(u64::from)),
            ("tool_action_cap", self.tool_action_cap.map(u64::from)),
            ("output_bytes_cap", self.output_bytes_cap),
            ("wall_clock_minutes", self.wall_clock_minutes),
        ] {
            if let Some(n) = n {
                if n == 0 {
                    bail!("agent '{agent}': policy.{field} must be greater than zero");
                }
            }
        }
        let mut seen = std::collections::BTreeSet::new();
        for b in &self.trigger_bindings {
            if !matches!(b.as_str(), "manual" | "cron" | "webhook") {
                bail!("agent '{agent}': policy.trigger_bindings entry '{b}' is unknown");
            }
            if !seen.insert(b.clone()) {
                bail!("agent '{agent}': policy.trigger_bindings entry '{b}' is duplicated");
            }
        }
        if !self.model_allowlist.is_empty()
            && !model.is_empty()
            && !self.model_allowlist.iter().any(|m| m == model)
        {
            bail!(
                "agent '{agent}': model '{model}' is not in policy.model_allowlist (allowed: {})",
                self.model_allowlist.join(", ")
            );
        }
        Ok(())
    }
}

fn default_version() -> u32 {
    1
}

#[derive(Debug, Clone, Deserialize)]
pub struct TaskSpec {
    pub agent: String,
    #[serde(default = "default_substrate")]
    pub substrate: String,
    #[serde(default)]
    pub workspace: WorkspaceSpec,
    #[serde(default)]
    pub budget: TaskBudget,
    #[serde(default, rename = "trigger")]
    pub triggers: Vec<TriggerSpec>,
    pub pre_command: Option<String>,
    pub post_command: Option<String>,
    pub verdict: Option<String>,
    /// Artifact paths, relative to the attempt artifact dir, that must exist
    /// after a zero-exit harness run for the run to count as success. Current
    /// substrates release `REPORT.json`; other paths are rejected until the
    /// artifact transport is generalized.
    #[serde(default)]
    pub required_artifacts: Vec<String>,
}

fn default_substrate() -> String {
    "local".to_string()
}

#[derive(Debug, Clone, Deserialize, Default)]
pub struct WorkspaceSpec {
    pub host: Option<String>,
    #[serde(default)]
    pub repos: Vec<RepoSpec>,
    pub checkpoint: Option<String>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct RepoSpec {
    pub url: String,
    #[serde(default = "default_ref")]
    pub r#ref: String,
}

fn default_ref() -> String {
    "master".to_string()
}

#[derive(Debug, Clone, Deserialize, Default)]
pub struct TaskBudget {
    pub timeout_minutes: Option<u64>,
    pub max_runs_per_day: Option<u32>,
    pub max_cost_per_run_usd: Option<f64>,
    pub turn_cap: Option<u32>,
    pub tool_action_cap: Option<u32>,
    pub output_bytes_cap: Option<u64>,
}

#[derive(Debug, Clone, Deserialize)]
#[serde(tag = "kind", rename_all = "snake_case")]
pub enum TriggerSpec {
    Manual,
    Cron {
        schedule: String,
    },
    Webhook {
        route: String,
        secret_env: String,
        dedupe_key: Option<String>,
        action: Option<WebhookActionSpec>,
        #[serde(default)]
        filter: Vec<FilterSpec>,
    },
}

#[derive(Debug, Clone, Deserialize)]
#[serde(tag = "kind", rename_all = "snake_case", deny_unknown_fields)]
pub enum WebhookActionSpec {
    SubmissionStorm {
        change: String,
        rev: String,
        repo: Option<String>,
        version: Option<String>,
    },
}

#[derive(Debug, Clone, Deserialize)]
pub struct FilterSpec {
    pub pointer: String,
    pub equals: Option<serde_json::Value>,
    pub any_of: Option<Vec<serde_json::Value>>,
    pub not_any_of: Option<Vec<serde_json::Value>>,
    pub max: Option<f64>,
}

impl FilterSpec {
    fn validate(&self) -> Result<()> {
        let n = [
            self.equals.is_some(),
            self.any_of.is_some(),
            self.not_any_of.is_some(),
            self.max.is_some(),
        ]
        .iter()
        .filter(|b| **b)
        .count();
        if n != 1 {
            bail!(
                "filter on '{}' needs exactly one of equals / any_of / not_any_of / max",
                self.pointer
            );
        }
        Ok(())
    }
    pub fn reject_reason(&self, payload: &serde_json::Value) -> Option<String> {
        let Some(found) = payload.pointer(&self.pointer) else {
            return Some(format!("pointer '{}' missing from payload", self.pointer));
        };
        if let Some(expected) = &self.equals {
            if found != expected {
                return Some(format!("'{}' is {found}, want {expected}", self.pointer));
            }
        }
        if let Some(allowed) = &self.any_of {
            if !allowed.contains(found) {
                return Some(format!("'{}' is {found}, not in allowlist", self.pointer));
            }
        }
        if let Some(denied) = &self.not_any_of {
            if denied.contains(found) {
                return Some(format!("'{}' is {found}, in denylist", self.pointer));
            }
        }
        if let Some(max) = self.max {
            match found.as_f64() {
                Some(v) if v <= max => {}
                Some(v) => return Some(format!("'{}' is {v}, max {max}", self.pointer)),
                None => return Some(format!("'{}' is not numeric", self.pointer)),
            }
        }
        None
    }
}
#[derive(Debug, Clone)]
pub struct Task {
    pub name: String,
    pub spec: TaskSpec,
    pub card: String,
    pub agent_name: String,
    pub agent: AgentSpec,
    pub source: Option<TaskSource>,
}

#[derive(Debug, Clone, Serialize)]
pub struct TaskSource {
    pub repo: String,
    #[serde(rename = "ref")]
    pub r#ref: String,
}

impl Task {
    pub fn host(&self) -> String {
        self.spec
            .workspace
            .host
            .clone()
            .unwrap_or_else(|| "local".to_string())
    }
}
#[derive(Debug, Clone)]
pub struct Plane {
    pub root: PathBuf,
    pub spec: PlaneSpec,
    pub agents: BTreeMap<String, AgentSpec>,
    pub tasks: BTreeMap<String, Task>,
}

impl Plane {
    pub fn load(root: &Path) -> Result<Self> {
        let root = &root
            .canonicalize()
            .with_context(|| format!("plane root {}", root.display()))?;
        let plane_path = root.join("plane.toml");
        let spec: PlaneSpec = if plane_path.exists() {
            toml::from_str(&std::fs::read_to_string(&plane_path)?)
                .with_context(|| format!("parse {}", plane_path.display()))?
        } else {
            PlaneSpec::default()
        };

        let mut agents = BTreeMap::new();
        let agents_dir = root.join("agents");
        if agents_dir.is_dir() {
            for entry in std::fs::read_dir(&agents_dir)? {
                let path = entry?.path();
                if path.extension().and_then(|e| e.to_str()) != Some("toml") {
                    continue;
                }
                let name = path
                    .file_stem()
                    .and_then(|s| s.to_str())
                    .context("agent file name")?
                    .to_string();
                let agent: AgentSpec = toml::from_str(&std::fs::read_to_string(&path)?)
                    .with_context(|| format!("parse {}", path.display()))?;
                agents.insert(name, agent);
            }
        }

        let mut tasks = BTreeMap::new();
        let tasks_dir = root.join("tasks");
        if tasks_dir.is_dir() {
            for entry in std::fs::read_dir(&tasks_dir)? {
                let dir = entry?.path();
                if !dir.is_dir() {
                    continue;
                }
                let name = dir
                    .file_name()
                    .and_then(|s| s.to_str())
                    .context("task dir name")?
                    .to_string();
                let spec_path = dir.join("task.toml");
                if !spec_path.exists() {
                    continue;
                }
                let task_spec: TaskSpec = toml::from_str(&std::fs::read_to_string(&spec_path)?)
                    .with_context(|| format!("parse {}", spec_path.display()))?;
                let card_path = dir.join("card.md");
                let card = std::fs::read_to_string(&card_path)
                    .with_context(|| format!("read {}", card_path.display()))?;
                let agent = agents
                    .get(&task_spec.agent)
                    .with_context(|| {
                        format!("task '{name}' binds unknown agent '{}'", task_spec.agent)
                    })?
                    .clone();
                tasks.insert(
                    name.clone(),
                    Task {
                        name,
                        agent_name: task_spec.agent.clone(),
                        agent,
                        spec: task_spec,
                        card,
                        source: None,
                    },
                );
            }
        }
        load_workload_repo_tasks(root, &spec, &agents, &mut tasks)?;
        for (name, agent) in &agents {
            let auth = agent
                .auth_class()
                .with_context(|| format!("agent '{name}'"))?;
            if agent.harness == "command" {
                if agent.bin.is_none() {
                    bail!("agent '{name}': harness = \"command\" requires bin");
                }
            } else if agent.model.is_empty() {
                bail!(
                    "agent '{name}': model is required for harness '{}'",
                    agent.harness
                );
            }
            match (agent.harness.as_str(), auth) {
                ("claude" | "codex", AuthClass::Api) => bail!(
                    "agent '{name}': {} runs on subscription auth only — \
                     Anthropic/OpenAI API keys are forbidden on this plane",
                    agent.harness
                ),
                ("pi" | "omp", AuthClass::Subscription) => {
                    bail!(
                        "agent '{name}': {} has no subscription auth; use auth = \"api\"",
                        agent.harness
                    )
                }
                _ => {}
            }
            for secret in &agent.secrets {
                if secret == "ANTHROPIC_API_KEY" || secret == "OPENAI_API_KEY" {
                    bail!(
                        "agent '{name}': secret '{secret}' is forbidden — \
                         claude/codex run on subscription auth, never API keys"
                    );
                }
            }
            agent
                .policy
                .validate(name, &agent.model)
                .with_context(|| format!("agent '{name}'"))?;
        }
        validate_backup_spec(&spec.backup)?;
        let mut routes = std::collections::BTreeSet::new();
        for task in tasks.values() {
            validate_required_artifacts(&task.name, &task.spec.required_artifacts)?;
            if task.spec.substrate != "local" && task.spec.workspace.host.is_none() {
                bail!(
                    "task '{}': substrate '{}' requires workspace.host",
                    task.name,
                    task.spec.substrate
                );
            }
            if task.spec.substrate == "local" && !spec.dev {
                bail!(
                    "task '{}': the local substrate is dev/test machinery — \
                         production planes dispatch to a configured remote substrate. \
                         Set `dev = true` in plane.toml only for a development plane.",
                    task.name
                );
            }
            let auth = task
                .agent
                .auth_class()
                .with_context(|| format!("task '{}'", task.name))?;
            let reflex = task
                .spec
                .triggers
                .iter()
                .any(|t| !matches!(t, TriggerSpec::Manual));
            if reflex && auth == AuthClass::Subscription {
                bail!(
                    "task '{}': reflex triggers (webhook/cron) require an auth = \"api\" \
                     agent on an open harness — subscription agents ({}) run dispatch \
                     (manual) work only",
                    task.name,
                    task.agent_name
                );
            }
            for trigger in &task.spec.triggers {
                match trigger {
                    TriggerSpec::Webhook {
                        route,
                        filter,
                        action,
                        ..
                    } => {
                        if !routes.insert(route.clone()) {
                            bail!("webhook route '{route}' declared by more than one trigger");
                        }
                        if action.is_some() && spec.gate.is_none() {
                            bail!(
                                "task '{}': submission_storm action requires [gate]",
                                task.name
                            );
                        }
                        for f in filter {
                            f.validate()
                                .with_context(|| format!("task '{}'", task.name))?;
                        }
                    }
                    TriggerSpec::Cron { schedule } => {
                        crate::ingress::parse_schedule(schedule)
                            .with_context(|| format!("task '{}': bad cron trigger", task.name))?;
                    }
                    TriggerSpec::Manual => {}
                }
            }
        }
        if let Some(gate) = &spec.gate {
            if gate.required.is_empty() {
                bail!("[gate] required must list at least one verdict kind");
            }
            let quorum = gate.effective_quorum();
            if quorum == 0 || quorum > gate.required.len() {
                bail!(
                    "[gate] quorum must be between 1 and required.len() ({})",
                    gate.required.len()
                );
            }
            if gate.arm_timeout_seconds == 0 {
                bail!("[gate] arm_timeout_seconds must be greater than zero");
            }
            for kind in &gate.required {
                if !tasks
                    .values()
                    .any(|t| t.spec.verdict.as_deref() == Some(kind.as_str()))
                {
                    bail!("[gate] requires verdict kind '{kind}' but no task declares it");
                }
            }
        }
        let mut kinds = std::collections::BTreeSet::new();
        for task in tasks.values() {
            if let Some(kind) = &task.spec.verdict {
                if !kinds.insert(kind.clone()) {
                    bail!("verdict kind '{kind}' declared by more than one task");
                }
            }
        }

        Ok(Self {
            root: root.to_path_buf(),
            spec,
            agents,
            tasks,
        })
    }

    pub fn task(&self, name: &str) -> Result<&Task> {
        self.tasks.get(name).with_context(|| {
            let known: Vec<&str> = self.tasks.keys().map(|s| s.as_str()).collect();
            format!("unknown task '{name}' (known: {})", known.join(", "))
        })
    }

    pub fn db_path(&self) -> PathBuf {
        let p = Path::new(&self.spec.db_path);
        if p.is_absolute() {
            p.to_path_buf()
        } else {
            self.root.join(p)
        }
    }
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct RepoOwnedTaskSpec {
    pub agent: Option<String>,
    pub substrate: Option<String>,
    #[serde(default)]
    pub workspace: WorkspaceSpec,
    #[serde(default)]
    pub budget: TaskBudget,
    #[serde(default, rename = "trigger")]
    pub triggers: Vec<TriggerSpec>,
    pub pre_command: Option<String>,
    pub post_command: Option<String>,
    pub verdict: Option<String>,
    #[serde(default)]
    pub required_artifacts: Vec<String>,
}

fn load_workload_repo_tasks(
    plane_root: &Path,
    plane_spec: &PlaneSpec,
    agents: &BTreeMap<String, AgentSpec>,
    tasks: &mut BTreeMap<String, Task>,
) -> Result<()> {
    let mut names = std::collections::BTreeSet::new();
    for repo in &plane_spec.workload_repos {
        if !names.insert(repo.name.clone()) {
            bail!(
                "workload repo '{}': name declared more than once",
                repo.name
            );
        }
        if repo.name.contains('/') || repo.name.trim().is_empty() {
            bail!(
                "workload repo '{}': name must be a non-empty namespace segment",
                repo.name
            );
        }
        if repo.url.is_some() {
            bail!(
                "workload repo '{}': url checkout is not in v1; use path to a checked-out repo",
                repo.name
            );
        }
        if repo.path.is_none() {
            bail!("workload repo '{}': path is required", repo.name);
        }
        if repo.substrate == "local" && !plane_spec.dev {
            bail!(
                "workload repo '{}': local substrate is dev/test machinery; set dev = true only for a development plane",
                repo.name
            );
        }
        let (repo_dir, source_repo) = workload_repo_dir(plane_root, repo)?;
        let workspace_repo = repo.repo_url.as_ref().unwrap_or(&source_repo);
        let task_root = repo_dir.join(".bb/tasks");
        if !task_root.is_dir() {
            continue;
        }
        for entry in std::fs::read_dir(&task_root)? {
            let dir = entry?.path();
            if !dir.is_dir() {
                continue;
            }
            let short = dir
                .file_name()
                .and_then(|s| s.to_str())
                .context("repo task dir name")?;
            let name = format!("{}/{}", repo.name, short);
            let spec_path = dir.join("task.toml");
            if !spec_path.exists() {
                continue;
            }
            let raw: RepoOwnedTaskSpec = toml::from_str(&std::fs::read_to_string(&spec_path)?)
                .with_context(|| {
                    format!(
                        "workload repo '{}': parse {}",
                        repo.name,
                        spec_path.display()
                    )
                })?;
            let spec = repo_task_spec(repo, workspace_repo, short, raw, agents)?;
            let card_path = dir.join("card.md");
            let card = std::fs::read_to_string(&card_path).with_context(|| {
                format!(
                    "workload repo '{}': read {}",
                    repo.name,
                    card_path.display()
                )
            })?;
            let agent = agents
                .get(&spec.agent)
                .with_context(|| format!("workload repo '{}': agent '{}'", repo.name, spec.agent))?
                .clone();
            if tasks.contains_key(&name) {
                bail!("task '{name}' declared by more than one source");
            }
            tasks.insert(
                name.clone(),
                Task {
                    name,
                    agent_name: spec.agent.clone(),
                    agent,
                    spec,
                    card,
                    source: Some(TaskSource {
                        repo: source_repo.clone(),
                        r#ref: repo.r#ref.clone(),
                    }),
                },
            );
        }
    }
    Ok(())
}

fn repo_task_spec(
    repo: &WorkloadRepoSpec,
    workspace_repo: &str,
    task_name: &str,
    raw: RepoOwnedTaskSpec,
    agents: &BTreeMap<String, AgentSpec>,
) -> Result<TaskSpec> {
    if raw.workspace.host.is_some()
        || raw.workspace.checkpoint.is_some()
        || !raw.workspace.repos.is_empty()
    {
        bail!(
            "workload repo '{}': task '{task_name}' declares workspace authority; workspace is plane-owned",
            repo.name
        );
    }
    if let Some(agent) = &raw.agent {
        if !agents.contains_key(agent) {
            bail!(
                "workload repo '{}': task '{task_name}' binds unknown agent '{agent}'",
                repo.name
            );
        }
        if agent != &repo.agent {
            bail!(
                "workload repo '{}': task '{task_name}' requests agent '{agent}' but plane grants '{}'",
                repo.name,
                repo.agent
            );
        }
    }
    if let Some(substrate) = &raw.substrate {
        if substrate != &repo.substrate {
            bail!(
                "workload repo '{}': task '{task_name}' requests substrate '{substrate}' but plane grants '{}'",
                repo.name,
                repo.substrate
            );
        }
    }
    Ok(TaskSpec {
        agent: repo.agent.clone(),
        substrate: repo.substrate.clone(),
        workspace: WorkspaceSpec {
            host: repo.workspace.host.clone(),
            repos: vec![RepoSpec {
                url: workspace_repo.to_string(),
                r#ref: repo.r#ref.clone(),
            }],
            checkpoint: repo.workspace.checkpoint.clone(),
        },
        budget: bounded_budget(&repo.name, task_name, &raw.budget, &repo.budget_caps)?,
        triggers: raw.triggers,
        pre_command: raw.pre_command,
        post_command: raw.post_command,
        verdict: raw.verdict,
        required_artifacts: raw.required_artifacts,
    })
}

fn validate_backup_spec(backup: &BackupSpec) -> Result<()> {
    if !backup.enabled {
        return Ok(());
    }
    let required = [
        ("provider", backup.provider.as_deref()),
        ("last_success_path", backup.last_success_path.as_deref()),
    ];
    for (field, value) in required {
        if value.is_none_or(|v| v.trim().is_empty()) {
            bail!("[backup] enabled=true requires {field}");
        }
    }
    if backup.rpo_seconds.is_none_or(|v| v == 0) {
        bail!("[backup] enabled=true requires rpo_seconds > 0");
    }
    if backup.rto_seconds.is_none_or(|v| v == 0) {
        bail!("[backup] enabled=true requires rto_seconds > 0");
    }
    if backup
        .replica_env
        .as_deref()
        .is_some_and(|v| v.trim().is_empty())
    {
        bail!("[backup] replica_env must be a non-empty env var name when set");
    }
    Ok(())
}

fn validate_required_artifacts(task: &str, artifacts: &[String]) -> Result<()> {
    for artifact in artifacts {
        let path = Path::new(artifact);
        let valid = !artifact.trim().is_empty()
            && !path.is_absolute()
            && path
                .components()
                .all(|component| matches!(component, Component::Normal(_)));
        if !valid {
            bail!(
                "task '{task}': required_artifacts entry {artifact:?} must be a non-empty relative path without '.' or '..'"
            );
        }
        if artifact != crate::substrate::REPORT_FILENAME {
            bail!(
                "task '{task}': required_artifacts entry {artifact:?} is not released by current substrates; only REPORT.json is supported today"
            );
        }
    }
    Ok(())
}

fn bounded_budget(
    repo: &str,
    task: &str,
    req: &TaskBudget,
    cap: &TaskBudget,
) -> Result<TaskBudget> {
    Ok(TaskBudget {
        timeout_minutes: cap_u64(
            repo,
            task,
            "timeout_minutes",
            req.timeout_minutes,
            cap.timeout_minutes,
        )?,
        max_runs_per_day: cap_u32(
            repo,
            task,
            "max_runs_per_day",
            req.max_runs_per_day,
            cap.max_runs_per_day,
        )?,
        max_cost_per_run_usd: cap_f64(
            repo,
            task,
            "max_cost_per_run_usd",
            req.max_cost_per_run_usd,
            cap.max_cost_per_run_usd,
        )?,
        turn_cap: cap_u32(repo, task, "turn_cap", req.turn_cap, cap.turn_cap)?,
        tool_action_cap: cap_u32(
            repo,
            task,
            "tool_action_cap",
            req.tool_action_cap,
            cap.tool_action_cap,
        )?,
        output_bytes_cap: cap_u64(
            repo,
            task,
            "output_bytes_cap",
            req.output_bytes_cap,
            cap.output_bytes_cap,
        )?,
    })
}

fn cap_u64(
    repo: &str,
    task: &str,
    field: &str,
    req: Option<u64>,
    cap: Option<u64>,
) -> Result<Option<u64>> {
    match (req, cap) {
        (Some(r), Some(c)) if r > c => {
            bail!("workload repo '{repo}': task '{task}' budget {field} {r} exceeds plane cap {c}")
        }
        (Some(_), None) => bail!(
            "workload repo '{repo}': task '{task}' budget {field} requested but plane cap is unset"
        ),
        (Some(r), Some(_)) => Ok(Some(r)),
        (None, c) => Ok(c),
    }
}

fn cap_u32(
    repo: &str,
    task: &str,
    field: &str,
    req: Option<u32>,
    cap: Option<u32>,
) -> Result<Option<u32>> {
    cap_u64(repo, task, field, req.map(u64::from), cap.map(u64::from)).map(|v| v.map(|n| n as u32))
}

fn cap_f64(
    repo: &str,
    task: &str,
    field: &str,
    req: Option<f64>,
    cap: Option<f64>,
) -> Result<Option<f64>> {
    match (req, cap) {
        (Some(r), Some(c)) if r > c => {
            bail!("workload repo '{repo}': task '{task}' budget {field} {r} exceeds plane cap {c}")
        }
        (Some(_), None) => bail!(
            "workload repo '{repo}': task '{task}' budget {field} requested but plane cap is unset"
        ),
        (Some(r), Some(_)) => Ok(Some(r)),
        (None, c) => Ok(c),
    }
}

fn workload_repo_dir(root: &Path, repo: &WorkloadRepoSpec) -> Result<(PathBuf, String)> {
    let path = repo.path.as_ref().expect("validated path");
    let p = Path::new(path);
    let dir = if p.is_absolute() {
        p.to_path_buf()
    } else {
        root.join(p)
    };
    if !dir.is_dir() {
        bail!(
            "workload repo '{}': path {} is not a directory",
            repo.name,
            dir.display()
        );
    }
    let dir = dir.canonicalize()?;
    let source = dir.to_string_lossy().into_owned();
    Ok((dir, source))
}
