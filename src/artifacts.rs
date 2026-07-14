//! Run-artifact inspection helpers shared by the CLI and MCP.
//!
//! Mechanism only: enumerate and safely read artifact files a run's attempts
//! left on disk or snapshotted into the durable ledger, without letting a
//! caller escape the attempt artifact root or flood stdout with
//! binary/oversized output. The CLI surface in `main.rs`
//! and the read-only MCP server (backlog 078) both call these helpers so
//! their outputs agree by construction.

use std::fmt;
use std::fs;
use std::io::Read;
use std::path::{Component, Path, PathBuf};

use anyhow::{bail, Context, Result};
use serde::Serialize;

use crate::ledger::{ArtifactSnapshotRow, Ledger};

/// Maximum bytes `read` will load into memory and print. Oversized artifacts
/// are rejected with a structured `Oversized` outcome instead of being
/// streamed, so a multi-MiB workspace scrap file cannot flood a consumer.
pub const READ_LIMIT: u64 = 1 << 20; // 1 MiB

/// Files the harness writes into the attempt artifact dir for its own
/// bookkeeping. They are not agent artifacts and are hidden from `list`.
const INTERNAL_ARTIFACTS: &[&str] = &["harness.pid"];

/// Persist public artifacts for one completed attempt, including nested
/// declared receipts. Every bounded regular-file body is retained byte for
/// byte, including binary receipts; oversized files retain metadata only.
/// Directories, internal harness markers, and symlinks are deliberately absent
/// from the durable snapshot.
pub(crate) fn snapshot_attempt(ledger: &Ledger, attempt_id: i64, dir: &Path) -> Result<()> {
    match fs::read_dir(dir) {
        Ok(_) => {}
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
            bail!("artifact snapshot dir {} not found", dir.display())
        }
        Err(error) => {
            return Err(error)
                .with_context(|| format!("read artifact snapshot dir {}", dir.display()))
        }
    };
    let mut candidates = Vec::new();
    collect_snapshot_candidates(dir, dir, &mut candidates)?;
    candidates.sort();

    let mut snapshots = Vec::new();
    for relative in candidates {
        if bundle_skip_path(&relative) {
            continue;
        }
        let name = manifest_path(&relative);
        let Some(mut file) = crate::substrate::open_relative_nofollow(dir, &name)? else {
            continue;
        };
        let meta = file.metadata()?;
        if !meta.is_file() {
            continue;
        }
        let profile =
            bundle_file_profile_from_reader(&mut file, meta.len(), content_type(&relative))
                .with_context(|| format!("classify artifact snapshot candidate {name}"))?;
        let content = if profile.size <= READ_LIMIT {
            profile.bytes
        } else {
            None
        };
        snapshots.push(ArtifactSnapshotRow {
            path: name,
            size: profile.size,
            content_type: profile.content_type,
            binary: profile.binary,
            content,
        });
    }
    ledger.replace_artifact_snapshots(attempt_id, &snapshots)
}

fn collect_snapshot_candidates(root: &Path, dir: &Path, out: &mut Vec<PathBuf>) -> Result<()> {
    let mut entries = fs::read_dir(dir)
        .with_context(|| format!("read artifact snapshot dir {}", dir.display()))?
        .collect::<std::io::Result<Vec<_>>>()?;
    entries.sort_by_key(|entry| entry.file_name());
    for entry in entries {
        let path = entry.path();
        let metadata = path.symlink_metadata()?;
        if metadata.file_type().is_symlink() {
            continue;
        }
        let relative = path.strip_prefix(root)?.to_path_buf();
        if bundle_skip_path(&relative) {
            continue;
        }
        if metadata.is_dir() {
            collect_snapshot_candidates(root, &path, out)?;
        } else if metadata.is_file() {
            out.push(relative);
        }
    }
    Ok(())
}

#[derive(Debug)]
pub enum ArtifactError {
    InvalidPath { path: String },
    EscapesRoot { path: String },
    MissingRun { run_id: String },
}

impl ArtifactError {
    pub fn json_kind(&self) -> &'static str {
        match self {
            ArtifactError::InvalidPath { .. } | ArtifactError::EscapesRoot { .. } => "invalid_path",
            ArtifactError::MissingRun { .. } => "missing_run",
        }
    }
}

impl fmt::Display for ArtifactError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            ArtifactError::InvalidPath { path } => write!(
                f,
                "artifact path must be a non-empty relative path without '.' or '..': {path:?}"
            ),
            ArtifactError::EscapesRoot { path } => {
                write!(f, "artifact path escapes attempt artifact root: {path:?}")
            }
            ArtifactError::MissingRun { run_id } => write!(f, "run {run_id} not found"),
        }
    }
}

impl std::error::Error for ArtifactError {}

#[derive(Debug, Serialize)]
pub struct ArtifactEntry {
    pub attempt: i64,
    pub path: String,
    pub size: u64,
    pub content_type: String,
    pub binary: bool,
}

/// Outcome of reading one artifact path across a run's attempts. `read`
/// searches attempts newest-first and returns the first that contains the
/// path; `Missing` means no attempt produced it.
#[derive(Debug, Serialize)]
#[serde(tag = "kind")]
pub enum ReadOutcome {
    #[serde(rename = "text")]
    Text {
        attempt: i64,
        path: String,
        bytes: usize,
        content: String,
    },
    #[serde(rename = "binary")]
    Binary {
        attempt: i64,
        path: String,
        size: u64,
    },
    #[serde(rename = "oversized")]
    Oversized {
        attempt: i64,
        path: String,
        size: u64,
        limit: u64,
    },
    #[serde(rename = "missing")]
    Missing { path: String },
}

#[derive(Debug, Serialize)]
pub struct ArtifactBundleManifest {
    pub schema: &'static str,
    pub run_id: String,
    pub entries: Vec<ArtifactBundleEntry>,
}

#[derive(Debug, Serialize)]
pub struct ArtifactBundleEntry {
    pub attempt: i64,
    pub path: String,
    pub size: u64,
    pub content_type: String,
    pub binary: bool,
    pub included: bool,
    pub bundle_path: Option<String>,
    pub policy: Option<ArtifactBundlePolicy>,
}

#[derive(Debug, Serialize)]
pub struct ArtifactBundlePolicy {
    pub kind: &'static str,
    pub reason: &'static str,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub limit: Option<u64>,
}

/// List artifact files across every attempt of a run, in attempt order.
/// Only top-level files of each attempt's artifact dir are returned — the
/// `workspace/` scratch clone and internal harness markers are excluded.
/// Subdirectory artifacts are reachable via `read` but not enumerated yet;
/// bundling/listing nested trees is deferred (backlog 079 oracle).
pub fn list(ledger: &Ledger, run_id: &str) -> Result<Vec<ArtifactEntry>> {
    ensure_run_exists(ledger, run_id)?;
    let mut out = Vec::new();
    for a in ledger.attempts(run_id)? {
        let live_entries = match &a.artifact_dir {
            Some(dir) => list_live_attempt(run_id, a.n, Path::new(dir))?,
            None => None,
        };
        if let Some(entries) = live_entries {
            out.extend(entries);
            continue;
        }
        for snapshot in ledger.artifact_snapshots(a.id)? {
            out.push(ArtifactEntry {
                attempt: a.n,
                path: snapshot.path,
                size: snapshot.size,
                content_type: snapshot.content_type,
                binary: snapshot.binary,
            });
        }
    }
    Ok(out)
}

/// Read one artifact path from the newest attempt that has it. Rejects paths
/// that are absolute, empty, or contain `.`/`..` components, and rejects
/// symlinks that escape the attempt artifact root. Binary and oversized
/// artifacts are summarized, never streamed to stdout.
pub fn read(ledger: &Ledger, run_id: &str, path: &str) -> Result<ReadOutcome> {
    let rel = safe_relative(path)?;
    let rel_display = manifest_path(&rel);
    ensure_run_exists(ledger, run_id)?;
    let mut attempts = ledger.attempts(run_id)?;
    // Newest-first so a retry's artifact wins over an earlier attempt's.
    attempts.reverse();
    for a in &attempts {
        if let Some(dir) = &a.artifact_dir {
            if let Some(outcome) = read_live_attempt(run_id, a.n, Path::new(dir), &rel, path)? {
                return Ok(outcome);
            }
        }
        let Some(snapshot) = ledger.artifact_snapshot(a.id, &rel_display)? else {
            continue;
        };
        return snapshot_read_outcome(a.n, snapshot);
    }
    Ok(ReadOutcome::Missing { path: rel_display })
}

fn list_live_attempt(run_id: &str, attempt: i64, dir: &Path) -> Result<Option<Vec<ArtifactEntry>>> {
    let entries = match fs::read_dir(dir) {
        Ok(entries) => entries,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(None),
        Err(error) => bail!(
            "read artifact dir for run {run_id} attempt {attempt} at {}: {error}",
            dir.display()
        ),
    };
    let mut out = Vec::new();
    for entry in entries {
        let entry = entry.with_context(|| {
            format!(
                "read artifact dir entry for run {run_id} attempt {attempt} at {}",
                dir.display()
            )
        })?;
        // Do not follow symlinks while listing. `read` has a containment
        // guard for explicit paths; `list` should never stat or sniff a target
        // outside the attempt artifact root just because a symlink exists.
        let meta = entry.path().symlink_metadata().with_context(|| {
            format!(
                "stat artifact for run {run_id} attempt {attempt} at {}",
                entry.path().display()
            )
        })?;
        if !meta.is_file() {
            continue;
        }
        let name = entry.file_name().to_string_lossy().into_owned();
        if INTERNAL_ARTIFACTS.contains(&name.as_str()) {
            continue;
        }
        let profile = artifact_profile(&entry.path(), meta.len()).with_context(|| {
            format!(
                "classify artifact for run {run_id} attempt {attempt} at {}",
                entry.path().display()
            )
        })?;
        out.push(ArtifactEntry {
            attempt,
            path: name,
            size: meta.len(),
            content_type: profile.content_type,
            binary: profile.binary,
        });
    }
    Ok(Some(out))
}

fn read_live_attempt(
    run_id: &str,
    attempt: i64,
    dir: &Path,
    rel: &Path,
    requested_path: &str,
) -> Result<Option<ReadOutcome>> {
    let file = dir.join(rel);
    let meta = match fs::metadata(&file) {
        Ok(meta) if meta.is_file() => meta,
        Ok(_) => return Ok(None),
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(None),
        Err(error) => {
            return Err(error).with_context(|| {
                format!(
                    "stat artifact for run {run_id} attempt {attempt} at {}",
                    file.display()
                )
            })
        }
    };
    // Defense-in-depth: safe_relative already blocks lexical escapes;
    // canonicalize catches symlinks pointing outside the artifact root.
    let (root_c, file_c) = (
        fs::canonicalize(dir).context("canonicalize artifact dir")?,
        fs::canonicalize(&file).context("canonicalize artifact path")?,
    );
    if !file_c.starts_with(&root_c) {
        return Err(ArtifactError::EscapesRoot {
            path: requested_path.into(),
        }
        .into());
    }
    let path = manifest_path(rel);
    let size = meta.len();
    if size > READ_LIMIT {
        return Ok(Some(ReadOutcome::Oversized {
            attempt,
            path,
            size,
            limit: READ_LIMIT,
        }));
    }
    let bytes = fs::read(&file).context("read artifact")?;
    if is_binary_full_bytes(&bytes) {
        return Ok(Some(ReadOutcome::Binary {
            attempt,
            path,
            size,
        }));
    }
    Ok(Some(ReadOutcome::Text {
        attempt,
        path,
        bytes: bytes.len(),
        content: String::from_utf8(bytes)?,
    }))
}

fn snapshot_read_outcome(attempt: i64, snapshot: ArtifactSnapshotRow) -> Result<ReadOutcome> {
    if snapshot.size > READ_LIMIT {
        return Ok(ReadOutcome::Oversized {
            attempt,
            path: snapshot.path,
            size: snapshot.size,
            limit: READ_LIMIT,
        });
    }
    if snapshot.binary {
        return Ok(ReadOutcome::Binary {
            attempt,
            path: snapshot.path,
            size: snapshot.size,
        });
    }
    let content = snapshot
        .content
        .context("durable artifact snapshot is missing its bounded text body")?;
    Ok(ReadOutcome::Text {
        attempt,
        path: snapshot.path,
        bytes: content.len(),
        content: String::from_utf8(content)?,
    })
}

/// Export a portable artifact bundle directory for a run. Text artifacts at or
/// below `READ_LIMIT` are copied byte-for-byte under `attempt-<n>/`; oversized
/// and symlink artifacts are represented in `manifest.json` only. The bundle
/// never follows symlinks and never records host artifact-dir paths. CLI reads
/// still refuse to stream binary bytes to stdout.
pub fn bundle(ledger: &Ledger, run_id: &str, out_dir: &Path) -> Result<ArtifactBundleManifest> {
    ensure_run_exists(ledger, run_id)?;
    prepare_bundle_out_dir(out_dir)?;

    let mut attempts = ledger.attempts(run_id)?;
    attempts.sort_by_key(|attempt| attempt.n);
    let mut entries = Vec::new();
    for attempt in attempts {
        let Some(dir) = &attempt.artifact_dir else {
            append_snapshot_bundle_entries(ledger, attempt.id, attempt.n, out_dir, &mut entries)?;
            continue;
        };
        let dir = Path::new(dir);
        if !dir.exists() {
            append_snapshot_bundle_entries(ledger, attempt.id, attempt.n, out_dir, &mut entries)?;
            continue;
        }
        let root_c = fs::canonicalize(dir).context("canonicalize artifact dir")?;
        let candidates = bundle_candidates(dir).with_context(|| {
            format!(
                "enumerate artifact bundle candidates for run {run_id} attempt {}",
                attempt.n
            )
        })?;
        for candidate in candidates {
            let rel = manifest_path(&candidate.rel);
            let content_type = content_type(&candidate.rel);
            if candidate.symlink {
                entries.push(ArtifactBundleEntry {
                    attempt: attempt.n,
                    path: rel,
                    size: candidate.size,
                    content_type,
                    binary: true,
                    included: false,
                    bundle_path: None,
                    policy: Some(ArtifactBundlePolicy {
                        kind: "manifest_only_symlink",
                        reason: "symlink artifacts are never followed",
                        limit: None,
                    }),
                });
                continue;
            }

            let file = dir.join(&candidate.rel);
            let file_c = fs::canonicalize(&file).with_context(|| {
                format!(
                    "canonicalize artifact for run {run_id} attempt {} at {}",
                    attempt.n, rel
                )
            })?;
            if !file_c.starts_with(&root_c) {
                entries.push(ArtifactBundleEntry {
                    attempt: attempt.n,
                    path: rel,
                    size: candidate.size,
                    content_type,
                    binary: true,
                    included: false,
                    bundle_path: None,
                    policy: Some(ArtifactBundlePolicy {
                        kind: "manifest_only_escapes_root",
                        reason: "artifact path escapes attempt artifact root",
                        limit: None,
                    }),
                });
                continue;
            }

            let profile = bundle_file_profile(&file, &candidate.rel).with_context(|| {
                format!(
                    "classify artifact for run {run_id} attempt {} at {}",
                    attempt.n, rel
                )
            })?;
            let policy = if profile.size > READ_LIMIT {
                Some(ArtifactBundlePolicy {
                    kind: "manifest_only_oversized",
                    reason: "artifact exceeds bundle inline byte limit",
                    limit: Some(READ_LIMIT),
                })
            } else {
                None
            };
            if let Some(policy) = policy {
                entries.push(ArtifactBundleEntry {
                    attempt: attempt.n,
                    path: rel,
                    size: profile.size,
                    content_type: profile.content_type,
                    binary: profile.binary,
                    included: false,
                    bundle_path: None,
                    policy: Some(policy),
                });
                continue;
            }

            let bundle_path = format!("attempt-{}/{}", attempt.n, rel);
            let dest = out_dir.join(&bundle_path);
            if let Some(parent) = dest.parent() {
                fs::create_dir_all(parent).with_context(|| {
                    format!("create artifact bundle directory {}", parent.display())
                })?;
            }
            let bytes = profile
                .bytes
                .expect("included bundle artifacts are read into memory");
            fs::write(&dest, bytes)
                .with_context(|| format!("copy artifact into bundle at {}", dest.display()))?;
            entries.push(ArtifactBundleEntry {
                attempt: attempt.n,
                path: rel,
                size: profile.size,
                content_type: profile.content_type,
                binary: profile.binary,
                included: true,
                bundle_path: Some(bundle_path),
                policy: None,
            });
        }
    }

    let manifest = ArtifactBundleManifest {
        schema: "bb.artifact_bundle.v1",
        run_id: run_id.to_string(),
        entries,
    };
    fs::write(
        out_dir.join("manifest.json"),
        serde_json::to_vec_pretty(&manifest)?,
    )
    .with_context(|| format!("write artifact bundle manifest {}", out_dir.display()))?;
    Ok(manifest)
}

fn append_snapshot_bundle_entries(
    ledger: &Ledger,
    attempt_id: i64,
    attempt: i64,
    out_dir: &Path,
    entries: &mut Vec<ArtifactBundleEntry>,
) -> Result<()> {
    for snapshot in ledger.artifact_snapshots(attempt_id)? {
        let policy = if snapshot.size > READ_LIMIT {
            Some(ArtifactBundlePolicy {
                kind: "manifest_only_oversized",
                reason: "artifact exceeds bundle inline byte limit",
                limit: Some(READ_LIMIT),
            })
        } else {
            None
        };
        if let Some(policy) = policy {
            entries.push(ArtifactBundleEntry {
                attempt,
                path: snapshot.path,
                size: snapshot.size,
                content_type: snapshot.content_type,
                binary: snapshot.binary,
                included: false,
                bundle_path: None,
                policy: Some(policy),
            });
            continue;
        }

        let bundle_path = format!("attempt-{attempt}/{}", snapshot.path);
        let dest = out_dir.join(&bundle_path);
        if let Some(parent) = dest.parent() {
            fs::create_dir_all(parent).with_context(|| {
                format!("create artifact bundle directory {}", parent.display())
            })?;
        }
        let content = snapshot
            .content
            .context("durable artifact snapshot is missing its bounded body")?;
        fs::write(&dest, content)
            .with_context(|| format!("copy durable artifact into bundle at {}", dest.display()))?;
        entries.push(ArtifactBundleEntry {
            attempt,
            path: snapshot.path,
            size: snapshot.size,
            content_type: snapshot.content_type,
            binary: snapshot.binary,
            included: true,
            bundle_path: Some(bundle_path),
            policy: None,
        });
    }
    Ok(())
}

/// Validate a caller-supplied artifact path: non-empty, relative, no `.`/`..`
/// or prefix components. Mirrors `spec::validate_required_artifacts` at the
/// read boundary so a consumer can never traverse out of the artifact root.
fn safe_relative(path: &str) -> Result<PathBuf> {
    let p = Path::new(path);
    if path.trim().is_empty() || p.components().any(|c| !matches!(c, Component::Normal(_))) {
        return Err(ArtifactError::InvalidPath { path: path.into() }.into());
    }
    Ok(p.to_path_buf())
}

fn ensure_run_exists(ledger: &Ledger, run_id: &str) -> Result<()> {
    match ledger.run(run_id) {
        Ok(_) => Ok(()),
        Err(err) if is_not_found(&err) => Err(ArtifactError::MissingRun {
            run_id: run_id.into(),
        }
        .into()),
        Err(err) => Err(err),
    }
}

fn is_not_found(err: &anyhow::Error) -> bool {
    err.chain().any(|cause| {
        cause
            .downcast_ref::<rusqlite::Error>()
            .is_some_and(|e| matches!(e, rusqlite::Error::QueryReturnedNoRows))
    })
}

struct BundleCandidate {
    rel: PathBuf,
    size: u64,
    symlink: bool,
}

fn prepare_bundle_out_dir(out_dir: &Path) -> Result<()> {
    if out_dir.exists() {
        if !out_dir.is_dir() {
            bail!(
                "artifact bundle output path is not a directory: {}",
                out_dir.display()
            );
        }
        if fs::read_dir(out_dir)?.next().transpose()?.is_some() {
            bail!(
                "artifact bundle output directory must be empty: {}",
                out_dir.display()
            );
        }
        return Ok(());
    }
    fs::create_dir_all(out_dir)
        .with_context(|| format!("create artifact bundle output {}", out_dir.display()))
}

fn bundle_candidates(root: &Path) -> Result<Vec<BundleCandidate>> {
    let mut out = Vec::new();
    collect_bundle_candidates(root, root, &mut out)?;
    out.sort_by_key(|candidate| manifest_path(&candidate.rel));
    Ok(out)
}

fn collect_bundle_candidates(
    root: &Path,
    dir: &Path,
    out: &mut Vec<BundleCandidate>,
) -> Result<()> {
    let mut entries = fs::read_dir(dir)?
        .collect::<std::io::Result<Vec<_>>>()
        .with_context(|| format!("read artifact bundle dir {}", dir.display()))?;
    entries.sort_by_key(|entry| entry.file_name());
    for entry in entries {
        let path = entry.path();
        let rel = path
            .strip_prefix(root)
            .expect("candidate path is under bundle root")
            .to_path_buf();
        if bundle_skip_path(&rel) {
            continue;
        }
        let meta = path
            .symlink_metadata()
            .with_context(|| format!("stat artifact bundle candidate {}", path.display()))?;
        let file_type = meta.file_type();
        if file_type.is_symlink() {
            out.push(BundleCandidate {
                rel,
                size: meta.len(),
                symlink: true,
            });
        } else if file_type.is_file() {
            out.push(BundleCandidate {
                rel,
                size: meta.len(),
                symlink: false,
            });
        } else if file_type.is_dir() {
            collect_bundle_candidates(root, &path, out)?;
        }
    }
    Ok(())
}

fn bundle_skip_path(rel: &Path) -> bool {
    let mut components = rel.components();
    let Some(Component::Normal(first)) = components.next() else {
        return true;
    };
    if first == "workspace" {
        return true;
    }
    components.next().is_none() && INTERNAL_ARTIFACTS.contains(&first.to_string_lossy().as_ref())
}

fn manifest_path(path: &Path) -> String {
    path.components()
        .filter_map(|component| match component {
            Component::Normal(part) => Some(part.to_string_lossy()),
            _ => None,
        })
        .collect::<Vec<_>>()
        .join("/")
}

struct ArtifactProfile {
    content_type: String,
    binary: bool,
}

struct BundleFileProfile {
    size: u64,
    content_type: String,
    binary: bool,
    bytes: Option<Vec<u8>>,
}

fn bundle_file_profile(path: &Path, rel: &Path) -> Result<BundleFileProfile> {
    let mut file = open_bundle_file_no_symlink(path)?;
    let meta = file
        .metadata()
        .with_context(|| format!("stat opened artifact {}", path.display()))?;
    if !meta.is_file() {
        bail!(
            "artifact bundle candidate is not a regular file: {}",
            path.display()
        );
    }
    let size = meta.len();
    let content_type = content_type(rel);
    bundle_file_profile_from_reader(&mut file, size, content_type)
}

fn bundle_file_profile_from_reader(
    file: &mut impl Read,
    size: u64,
    content_type: String,
) -> Result<BundleFileProfile> {
    if size <= READ_LIMIT {
        let mut bytes = Vec::new();
        file.take(READ_LIMIT + 1)
            .read_to_end(&mut bytes)
            .context("read artifact")?;
        let size = size.max(bytes.len() as u64);
        if size > READ_LIMIT {
            return Ok(BundleFileProfile {
                size,
                content_type,
                binary: is_binary_sniff_bytes(&bytes[..bytes.len().min(8192)]),
                bytes: None,
            });
        }
        let binary = is_binary_full_bytes(&bytes);
        Ok(BundleFileProfile {
            size,
            content_type,
            binary,
            bytes: Some(bytes),
        })
    } else {
        let mut buf = [0u8; 8192];
        let n = file.read(&mut buf).context("read artifact sniff")?;
        Ok(BundleFileProfile {
            size,
            content_type,
            binary: is_binary_sniff_bytes(&buf[..n]),
            bytes: None,
        })
    }
}

fn open_bundle_file_no_symlink(path: &Path) -> Result<fs::File> {
    #[cfg(unix)]
    {
        use std::os::unix::fs::OpenOptionsExt;
        fs::OpenOptions::new()
            .read(true)
            .custom_flags(libc::O_NOFOLLOW)
            .open(path)
            .with_context(|| {
                format!(
                    "open artifact without following symlinks {}",
                    path.display()
                )
            })
    }
    #[cfg(not(unix))]
    {
        fs::File::open(path).with_context(|| format!("open artifact {}", path.display()))
    }
}

fn artifact_profile(path: &Path, size: u64) -> Result<ArtifactProfile> {
    let binary = if size <= READ_LIMIT {
        is_binary_full_bytes(&fs::read(path).context("read artifact for classification")?)
    } else {
        looks_binary(path, size)?
    };
    Ok(ArtifactProfile {
        content_type: content_type(path),
        binary,
    })
}

fn content_type(path: &Path) -> String {
    match path.extension().and_then(|e| e.to_str()) {
        Some("json") => "application/json",
        Some("md") => "text/markdown",
        Some("txt") | Some("log") => "text/plain",
        _ => "application/octet-stream",
    }
    .into()
}

/// Null-byte scan plus full-buffer UTF-8 validation. Complete artifacts with
/// invalid UTF-8 are binary even when the invalid sequence is an incomplete
/// trailing codepoint.
fn is_binary_full_bytes(bytes: &[u8]) -> bool {
    if bytes.contains(&0u8) {
        return true;
    }
    std::str::from_utf8(bytes).is_err()
}

/// Null-byte scan plus partial-sniff UTF-8 validation. A large-file sniff that
/// ends mid-codepoint is still text so `list` does not misclassify oversized
/// UTF-8 artifacts whose sampled prefix is valid except for the cut boundary.
fn is_binary_sniff_bytes(bytes: &[u8]) -> bool {
    if bytes.contains(&0u8) {
        return true;
    }
    std::str::from_utf8(bytes).is_err_and(|e| e.error_len().is_some())
}

fn looks_binary(path: &Path, size: u64) -> Result<bool> {
    if size == 0 {
        return Ok(false);
    }
    let mut f = fs::File::open(path).context("open artifact for binary sniff")?;
    let mut buf = [0u8; 8192];
    let n = f.read(&mut buf).context("read artifact for binary sniff")?;
    Ok(is_binary_sniff_bytes(&buf[..n]))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn stale_small_metadata_never_allows_an_oversized_inline_body() {
        let mut input = std::io::Cursor::new(vec![b'a'; READ_LIMIT as usize + 2]);
        let profile = bundle_file_profile_from_reader(&mut input, 1, "text/plain".into()).unwrap();
        assert_eq!(profile.size, READ_LIMIT + 1);
        assert!(profile.bytes.is_none());
        assert_eq!(input.position(), READ_LIMIT + 1);
    }
}
