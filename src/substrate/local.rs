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
            release_artifacts: Vec::new(),
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
    release_artifacts: Vec<String>,
}

impl LocalSession {
    fn run_checkout_shell(
        &self,
        script: &str,
        checkout_secrets: &[(String, String)],
        timeout: Duration,
    ) -> Result<ExecResult> {
        let mut env = self.workload_env()?;
        for (name, _) in &self.secrets {
            env.retain(|(candidate, _)| candidate != name);
        }
        env.extend(checkout_secrets.iter().cloned());
        run_with_timeout(
            &["sh".into(), "-c".into(), script.into()],
            None,
            &self.workspace,
            &env,
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
            // PATH may resolve a rustup/cargo shim. These are non-secret tool
            // roots required for the declared, lock-verified toolchain to run
            // inside the relocated HOME; omitting them makes hermetic command
            // workloads depend on whether cargo happens to be a standalone
            // binary.
            "RUSTUP_HOME",
            "CARGO_HOME",
            "RUSTUP_TOOLCHAIN",
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
        let parent_home = std::env::var("HOME").context("HOME not set")?;
        let home = if self.hermetic {
            let home = self.workspace.join(".home");
            std::fs::create_dir_all(&home)?;
            home.to_string_lossy().into_owned()
        } else {
            parent_home.clone()
        };
        env.push(("HOME".to_string(), home));
        if self.hermetic {
            for (name, suffix) in [("RUSTUP_HOME", ".rustup"), ("CARGO_HOME", ".cargo")] {
                if !env.iter().any(|(key, _)| key == name) {
                    let path = Path::new(&parent_home).join(suffix);
                    if path.is_dir() {
                        env.push((name.to_string(), path.to_string_lossy().into_owned()));
                    }
                }
            }
        }
        env.extend(self.secrets.iter().cloned());
        // bitterblossom-955: force OFF 1Password desktop-app settings loading
        // for every local workload, regardless of the parent env. On macOS
        // Tahoe `op` -- even in pure service-account mode with the token above
        // -- otherwise opens ~/Library/Group Containers/…1password to load
        // desktop settings; Tahoe App Data Protection blocks that open() behind
        // the "op would like to access data from other apps" TCC prompt, so op
        // hangs on the syscall and strands a wedged `op daemon --background`
        // zombie per call. Setting it false is the load-bearing fix
        // (OP_CACHE=false alone does not stop the hang). Set, not passed
        // through, so a workload is immune even when the spawning context
        // (bash -c, MCP bootstrap, cleared-env runner) never exported it.
        // Pushed last so nothing can override it. Harmless off macOS: there is
        // no desktop app to skip loading.
        env.push((
            "OP_LOAD_DESKTOP_APP_SETTINGS".to_string(),
            "false".to_string(),
        ));
        Ok(env)
    }
}

impl Session for LocalSession {
    fn prepare(&mut self, plan: &WorkspacePlan) -> Result<()> {
        self.hermetic = plan.hermetic;
        self.release_artifacts = plan.artifacts.clone();
        for repo in &plan.repos {
            let dest = self
                .workspace
                .join(repo_dir_name(&repo.url))
                .to_string_lossy()
                .into_owned();
            let clone = super::repo_materialize_script(repo, &dest);
            let clone = format!("{}\n{clone}", git_auth_setup_script());
            let out =
                self.run_checkout_shell(&clone, &plan.checkout_secrets, Duration::from_secs(300))?;
            if out.exit_code != 0 {
                anyhow::bail!("clone {} failed: {}", repo.url, out.stderr.trim());
            }
        }
        self.secrets = plan.secrets.clone();
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
        super::write_relative_nofollow(&self.artifacts, name, data)
    }

    fn release(&mut self) -> Result<()> {
        for name in &self.release_artifacts {
            if let Some(bytes) = super::read_relative_nofollow(&self.workspace, name)? {
                super::write_relative_nofollow(&self.artifacts, name, &bytes)?;
            }
        }
        Ok(())
    }
}

/// Write to a child's stdin without letting a closed pipe raise a fatal
/// SIGPIPE (the process runs with SIG_DFL; see the call site). On macOS the
/// fd is marked F_SETNOSIGPIPE so the write returns EPIPE instead of
/// signaling; elsewhere SIGPIPE is blocked for this thread around the write
/// and a pending instance is consumed before the mask is restored.
fn write_stdin_sigpipe_safe(
    pipe: &mut std::process::ChildStdin,
    bytes: &[u8],
) -> std::io::Result<()> {
    #[cfg(target_os = "macos")]
    {
        use std::os::fd::AsRawFd;
        // Darwin's F_SETNOSIGPIPE (ABI-stable value 73); the libc crate
        // exposes only the socket variant SO_NOSIGPIPE. Darwin has no
        // sigtimedwait, so there is no portable fallback: if the fcntl is
        // refused, fail loudly rather than risk a fatal SIGPIPE.
        const F_SETNOSIGPIPE: libc::c_int = 73;
        if unsafe { libc::fcntl(pipe.as_raw_fd(), F_SETNOSIGPIPE, 1) } == -1 {
            return Err(std::io::Error::last_os_error());
        }
        pipe.write_all(bytes)
    }
    #[cfg(not(target_os = "macos"))]
    {
        unsafe {
            let mut block: libc::sigset_t = std::mem::zeroed();
            libc::sigemptyset(&mut block);
            libc::sigaddset(&mut block, libc::SIGPIPE);
            let mut prev: libc::sigset_t = std::mem::zeroed();
            libc::pthread_sigmask(libc::SIG_BLOCK, &block, &mut prev);
            let result = pipe.write_all(bytes);
            if result
                .as_ref()
                .err()
                .is_some_and(|e| e.kind() == std::io::ErrorKind::BrokenPipe)
            {
                // The blocked SIGPIPE is pending on this thread; consume it
                // so restoring the mask cannot deliver it.
                let ts = libc::timespec {
                    tv_sec: 0,
                    tv_nsec: 0,
                };
                libc::sigtimedwait(&block, std::ptr::null_mut(), &ts);
            }
            libc::pthread_sigmask(libc::SIG_SETMASK, &prev, std::ptr::null_mut());
            result
        }
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
# A worker must never consult or mutate the operator host's credential helper
# (for example macOS credential-osxkeychain). Authentication is attempt-scoped
# through the declared GH_TOKEN askpass below.
export GIT_CONFIG_COUNT=1
export GIT_CONFIG_KEY_0=credential.helper
export GIT_CONFIG_VALUE_0=
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
        // A workload that exits without reading stdin must not kill bb:
        // main() restores SIGPIPE=SIG_DFL for CLI pipe hygiene, so a plain
        // write to a closed pipe would deliver a fatal SIGPIPE to this whole
        // process (observed: `bb serve`/`bb workflow execute` dying mid-run
        // with exit 141 when a fast stub exited before the prompt write).
        // The child owns its stdin; a broken pipe here is its choice.
        match write_stdin_sigpipe_safe(&mut pipe, input.as_bytes()) {
            Ok(()) => {}
            Err(e) if e.kind() == std::io::ErrorKind::BrokenPipe => {}
            Err(e) => return Err(e).context("write workload stdin"),
        }
        drop(pipe);
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

    #[test]
    fn workload_env_forces_op_load_desktop_app_settings_false() {
        // bitterblossom-955: a local workload must always see
        // OP_LOAD_DESKTOP_APP_SETTINGS=false so `op` never hangs on the macOS
        // Tahoe desktop-settings open(). Forced, not passed through: set the
        // parent to a hostile value and prove the spawned workload still sees
        // false. Runs a real subprocess through execute() (clear_env=true).
        let _guard = ENV_LOCK.lock().unwrap();
        std::env::set_var("OP_LOAD_DESKTOP_APP_SETTINGS", "true");

        let tmp = tempfile::tempdir().unwrap();
        let mut session = LocalSubstrate.acquire("local", tmp.path()).unwrap();
        let result = session
            .execute(
                &[
                    "sh".to_string(),
                    "-c".to_string(),
                    "printf '%s' \"$OP_LOAD_DESKTOP_APP_SETTINGS\"".to_string(),
                ],
                None,
                Duration::from_secs(5),
                None,
            )
            .unwrap();

        std::env::remove_var("OP_LOAD_DESKTOP_APP_SETTINGS");
        assert_eq!(result.stdout, "false");
    }
}
