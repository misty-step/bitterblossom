//! Run-artifact inspection helpers shared by the CLI and (future) MCP.
//!
//! Mechanism only: enumerate and safely read artifact files a run's attempts
//! left on disk, without letting a caller escape the attempt artifact root or
//! flood stdout with binary/oversized output. The CLI surface in `main.rs`
//! and the read-only MCP server (backlog 078) both call these helpers so
//! their outputs agree by construction.

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
    ledger.run(run_id)?; // bails with "run <id> not found" if absent
    let mut out = Vec::new();
    for a in ledger.attempts(run_id)? {
        let Some(dir) = &a.artifact_dir else { continue };
        let dir = Path::new(dir);
        let Ok(entries) = fs::read_dir(dir) else {
            continue;
        };
        for entry in entries.flatten() {
            let meta = match entry.metadata() {
                Ok(m) if m.is_file() => m,
                _ => continue,
            };
            let name = entry.file_name().to_string_lossy().into_owned();
            if INTERNAL_ARTIFACTS.contains(&name.as_str()) {
                continue;
            }
            out.push(ArtifactEntry {
                attempt: a.n,
                path: name,
                size: meta.len(),
                content_type: content_type(&entry.path()),
                binary: looks_binary(&entry.path(), meta.len()),
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
    ledger.run(run_id)?;
    let mut attempts = ledger.attempts(run_id)?;
    // Newest-first so a retry's artifact wins over an earlier attempt's.
    attempts.reverse();
    for a in &attempts {
        let Some(dir) = &a.artifact_dir else { continue };
        let dir = Path::new(dir);
        let file = dir.join(&rel);
        let meta = match fs::metadata(&file) {
            Ok(m) if m.is_file() => m,
            _ => continue,
        };
        // Defense-in-depth: safe_relative already blocks lexical escapes;
        // canonicalize catches symlinks pointing outside the artifact root.
        let (root_c, file_c) = (
            fs::canonicalize(dir).context("canonicalize artifact dir")?,
            fs::canonicalize(&file).context("canonicalize artifact path")?,
        );
        if !file_c.starts_with(&root_c) {
            bail!("artifact path escapes attempt artifact root: {path:?}");
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
        if bytes.contains(&0u8) || std::str::from_utf8(&bytes).is_err() {
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
        bail!("artifact path must be a non-empty relative path without '.' or '..': {path:?}");
    }
    Ok(p.to_path_buf())
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

/// Null-byte scan over the first 8 KiB: a conservative binary heuristic that
/// flags non-text artifacts without reading the whole file.
fn looks_binary(path: &Path, size: u64) -> bool {
    if size == 0 {
        return false;
    }
    let mut f = match fs::File::open(path) {
        Ok(f) => f,
        Err(_) => return false,
    };
    use std::io::Read;
    let mut buf = [0u8; 8192];
    let n = f.read(&mut buf).unwrap_or(0);
    buf[..n].contains(&0u8)
}
