pub mod local;
pub mod sprites;
pub mod tailnet;

use std::path::Path;
use std::time::Duration;

use anyhow::Result;

use crate::spec::RepoSpec;

pub struct WorkspacePlan {
    pub repos: Vec<RepoSpec>,
    pub card: String,
    pub run_context: String,
    pub payload: Option<String>,
    pub report: Option<String>,
    pub pre_command: Option<String>,
    pub post_command: Option<String>,
    pub marker: String,
    pub workspace_name: String,
    pub checkpoint: Option<String>,
    pub secrets: Vec<(String, String)>,
    pub hermetic: bool,
}

pub struct ExecResult {
    pub exit_code: i64,
    pub stdout: String,
    pub stderr: String,
    pub timed_out: bool,
    pub termination_reason: Option<String>,
}

pub struct ExecSnapshot<'a> {
    pub elapsed: Duration,
    pub stdout: &'a str,
    pub stderr: &'a str,
}

pub struct ExecMonitor<'a> {
    pub poll_interval: Duration,
    pub check: &'a mut dyn FnMut(&ExecSnapshot<'_>) -> Option<String>,
}
#[derive(Debug, PartialEq, Eq)]
pub enum ProbeResult {
    Alive,
    Dead,
    Unknown(String),
}

impl ProbeResult {
    pub fn state(&self) -> &'static str {
        match self {
            ProbeResult::Alive => "alive",
            ProbeResult::Dead => "dead",
            ProbeResult::Unknown(_) => "unknown",
        }
    }

    pub fn reason(&self) -> Option<&str> {
        match self {
            ProbeResult::Unknown(reason) => Some(reason.as_str()),
            ProbeResult::Alive | ProbeResult::Dead => None,
        }
    }

    pub fn description(&self) -> String {
        match self {
            ProbeResult::Alive => "alive".to_string(),
            ProbeResult::Dead => "dead".to_string(),
            ProbeResult::Unknown(reason) => format!("unknown: {reason}"),
        }
    }
}

pub trait Substrate {
    fn acquire(&self, host: &str, attempt_dir: &Path) -> Result<Box<dyn Session>>;
    fn probe(&self, host: &str, attempt_dir: &Path, marker: &str) -> ProbeResult;
}

pub trait Session {
    fn prepare(&mut self, plan: &WorkspacePlan) -> Result<()>;
    fn execute(
        &mut self,
        cmd: &[String],
        stdin: Option<&str>,
        timeout: Duration,
        monitor: Option<&mut ExecMonitor<'_>>,
    ) -> Result<ExecResult>;
    fn write_artifact(&mut self, name: &str, data: &[u8]) -> Result<()>;
    fn release(&mut self) -> Result<()>;
}
pub const CARD_FILENAME: &str = "LANE_CARD.md";
pub const EVENT_FILENAME: &str = "EVENT.json";
pub const REPORT_FILENAME: &str = "REPORT.json";
/// bitterblossom-930: the episodic handoff packet an attempt writes to its
/// workspace when it parks on an unanswered ask. Collected the same way as
/// `REPORT_FILENAME` -- copied into the attempt's artifact dir on release,
/// never parsed by the substrate or dispatch, just relayed.
pub const ASK_PACKET_FILENAME: &str = "ASK_PACKET.json";

pub fn for_task(kind: &str) -> Result<Box<dyn Substrate>> {
    match kind {
        "local" => Ok(Box::new(local::LocalSubstrate)),
        "sprites" => Ok(Box::new(sprites::SpritesSubstrate)),
        "tailnet" => Ok(Box::new(tailnet::TailnetSubstrate)),
        other => anyhow::bail!("unknown substrate '{other}' (known: local, sprites, tailnet)"),
    }
}
