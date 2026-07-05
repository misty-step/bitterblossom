//! Lane-checkout lifecycle (bitterblossom-921): finished lanes leave full
//! repo clones and build caches on disk with nothing to reap them. The
//! governor's repo-artifact class handles stale `target/`/`node_modules`
//! *inside* an active repo; it never touches whole checkouts, and it is
//! blind to a lane that fleet sweeps keep touching (`git status`, `orient`)
//! long after the actual work is done. This module is the janitor for the
//! other half: whole worktree/clone directories.
//!
//! Mechanism only, no judgment: a candidate is reap-eligible only if its
//! tree is clean, its HEAD commit is reachable from some remote branch (so
//! removing the checkout loses nothing -- git's object store, not this
//! directory, is the source of truth for anything already pushed), and it
//! has been idle at least `grace_hours`. Anything else is refused with a
//! reason, never deleted.

use std::path::{Path, PathBuf};
use std::process::Command;

use anyhow::{bail, Context, Result};
use serde::Serialize;

use crate::ledger::Ledger;

#[derive(Debug, Clone, Serialize)]
pub struct ReapCandidate {
    pub path: String,
    pub source: &'static str, // "registered" (bb run) | "discovered" (git worktree list)
    pub run_id: Option<String>,
    pub eligible: bool,
    pub reason: String,
    pub age_hours: Option<f64>,
    pub removed: bool,
}

/// List every non-primary worktree of the repo rooted at `primary_repo`,
/// via `git worktree list --porcelain` -- git's own bookkeeping, not a
/// filesystem heuristic. The first entry is always the primary checkout
/// itself; it is never a candidate.
pub fn discover_worktrees(primary_repo: &Path) -> Result<Vec<PathBuf>> {
    let output = Command::new("git")
        .args(["worktree", "list", "--porcelain"])
        .current_dir(primary_repo)
        .output()
        .with_context(|| format!("git worktree list in {}", primary_repo.display()))?;
    if !output.status.success() {
        bail!(
            "git worktree list failed in {}: {}",
            primary_repo.display(),
            String::from_utf8_lossy(&output.stderr)
        );
    }
    let stdout = String::from_utf8_lossy(&output.stdout);
    let paths: Vec<PathBuf> = stdout
        .lines()
        .filter_map(|line| line.strip_prefix("worktree "))
        .map(PathBuf::from)
        .collect();
    let primary = primary_repo
        .canonicalize()
        .unwrap_or_else(|_| primary_repo.to_path_buf());
    Ok(paths
        .into_iter()
        .filter(|p| p.canonicalize().unwrap_or_else(|_| p.clone()) != primary)
        .collect())
}

fn git_output(path: &Path, args: &[&str]) -> Result<Option<String>> {
    let output = Command::new("git")
        .args(args)
        .current_dir(path)
        .output()
        .with_context(|| format!("git {} in {}", args.join(" "), path.display()))?;
    if !output.status.success() {
        return Ok(None);
    }
    Ok(Some(String::from_utf8_lossy(&output.stdout).into_owned()))
}

fn is_clean(path: &Path) -> Result<bool> {
    match git_output(path, &["status", "--porcelain"])? {
        Some(out) => Ok(out.trim().is_empty()),
        None => bail!("git status failed in {}", path.display()),
    }
}

/// Nothing is lost by removing this checkout if HEAD is reachable from at
/// least one remote-tracking branch -- pushed-and-merged, pushed-but-not-yet-
/// merged, and pushed-then-remote-deleted-after-merge all satisfy this.
/// Never-pushed local-only commits do not, and are refused.
fn is_safely_preserved(path: &Path) -> Result<bool> {
    let sha = git_output(path, &["rev-parse", "HEAD"])?
        .context("git rev-parse HEAD")?
        .trim()
        .to_string();
    let refs = git_output(path, &["branch", "-r", "--contains", &sha])?
        .context("git branch -r --contains")?;
    Ok(!refs.trim().is_empty())
}

/// Hours since HEAD's commit time -- a real, portable signal that survives
/// `git status`/`git fetch` touching mtimes without representing new work.
fn age_hours(path: &Path) -> Result<f64> {
    let ts = git_output(path, &["log", "-1", "--format=%ct"])?
        .context("git log -1 --format=%ct")?
        .trim()
        .parse::<i64>()
        .context("parse commit timestamp")?;
    let now = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)?
        .as_secs() as i64;
    Ok(((now - ts).max(0) as f64) / 3600.0)
}

pub fn evaluate(
    path: &Path,
    source: &'static str,
    run_id: Option<String>,
    grace_hours: f64,
) -> ReapCandidate {
    let path_str = path.to_string_lossy().into_owned();
    if !path.is_dir() {
        return ReapCandidate {
            path: path_str,
            source,
            run_id,
            eligible: false,
            reason: "path does not exist".to_string(),
            age_hours: None,
            removed: false,
        };
    }
    let clean = is_clean(path);
    let preserved = is_safely_preserved(path);
    let age = age_hours(path);

    let (eligible, reason) = match (clean, preserved, age) {
        (Ok(false), _, _) => (
            false,
            "dirty tree: uncommitted or untracked changes".to_string(),
        ),
        (_, Ok(false), _) => (
            false,
            "unpushed commits: HEAD is not reachable from any remote branch".to_string(),
        ),
        (Ok(true), Ok(true), Ok(hours)) if hours < grace_hours => (
            false,
            format!("too recent: age {hours:.1}h < grace {grace_hours:.1}h"),
        ),
        (Ok(true), Ok(true), Ok(hours)) => (
            true,
            format!("eligible: clean, pushed, age {hours:.1}h >= grace {grace_hours:.1}h"),
        ),
        (Err(e), _, _) | (_, Err(e), _) | (_, _, Err(e)) => {
            (false, format!("could not evaluate: {e}"))
        }
    };
    ReapCandidate {
        path: path_str,
        source,
        run_id,
        eligible,
        reason,
        age_hours: age_hours(path).ok(),
        removed: false,
    }
}

/// `git worktree remove` (no `--force`) so git's own dirty-tree refusal is a
/// second, independent gate behind `evaluate`'s clean check.
fn remove_worktree(primary_repo: &Path, path: &Path) -> Result<()> {
    let output = Command::new("git")
        .args(["worktree", "remove", &path.to_string_lossy()])
        .current_dir(primary_repo)
        .output()
        .with_context(|| format!("git worktree remove {}", path.display()))?;
    if !output.status.success() {
        bail!(
            "git worktree remove refused {}: {}",
            path.display(),
            String::from_utf8_lossy(&output.stderr)
        );
    }
    Ok(())
}

/// Sweep both discovery sources: worktrees registered against each primary
/// repo in `primary_repos`, and any terminal bb run that recorded a
/// `checkout_path`. `apply=false` (the default call site) never mutates
/// anything -- it only evaluates and reports. A refused candidate is
/// recorded as a `checkout_reap_refused` run event when it has a run_id, so
/// the reason is visible from `bb runs show`, never silently dropped.
pub fn sweep(
    ledger: &Ledger,
    primary_repos: &[PathBuf],
    grace_hours: f64,
    apply: bool,
) -> Result<Vec<ReapCandidate>> {
    let mut candidates = Vec::new();

    for primary in primary_repos {
        for path in discover_worktrees(primary)? {
            let mut c = evaluate(&path, "discovered", None, grace_hours);
            if c.eligible && apply {
                match remove_worktree(primary, &path) {
                    Ok(()) => c.removed = true,
                    Err(e) => {
                        c.eligible = false;
                        c.reason = format!("removal refused: {e}");
                    }
                }
            }
            candidates.push(c);
        }
    }

    for run in ledger.runs_with_reapable_checkout()? {
        let Some(checkout_path) = run.checkout_path.clone() else {
            continue;
        };
        let path = PathBuf::from(&checkout_path);
        let mut c = evaluate(&path, "registered", Some(run.id.clone()), grace_hours);
        if c.eligible && apply {
            // Registered checkouts are not necessarily linked worktrees of a
            // known primary repo (a lane may have used a plain clone), so
            // remove the directory directly rather than via `git worktree
            // remove`. `is_clean`/`is_safely_preserved` already gated this.
            match std::fs::remove_dir_all(&path) {
                Ok(()) => c.removed = true,
                Err(e) => {
                    c.eligible = false;
                    c.reason = format!("removal failed: {e}");
                }
            }
        }
        if !c.eligible {
            ledger.record_event(&run.id, "checkout_reap_refused", Some(&c.reason))?;
        } else if c.removed {
            ledger.record_event(&run.id, "checkout_reaped", Some(&checkout_path))?;
        }
        candidates.push(c);
    }

    Ok(candidates)
}
