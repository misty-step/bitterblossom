use std::io::Write as _;
use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};
use std::time::{Duration, Instant};

use anyhow::{Context, Result};

use super::{
    ExecMonitor, ExecResult, ExecSnapshot, ProbeResult, Session, Substrate, WorkspacePlan,
    CARD_FILENAME,
};
pub const PIDFILE: &str = "harness.pid";

pub struct LocalSubstrate;

impl Substrate for LocalSubstrate {
    fn acquire(&self, _host: &str, attempt_dir: &Path) -> Result<Box<dyn Session>> {
        let workspace = attempt_dir.join("workspace");
        std::fs::create_dir_all(&workspace)?;
        Ok(Box::new(LocalSession {
            workspace,
            artifacts: attempt_dir.to_path_buf(),
            secrets: Vec::new(),
            hermetic: true,
        }))
    }

    fn probe(&self, _host: &str, attempt_dir: &Path, _marker: &str) -> ProbeResult {
        let pidfile = attempt_dir.join(PIDFILE);
        let Ok(content) = std::fs::read_to_string(&pidfile) else {
            return ProbeResult::Unknown(format!("no pidfile at {}", pidfile.display()));
        };
        let Ok(pid) = content.trim().parse::<i32>() else {
            return ProbeResult::Unknown("unparseable pidfile".into());
        };
        if unsafe { libc::kill(pid, 0) } == 0 {
            ProbeResult::Alive
        } else {
            ProbeResult::Dead
        }
    }
}

pub struct LocalSession {
    workspace: PathBuf,
    artifacts: PathBuf,
    secrets: Vec<(String, String)>,
    hermetic: bool,
}

impl LocalSession {
    fn run_workload_shell(&self, script: &str, timeout: Duration) -> Result<ExecResult> {
        run_with_timeout(
            &["sh".into(), "-c".into(), script.into()],
            None,
            &self.workspace,
            &self.workload_env()?,
            self.hermetic,
            timeout,
            RunControl::default(),
        )
    }
    fn workload_env(&self) -> Result<Vec<(String, String)>> {
        const PASS: &[&str] = &[
            "PATH",
            "TMPDIR",
            "TERM",
            "COLORTERM",
            "LANG",
            "LC_ALL",
            "LC_CTYPE",
            "USER",
            "LOGNAME",
            "SHELL",
            "TZ",
            // bitterblossom-915: execute() always clear_env=true, so any
            // workload shelling out to `op` (bash/sh scripts, MCP-server
            // bootstraps spawned with a minimal env) had no service-account
            // token and fell back to the 1Password desktop-app integration,
            // popping an interactive authorize modal on the operator's
            // screen. Pass it through like PATH so local workloads inherit
            // the agent-fleet token the same way an interactive zsh shell
            // does via ~/.zshenv.
            "OP_SERVICE_ACCOUNT_TOKEN",
        ];
        let mut env: Vec<(String, String)> = PASS
            .iter()
            .filter_map(|k| std::env::var(k).ok().map(|v| (k.to_string(), v)))
            .collect();
        let home = if self.hermetic {
            let home = self.workspace.join(".home");
            std::fs::create_dir_all(&home)?;
            home.to_string_lossy().into_owned()
        } else {
            std::env::var("HOME").context("HOME not set")?
        };
        env.push(("HOME".to_string(), home));
        env.extend(self.secrets.iter().cloned());
        Ok(env)
    }
}

impl Session for LocalSession {
    fn prepare(&mut self, plan: &WorkspacePlan) -> Result<()> {
        self.secrets = plan.secrets.clone();
        self.hermetic = plan.hermetic;
        for repo in &plan.repos {
            let dest = self
                .workspace
                .join(repo_dir_name(&repo.url))
                .to_string_lossy()
                .into_owned();
            let clone = format!(
                "git clone --depth 1 --branch {ref_} {url} {dest} && git -C {dest} reset --hard",
                ref_ = shell_quote(&repo.r#ref),
                url = shell_quote(&repo.url),
                dest = shell_quote(&dest),
            );
            let clone = format!("{}\n{clone}", git_auth_setup_script());
            let out = self.run_workload_shell(&clone, Duration::from_secs(300))?;
            if out.exit_code != 0 {
                anyhow::bail!("clone {} failed: {}", repo.url, out.stderr.trim());
            }
        }
        std::fs::write(self.workspace.join(CARD_FILENAME), &plan.card)?;
        std::fs::write(self.workspace.join("RUN.json"), &plan.run_context)?;
        if let Some(payload) = &plan.payload {
            std::fs::write(self.workspace.join(super::EVENT_FILENAME), payload)?;
        }
        if let Some(report) = &plan.report {
            std::fs::write(self.workspace.join(super::REPORT_FILENAME), report)?;
        }
        if let Some(pre) = &plan.pre_command {
            let out = run_with_timeout(
                &["sh".into(), "-c".into(), pre.clone()],
                None,
                &self.workspace,
                &self.workload_env()?,
                true,
                Duration::from_secs(600),
                RunControl::default(),
            )?;
            if out.exit_code != 0 {
                anyhow::bail!("pre_command failed: {}", out.stderr.trim());
            }
        }
        Ok(())
    }

    fn execute(
        &mut self,
        cmd: &[String],
        stdin: Option<&str>,
        timeout: Duration,
        monitor: Option<&mut ExecMonitor<'_>>,
    ) -> Result<ExecResult> {
        run_with_timeout(
            cmd,
            stdin,
            &self.workspace,
            &self.workload_env()?,
            true,
            timeout,
            RunControl {
                pidfile: Some(&self.artifacts.join(PIDFILE)),
                monitor,
            },
        )
    }

    fn write_artifact(&mut self, name: &str, data: &[u8]) -> Result<()> {
        let path = self.artifacts.join(name);
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent)?;
        }
        std::fs::write(path, data)?;
        Ok(())
    }

    fn release(&mut self) -> Result<()> {
        let report = self.workspace.join(super::REPORT_FILENAME);
        if report.exists() {
            std::fs::copy(report, self.artifacts.join(super::REPORT_FILENAME))?;
        }
        let packet = self.workspace.join(super::ASK_PACKET_FILENAME);
        if packet.exists() {
            std::fs::copy(packet, self.artifacts.join(super::ASK_PACKET_FILENAME))?;
        }
        Ok(())
    }
}

fn repo_dir_name(url: &str) -> String {
    url.trim_end_matches('/')
        .trim_end_matches(".git")
        .rsplit('/')
        .next()
        .unwrap_or("repo")
        .to_string()
}

pub fn shell_quote(s: &str) -> String {
    format!("'{}'", s.replace('\'', "'\\''"))
}

pub(crate) fn git_auth_setup_script() -> &'static str {
    r#"export GIT_TERMINAL_PROMPT=0
if [ -n "${GH_TOKEN:-}" ]; then
  cat > .bb-git-askpass <<'BB_GIT_ASKPASS'
#!/bin/sh
case "$1" in
  *Username*) printf '%s\n' x-access-token ;;
  *Password*) printf '%s\n' "$GH_TOKEN" ;;
  *) printf '\n' ;;
esac
BB_GIT_ASKPASS
  chmod 700 .bb-git-askpass
  export GIT_ASKPASS="$PWD/.bb-git-askpass"
fi"#
}

#[derive(Default)]
pub(crate) struct RunControl<'pid, 'monitor, 'callback> {
    pub pidfile: Option<&'pid Path>,
    pub monitor: Option<&'monitor mut ExecMonitor<'callback>>,
}

pub(crate) fn run_with_timeout(
    cmd: &[String],
    stdin: Option<&str>,
    cwd: &Path,
    envs: &[(String, String)],
    clear_env: bool,
    timeout: Duration,
    mut control: RunControl<'_, '_, '_>,
) -> Result<ExecResult> {
    use std::sync::atomic::{AtomicU64, Ordering};
    static CAPTURE_SEQ: AtomicU64 = AtomicU64::new(0);

    let (program, args) = cmd.split_first().context("empty command")?;
    let seq = CAPTURE_SEQ.fetch_add(1, Ordering::Relaxed);
    let capture = |stream: &str| {
        std::env::temp_dir().join(format!("bb-cap-{}-{seq}.{stream}", std::process::id()))
    };
    let stdout_path = capture("out");
    let stderr_path = capture("err");
    let mut command = Command::new(program);
    command
        .args(args)
        .current_dir(cwd)
        .stdin(if stdin.is_some() {
            Stdio::piped()
        } else {
            Stdio::null()
        })
        .stdout(std::fs::File::create(&stdout_path)?)
        .stderr(std::fs::File::create(&stderr_path)?);
    {
        use std::os::unix::process::CommandExt;
        command.process_group(0);
    }
    if clear_env {
        command.env_clear();
    }
    for (k, v) in envs {
        command.env(k, v);
    }
    let mut child = command
        .spawn()
        .with_context(|| format!("spawn {program}"))?;
    if let Some(path) = control.pidfile {
        let _ = std::fs::write(path, child.id().to_string());
    }

    if let Some(input) = stdin {
        let mut pipe = child.stdin.take().context("stdin pipe")?;
        pipe.write_all(input.as_bytes())?;
    }

    let started = Instant::now();
    let mut next_monitor_at = started;
    let mut timed_out = false;
    let mut termination_reason = None;
    let exit_code = loop {
        if let Some(status) = child.try_wait()? {
            break status.code().unwrap_or(-1) as i64;
        }
        if started.elapsed() >= timeout {
            timed_out = true;
            unsafe {
                libc::kill(-(child.id() as i32), libc::SIGKILL);
            }
            let _ = child.kill();
            let status = child.wait()?;
            break status.code().unwrap_or(-1) as i64;
        }
        if let Some(monitor) = control.monitor.as_deref_mut() {
            let now = Instant::now();
            if now >= next_monitor_at {
                let stdout = std::fs::read_to_string(&stdout_path).unwrap_or_default();
                let stderr = std::fs::read_to_string(&stderr_path).unwrap_or_default();
                let snapshot = ExecSnapshot {
                    elapsed: started.elapsed(),
                    stdout: &stdout,
                    stderr: &stderr,
                };
                if let Some(reason) = (monitor.check)(&snapshot) {
                    termination_reason = Some(reason);
                    unsafe {
                        libc::kill(-(child.id() as i32), libc::SIGKILL);
                    }
                    let _ = child.kill();
                    let status = child.wait()?;
                    break status.code().unwrap_or(-1) as i64;
                }
                next_monitor_at = now + monitor.poll_interval;
            }
        }
        std::thread::sleep(Duration::from_millis(100));
    };

    let stdout = std::fs::read_to_string(&stdout_path).unwrap_or_default();
    let stderr = std::fs::read_to_string(&stderr_path).unwrap_or_default();
    let _ = std::fs::remove_file(&stdout_path);
    let _ = std::fs::remove_file(&stderr_path);
    Ok(ExecResult {
        exit_code,
        stdout,
        stderr,
        timed_out,
        termination_reason,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Mutex;

    // Guards OP_SERVICE_ACCOUNT_TOKEN, a process-global env var, against
    // concurrent cargo test threads.
    static ENV_LOCK: Mutex<()> = Mutex::new(());

    #[test]
    fn workload_env_passes_through_op_service_account_token() {
        let _guard = ENV_LOCK.lock().unwrap();
        std::env::set_var("OP_SERVICE_ACCOUNT_TOKEN", "test-op-token-value");

        let tmp = tempfile::tempdir().unwrap();
        let mut session = LocalSubstrate.acquire("local", tmp.path()).unwrap();
        let result = session
            .execute(
                &[
                    "sh".to_string(),
                    "-c".to_string(),
                    "printf '%s' \"$OP_SERVICE_ACCOUNT_TOKEN\"".to_string(),
                ],
                None,
                Duration::from_secs(5),
                None,
            )
            .unwrap();

        std::env::remove_var("OP_SERVICE_ACCOUNT_TOKEN");
        assert_eq!(result.stdout, "test-op-token-value");
    }
}
