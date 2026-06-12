use std::collections::BTreeMap;
use std::path::{Path, PathBuf};

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
    #[serde(default = "default_max_rounds")]
    pub max_rounds: u32,
    #[serde(default = "default_arbiter")]
    pub arbiter: String,
}

fn default_max_rounds() -> u32 {
    3
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
}

fn default_bind() -> String {
    "127.0.0.1:7077".to_string()
}

impl Default for IngressSpec {
    fn default() -> Self {
        Self {
            bind: default_bind(),
        }
    }
}

#[derive(Debug, Clone, Deserialize, Default)]
pub struct NotifySpec {
    pub webhook_url: Option<String>,
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
    pub provider: Option<String>,
    pub auth: Option<String>,
    pub bin: Option<String>,
    #[serde(default)]
    pub args: Vec<String>,
    #[serde(default)]
    pub secrets: Vec<String>,
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
        #[serde(default)]
        filter: Vec<FilterSpec>,
    },
}
#[derive(Debug, Clone, Deserialize)]
pub struct FilterSpec {
    pub pointer: String,
    pub equals: Option<serde_json::Value>,
    pub any_of: Option<Vec<serde_json::Value>>,
    pub max: Option<f64>,
}

impl FilterSpec {
    fn validate(&self) -> Result<()> {
        let n = [
            self.equals.is_some(),
            self.any_of.is_some(),
            self.max.is_some(),
        ]
        .iter()
        .filter(|b| **b)
        .count();
        if n != 1 {
            bail!(
                "filter on '{}' needs exactly one of equals / any_of / max",
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
                ("pi", AuthClass::Subscription) => {
                    bail!("agent '{name}': pi has no subscription auth; use auth = \"api\"")
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
        }
        let mut routes = std::collections::BTreeSet::new();
        for task in tasks.values() {
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
                    TriggerSpec::Webhook { route, filter, .. } => {
                        if !routes.insert(route.clone()) {
                            bail!("webhook route '{route}' declared by more than one trigger");
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
    })
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
