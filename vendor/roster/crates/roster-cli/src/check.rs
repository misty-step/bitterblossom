//! `roster check` gates only what deterministic code consumes: skill
//! frontmatter shape, path references skill prose cites, index/disk parity,
//! and conflict markers. Semantic judgment (premise soundness, prose
//! quality) stays model work — see the ci and shape skills. Scope is
//! `primitives/`, git-tracked files only.

use anyhow::{Context, Result, ensure};
use std::{
    collections::BTreeSet,
    fs,
    path::{Path, PathBuf},
    process::Command,
};

pub fn run(root: &Path) -> Result<bool> {
    let tracked = tracked_files(root)?;
    let mut findings = Vec::new();

    for path in &tracked {
        let Ok(content) = fs::read_to_string(root.join(path)) else {
            continue;
        };
        if content.lines().any(|line| line.starts_with("<<<<<<<")) {
            findings.push(format!("{}: conflict marker", path.display()));
        }
        if let Some((is_external, _)) = skill_md_name(path) {
            check_frontmatter(path, &content, is_external, &mut findings);
        }
        if is_first_party_prose(path) {
            for reference in extract_path_refs(&content) {
                if !root.join(&reference).exists() {
                    findings.push(format!("{}: dead reference {reference}", path.display()));
                }
            }
        }
    }
    check_index_drift(root, &tracked, &mut findings)?;

    for finding in &findings {
        println!("FAIL {finding}");
    }
    if findings.is_empty() {
        println!(
            "roster check: ok ({} tracked primitives files)",
            tracked.len()
        );
    }
    Ok(findings.is_empty())
}

fn tracked_files(root: &Path) -> Result<Vec<PathBuf>> {
    let output = Command::new("git")
        .args(["ls-files", "--", "primitives"])
        .current_dir(root)
        .output()
        .context("run git ls-files")?;
    ensure!(
        output.status.success(),
        "git ls-files failed: {}",
        String::from_utf8_lossy(&output.stderr)
    );
    Ok(String::from_utf8(output.stdout)?
        .lines()
        .map(PathBuf::from)
        .collect())
}

/// `primitives/skills/<name>/SKILL.md` or `.../.external/<name>/SKILL.md`.
fn skill_md_name(path: &Path) -> Option<(bool, &str)> {
    let rest = path.to_str()?.strip_prefix("primitives/skills/")?;
    let (is_external, rest) = rest
        .strip_prefix(".external/")
        .map_or((false, rest), |r| (true, r));
    let name = rest.strip_suffix("/SKILL.md")?;
    (!name.contains('/')).then_some((is_external, name))
}

fn is_first_party_prose(path: &Path) -> bool {
    let raw = path.to_str().unwrap_or_default();
    raw.starts_with("primitives/skills/")
        && raw.ends_with(".md")
        && !raw.contains("/.external/")
        && !raw.contains("/evals/")
}

fn check_frontmatter(path: &Path, content: &str, is_external: bool, findings: &mut Vec<String>) {
    let Some(frontmatter) = content
        .strip_prefix("---\n")
        .and_then(|body| body.find("\n---").map(|end| &body[..end]))
    else {
        findings.push(format!("{}: no frontmatter block", path.display()));
        return;
    };
    let doc: serde_yaml::Value = match serde_yaml::from_str(frontmatter) {
        Ok(doc) => doc,
        Err(error) => {
            findings.push(format!(
                "{}: frontmatter does not parse: {error}",
                path.display()
            ));
            return;
        }
    };
    let empty = |key: &str| {
        doc.get(key)
            .and_then(|v| v.as_str())
            .unwrap_or_default()
            .trim()
            .is_empty()
    };
    if empty("name") {
        findings.push(format!("{}: missing/empty name", path.display()));
    }
    if empty("description") {
        findings.push(format!("{}: missing/empty description", path.display()));
    }
    let disabled = doc
        .get("disable-model-invocation")
        .and_then(serde_yaml::Value::as_bool)
        .unwrap_or(false);
    if !is_external && !disabled && empty("argument-hint") {
        findings.push(format!(
            "{}: missing argument-hint (first-party, model-invocable)",
            path.display()
        ));
    }
}

/// Scans prose for `primitives/skills/...` and `primitives/shared/...` path
/// tokens (backticked or bare) so this doesn't need a markdown parser.
fn extract_path_refs(content: &str) -> BTreeSet<String> {
    let mut refs = BTreeSet::new();
    for prefix in ["primitives/skills/", "primitives/shared/"] {
        let mut rest = content;
        while let Some(start) = rest.find(prefix) {
            let tail = &rest[start..];
            let end = tail
                .find(|c: char| !(c.is_alphanumeric() || "_./-".contains(c)))
                .unwrap_or(tail.len());
            let candidate = tail[..end].trim_end_matches('/');
            if candidate.len() > prefix.len() {
                refs.insert(candidate.to_string());
            }
            rest = &tail[end.max(1)..];
        }
    }
    refs
}

fn check_index_drift(root: &Path, tracked: &[PathBuf], findings: &mut Vec<String>) -> Result<()> {
    let index_rel = "primitives/skills/skills-index.yaml";
    let content = fs::read_to_string(root.join(index_rel)).context("read skills-index.yaml")?;
    let doc: serde_yaml::Value =
        serde_yaml::from_str(&content).context("parse skills-index.yaml")?;
    let entries = doc
        .get("skills")
        .and_then(|v| v.as_sequence())
        .cloned()
        .unwrap_or_default();

    let mut indexed = BTreeSet::new();
    for entry in &entries {
        let path = entry
            .get("path")
            .and_then(|v| v.as_str())
            .unwrap_or_default();
        // Index paths are repo-relative; absolute paths are a portability
        // failure (they only resolve on the machine that wrote them).
        if Path::new(path).is_absolute() {
            let name = entry.get("name").and_then(|v| v.as_str()).unwrap_or("?");
            findings.push(format!(
                "{index_rel}: absolute path for {name} (must be repo-relative) -> {path}"
            ));
        } else if !root.join(path).exists() {
            let name = entry.get("name").and_then(|v| v.as_str()).unwrap_or("?");
            findings.push(format!("{index_rel}: orphan entry {name} -> {path}"));
        }
        if let Some(dir) = path
            .split("primitives/skills/")
            .nth(1)
            .and_then(|r| r.split('/').next())
        {
            indexed.insert(dir.to_string());
        }
    }
    for (is_external, name) in tracked.iter().filter_map(|p| skill_md_name(p)) {
        if !is_external && !indexed.contains(name) {
            findings.push(format!(
                "{index_rel}: missing entry for primitives/skills/{name}"
            ));
        }
    }
    Ok(())
}
