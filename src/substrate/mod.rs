pub mod local;
pub mod sprites;
pub mod tailnet;

use std::ffi::CString;
use std::fs::File;
use std::io::{Read, Write};
use std::os::fd::{AsRawFd, FromRawFd, OwnedFd};
use std::os::unix::ffi::OsStrExt;
use std::path::{Component, Path};
use std::time::Duration;

use anyhow::{Context, Result};

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
    /// Attempt-scoped repository transport credentials. Substrates may expose
    /// these only to checkout setup, never to pre-command or workload exec.
    pub checkout_secrets: Vec<(String, String)>,
    pub hermetic: bool,
    /// Workspace-relative evidence paths to collect on release. These are
    /// validated as safe relative paths while loading the task declaration.
    pub artifacts: Vec<String>,
}

#[derive(Debug)]
pub struct NoWorkloadStarted(pub String);

impl std::fmt::Display for NoWorkloadStarted {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str(&self.0)
    }
}

impl std::error::Error for NoWorkloadStarted {}

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

/// Receipts are control-plane evidence, not an unbounded file-transfer
/// channel. The same one-MiB ceiling protects local and remote collectors.
pub(crate) const RELEASE_ARTIFACT_LIMIT: u64 = 1 << 20;

fn cstring(value: &[u8]) -> Result<CString> {
    CString::new(value).map_err(|_| anyhow::anyhow!("artifact path contains NUL"))
}

fn open_dir_at(parent: i32, name: &[u8]) -> std::io::Result<OwnedFd> {
    let name = CString::new(name)
        .map_err(|_| std::io::Error::new(std::io::ErrorKind::InvalidInput, "path contains NUL"))?;
    let fd = unsafe {
        libc::openat(
            parent,
            name.as_ptr(),
            libc::O_RDONLY | libc::O_DIRECTORY | libc::O_NOFOLLOW | libc::O_CLOEXEC,
        )
    };
    if fd < 0 {
        Err(std::io::Error::last_os_error())
    } else {
        Ok(unsafe { OwnedFd::from_raw_fd(fd) })
    }
}

fn open_root(root: &Path) -> Result<OwnedFd> {
    let root = cstring(root.as_os_str().as_bytes())?;
    let fd = unsafe {
        libc::open(
            root.as_ptr(),
            libc::O_RDONLY | libc::O_DIRECTORY | libc::O_NOFOLLOW | libc::O_CLOEXEC,
        )
    };
    if fd < 0 {
        Err(std::io::Error::last_os_error()).context("open artifact root without symlinks")
    } else {
        Ok(unsafe { OwnedFd::from_raw_fd(fd) })
    }
}

pub(crate) fn open_relative_nofollow(root: &Path, relative: &str) -> Result<Option<File>> {
    let components = Path::new(relative)
        .components()
        .map(|component| match component {
            Component::Normal(name) => Ok(name.as_bytes().to_vec()),
            _ => anyhow::bail!("artifact path is not a safe relative path: {relative:?}"),
        })
        .collect::<Result<Vec<_>>>()?;
    let (leaf, parents) = components
        .split_last()
        .ok_or_else(|| anyhow::anyhow!("artifact path is empty"))?;
    let root = open_root(root)?;
    let mut current = root;
    for parent in parents {
        match open_dir_at(current.as_raw_fd(), parent) {
            Ok(next) => current = next,
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(None),
            Err(error) => return Err(error).context("open artifact parent without symlinks"),
        }
    }
    let leaf = cstring(leaf)?;
    let fd = unsafe {
        libc::openat(
            current.as_raw_fd(),
            leaf.as_ptr(),
            libc::O_RDONLY | libc::O_NOFOLLOW | libc::O_CLOEXEC,
        )
    };
    if fd < 0 {
        let error = std::io::Error::last_os_error();
        if error.kind() == std::io::ErrorKind::NotFound {
            Ok(None)
        } else {
            Err(error).context("open released artifact without symlinks")
        }
    } else {
        Ok(Some(unsafe { File::from_raw_fd(fd) }))
    }
}

pub(crate) fn read_relative_nofollow(root: &Path, relative: &str) -> Result<Option<Vec<u8>>> {
    let Some(file) = open_relative_nofollow(root, relative)? else {
        return Ok(None);
    };
    let metadata = file.metadata()?;
    if !metadata.is_file() {
        anyhow::bail!("released artifact {relative} is not a regular file");
    }
    if metadata.len() > RELEASE_ARTIFACT_LIMIT {
        anyhow::bail!(
            "released artifact {relative} exceeds {} bytes",
            RELEASE_ARTIFACT_LIMIT
        );
    }
    let mut bytes = Vec::with_capacity(metadata.len() as usize);
    file.take(RELEASE_ARTIFACT_LIMIT + 1)
        .read_to_end(&mut bytes)?;
    if bytes.len() as u64 > RELEASE_ARTIFACT_LIMIT {
        anyhow::bail!(
            "released artifact {relative} grew beyond {} bytes while reading",
            RELEASE_ARTIFACT_LIMIT
        );
    }
    Ok(Some(bytes))
}

pub(crate) fn write_relative_nofollow(root: &Path, relative: &str, data: &[u8]) -> Result<()> {
    if data.len() as u64 > RELEASE_ARTIFACT_LIMIT {
        anyhow::bail!(
            "artifact {relative} exceeds {} bytes",
            RELEASE_ARTIFACT_LIMIT
        );
    }
    let components = Path::new(relative)
        .components()
        .map(|component| match component {
            Component::Normal(name) => Ok(name.as_bytes().to_vec()),
            _ => anyhow::bail!("artifact path is not a safe relative path: {relative:?}"),
        })
        .collect::<Result<Vec<_>>>()?;
    let (leaf, parents) = components
        .split_last()
        .ok_or_else(|| anyhow::anyhow!("artifact path is empty"))?;
    let root = open_root(root)?;
    let mut current = root;
    for parent in parents {
        let next = match open_dir_at(current.as_raw_fd(), parent) {
            Ok(next) => next,
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
                let name = cstring(parent)?;
                let rc = unsafe { libc::mkdirat(current.as_raw_fd(), name.as_ptr(), 0o700) };
                if rc < 0
                    && std::io::Error::last_os_error().kind() != std::io::ErrorKind::AlreadyExists
                {
                    return Err(std::io::Error::last_os_error()).context("create artifact parent");
                }
                open_dir_at(current.as_raw_fd(), parent)
                    .context("open created artifact parent without symlinks")?
            }
            Err(error) => return Err(error).context("open artifact destination parent"),
        };
        current = next;
    }
    let leaf = cstring(leaf)?;
    let fd = unsafe {
        libc::openat(
            current.as_raw_fd(),
            leaf.as_ptr(),
            libc::O_WRONLY | libc::O_CREAT | libc::O_TRUNC | libc::O_NOFOLLOW | libc::O_CLOEXEC,
            0o600,
        )
    };
    if fd < 0 {
        return Err(std::io::Error::last_os_error())
            .context("open artifact destination without symlinks");
    }
    let mut file = unsafe { File::from_raw_fd(fd) };
    if !file.metadata()?.is_file() {
        anyhow::bail!("artifact destination {relative} is not a regular file");
    }
    file.write_all(data)?;
    Ok(())
}

pub(crate) fn decode_hex_artifact(name: &str, encoded: &str) -> Result<Vec<u8>> {
    let encoded = encoded.trim();
    if !encoded.len().is_multiple_of(2) || encoded.len() as u64 > RELEASE_ARTIFACT_LIMIT * 2 {
        anyhow::bail!("remote artifact {name} returned invalid or oversized hex");
    }
    (0..encoded.len())
        .step_by(2)
        .map(|index| {
            u8::from_str_radix(&encoded[index..index + 2], 16)
                .with_context(|| format!("remote artifact {name} returned invalid hex"))
        })
        .collect()
}

pub(crate) fn remote_collect_script(root: &str, relative: &str) -> String {
    let quote = crate::substrate::local::shell_quote;
    format!(
        r#"python3 - {root} {relative} {limit} <<'PY'
import ctypes, os, stat, sys
root, relative, limit = sys.argv[1], sys.argv[2], int(sys.argv[3])
flags = os.O_RDONLY | os.O_NOFOLLOW
libc = ctypes.CDLL(None, use_errno=True)
def open_at(parent, name, flags):
    value = libc.openat(parent, name.encode(), flags, 0)
    if value < 0:
        code = ctypes.get_errno()
        raise OSError(code, os.strerror(code), name)
    return value
fd = os.open(root, flags | os.O_DIRECTORY)
try:
    parts = relative.split('/')
    for part in parts[:-1]:
        nxt = open_at(fd, part, flags | os.O_DIRECTORY)
        os.close(fd)
        fd = nxt
    leaf = open_at(fd, parts[-1], flags)
except FileNotFoundError:
    sys.exit(42)
except OSError:
    sys.exit(43)
finally:
    os.close(fd)
st = os.fstat(leaf)
if not stat.S_ISREG(st.st_mode):
    os.close(leaf); sys.exit(43)
if st.st_size > limit:
    os.close(leaf); sys.exit(44)
data = bytearray()
while len(data) <= limit:
    chunk = os.read(leaf, min(65536, limit + 1 - len(data)))
    if not chunk: break
    data.extend(chunk)
os.close(leaf)
if len(data) > limit: sys.exit(44)
sys.stdout.write(data.hex())
PY"#,
        root = quote(root),
        relative = quote(relative),
        limit = RELEASE_ARTIFACT_LIMIT,
    )
}

pub(crate) fn remote_create_workspace_script(workspace: &str) -> String {
    let quote = crate::substrate::local::shell_quote;
    format!(
        r#"python3 - {workspace} <<'PY'
import ctypes, errno, os, sys
path = sys.argv[1]
if not path.startswith('/'):
    sys.exit(45)
flags = os.O_RDONLY | os.O_NOFOLLOW | os.O_DIRECTORY
libc = ctypes.CDLL(None, use_errno=True)
def open_at(parent, name):
    value = libc.openat(parent, name.encode(), flags, 0)
    if value < 0:
        code = ctypes.get_errno()
        raise OSError(code, os.strerror(code), name)
    return value
def mkdir_at(parent, name):
    if libc.mkdirat(parent, name.encode(), 0o700) < 0:
        code = ctypes.get_errno()
        raise OSError(code, os.strerror(code), name)
parts = [part for part in path.split('/') if part]
fd = os.open('/', flags)
try:
    for part in parts[:-1]:
        try:
            nxt = open_at(fd, part)
        except FileNotFoundError:
            mkdir_at(fd, part)
            nxt = open_at(fd, part)
        os.close(fd)
        fd = nxt
    try:
        mkdir_at(fd, parts[-1])
    except FileExistsError:
        sys.exit(46)
finally:
    os.close(fd)
PY"#,
        workspace = quote(workspace),
    )
}

/// One substrate-independent checkout law: the adapter-owned destination is
/// reset to the declared ref or immutable commit, then every declared lock
/// file is verified by its exact Git blob identity before workload execution.
pub(crate) fn repo_materialize_script(repo: &RepoSpec, dest: &str) -> String {
    let quote = crate::substrate::local::shell_quote;
    let dest_q = quote(dest);
    let url_q = quote(&repo.url);
    let target_q = quote(repo.commit.as_deref().unwrap_or(&repo.r#ref));
    let mut script = format!(
        "if [ -d {dest_q}/.git ]; then \
           git -C {dest_q} remote set-url origin {url_q}; \
         else \
           rm -rf {dest_q} && git init -q {dest_q} && git -C {dest_q} remote add origin {url_q}; \
         fi && \
         git -C {dest_q} fetch --depth 1 origin {target_q} && \
         git -C {dest_q} checkout -q --detach FETCH_HEAD && \
         git -C {dest_q} reset --hard && git -C {dest_q} clean -fd"
    );
    if let Some(commit) = &repo.commit {
        let commit_q = quote(commit);
        script.push_str(&format!(
            " && test \"$(git -C {dest_q} rev-parse HEAD)\" = {commit_q}"
        ));
    }
    for lock in &repo.locks {
        let object_q = quote(&format!("HEAD:{}", lock.path));
        let path_q = quote(&format!("{dest}/{}", lock.path));
        let expected_q = quote(&lock.git_blob);
        script.push_str(&format!(
            " && test \"$(git -C {dest_q} rev-parse {object_q})\" = {expected_q} \
             && test -f {path_q} && test ! -L {path_q} \
             && test \"$(git -C {dest_q} hash-object --no-filters -- {path_q})\" = {expected_q}"
        ));
    }
    script
}
pub const CARD_FILENAME: &str = "LANE_CARD.md";
pub const EVENT_FILENAME: &str = "EVENT.json";
pub const REPORT_FILENAME: &str = "REPORT.json";
/// bitterblossom-930: the episodic handoff packet an attempt writes to its
/// workspace when it parks on an unanswered ask. Collected the same way as
/// `REPORT_FILENAME` -- copied into the attempt's artifact dir on release,
/// never parsed by the substrate or dispatch, just relayed.
pub const ASK_PACKET_FILENAME: &str = "ASK_PACKET.json";

/// The `pid|lstart` identity pidfile shell fragments shared by the remote
/// substrates (sprites, tailnet): one writer, one liveness probe, one
/// group-kill, differing only in where the pidfile lives. Keeping the only
/// parsers of this format here means a fix can never land in five of six
/// hand-synced copies.
///
/// Probe exit codes: 0 alive (identity match or legacy pid-only file),
/// 3 no pidfile, 4 dead, 5 malformed/nonpositive pid, 6 identity mismatch.
pub(crate) fn identity_pidfile_write(pidfile: &str) -> String {
    format!("echo \"$$|$(ps -p $$ -o lstart= | sed 's/^ *//;s/ *$//')\" > {pidfile}")
}

pub(crate) fn identity_pidfile_probe(pidfile: &str) -> String {
    format!(
        "raw=\"$(cat {pidfile} 2>/dev/null)\" || exit 3; pid=\"${{raw%%|*}}\"; expected=\"${{raw#*|}}\"; case \"$pid\" in ''|*[!0-9]*) exit 5;; esac; [ \"$pid\" -gt 0 ] 2>/dev/null || exit 5; kill -0 \"$pid\" 2>/dev/null || exit 4; if [ \"$expected\" = \"$raw\" ] || [ -z \"$expected\" ]; then exit 0; fi; actual=\"$(ps -p \"$pid\" -o lstart= 2>/dev/null | sed 's/^ *//;s/ *$//')\"; [ \"$actual\" = \"$expected\" ] && exit 0 || exit 6"
    )
}

pub(crate) fn identity_pidfile_kill(pidfile: &str) -> String {
    format!(
        "raw=\"$(cat {pidfile} 2>/dev/null)\"; pid=\"${{raw%%|*}}\"; expected=\"${{raw#*|}}\"; case \"$pid\" in \"\"|*[!0-9]*) exit 0;; esac; [ \"$pid\" -gt 0 ] 2>/dev/null || exit 0; if [ \"$expected\" != \"$raw\" ] && [ -n \"$expected\" ]; then actual=\"$(ps -p \"$pid\" -o lstart= 2>/dev/null | sed 's/^ *//;s/ *$//')\"; [ \"$actual\" = \"$expected\" ] || exit 0; fi; kill -9 -- \"-$pid\" 2>/dev/null || true"
    )
}

pub fn for_task(kind: &str) -> Result<Box<dyn Substrate>> {
    match kind {
        "local" => Ok(Box::new(local::LocalSubstrate)),
        "sprites" => Ok(Box::new(sprites::SpritesSubstrate)),
        "tailnet" => Ok(Box::new(tailnet::TailnetSubstrate)),
        other => anyhow::bail!("unknown substrate '{other}' (known: local, sprites, tailnet)"),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::os::unix::fs::symlink;

    #[test]
    fn opened_receipt_survives_parent_swap_without_following_replacement() {
        let dir = tempfile::tempdir().unwrap();
        let root = dir.path().join("root");
        let outside = dir.path().join("outside");
        std::fs::create_dir_all(root.join("receipts")).unwrap();
        std::fs::create_dir_all(&outside).unwrap();
        std::fs::write(root.join("receipts/action.bin"), b"approved\0bytes").unwrap();
        std::fs::write(outside.join("action.bin"), b"hostile").unwrap();

        let mut opened = open_relative_nofollow(&root, "receipts/action.bin")
            .unwrap()
            .unwrap();
        std::fs::rename(root.join("receipts"), root.join("receipts-old")).unwrap();
        symlink(&outside, root.join("receipts")).unwrap();
        let mut bytes = Vec::new();
        opened.read_to_end(&mut bytes).unwrap();
        assert_eq!(bytes, b"approved\0bytes");
        assert!(read_relative_nofollow(&root, "receipts/action.bin").is_err());
    }

    #[test]
    fn destination_rejects_parent_and_leaf_symlinks() {
        let dir = tempfile::tempdir().unwrap();
        let root = dir.path().join("root");
        let outside = dir.path().join("outside");
        std::fs::create_dir_all(&root).unwrap();
        std::fs::create_dir_all(&outside).unwrap();
        symlink(&outside, root.join("parent")).unwrap();
        assert!(write_relative_nofollow(&root, "parent/action", b"no").is_err());

        std::fs::remove_file(root.join("parent")).unwrap();
        std::fs::create_dir(root.join("parent")).unwrap();
        symlink(outside.join("leaf"), root.join("parent/action")).unwrap();
        assert!(write_relative_nofollow(&root, "parent/action", b"no").is_err());
        assert!(!outside.join("leaf").exists());
    }

    #[test]
    fn remote_collector_is_binary_exact_and_refuses_symlinks() {
        let dir = tempfile::tempdir().unwrap();
        std::fs::create_dir(dir.path().join("receipts")).unwrap();
        let expected = b"\0\xff\nreceipt\r\n";
        std::fs::write(dir.path().join("receipts/action.bin"), expected).unwrap();
        let output = std::process::Command::new("sh")
            .args([
                "-c",
                &remote_collect_script(dir.path().to_str().unwrap(), "receipts/action.bin"),
            ])
            .output()
            .unwrap();
        assert!(
            output.status.success(),
            "status={:?} stderr={}",
            output.status.code(),
            String::from_utf8_lossy(&output.stderr)
        );
        let encoded = String::from_utf8(output.stdout).unwrap();
        assert_eq!(decode_hex_artifact("action", &encoded).unwrap(), expected);

        std::fs::remove_file(dir.path().join("receipts/action.bin")).unwrap();
        symlink("../outside", dir.path().join("receipts/action.bin")).unwrap();
        let output = std::process::Command::new("sh")
            .args([
                "-c",
                &remote_collect_script(dir.path().to_str().unwrap(), "receipts/action.bin"),
            ])
            .output()
            .unwrap();
        assert_eq!(output.status.code(), Some(43));
    }

    #[test]
    fn remote_workspace_creation_refuses_existing_or_symlinked_components() {
        let dir = tempfile::tempdir().unwrap();
        let canonical = dir.path().canonicalize().unwrap();
        let workspace = canonical.join("owned/attempt-1");
        let output = std::process::Command::new("sh")
            .args([
                "-c",
                &remote_create_workspace_script(workspace.to_str().unwrap()),
            ])
            .output()
            .unwrap();
        assert!(
            output.status.success(),
            "{:?}: {}",
            output.status.code(),
            String::from_utf8_lossy(&output.stderr)
        );
        let output = std::process::Command::new("sh")
            .args([
                "-c",
                &remote_create_workspace_script(workspace.to_str().unwrap()),
            ])
            .output()
            .unwrap();
        assert_eq!(output.status.code(), Some(46));

        let hostile = canonical.join("hostile");
        symlink(canonical.join("owned"), &hostile).unwrap();
        let output = std::process::Command::new("sh")
            .args([
                "-c",
                &remote_create_workspace_script(hostile.join("attempt-2").to_str().unwrap()),
            ])
            .output()
            .unwrap();
        assert!(!output.status.success());
        assert!(!canonical.join("owned/attempt-2").exists());
    }
}
