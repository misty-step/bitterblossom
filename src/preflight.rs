//! Read-only pre-dispatch checks before run rows exist: declared secrets,
//! local command binaries, and manual subscription-auth readiness.

use std::ffi::OsString;
use std::path::Path;
use std::process::Command;

use anyhow::{bail, Context, Result};
use serde::Serialize;

use crate::spec::{AuthClass, Plane, Task, TriggerSpec};

#[derive(Debug, Default, Serialize)]
pub struct Finding {
    pub task: String,
    pub kind: &'static str,
    pub detail: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub classification: Option<&'static str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub host: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub substrate: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub harness: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub bin: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub model: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub remediation: Option<String>,
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
            Ok(check_tasks(plane, std::iter::once(t)))
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
            Ok(check_tasks(plane, members))
        }
    }
}

fn check_tasks<'a>(plane: &Plane, tasks: impl IntoIterator<Item = &'a Task>) -> Report {
    let mut tasks_checked = Vec::new();
    let mut findings = Vec::new();
    for t in tasks {
        tasks_checked.push(t.name.clone());
        for name in &t.agent.secrets {
            match crate::provider_keys::resolve_secret_for_task(plane, t, name) {
                Ok(Some(_)) => continue,
                Ok(None) => {}
                Err(e) => {
                    findings.push(simple_finding(
                        &t.name,
                        "missing_provider_key",
                        format!("{e:#}"),
                    ));
                    continue;
                }
            }
            if std::env::var(name)
                .map(|value| value.trim().is_empty())
                .unwrap_or(true)
            {
                findings.push(simple_finding(
                    &t.name,
                    "missing_secret",
                    format!("declared secret '{name}' is not set in the environment"),
                ));
            }
        }
        if t.agent.harness == "command" && t.spec.substrate == "local" {
            if let Some(bin) = &t.agent.bin {
                if let Some(detail) = unspawnable_detail(bin) {
                    findings.push(simple_finding(&t.name, "unspawnable_binary", detail));
                }
            }
        }
        if t.agent.harness == "command" && t.spec.substrate == "sprites" {
            if let Some(bin) = &t.agent.bin {
                let host = t.host();
                if let Some(detail) = crate::substrate::sprites::remote_command_unspawnable_detail(
                    &host, &t.name, bin,
                ) {
                    findings.push(command_binary_finding(t, &host, bin, detail));
                }
            }
        }
        if let Some(finding) = subscription_auth_readiness(t) {
            findings.push(finding);
        }
    }
    Report {
        tasks_checked,
        findings,
    }
}

fn simple_finding(task: &str, kind: &'static str, detail: String) -> Finding {
    Finding {
        task: task.to_string(),
        kind,
        detail,
        ..Finding::default()
    }
}

fn subscription_auth_readiness(t: &Task) -> Option<Finding> {
    if t.agent.auth_class().ok()? != AuthClass::Subscription || !manual_only(t) {
        return None;
    }
    let host = t.host();
    let bin = t
        .agent
        .bin
        .clone()
        .unwrap_or_else(|| t.agent.harness.clone());
    let remediation = subscription_auth_remediation(&t.agent.harness, &host, &t.name);
    let Some((probe_env, probe)) = subscription_auth_probe(&t.agent.harness) else {
        let probe_env = subscription_auth_probe_env_name(&t.agent.harness);
        return Some(readiness_finding(
            t,
            &host,
            &bin,
            "subscription_auth_unverified",
            format!(
                "subscription auth readiness for harness '{}' on host '{}' is unverified: set {probe_env} (or BB_PREFLIGHT_SUBSCRIPTION_AUTH_PROBE) to a read-only probe executable",
                t.agent.harness, host
            ),
            remediation,
        ));
    };
    match Command::new(&probe)
        .env("BB_PREFLIGHT_TASK", &t.name)
        .env("BB_PREFLIGHT_HOST", &host)
        .env("BB_PREFLIGHT_SUBSTRATE", &t.spec.substrate)
        .env("BB_PREFLIGHT_HARNESS", &t.agent.harness)
        .env("BB_PREFLIGHT_BIN", &bin)
        .env("BB_PREFLIGHT_MODEL", &t.agent.model)
        .output()
    {
        Ok(output) if output.status.success() => None,
        Ok(output) => {
            let body = probe_output_detail(&output.stdout, &output.stderr);
            let detail = if body.is_empty() {
                format!(
                    "subscription auth readiness probe '{}' exited with {}",
                    probe.to_string_lossy(),
                    output.status
                )
            } else {
                format!(
                    "subscription auth readiness probe '{}' exited with {}: {body}",
                    probe.to_string_lossy(),
                    output.status
                )
            };
            Some(readiness_finding(
                t,
                &host,
                &bin,
                "subscription_auth_unready",
                detail,
                remediation,
            ))
        }
        Err(e) => Some(readiness_finding(
            t,
            &host,
            &bin,
            "subscription_auth_unready",
            format!(
                "subscription auth readiness probe '{}' from {probe_env} could not start: {e}",
                probe.to_string_lossy()
            ),
            remediation,
        )),
    }
}

fn readiness_finding(
    t: &Task,
    host: &str,
    bin: &str,
    kind: &'static str,
    detail: String,
    remediation: String,
) -> Finding {
    Finding {
        classification: Some("readiness"),
        host: Some(host.to_string()),
        substrate: Some(t.spec.substrate.clone()),
        harness: Some(t.agent.harness.clone()),
        bin: Some(bin.to_string()),
        model: Some(t.agent.model.clone()),
        remediation: Some(remediation),
        ..simple_finding(&t.name, kind, detail)
    }
}

fn command_binary_finding(t: &Task, host: &str, bin: &str, detail: String) -> Finding {
    Finding {
        classification: Some("readiness"),
        host: Some(host.to_string()),
        substrate: Some(t.spec.substrate.clone()),
        harness: Some(t.agent.harness.clone()),
        bin: Some(bin.to_string()),
        model: Some(t.agent.model.clone()),
        remediation: Some(format!(
            "install command harness bin '{bin}' on substrate host '{host}' or update task '{}' to a command available there, then rerun `bb preflight {} --json`",
            t.name, t.name
        )),
        ..simple_finding(&t.name, "unspawnable_binary", detail)
    }
}

fn manual_only(t: &Task) -> bool {
    !t.spec
        .triggers
        .iter()
        .any(|trigger| !matches!(trigger, TriggerSpec::Manual))
}

fn subscription_auth_probe(harness: &str) -> Option<(String, OsString)> {
    let specific = subscription_auth_probe_env_name(harness);
    for name in [&specific, "BB_PREFLIGHT_SUBSCRIPTION_AUTH_PROBE"] {
        if let Some(value) = std::env::var_os(name).filter(|v| !v.is_empty()) {
            return Some((name.to_string(), value));
        }
    }
    None
}

fn subscription_auth_probe_env_name(harness: &str) -> String {
    format!(
        "BB_PREFLIGHT_SUBSCRIPTION_AUTH_PROBE_{}",
        harness.to_ascii_uppercase().replace('-', "_")
    )
}

fn subscription_auth_remediation(harness: &str, host: &str, task: &str) -> String {
    match harness {
        "codex" => format!(
            "run `codex login` or refresh Codex subscription auth on substrate host '{host}', then rerun `bb preflight {task} --json`"
        ),
        "claude" => format!(
            "refresh Claude Code subscription auth on substrate host '{host}', then rerun `bb preflight {task} --json`"
        ),
        other => format!(
            "refresh subscription auth for harness '{other}' on substrate host '{host}', then rerun `bb preflight {task} --json`"
        ),
    }
}

fn probe_output_detail(stdout: &[u8], stderr: &[u8]) -> String {
    let stderr = String::from_utf8_lossy(stderr).trim().to_string();
    let stdout = String::from_utf8_lossy(stdout).trim().to_string();
    let detail = [stderr, stdout]
        .into_iter()
        .filter(|s| !s.is_empty())
        .collect::<Vec<_>>()
        .join("\n");
    detail.chars().take(2_000).collect()
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
