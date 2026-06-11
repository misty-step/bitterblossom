//! Local-process substrate: a workspace directory on this machine. The
//! degenerate case that keeps every task terminal-runnable and tests cheap.

use std::io::Write as _;
use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};
use std::time::{Duration, Instant};

use anyhow::{Context, Result};

use super::{ExecResult, ProbeResult, Session, Substrate, WorkspacePlan, CARD_FILENAME};

/// Pidfile written next to the artifacts so a restarted plane can probe
/// whether the harness process survived.
pub const PIDFILE: &str = "harness.pid";

pub struct LocalSubstrate;

impl Substrate for LocalSubstrate {
    fn name(&self) -> &'static str {
        "local"
    }

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
        // kill(pid, 0): existence check without signaling.
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
    fn run_shell(&self, script: &str, timeout: Duration) -> Result<ExecResult> {
        run_with_timeout(
            &["sh".into(), "-c".into(), script.into()],
            None,
            &self.workspace,
            &[],
            false,
            None,
            timeout,
        )
    }

    /// The only env vars that cross the exec boundary. Provider API keys in
    /// the plane's environment (OPENAI_API_KEY, ANTHROPIC_API_KEY, …) never
    /// reach a workload unless declared as agent secrets.
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
            let out = self.run_shell(&clone, Duration::from_secs(300))?;
            if out.exit_code != 0 {
                anyhow::bail!("clone {} failed: {}", repo.url, out.stderr.trim());
            }
        }
        std::fs::write(self.workspace.join(CARD_FILENAME), &plan.card)?;
        if let Some(payload) = &plan.payload {
            std::fs::write(self.workspace.join(super::EVENT_FILENAME), payload)?;
        }
        if let Some(pre) = &plan.pre_command {
            let out = self.run_shell(pre, Duration::from_secs(600))?;
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
    ) -> Result<ExecResult> {
        run_with_timeout(
            cmd,
            stdin,
            &self.workspace,
            &self.workload_env()?,
            true,
            Some(&self.artifacts.join(PIDFILE)),
            timeout,
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
        // Workspace is kept on disk under the attempt dir for diagnosis;
        // nothing to tear down for a local process.
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

pub(crate) fn shell_quote(s: &str) -> String {
    format!("'{}'", s.replace('\'', "'\\''"))
}

pub(crate) fn run_with_timeout(
    cmd: &[String],
    stdin: Option<&str>,
    cwd: &Path,
    envs: &[(String, String)],
    clear_env: bool,
    pidfile: Option<&Path>,
    timeout: Duration,
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
    // Own process group so a timeout kills the harness AND everything it
    // spawned — a surviving grandchild is hidden concurrent work.
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
    if let Some(path) = pidfile {
        let _ = std::fs::write(path, child.id().to_string());
    }

    if let Some(input) = stdin {
        let mut pipe = child.stdin.take().context("stdin pipe")?;
        pipe.write_all(input.as_bytes())?;
        // Drop closes the pipe so the child sees EOF.
    }

    let started = Instant::now();
    let mut timed_out = false;
    let exit_code = loop {
        if let Some(status) = child.try_wait()? {
            break status.code().unwrap_or(-1) as i64;
        }
        if started.elapsed() >= timeout {
            timed_out = true;
            // Kill the whole process group, not just the direct child.
            unsafe {
                libc::kill(-(child.id() as i32), libc::SIGKILL);
            }
            let _ = child.kill();
            let status = child.wait()?;
            break status.code().unwrap_or(-1) as i64;
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
    })
}
