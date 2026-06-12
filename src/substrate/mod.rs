//! Execution substrates. The substrate knows environments and sessions,
//! not repos: workspace materialization is a declarative `WorkspacePlan`
//! executed by the adapter, testable identically on every adapter.

pub mod local;
pub mod sprites;

use std::path::Path;
use std::time::Duration;

use anyhow::Result;

use crate::spec::RepoSpec;

#[derive(Debug, Clone)]
pub struct WorkspacePlan {
    /// Substrate resource identity — the host-lease key.
    pub host: String,
    pub repos: Vec<RepoSpec>,
    /// Lane card content, materialized into the workspace.
    pub card: String,
    /// Trigger payload (webhook body, replay payload), materialized as
    /// EVENT.json so the agent can read what fired it.
    pub payload: Option<String>,
    /// Prior-round gate report for verdict tasks, materialized as
    /// REPORT.json — the canonical snapshot, never driver-rewritten.
    pub report: Option<String>,
    pub pre_command: Option<String>,
    pub post_command: Option<String>,
    /// Per-attempt marker used for pidfiles: the probe and cancel key.
    pub marker: String,
    /// Stable task workspace name. Adapters map it to their own path or
    /// resource; dispatch never constructs substrate-specific paths.
    pub workspace_name: String,
    /// Snapshot to restore before preparing; adapters without snapshots
    /// ignore it.
    pub checkpoint: Option<String>,
    /// Resolved per-exec credentials (env name, value). Never persisted.
    pub secrets: Vec<(String, String)>,
    /// Hermetic execs (auth = "api" agents) get a scrubbed environment and
    /// a workspace-local HOME: nothing of the operator's identity crosses
    /// the exec boundary except the declared secrets. Subscription agents
    /// keep the real HOME — they run *as* the operator by design.
    pub hermetic: bool,
}

#[derive(Debug)]
pub struct ExecResult {
    pub exit_code: i64,
    pub stdout: String,
    pub stderr: String,
    pub timed_out: bool,
}

/// What a probe of a (possibly dead) attempt's host found.
#[derive(Debug, PartialEq, Eq)]
pub enum ProbeResult {
    /// The attempt's process is still running on the host.
    Alive,
    /// The host is reachable and the process is gone.
    Dead,
    /// The host could not be probed (unreachable, no pidfile).
    Unknown(String),
}

pub trait Substrate {
    fn name(&self) -> &'static str;
    /// Acquire a session on `host`. The durable host lease lives in the
    /// ledger; this is resource setup (wake sprite, create workspace dir).
    fn acquire(&self, host: &str, attempt_dir: &Path) -> Result<Box<dyn Session>>;
    /// Probe an attempt by its marker: used at boot to classify inherited
    /// `running` runs instead of blindly orphaning them.
    fn probe(&self, host: &str, attempt_dir: &Path, marker: &str) -> ProbeResult;
}

pub trait Session {
    /// Materialize the workspace: optional snapshot restore, repo checkouts
    /// at declared refs, card file, pre_command. Nothing here may start the
    /// agent.
    fn prepare(&mut self, plan: &WorkspacePlan) -> Result<()>;
    /// Run a command in the workspace with a wall-clock timeout — the v1
    /// spend backstop. Timeout kills the process (best-effort remotely)
    /// and reports `timed_out`.
    fn execute(
        &mut self,
        cmd: &[String],
        stdin: Option<&str>,
        timeout: Duration,
    ) -> Result<ExecResult>;
    fn write_artifact(&mut self, name: &str, data: &[u8]) -> Result<()>;
    fn release(&mut self) -> Result<()>;
}

/// Path to the lane card inside a prepared workspace.
pub const CARD_FILENAME: &str = "LANE_CARD.md";
/// Path to the trigger payload inside a prepared workspace.
pub const EVENT_FILENAME: &str = "EVENT.json";
/// Path to the prior-round gate report inside a prepared workspace.
pub const REPORT_FILENAME: &str = "REPORT.json";

pub fn for_task(kind: &str) -> Result<Box<dyn Substrate>> {
    match kind {
        "local" => Ok(Box::new(local::LocalSubstrate)),
        "sprites" => Ok(Box::new(sprites::SpritesSubstrate)),
        other => anyhow::bail!("unknown substrate '{other}' (known: local, sprites)"),
    }
}
