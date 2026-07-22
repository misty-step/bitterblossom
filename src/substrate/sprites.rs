use std::path::{Path, PathBuf};
use std::time::Duration;

use anyhow::{bail, Result};

use super::local::{git_auth_setup_script, run_with_timeout, shell_quote, RunControl};
use super::{
    ExecMonitor, ExecResult, ProbeResult, Session, Substrate, WorkspacePlan, CARD_FILENAME,
};
pub const SPRITE_BIN_ENV: &str = "BB_SPRITE_BIN";

fn sprite_bin() -> String {
    std::env::var(SPRITE_BIN_ENV).unwrap_or_else(|_| "sprite".to_string())
}
fn selector_args(host: &str) -> Vec<String> {
    match host.split_once('/') {
        Some((org, name)) => vec!["-o".into(), org.into(), "-s".into(), name.into()],
        None => vec!["-s".into(), host.into()],
    }
}
fn relay_cwd() -> PathBuf {
    std::env::var_os("HOME").map_or_else(std::env::temp_dir, PathBuf::from)
}

pub struct SpritesSubstrate;

impl Substrate for SpritesSubstrate {
    fn acquire(&self, host: &str, attempt_dir: &Path) -> Result<Box<dyn Session>> {
        std::fs::create_dir_all(attempt_dir)?;
        let session = SpriteSession {
            host: host.to_string(),
            artifacts: attempt_dir.to_path_buf(),
            workspace: String::new(),
            marker: String::new(),
            secrets: Vec::new(),
            hermetic: false,
            release_artifacts: Vec::new(),
        };
        let out = session.sprite_exec(&["true".into()], None, Duration::from_secs(120), None)?;
        if out.exit_code != 0 {
            bail!(
                "sprite '{host}' unreachable: {}",
                out.stderr.trim().lines().last().unwrap_or("")
            );
        }
        Ok(Box::new(session))
    }

    fn probe(&self, host: &str, _attempt_dir: &Path, marker: &str) -> ProbeResult {
        let script = format!("raw=\"$(cat /tmp/{marker}.pid 2>/dev/null)\" || exit 3; pid=\"${{raw%%|*}}\"; expected=\"${{raw#*|}}\"; case \"$pid\" in ''|*[!0-9]*) exit 5;; esac; [ \"$pid\" -gt 0 ] 2>/dev/null || exit 5; kill -0 \"$pid\" 2>/dev/null || exit 4; if [ \"$expected\" = \"$raw\" ] || [ -z \"$expected\" ]; then exit 0; fi; actual=\"$(ps -p \"$pid\" -o lstart= 2>/dev/null | sed 's/^ *//;s/ *$//')\"; [ \"$actual\" = \"$expected\" ] && exit 0 || exit 6");
        let mut cmd = vec![sprite_bin(), "exec".into()];
        cmd.extend(selector_args(host));
        cmd.extend(["--".into(), "sh".into(), "-c".into(), script]);
        match run_with_timeout(
            &cmd,
            None,
            &relay_cwd(),
            &[],
            false,
            Duration::from_secs(60),
            RunControl::default(),
        ) {
            Ok(out) => match out.exit_code {
                0 => ProbeResult::Alive,
                4 => ProbeResult::Dead,
                3 => ProbeResult::Unknown(format!("no pidfile /tmp/{marker}.pid on {host}")),
                5 => ProbeResult::Unknown(format!("malformed pidfile /tmp/{marker}.pid on {host}")),
                code => ProbeResult::Unknown(format!(
                    "probe exit {code}: {}",
                    out.stderr.trim().lines().last().unwrap_or("")
                )),
            },
            Err(e) => ProbeResult::Unknown(format!("probe failed: {e:#}")),
        }
    }
}

/// Check whether a command-harness binary resolves on the sprite host without
/// preparing or mutating a workspace. Bare names use the remote PATH. Relative
/// paths with separators are checked from the same task workspace directory
/// that dispatch uses, if it already exists.
pub(crate) fn remote_command_unspawnable_detail(
    host: &str,
    workspace_name: &str,
    bin: &str,
) -> Option<String> {
    let workspace = remote_workspace_path(workspace_name);
    let script = remote_command_probe_script(&workspace, bin);
    let mut cmd = vec![sprite_bin(), "exec".into()];
    cmd.extend(selector_args(host));
    cmd.extend(["--".into(), "sh".into(), "-c".into(), script]);
    match run_with_timeout(
        &cmd,
        None,
        &relay_cwd(),
        &[],
        false,
        Duration::from_secs(60),
        RunControl::default(),
    ) {
        Ok(out) if out.exit_code == 0 => None,
        Ok(out) => {
            let detail = out.stderr.trim().lines().last().unwrap_or("");
            Some(format!(
                "command harness bin '{bin}' is not executable on sprite host '{host}' from workspace '{workspace}': {detail}"
            ))
        }
        Err(e) => Some(format!(
            "command harness bin '{bin}' could not be checked on sprite host '{host}' from workspace '{workspace}': {e:#}"
        )),
    }
}

fn remote_command_probe_script(workspace: &str, bin: &str) -> String {
    let workspace = shell_quote(workspace);
    let bin = shell_quote(bin);
    format!(
        "workspace={workspace}\n\
         bin={bin}\n\
         case \"$bin\" in\n\
           /*)\n\
             [ -x \"$bin\" ] || {{ echo \"absolute command '$bin' is not executable\" >&2; exit 127; }} ;;\n\
           */*)\n\
             [ -d \"$workspace\" ] || {{ echo \"workspace '$workspace' is absent; cannot verify relative command '$bin'\" >&2; exit 127; }}\n\
             cd \"$workspace\" || exit 126\n\
             [ -x \"$bin\" ] || {{ echo \"workspace-relative command '$bin' is not executable\" >&2; exit 127; }} ;;\n\
           *)\n\
             command -v \"$bin\" >/dev/null 2>&1 || {{ echo \"command '$bin' not found on PATH\" >&2; exit 127; }} ;;\n\
         esac"
    )
}

pub struct SpriteSession {
    host: String,
    artifacts: PathBuf,
    workspace: String,
    marker: String,
    secrets: Vec<(String, String)>,
    hermetic: bool,
    release_artifacts: Vec<String>,
}

impl SpriteSession {
    fn sprite_exec_with(
        &self,
        extra_args: &[String],
        remote: &[String],
        stdin: Option<&str>,
        timeout: Duration,
        monitor: Option<&mut ExecMonitor<'_>>,
    ) -> Result<ExecResult> {
        let mut cmd = vec![sprite_bin(), "exec".into()];
        cmd.extend(selector_args(&self.host));
        cmd.extend(extra_args.iter().cloned());
        cmd.push("--".into());
        cmd.extend(remote.iter().cloned());
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

    fn sprite_exec(
        &self,
        remote: &[String],
        stdin: Option<&str>,
        timeout: Duration,
        monitor: Option<&mut ExecMonitor<'_>>,
    ) -> Result<ExecResult> {
        self.sprite_exec_with(&[], remote, stdin, timeout, monitor)
    }

    fn remote_shell(&self, script: &str, timeout: Duration) -> Result<ExecResult> {
        self.sprite_exec(
            &["sh".into(), "-c".into(), script.into()],
            None,
            timeout,
            None,
        )
    }

    fn remote_shell_with_secrets(
        &self,
        script: &str,
        secrets: &[(String, String)],
        timeout: Duration,
    ) -> Result<ExecResult> {
        let exports: String = secrets
            .iter()
            .map(|(k, v)| format!("export {k}={}\n", shell_quote(v)))
            .collect();
        self.sprite_exec(
            &["setsid".into(), "-w".into(), "sh".into()],
            Some(&format!("{exports}{script}")),
            timeout,
            None,
        )
    }
}

impl Session for SpriteSession {
    fn prepare(&mut self, plan: &WorkspacePlan) -> Result<()> {
        self.workspace = remote_workspace_path(&format!("{}-{}", plan.workspace_name, plan.marker));
        self.marker = plan.marker.clone();
        self.hermetic = plan.hermetic;
        self.release_artifacts = plan.artifacts.clone();

        if let Some(checkpoint) = &plan.checkpoint {
            let mut cmd = vec![sprite_bin(), "restore".into()];
            cmd.extend(selector_args(&self.host));
            cmd.push(checkpoint.clone());
            let out = run_with_timeout(
                &cmd,
                None,
                &relay_cwd(),
                &[],
                false,
                Duration::from_secs(300),
                RunControl::default(),
            )?;
            if out.exit_code != 0 {
                bail!(
                    "checkpoint restore {checkpoint} failed: {}",
                    out.stderr.trim()
                );
            }
        }

        let out = self.remote_shell(
            &super::remote_create_workspace_script(&self.workspace),
            Duration::from_secs(60),
        )?;
        if out.exit_code != 0 {
            bail!("prepare workspace failed: {}", out.stderr.trim());
        }

        for repo in &plan.repos {
            let dir = repo_dir_name(&repo.url);
            let dest = format!("{}/{dir}", self.workspace);
            let script = format!(
                "{git_auth}\n{materialize}",
                git_auth = git_auth_setup_script(),
                materialize = super::repo_materialize_script(repo, &dest),
            );
            let out = self.remote_shell_with_secrets(
                &script,
                &plan.checkout_secrets,
                Duration::from_secs(600),
            )?;
            if out.exit_code != 0 {
                bail!("repo sync {} failed: {}", repo.url, out.stderr.trim());
            }
        }
        self.secrets = plan.secrets.clone();
        let card_local = self.artifacts.join(CARD_FILENAME);
        std::fs::write(&card_local, &plan.card)?;
        let upload = format!(
            "{}:{}/{CARD_FILENAME}",
            card_local.display(),
            self.workspace
        );
        let out = self.sprite_exec_with(
            &["--file".into(), upload],
            &["true".into()],
            None,
            Duration::from_secs(120),
            None,
        )?;
        if out.exit_code != 0 {
            bail!("card upload failed: {}", out.stderr.trim());
        }

        for (name, content) in [
            ("RUN.json", Some(&plan.run_context)),
            (super::EVENT_FILENAME, plan.payload.as_ref()),
            (super::REPORT_FILENAME, plan.report.as_ref()),
        ] {
            let Some(content) = content else { continue };
            let local = self.artifacts.join(name);
            std::fs::write(&local, content)?;
            let upload = format!("{}:{}/{}", local.display(), self.workspace, name);
            let out = self.sprite_exec_with(
                &["--file".into(), upload],
                &["true".into()],
                None,
                Duration::from_secs(120),
                None,
            )?;
            if out.exit_code != 0 {
                bail!("{name} upload failed: {}", out.stderr.trim());
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
            "cd {ws} || exit 1\necho \"$$|$(ps -p $$ -o lstart= | sed 's/^ *//;s/ *$//')\" > /tmp/{marker}.pid\nunset GH_TOKEN\n{scrub}{exports}{body}",
            ws = shell_quote(&self.workspace),
            marker = self.marker,
        );
        let result = self.sprite_exec(
            &["setsid".into(), "-w".into(), "sh".into()],
            Some(&script),
            timeout,
            monitor,
        )?;
        if result.timed_out || result.termination_reason.is_some() {
            let kill = format!(
                "raw=\"$(cat /tmp/{}.pid 2>/dev/null)\"; pid=\"${{raw%%|*}}\"; expected=\"${{raw#*|}}\"; case \"$pid\" in \"\"|*[!0-9]*) exit 0;; esac; [ \"$pid\" -gt 0 ] 2>/dev/null || exit 0; if [ \"$expected\" != \"$raw\" ] && [ -n \"$expected\" ]; then actual=\"$(ps -p \"$pid\" -o lstart= 2>/dev/null | sed 's/^ *//;s/ *$//')\"; [ \"$actual\" = \"$expected\" ] || exit 0; fi; kill -9 -- \"-$pid\" 2>/dev/null || true",
                self.marker
            );
            let _ = self.remote_shell(&kill, Duration::from_secs(30));
        }
        Ok(result)
    }

    fn write_artifact(&mut self, name: &str, data: &[u8]) -> Result<()> {
        super::write_relative_nofollow(&self.artifacts, name, data)
    }

    fn release(&mut self) -> Result<()> {
        for name in self.release_artifacts.clone() {
            let script = super::remote_collect_script(&self.workspace, &name);
            let out = self.remote_shell(&script, Duration::from_secs(120))?;
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
            let _ = self.remote_shell(
                &format!("rm -f /tmp/{}.pid", self.marker),
                Duration::from_secs(30),
            );
        }
        Ok(())
    }
}

fn remote_workspace_path(name: &str) -> String {
    format!("/home/sprite/bb/{name}")
}

fn repo_dir_name(url: &str) -> String {
    url.trim_end_matches('/')
        .trim_end_matches(".git")
        .rsplit('/')
        .next()
        .unwrap_or("repo")
        .to_string()
}
