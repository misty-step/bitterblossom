//! Run-artifact inspection helpers shared by the CLI and (future) MCP.
//!
//! Mechanism only: enumerate and safely read artifact files a run's attempts
//! left on disk, without letting a caller escape the attempt artifact root or
//! flood stdout with binary/oversized output. The CLI surface in `main.rs`
//! and the read-only MCP server (backlog 078) both call these helpers so
//! their outputs agree by construction.

use std::fmt;
use std::fs;
use std::path::{Component, Path, PathBuf};

use anyhow::{bail, Context, Result};
use serde::Serialize;

use crate::ledger::Ledger;

/// Maximum bytes `read` will load into memory and print. Oversized artifacts
/// are rejected with a structured `Oversized` outcome instead of being
/// streamed, so a multi-MiB workspace scrap file cannot flood a consumer.
pub const READ_LIMIT: u64 = 1 << 20; // 1 MiB

/// Files the harness writes into the attempt artifact dir for its own
/// bookkeeping. They are not agent artifacts and are hidden from `list`.
const INTERNAL_ARTIFACTS: &[&str] = &["harness.pid"];

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

/// List artifact files across every attempt of a run, in attempt order.
/// Only top-level files of each attempt's artifact dir are returned — the
/// `workspace/` scratch clone and internal harness markers are excluded.
/// Subdirectory artifacts are reachable via `read` but not enumerated yet;
/// bundling/listing nested trees is deferred (backlog 079 oracle).
pub fn list(ledger: &Ledger, run_id: &str) -> Result<Vec<ArtifactEntry>> {
    ensure_run_exists(ledger, run_id)?;
    let mut out = Vec::new();
    for a in ledger.attempts(run_id)? {
        let Some(dir) = &a.artifact_dir else { continue };
        let dir = Path::new(dir);
        let entries = match fs::read_dir(dir) {
            Ok(entries) => entries,
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => continue,
            Err(e) => bail!(
                "read artifact dir for run {run_id} attempt {} at {}: {e}",
                a.n,
                dir.display()
            ),
        };
        for entry in entries {
            let entry = entry.with_context(|| {
                format!(
                    "read artifact dir entry for run {run_id} attempt {} at {}",
                    a.n,
                    dir.display()
                )
            })?;
            // Do not follow symlinks while listing. `read` has a containment
            // guard for explicit paths; `list` should never stat or sniff a
            // target outside the attempt artifact root just because a symlink
            // exists in the directory.
            let meta = entry.path().symlink_metadata().with_context(|| {
                format!(
                    "stat artifact for run {run_id} attempt {} at {}",
                    a.n,
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
                    "classify artifact for run {run_id} attempt {} at {}",
                    a.n,
                    entry.path().display()
                )
            })?;
            out.push(ArtifactEntry {
                attempt: a.n,
                path: name,
                size: meta.len(),
                content_type: profile.content_type,
                binary: profile.binary,
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
    ensure_run_exists(ledger, run_id)?;
    let mut attempts = ledger.attempts(run_id)?;
    // Newest-first so a retry's artifact wins over an earlier attempt's.
    attempts.reverse();
    for a in &attempts {
        let Some(dir) = &a.artifact_dir else { continue };
        let dir = Path::new(dir);
        let file = dir.join(&rel);
        let meta = match fs::metadata(&file) {
            Ok(m) if m.is_file() => m,
            Ok(_) => continue,
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => continue,
            Err(e) => {
                return Err(e).with_context(|| {
                    format!(
                        "stat artifact for run {run_id} attempt {} at {}",
                        a.n,
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
            return Err(ArtifactError::EscapesRoot { path: path.into() }.into());
        }
        let size = meta.len();
        if size > READ_LIMIT {
            return Ok(ReadOutcome::Oversized {
                attempt: a.n,
                path: rel.to_string_lossy().into_owned(),
                size,
                limit: READ_LIMIT,
            });
        }
        let bytes = fs::read(&file).context("read artifact")?;
        if is_binary_full_bytes(&bytes) {
            return Ok(ReadOutcome::Binary {
                attempt: a.n,
                path: rel.to_string_lossy().into_owned(),
                size,
            });
        }
        return Ok(ReadOutcome::Text {
            attempt: a.n,
            path: rel.to_string_lossy().into_owned(),
            bytes: bytes.len(),
            content: String::from_utf8(bytes)?,
        });
    }
    Ok(ReadOutcome::Missing {
        path: rel.to_string_lossy().into_owned(),
    })
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

struct ArtifactProfile {
    content_type: String,
    binary: bool,
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
    use std::io::Read;
    let mut buf = [0u8; 8192];
    let n = f.read(&mut buf).context("read artifact for binary sniff")?;
    Ok(is_binary_sniff_bytes(&buf[..n]))
}
