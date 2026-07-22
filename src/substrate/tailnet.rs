//! bitterblossom-938: dispatch to an arbitrary machine reachable over the
//! operator's Tailscale network -- same shape as `sprites.rs` (remote exec,
//! workspace dir, marker/pidfile liveness), but the transport is plain `ssh`
//! instead of the Fly-specific `sprite` CLI, and there is no checkpoint/
//! restore concept (tailnet hosts are long-lived machines, not disposable
//! sprites). `host` is the Tailscale MagicDNS name (or any address `ssh`
//! accepts), matching the same `WorkspaceSpec.host` field sprites already use.
use std::path::{Path, PathBuf};
use std::time::Duration;

use anyhow::{bail, Result};

use super::local::{git_auth_setup_script, run_with_timeout, shell_quote, RunControl};
use super::{
    ExecMonitor, ExecResult, ProbeResult, Session, Substrate, WorkspacePlan, CARD_FILENAME,
};

pub const SSH_BIN_ENV: &str = "BB_SSH_BIN";
pub const SSH_CONNECT_TIMEOUT_ENV: &str = "BB_SSH_CONNECT_TIMEOUT_SECONDS";

fn ssh_bin() -> String {
    std::env::var(SSH_BIN_ENV).unwrap_or_else(|_| "ssh".to_string())
}

fn ssh_connect_timeout() -> String {
    std::env::var(SSH_CONNECT_TIMEOUT_ENV).unwrap_or_else(|_| "10".to_string())
}

fn relay_cwd() -> PathBuf {
    std::env::var_os("HOME").map_or_else(std::env::temp_dir, PathBuf::from)
}

fn ssh_exec(
    host: &str,
    remote: &[String],
    stdin: Option<&str>,
    timeout: Duration,
    monitor: Option<&mut ExecMonitor<'_>>,
) -> Result<ExecResult> {
    let mut cmd = vec![
        ssh_bin(),
        "-o".into(),
        "BatchMode=yes".into(),
        "-o".into(),
        format!("ConnectTimeout={}", ssh_connect_timeout()),
        host.to_string(),
        "--".into(),
    ];
    // OpenSSH joins every argument after the destination into one command
    // string for the remote login shell. Quote each intended argv element so
    // scripts, redirects, and whitespace survive that lossy boundary.
    cmd.extend(remote.iter().map(|arg| shell_quote(arg)));
    run_with_timeout(
        &cmd,
        stdin,
        &relay_cwd(),
        &[],
        false,
        timeout,
        RunControl {
            pidfile: None,
            monitor,
        },
    )
}

fn remote_shell(host: &str, script: &str, timeout: Duration) -> Result<ExecResult> {
    ssh_exec(
        host,
        &["sh".into(), "-c".into(), script.into()],
        None,
        timeout,
        None,
    )
}

pub struct TailnetSubstrate;

impl Substrate for TailnetSubstrate {
    fn acquire(&self, host: &str, attempt_dir: &Path) -> Result<Box<dyn Session>> {
        std::fs::create_dir_all(attempt_dir)?;
        let out = remote_shell(host, "true", Duration::from_secs(30));
        let out = match out {
            Ok(out) => out,
            Err(e) => bail!("tailnet host '{host}' unreachable: {e:#}"),
        };
        if out.exit_code != 0 {
            bail!(
                "tailnet host '{host}' unreachable: {}",
                out.stderr.trim().lines().last().unwrap_or("ssh failed")
            );
        }
        Ok(Box::new(TailnetSession {
            host: host.to_string(),
            artifacts: attempt_dir.to_path_buf(),
            remote_home: String::new(),
            workspace: String::new(),
            marker: String::new(),
            secrets: Vec::new(),
            hermetic: false,
            release_artifacts: Vec::new(),
        }))
    }

    fn probe(&self, host: &str, _attempt_dir: &Path, marker: &str) -> ProbeResult {
        // Unquoted $HOME interpolation (never single-quoted like a path
        // literal) so it expands remotely regardless of the connecting
        // account's home directory.
        let script = format!(
            "raw=\"$(cat \"$HOME/.bb-tailnet/{marker}.pid\" 2>/dev/null)\" || exit 3; \
             pid=\"${{raw%%|*}}\"; expected=\"${{raw#*|}}\"; \
             case \"$pid\" in ''|*[!0-9]*) exit 5;; esac; \
             [ \"$pid\" -gt 0 ] 2>/dev/null || exit 5; \
             kill -0 \"$pid\" 2>/dev/null || exit 4; \
             if [ \"$expected\" = \"$raw\" ] || [ -z \"$expected\" ]; then exit 0; fi; \
             actual=\"$(ps -p \"$pid\" -o lstart= 2>/dev/null | sed 's/^ *//;s/ *$//')\"; \
             [ \"$actual\" = \"$expected\" ] && exit 0 || exit 6"
        );
        match remote_shell(host, &script, Duration::from_secs(30)) {
            Ok(out) => match out.exit_code {
                0 => ProbeResult::Alive,
                4 => ProbeResult::Dead,
                3 => ProbeResult::Unknown(format!(
                    "no pidfile $HOME/.bb-tailnet/{marker}.pid on {host}"
                )),
                5 => ProbeResult::Unknown(format!(
                    "malformed pidfile $HOME/.bb-tailnet/{marker}.pid on {host}"
                )),
                6 => ProbeResult::Unknown(format!(
                    "pid identity does not match marker $HOME/.bb-tailnet/{marker}.pid on {host}"
                )),
                code => ProbeResult::Unknown(format!(
                    "probe exit {code}: {}",
                    out.stderr.trim().lines().last().unwrap_or("")
                )),
            },
            Err(e) => ProbeResult::Unknown(format!("probe failed: {e:#}")),
        }
    }
}

struct TailnetSession {
    host: String,
    artifacts: PathBuf,
    /// Resolved once in `prepare()` via `printf %s "$HOME"` -- an absolute
    /// path, so every later `shell_quote(...)` on a workspace path is safe
    /// (single quotes suppress `~`/`$HOME` expansion, so those must never
    /// appear inside a quoted argument; resolving once up front avoids that
    /// trap entirely instead of threading unquoted interpolation everywhere).
    remote_home: String,
    workspace: String,
    marker: String,
    secrets: Vec<(String, String)>,
    hermetic: bool,
    release_artifacts: Vec<String>,
}

impl TailnetSession {
    /// Writes `content` to `<workspace>/<name>` on the remote host by piping
    /// it as stdin to a remote `cat > file` -- no scp/rsync dependency, same
    /// "outbound follows a plain exec" convention as `git_auth_setup_script`.
    fn upload(&self, name: &str, content: &str) -> Result<()> {
        let dest = shell_quote(&format!("{}/{name}", self.workspace));
        let out = ssh_exec(
            &self.host,
            &["sh".into(), "-c".into(), format!("cat > {dest}")],
            Some(content),
            Duration::from_secs(60),
            None,
        )?;
        if out.exit_code != 0 {
            bail!("{name} upload failed: {}", out.stderr.trim());
        }
        Ok(())
    }
}

impl Session for TailnetSession {
    fn prepare(&mut self, plan: &WorkspacePlan) -> Result<()> {
        let home = remote_shell(&self.host, "printf %s \"$HOME\"", Duration::from_secs(30))?;
        if home.exit_code != 0 || home.stdout.trim().is_empty() {
            bail!(
                "resolve remote $HOME on '{}' failed: {}",
                self.host,
                home.stderr.trim()
            );
        }
        self.remote_home = home.stdout.trim().to_string();
        self.workspace = format!(
            "{}/.bb-tailnet/{}-{}",
            self.remote_home, plan.workspace_name, plan.marker
        );
        self.marker = plan.marker.clone();
        self.hermetic = plan.hermetic;
        self.release_artifacts = plan.artifacts.clone();

        let out = remote_shell(
            &self.host,
            &super::remote_create_workspace_script(&self.workspace),
            Duration::from_secs(60),
        )?;
        if out.exit_code != 0 {
            bail!("prepare workspace failed: {}", out.stderr.trim());
        }

        for repo in &plan.repos {
            let dir = repo_dir_name(&repo.url);
            let dest = format!("{}/{dir}", self.workspace);
            let transport: String = plan
                .checkout_secrets
                .iter()
                .filter(|(name, _)| name == "GH_TOKEN")
                .map(|(name, value)| format!("export {name}={}\n", shell_quote(value)))
                .collect();
            let script = format!(
                "{transport}{git_auth}\n{materialize}",
                git_auth = git_auth_setup_script(),
                materialize = super::repo_materialize_script(repo, &dest),
            );
            // The checkout credential is carried only in SSH stdin. Putting
            // this script in the remote-command argv exposes the token to the
            // local process table and transport telemetry.
            let out = ssh_exec(
                &self.host,
                &["sh".into()],
                Some(&script),
                Duration::from_secs(600),
                None,
            )?;
            if out.exit_code != 0 {
                bail!("repo sync {} failed: {}", repo.url, out.stderr.trim());
            }
        }
        self.secrets = plan.secrets.clone();

        self.upload(CARD_FILENAME, &plan.card)?;
        for (name, content) in [
            ("RUN.json", Some(&plan.run_context)),
            (super::EVENT_FILENAME, plan.payload.as_ref()),
            (super::REPORT_FILENAME, plan.report.as_ref()),
        ] {
            if let Some(content) = content {
                self.upload(name, content)?;
            }
        }

        if let Some(pre) = &plan.pre_command {
            let out = self.execute(
                &["sh".into(), "-c".into(), pre.clone()],
                None,
                Duration::from_secs(600),
                None,
            )?;
            if out.exit_code != 0 {
                bail!("pre_command failed: {}", out.stderr.trim());
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
        let exports: String = self
            .secrets
            .iter()
            .map(|(k, v)| format!("export {k}={}\n", shell_quote(v)))
            .collect();
        let quoted: Vec<String> = cmd.iter().map(|c| shell_quote(c)).collect();
        let delimiter = loop {
            let candidate = format!("BB_STDIN_EOF_{}", crate::ledger::new_id());
            let collides = stdin
                .map(|input| input.lines().any(|l| l.trim() == candidate))
                .unwrap_or(false);
            if !collides {
                break candidate;
            }
        };
        let body = match stdin {
            Some(input) => format!(
                "exec {cmd} <<'{delimiter}'\n{input}\n{delimiter}\n",
                cmd = quoted.join(" "),
            ),
            None => format!("exec {} < /dev/null\n", quoted.join(" ")),
        };
        let scrub = if self.hermetic {
            "for v in $(env | cut -d= -f1); do case \"$v\" in \
             PATH|TERM|LANG|LC_ALL|PWD|SHLVL|TMPDIR) ;; *) unset \"$v\" 2>/dev/null;; esac; done\n\
             mkdir -p .home && export HOME=\"$PWD/.home\"\n"
        } else {
            ""
        };
        let script = format!(
            "cd {ws} || exit 1\nmkdir -p ~/.bb-tailnet\necho \"$$|$(ps -p $$ -o lstart= | sed 's/^ *//;s/ *$//')\" > ~/.bb-tailnet/{marker}.pid\nunset GH_TOKEN\n{scrub}{exports}{body}",
            ws = shell_quote(&self.workspace),
            marker = self.marker,
        );
        let result = ssh_exec(
            &self.host,
            &["setsid".into(), "-w".into(), "sh".into()],
            Some(&script),
            timeout,
            monitor,
        )?;
        if result.timed_out || result.termination_reason.is_some() {
            let kill = format!(
                "raw=\"$(cat ~/.bb-tailnet/{}.pid 2>/dev/null)\"; pid=\"${{raw%%|*}}\"; expected=\"${{raw#*|}}\"; case \"$pid\" in \"\"|*[!0-9]*) exit 0;; esac; [ \"$pid\" -gt 0 ] 2>/dev/null || exit 0; if [ \"$expected\" != \"$raw\" ] && [ -n \"$expected\" ]; then actual=\"$(ps -p \"$pid\" -o lstart= 2>/dev/null | sed 's/^ *//;s/ *$//')\"; [ \"$actual\" = \"$expected\" ] || exit 0; fi; kill -9 -- \"-$pid\" 2>/dev/null || true",
                self.marker
            );
            let _ = remote_shell(&self.host, &kill, Duration::from_secs(30));
        }
        Ok(result)
    }

    fn write_artifact(&mut self, name: &str, data: &[u8]) -> Result<()> {
        super::write_relative_nofollow(&self.artifacts, name, data)
    }

    fn release(&mut self) -> Result<()> {
        for name in self.release_artifacts.clone() {
            let script = super::remote_collect_script(&self.workspace, &name);
            let out = remote_shell(&self.host, &script, Duration::from_secs(120))?;
            match out.exit_code {
                0 => {
                    let bytes = super::decode_hex_artifact(&name, &out.stdout)?;
                    self.write_artifact(&name, &bytes)?;
                }
                42 => {}
                43 => bail!("released artifact {name} crossed a symlink or non-file"),
                44 => bail!(
                    "released artifact {name} exceeds {} bytes",
                    super::RELEASE_ARTIFACT_LIMIT
                ),
                _ => bail!("read artifact {name} failed: {}", out.stderr.trim()),
            }
        }
        if !self.marker.is_empty() {
            let _ = remote_shell(
                &self.host,
                &format!("rm -f ~/.bb-tailnet/{}.pid", self.marker),
                Duration::from_secs(30),
            );
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
