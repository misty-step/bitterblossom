//! Pre-dispatch preflight: report missing declared secrets and unspawnable
//! command-harness binaries *before* dispatch creates run rows. This is
//! operator-state plumbing — it inspects config + the local environment, never
//! workload judgment.
//!
//! Two check types, both read-only:
//! - **missing_secret**: an agent-declared secret env var is unset in the
//!   environment `bb` runs in. Secrets travel from this environment to the
//!   substrate, so an unset secret here fails on every substrate.
//! - **unspawnable_binary**: a `command`-harness bin cannot be executed on the
//!   host running `bb`. Only checked for `substrate = "local"` — that is the
//!   only substrate whose binaries `bb` can actually see. A sprites command
//!   harness runs on the remote sprite; preflight cannot inspect that
//!   filesystem and does not pretend to.
//!
//! Preflight targets either one task or the submission-storm member set (the
//! gate-required verdict tasks). It is a report, not a gate: it never mutates
//! ledger state and never blocks dispatch on its own.

use std::path::Path;

use anyhow::{bail, Context, Result};
use serde::Serialize;

use crate::spec::{Plane, Task};

#[derive(Debug, Serialize)]
pub struct Finding {
    pub task: String,
    pub kind: &'static str,
    pub detail: String,
}

#[derive(Debug, Serialize)]
pub struct Report {
    pub tasks_checked: Vec<String>,
    pub findings: Vec<Finding>,
}

/// Inspect one task (`Some(task)`) or the submission-storm member set
/// (`storm = true`). Exactly one of the two must be requested.
pub fn run(plane: &Plane, task: Option<&str>, storm: bool) -> Result<Report> {
    match (task, storm) {
        (Some(_), true) => bail!("pass either a task or --storm, not both"),
        (None, false) => bail!("preflight needs a task or --storm"),
        (Some(name), false) => {
            let t = plane.task(name)?;
            Ok(check_tasks(std::iter::once(t)))
        }
        (None, true) => {
            let Some(gate) = &plane.spec.gate else {
                bail!("--storm requested but this plane has no [gate]; no storm member set to preflight");
            };
            if gate.required.is_empty() {
                bail!("--storm requested but [gate].required is empty");
            }
            // Each required verdict kind maps to the task declaring it.
            let mut members: Vec<&Task> = Vec::new();
            for kind in &gate.required {
                let t = plane
                    .tasks
                    .values()
                    .find(|t| t.spec.verdict.as_deref() == Some(kind.as_str()))
                    .with_context(|| {
                        format!("no task declares verdict = \"{kind}\" (gate.required member)")
                    })?;
                members.push(t);
            }
            Ok(check_tasks(members))
        }
    }
}

fn check_tasks<'a>(tasks: impl IntoIterator<Item = &'a Task>) -> Report {
    let mut tasks_checked = Vec::new();
    let mut findings = Vec::new();
    for t in tasks {
        tasks_checked.push(t.name.clone());
        for name in &t.agent.secrets {
            if std::env::var(name).is_err() {
                findings.push(Finding {
                    task: t.name.clone(),
                    kind: "missing_secret",
                    detail: format!("declared secret '{name}' is not set in the environment"),
                });
            }
        }
        if t.agent.harness == "command" && t.spec.substrate == "local" {
            if let Some(bin) = &t.agent.bin {
                if let Some(detail) = unspawnable_detail(bin) {
                    findings.push(Finding {
                        task: t.name.clone(),
                        kind: "unspawnable_binary",
                        detail,
                    });
                }
            }
        }
    }
    Report {
        tasks_checked,
        findings,
    }
}

/// `None` if `bin` resolves to an executable file on this host, else a reason.
/// A path containing a separator is checked directly; a bare name is searched
/// on `PATH` (mirroring how a shell would resolve it).
fn unspawnable_detail(bin: &str) -> Option<String> {
    use std::os::unix::fs::PermissionsExt;
    let is_exec = |p: &Path| {
        std::fs::metadata(p)
            .map(|m| m.is_file() && (m.permissions().mode() & 0o111 != 0))
            .unwrap_or(false)
    };
    if bin.contains(std::path::MAIN_SEPARATOR) || bin.contains('/') {
        let p = Path::new(bin);
        if is_exec(p) {
            return None;
        }
        return Some(format!(
            "command harness bin '{bin}' is not an executable file on this host"
        ));
    }
    if let Some(path) = std::env::var_os("PATH") {
        for dir in std::env::split_paths(&path) {
            if is_exec(&dir.join(bin)) {
                return None;
            }
        }
    }
    Some(format!("command harness bin '{bin}' not found on PATH"))
}
