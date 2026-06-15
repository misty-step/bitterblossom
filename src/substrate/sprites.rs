use std::path::{Path, PathBuf};
use std::time::Duration;

use anyhow::{bail, Context, Result};

use super::local::{run_with_timeout, shell_quote};
use super::{ExecResult, ProbeResult, Session, Substrate, WorkspacePlan, CARD_FILENAME};
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
    std::env::var_os("HOME")
        .map(PathBuf::from)
        .unwrap_or_else(std::env::temp_dir)
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
        };
        let out = session.sprite_exec(&["true".into()], None, Duration::from_secs(120))?;
        if out.exit_code != 0 {
            bail!(
                "sprite '{host}' unreachable: {}",
                out.stderr.trim().lines().last().unwrap_or("")
            );
        }
        Ok(Box::new(session))
    }

    fn probe(&self, host: &str, _attempt_dir: &Path, marker: &str) -> ProbeResult {
        let script = format!("pid=\"$(cat /tmp/{marker}.pid 2>/dev/null)\" || exit 3; case \"$pid\" in ''|*[!0-9]*) exit 5;; esac; kill -0 \"$pid\" 2>/dev/null && exit 0 || exit 4");
        let mut cmd = vec![sprite_bin(), "exec".into()];
        cmd.extend(selector_args(host));
        cmd.extend(["--".into(), "sh".into(), "-c".into(), script]);
        match run_with_timeout(
            &cmd,
            None,
            &relay_cwd(),
            &[],
            false,
            None,
            Duration::from_secs(60),
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

pub struct SpriteSession {
    host: String,
    artifacts: PathBuf,
    workspace: String,
    marker: String,
    secrets: Vec<(String, String)>,
    hermetic: bool,
}

impl SpriteSession {
    fn sprite_exec_with(
        &self,
        extra_args: &[String],
        remote: &[String],
        stdin: Option<&str>,
        timeout: Duration,
    ) -> Result<ExecResult> {
        let mut cmd = vec![sprite_bin(), "exec".into()];
        cmd.extend(selector_args(&self.host));
        cmd.extend(extra_args.iter().cloned());
        cmd.push("--".into());
        cmd.extend(remote.iter().cloned());
        run_with_timeout(&cmd, stdin, &relay_cwd(), &[], false, None, timeout)
    }

    fn sprite_exec(
        &self,
        remote: &[String],
        stdin: Option<&str>,
        timeout: Duration,
    ) -> Result<ExecResult> {
        self.sprite_exec_with(&[], remote, stdin, timeout)
    }

    fn remote_shell(&self, script: &str, timeout: Duration) -> Result<ExecResult> {
        self.sprite_exec(&["sh".into(), "-c".into(), script.into()], None, timeout)
    }
}

impl Session for SpriteSession {
    fn prepare(&mut self, plan: &WorkspacePlan) -> Result<()> {
        self.workspace = remote_workspace_path(&plan.workspace_name);
        self.marker = plan.marker.clone();
        self.secrets = plan.secrets.clone();
        self.hermetic = plan.hermetic;

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
                None,
                Duration::from_secs(300),
            )?;
            if out.exit_code != 0 {
                bail!(
                    "checkpoint restore {checkpoint} failed: {}",
                    out.stderr.trim()
                );
            }
        }

        let ws = shell_quote(&self.workspace);
        let cleanup = format!("mkdir -p {ws} && rm -f {ws}/EVENT.json {ws}/REPORT.json");
        let out = self.remote_shell(&cleanup, Duration::from_secs(60))?;
        if out.exit_code != 0 {
            bail!("prepare workspace failed: {}", out.stderr.trim());
        }

        for repo in &plan.repos {
            let dir = repo_dir_name(&repo.url);
            let dest = shell_quote(&format!("{}/{dir}", self.workspace));
            let url = shell_quote(&repo.url);
            let r#ref = shell_quote(&repo.r#ref);
            let script = format!(
                "if [ -d {dest}/.git ]; then \
                   git -C {dest} fetch origin {ref_} --depth 1 && \
                   git -C {dest} checkout -q FETCH_HEAD && \
                   git -C {dest} reset --hard && git -C {dest} clean -fd; \
                 else \
                   git clone --depth 1 --branch {ref_} {url} {dest}; \
                 fi",
                dest = dest,
                url = url,
                ref_ = r#ref,
            );
            let out = self.remote_shell(&script, Duration::from_secs(600))?;
            if out.exit_code != 0 {
                bail!("repo sync {} failed: {}", repo.url, out.stderr.trim());
            }
        }
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
        )?;
        if out.exit_code != 0 {
            bail!("card upload failed: {}", out.stderr.trim());
        }

        for (name, content) in [
            (super::EVENT_FILENAME, &plan.payload),
            (super::REPORT_FILENAME, &plan.report),
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
            "cd {ws} || exit 1\necho $$ > /tmp/{marker}.pid\n{scrub}{exports}{body}",
            ws = shell_quote(&self.workspace),
            marker = self.marker,
        );
        let result = self.sprite_exec(
            &["setsid".into(), "-w".into(), "sh".into()],
            Some(&script),
            timeout,
        )?;
        if result.timed_out {
            let kill = format!(
                "pid=\"$(cat /tmp/{}.pid 2>/dev/null)\" && kill -9 -- \"-$pid\" 2>/dev/null; true",
                self.marker
            );
            let _ = self.remote_shell(&kill, Duration::from_secs(30));
        }
        Ok(result)
    }

    fn write_artifact(&mut self, name: &str, data: &[u8]) -> Result<()> {
        let path = self.artifacts.join(name);
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent).context("artifact dir")?;
        }
        std::fs::write(path, data)?;
        Ok(())
    }

    fn release(&mut self) -> Result<()> {
        let path = shell_quote(&format!("{}/{}", self.workspace, super::REPORT_FILENAME));
        let out = self.remote_shell(
            &format!("if [ -f {path} ]; then cat {path}; else exit 42; fi"),
            Duration::from_secs(120),
        )?;
        match out.exit_code {
            0 => self.write_artifact(super::REPORT_FILENAME, out.stdout.as_bytes())?,
            42 => {}
            _ => bail!("read artifact REPORT.json failed: {}", out.stderr.trim()),
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
