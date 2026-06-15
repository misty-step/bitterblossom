pub mod local;
pub mod sprites;

use std::path::Path;
use std::time::Duration;

use anyhow::Result;

use crate::spec::RepoSpec;

pub struct WorkspacePlan {
    pub repos: Vec<RepoSpec>,
    pub card: String,
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
}
#[derive(Debug, PartialEq, Eq)]
pub enum ProbeResult {
    Alive,
    Dead,
    Unknown(String),
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
    ) -> Result<ExecResult>;
    fn write_artifact(&mut self, name: &str, data: &[u8]) -> Result<()>;
    fn release(&mut self) -> Result<()>;
}
pub const CARD_FILENAME: &str = "LANE_CARD.md";
pub const EVENT_FILENAME: &str = "EVENT.json";
pub const REPORT_FILENAME: &str = "REPORT.json";

pub fn for_task(kind: &str) -> Result<Box<dyn Substrate>> {
    match kind {
        "local" => Ok(Box::new(local::LocalSubstrate)),
        "sprites" => Ok(Box::new(sprites::SpritesSubstrate)),
        other => anyhow::bail!("unknown substrate '{other}' (known: local, sprites)"),
    }
}
