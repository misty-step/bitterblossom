//! Config loading: plane.toml, agents/<name>.toml, tasks/<name>/{task.toml,card.md}.
//! Tasks, agents, and triggers are data; the plane holds no workload logic.

use std::collections::BTreeMap;
use std::path::{Path, PathBuf};

use anyhow::{bail, Context, Result};
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Deserialize)]
pub struct PlaneSpec {
    #[serde(default = "default_db_path")]
    pub db_path: String,
    /// Dev planes may run workloads as local processes (tests, config
    /// hacking). Production planes always dispatch to a remote substrate —
    /// the plane never manages workload processes on the operator's machine.
    #[serde(default)]
    pub dev: bool,
    #[serde(default)]
    pub ingress: IngressSpec,
    #[serde(default)]
    pub notify: NotifySpec,
    #[serde(default)]
    pub budget: GlobalBudget,
    /// Merge-gate policy for the submission loop (None = no gate).
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
            gate: None,
        }
    }
}

/// Gate arithmetic config. Only `blocking`-severity findings block;
/// termination rests solely on the round cap — see docs/spine.md.
#[derive(Debug, Clone, Deserialize)]
pub struct GateSpec {
    /// Verdict kinds that must reach a terminal run before any `clear`.
    pub required: Vec<String>,
    #[serde(default = "default_max_rounds")]
    pub max_rounds: u32,
    /// Verdict kind whose `pass` sustains a rejected blocking finding.
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
    /// Required for LLM harnesses; the command harness runs no model.
    #[serde(default)]
    pub model: String,
    /// Model provider for open harnesses (pi); defaults to "openrouter".
    pub provider: Option<String>,
    /// "subscription" (operator identity: claude/codex OAuth) or "api"
    /// (hermetic: only declared secrets cross the exec boundary).
    /// Defaults by harness: claude/codex → subscription, pi → api.
    pub auth: Option<String>,
    pub bin: Option<String>,
    #[serde(default)]
    pub args: Vec<String>,
    /// Env var names resolved from the plane's environment at dispatch and
    /// passed per-exec; values are never persisted (on disk or remotely).
    #[serde(default)]
    pub secrets: Vec<String>,
}

/// How an agent authenticates — the policy hinge. Subscription agents act
/// as the operator and may only run dispatch (manual) work; api agents are
/// hermetic and are the only class allowed on reflex (webhook/cron) work.
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
    /// Storm-member marker: a successful run's result must be verdict
    /// JSON, recorded against the submission named in the payload.
    pub verdict: Option<String>,
}

fn default_substrate() -> String {
    "local".to_string()
}

#[derive(Debug, Clone, Deserialize, Default)]
pub struct WorkspaceSpec {
    /// Substrate resource identity: the host-lease key. Local substrate
    /// defaults to "local"; non-local substrates require an explicit host.
    pub host: Option<String>,
    #[serde(default)]
    pub repos: Vec<RepoSpec>,
    /// Snapshot to restore during prepare; adapters without snapshots ignore it.
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
        /// Dedupe key derivation: "header:<Name>" or "json:<pointer>".
        dedupe_key: Option<String>,
        /// Payload conditions, ANDed; a delivery failing any is
        /// acknowledged but never becomes a run. Fail-closed: a missing
        /// pointer rejects. Workload-agnostic containment — repo
        /// allowlists, action filters, size caps — lives here, not in
        /// card prose an agent may or may not honor.
        #[serde(default)]
        filter: Vec<FilterSpec>,
    },
}

/// One payload condition: an RFC 6901 pointer plus exactly one predicate.
#[derive(Debug, Clone, Deserialize)]
pub struct FilterSpec {
    pub pointer: String,
    pub equals: Option<serde_json::Value>,
    pub any_of: Option<Vec<serde_json::Value>>,
    /// Numeric ceiling (inclusive): e.g. cap PR additions.
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

    /// `Some(reason)` when the payload fails this condition.
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

/// A fully loaded task: spec + lane card + resolved agent.
#[derive(Debug, Clone)]
pub struct Task {
    pub name: String,
    pub spec: TaskSpec,
    pub card: String,
    pub agent_name: String,
    pub agent: AgentSpec,
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

/// The plane's config directory, loaded eagerly so bad config fails fast.
#[derive(Debug, Clone)]
pub struct Plane {
    pub root: PathBuf,
    pub spec: PlaneSpec,
    pub agents: BTreeMap<String, AgentSpec>,
    pub tasks: BTreeMap<String, Task>,
}

impl Plane {
    /// Load from `root` (a directory containing plane.toml). Missing
    /// plane.toml is allowed — defaults apply — but a missing agent
    /// referenced by a task is an error.
    pub fn load(root: &Path) -> Result<Self> {
        // Absolute root: artifact paths cross process-cwd boundaries when
        // adapters invoke external relays, so relative roots corrupt uploads.
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
                if task_spec.substrate != "local" && task_spec.workspace.host.is_none() {
                    bail!(
                        "task '{name}': substrate '{}' requires workspace.host",
                        task_spec.substrate
                    );
                }
                if task_spec.substrate == "local" && !spec.dev {
                    bail!(
                        "task '{name}': the local substrate is dev/test machinery — \
                         production planes dispatch to a configured remote substrate. \
                         Set `dev = true` in plane.toml only for a development plane."
                    );
                }
                tasks.insert(
                    name.clone(),
                    Task {
                        name,
                        agent_name: task_spec.agent.clone(),
                        agent,
                        spec: task_spec,
                        card,
                    },
                );
            }
        }

        // Model & auth policy is code, not intent: violations fail at load
        // (and therefore at `bb check`), never at first dispatch.
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

        // Bad trigger config fails at load, not at first delivery.
        let mut routes = std::collections::BTreeSet::new();
        for task in tasks.values() {
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

        // Gate policy must resolve to real verdict tasks at load time.
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
