//! `roster sync` materializes roster declarations onto a harness `$HOME`:
//! per-harness agent briefs, a skill symlink farm, and doctrine links for
//! the shared `AGENTS.md`. Everything it writes is recorded in a manifest so
//! `--disable` can reverse it, and a re-run self-heals anything that went
//! stale (a dangling symlink, a catalog that shrank) without touching files
//! it does not own.

use anyhow::{Context, Result, anyhow, bail};
use clap::ValueEnum;
use roster_core::{Agent, Models, Roster, render_brief, render_claude_agent, render_home_doctrine};
use serde_json::{Value, json};
use std::{
    collections::BTreeSet,
    fs,
    path::{Component, Path, PathBuf},
};

use crate::find_agent;

pub const DEFAULT_AGENT: &str = "orchestrator";
const SYNC_MARKER: &str = "<!-- roster-sync:orchestrator:v1 -->";
const SYNC_DIR_REL: &str = ".roster/orchestrator";
const MANIFEST_REL: &str = ".roster/orchestrator/manifest.json";

#[derive(Clone, Copy, Debug, PartialEq, Eq, ValueEnum)]
pub enum Catalog {
    /// Every skill with a SKILL.md under primitives/skills, including
    /// .external/* vendored skills — day-1 cutover parity with harness-kit.
    Full,
    /// Only the orchestrator role.yaml's skills list — the inversion
    /// ratchet. Off by default until the orchestrator's own skill list is
    /// trusted as the whole-machine catalog.
    Curated,
}

impl Catalog {
    fn as_str(self) -> &'static str {
        match self {
            Catalog::Full => "full",
            Catalog::Curated => "curated",
        }
    }
}

pub fn run(
    root: &Path,
    home: Option<PathBuf>,
    disable: bool,
    catalog: Catalog,
    all_agents: bool,
) -> Result<()> {
    let home = home.unwrap_or_else(|| {
        std::env::var_os("HOME")
            .map(PathBuf::from)
            .unwrap_or_default()
    });
    if home.as_os_str().is_empty() {
        bail!("HOME is required for roster sync; pass --home to choose an install root");
    }

    if disable {
        disable_sync(&home)
    } else {
        // Symlink targets are resolved relative to the symlink's own
        // directory, not this process's cwd — a relative `root` (the
        // default is ".") would produce broken links once installed under
        // an unrelated `home`. Canonicalize once, up front.
        let root = root
            .canonicalize()
            .with_context(|| format!("resolve sync root {}", root.display()))?;
        install_sync(&root, &home, catalog, all_agents)
    }
}

/// A path this run intends to manage, plus how to produce it.
enum PlannedEntry {
    File {
        relative_path: String,
        contents: String,
    },
    Symlink {
        relative_path: String,
        target: PathBuf,
    },
}

fn install_sync(root: &Path, home: &Path, catalog: Catalog, all_agents: bool) -> Result<()> {
    let roster = Roster::load(root)?;
    let orchestrator = find_agent(&roster, DEFAULT_AGENT)?;
    let models = Models::load(root)?;

    let prior = read_manifest(home)?;
    let prior_symlinks: BTreeSet<String> = prior
        .as_ref()
        .map(|manifest| manifest.symlinks.clone())
        .unwrap_or_default();

    let plan = build_plan(
        root,
        home,
        &roster,
        orchestrator,
        &models,
        catalog,
        all_agents,
    )?;

    let mut written_files = Vec::new();
    let mut written_symlinks = Vec::new();
    let mut skipped = Vec::new();

    for entry in &plan {
        match entry {
            PlannedEntry::File {
                relative_path,
                contents,
            } => {
                write_managed_file(home, relative_path, contents)?;
                written_files.push(relative_path.clone());
            }
            PlannedEntry::Symlink {
                relative_path,
                target,
            } => match sync_symlink(home, relative_path, target, &prior_symlinks)? {
                SymlinkOutcome::Applied => written_symlinks.push(relative_path.clone()),
                SymlinkOutcome::Skipped(note) => skipped.push(note),
            },
        }
    }

    cleanup_stale(home, prior.as_ref(), &written_files, &written_symlinks)?;

    let mut managed_paths = written_files.clone();
    managed_paths.extend(written_symlinks.iter().cloned());
    managed_paths.push(MANIFEST_REL.to_string());

    let manifest = serde_json::to_string_pretty(&json!({
        "schema_version": "roster.sync.v1",
        "managed_by": "roster sync",
        "agent": orchestrator.role.name,
        "mode": "parallel-run",
        "catalog": catalog.as_str(),
        "all_agents": all_agents,
        "disable_command": "roster sync --disable",
        "files": managed_paths,
        "symlinks": written_symlinks,
        "preserved_harness_kit_files": [
            ".codex/CLAUDE.md",
            ".pi/settings.json"
        ]
    }))?;
    write_managed_file(home, MANIFEST_REL, &format!("{manifest}\n"))?;

    println!(
        "Installed roster orchestrator sync at {}",
        home.join(SYNC_DIR_REL).display()
    );
    println!(
        "Skills: {} entries linked ({})",
        written_symlinks.len(),
        catalog.as_str()
    );
    println!("Manifest: {}", home.join(MANIFEST_REL).display());
    println!("Disable: roster sync --disable");
    if skipped.is_empty() {
        println!("No unmanaged files were touched.");
    } else {
        println!("Skipped (left in place):");
        for note in &skipped {
            println!("- {note}");
        }
    }

    Ok(())
}

#[allow(clippy::too_many_arguments)]
fn build_plan(
    root: &Path,
    home: &Path,
    roster: &Roster,
    orchestrator: &Agent,
    models: &Models,
    catalog: Catalog,
    all_agents: bool,
) -> Result<Vec<PlannedEntry>> {
    let mut plan = Vec::new();

    // Orchestrator supplementary docs — unchanged from the pre-farm sync.
    let brief = render_brief(orchestrator, &[], &[], None);
    let claude_agent = render_claude_agent(orchestrator, models);
    let skills_index = skills_index_json(orchestrator)?;
    plan.push(PlannedEntry::File {
        relative_path: format!("{SYNC_DIR_REL}/brief.md"),
        contents: managed_markdown(&brief),
    });
    plan.push(PlannedEntry::File {
        relative_path: format!("{SYNC_DIR_REL}/claude.md"),
        contents: managed_markdown(&claude_agent),
    });
    plan.push(PlannedEntry::File {
        relative_path: format!("{SYNC_DIR_REL}/primitives/skills-index.json"),
        contents: skills_index,
    });
    plan.push(PlannedEntry::File {
        relative_path: format!("{SYNC_DIR_REL}/ROLLBACK.md"),
        contents: rollback_doc(),
    });

    // Per-harness agent files: orchestrator always; every agent with --all-agents.
    let agents: Vec<&Agent> = if all_agents {
        roster.agents().iter().collect()
    } else {
        vec![orchestrator]
    };
    for agent in agents {
        let name = &agent.role.name;
        let is_orchestrator = name == &orchestrator.role.name;
        let brief_rendered = if is_orchestrator {
            brief.clone()
        } else {
            render_brief(agent, &[], &[], None)
        };
        let claude_rendered = if is_orchestrator {
            claude_agent.clone()
        } else {
            render_claude_agent(agent, models)
        };
        plan.push(PlannedEntry::File {
            relative_path: format!(".codex/agents/{name}.md"),
            contents: managed_markdown(&brief_rendered),
        });
        plan.push(PlannedEntry::File {
            relative_path: format!(".claude/agents/{name}.md"),
            contents: managed_markdown(&claude_rendered),
        });
        plan.push(PlannedEntry::File {
            relative_path: format!(".pi/agents/{name}.md"),
            contents: managed_markdown(&brief_rendered),
        });
    }

    // Skill symlink farm.
    let skill_dirs = match catalog {
        Catalog::Full => discover_full_skill_dirs(root)?,
        Catalog::Curated => curated_skill_dirs(orchestrator),
    };
    for harness_skills_dir in detect_skill_harness_dirs(home) {
        for (skill_name, target) in &skill_dirs {
            plan.push(PlannedEntry::Symlink {
                relative_path: format!("{harness_skills_dir}/{skill_name}"),
                target: target.clone(),
            });
        }
    }

    // Home doctrine — one composed file (shared AGENTS.md + the
    // orchestrator's identity, skills, and MCP bindings) that every
    // harness's doctrine link points at, so any default agent session on
    // this machine boots as the declared roster orchestrator rather than a
    // bare copy of shared AGENTS.md (operator ruling 2026-07-07).
    let home_doctrine = render_home_doctrine(root, orchestrator)?;
    plan.push(PlannedEntry::File {
        relative_path: format!("{SYNC_DIR_REL}/home-doctrine.md"),
        contents: managed_markdown(&home_doctrine),
    });

    // Doctrine links — replace only a harness-kit symlink or a prior roster
    // symlink; refuse anything else real and unmanaged. pi and opencode only
    // get a link when their own native surface is already on the machine
    // (same presence rule the skill farm uses below), so a bare sync never
    // half-installs a harness that isn't there.
    let doctrine_target = home.join(SYNC_DIR_REL).join("home-doctrine.md");
    plan.push(PlannedEntry::Symlink {
        relative_path: ".claude/CLAUDE.md".to_string(),
        target: doctrine_target.clone(),
    });
    plan.push(PlannedEntry::Symlink {
        relative_path: ".codex/AGENTS.md".to_string(),
        target: doctrine_target.clone(),
    });
    if pi_present(home) {
        plan.push(PlannedEntry::Symlink {
            relative_path: ".pi/agent/AGENTS.md".to_string(),
            target: doctrine_target.clone(),
        });
    }
    if opencode_present(home) {
        // opencode's own docs: "You can also have global rules in a
        // `~/.config/opencode/AGENTS.md` file. This gets applied across all
        // opencode sessions."
        plan.push(PlannedEntry::Symlink {
            relative_path: ".config/opencode/AGENTS.md".to_string(),
            target: doctrine_target,
        });
    }

    Ok(plan)
}

fn discover_full_skill_dirs(root: &Path) -> Result<Vec<(String, PathBuf)>> {
    let skills_root = root.join("primitives/skills");
    let mut found = Vec::new();
    let entries =
        fs::read_dir(&skills_root).with_context(|| format!("read {}", skills_root.display()))?;
    for entry in entries {
        let entry = entry.with_context(|| format!("read entry in {}", skills_root.display()))?;
        if !entry.file_type()?.is_dir() {
            continue;
        }
        let path = entry.path();
        let name = entry.file_name().to_string_lossy().to_string();
        if name == ".external" {
            for sub_entry in
                fs::read_dir(&path).with_context(|| format!("read {}", path.display()))?
            {
                let sub_entry = sub_entry?;
                if !sub_entry.file_type()?.is_dir() {
                    continue;
                }
                let sub_path = sub_entry.path();
                if sub_path.join("SKILL.md").is_file() {
                    found.push((
                        sub_entry.file_name().to_string_lossy().to_string(),
                        sub_path,
                    ));
                }
            }
        } else if path.join("SKILL.md").is_file() {
            found.push((name, path));
        }
    }
    found.sort();
    Ok(found)
}

fn curated_skill_dirs(agent: &Agent) -> Vec<(String, PathBuf)> {
    let mut found: Vec<(String, PathBuf)> = agent
        .role
        .skills
        .iter()
        .filter_map(|skill| {
            let dir = Path::new(&skill.path).parent()?.to_path_buf();
            let name = dir.file_name()?.to_string_lossy().to_string();
            Some((name, dir))
        })
        .collect();
    found.sort();
    found.dedup_by(|left, right| left.0 == right.0);
    found
}

/// pi presence must be judged from a marker roster sync never writes
/// itself — `.pi/agents/orchestrator.md` is always materialized
/// unconditionally (existing behavior, matching claude/codex), so it would
/// make every second sync run see "pi present" from its own prior side
/// effect. `.pi/settings.json` and `.pi/skills` are pi's own native
/// surfaces; either existing means pi genuinely runs on this machine.
fn pi_present(home: &Path) -> bool {
    home.join(".pi/settings.json").exists() || home.join(".pi/skills").exists()
}

/// `~/.config/opencode/opencode.json` is opencode's own native config file,
/// never written by roster sync — its presence means opencode genuinely
/// runs on this machine.
fn opencode_present(home: &Path) -> bool {
    home.join(".config/opencode/opencode.json").exists()
}

fn detect_skill_harness_dirs(home: &Path) -> Vec<String> {
    let mut dirs = vec![".claude/skills".to_string(), ".codex/skills".to_string()];
    if pi_present(home) {
        dirs.push(".pi/skills".to_string());
    }
    dirs
}

fn skills_index_json(agent: &roster_core::Agent) -> Result<String> {
    let skills = agent
        .role
        .skills
        .iter()
        .map(|skill| {
            json!({
                "name": skill.name,
                "path": skill.path,
                "reason": skill.reason,
            })
        })
        .collect::<Vec<_>>();

    let value = json!({
        "schema_version": "roster.sync.skills.v1",
        "phase": "P2-reference-only",
        "agent": agent.role.name,
        "note": "Skill bodies remain in harness-kit until the P3 primitives migration; this is the curated orchestrator subset.",
        "skills": skills,
        "mcps": agent.role.mcps,
    });

    Ok(format!("{}\n", serde_json::to_string_pretty(&value)?))
}

fn rollback_doc() -> String {
    format!(
        r#"{SYNC_MARKER}
# Roster Orchestrator Sync Rollback

Run:

```sh
roster sync --disable
```

For a staged install root, pass the same home used during install:

```sh
roster sync --home <path> --disable
```

The disable path removes only files and symlinks recorded in
`.roster/orchestrator/manifest.json`. It leaves anything roster sync
declined to touch (unmanaged real files, foreign symlinks) untouched.
"#
    )
}

fn managed_markdown(contents: &str) -> String {
    // Claude Code agent files require frontmatter at byte 0 — a marker line
    // before the opening `---` makes the harness silently ignore the agent
    // (found live: fresh sessions listed no roster agents). When the content
    // opens with a frontmatter block, the marker goes right after it;
    // ownership detection is `contains`, so position is free.
    if let Some(end) = contents
        .strip_prefix("---\n")
        .and_then(|rest| rest.find("\n---\n"))
    {
        let split = 4 + end + 5;
        let (front, body) = contents.split_at(split);
        return format!("{front}{SYNC_MARKER}\n{body}");
    }
    format!("{SYNC_MARKER}\n{contents}")
}

fn write_managed_file(home: &Path, relative_path: &str, contents: &str) -> Result<()> {
    let path = safe_home_path(home, relative_path)?;
    if path.exists() && !is_roster_state_path(relative_path) {
        let existing = fs::read_to_string(&path)
            .with_context(|| format!("inspect existing {}", path.display()))?;
        if !existing.contains(SYNC_MARKER) {
            bail!(
                "{} already exists and is not managed by roster sync",
                path.display()
            );
        }
    }

    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent).with_context(|| format!("create {}", parent.display()))?;
    }
    fs::write(&path, contents).with_context(|| format!("write {}", path.display()))?;
    Ok(())
}

enum SymlinkOutcome {
    Applied,
    Skipped(String),
}

/// Writes (or self-heals) a managed symlink at `relative_path` pointing at
/// `target`. Refuses to clobber anything it does not recognize as its own:
/// a real unmanaged file, or a symlink neither dangling, nor into
/// harness-kit, nor previously roster-managed.
fn sync_symlink(
    home: &Path,
    relative_path: &str,
    target: &Path,
    prior_symlinks: &BTreeSet<String>,
) -> Result<SymlinkOutcome> {
    let path = safe_home_path(home, relative_path)?;

    match fs::symlink_metadata(&path) {
        Err(_) => {
            create_symlink(home, &path, target)?;
            Ok(SymlinkOutcome::Applied)
        }
        Ok(meta) if meta.file_type().is_symlink() => {
            let existing_target = fs::read_link(&path)
                .with_context(|| format!("read existing symlink {}", path.display()))?;
            if existing_target == target {
                return Ok(SymlinkOutcome::Applied);
            }
            let dangling = !existing_target.exists();
            let into_harness_kit = existing_target.to_string_lossy().contains("harness-kit");
            let previously_managed = prior_symlinks.contains(relative_path);
            if dangling || into_harness_kit || previously_managed {
                fs::remove_file(&path)
                    .with_context(|| format!("remove stale symlink {}", path.display()))?;
                create_symlink(home, &path, target)?;
                Ok(SymlinkOutcome::Applied)
            } else {
                Ok(SymlinkOutcome::Skipped(format!(
                    "{relative_path} already exists as an unmanaged symlink -> {}",
                    existing_target.display()
                )))
            }
        }
        Ok(meta) if meta.is_file() => {
            let existing = fs::read_to_string(&path).unwrap_or_default();
            if existing.contains(SYNC_MARKER) {
                fs::remove_file(&path)
                    .with_context(|| format!("remove managed file {}", path.display()))?;
                create_symlink(home, &path, target)?;
                Ok(SymlinkOutcome::Applied)
            } else {
                Ok(SymlinkOutcome::Skipped(format!(
                    "{relative_path} already exists and is not managed by roster sync"
                )))
            }
        }
        Ok(_) => Ok(SymlinkOutcome::Skipped(format!(
            "{relative_path} already exists as a real directory; left in place"
        ))),
    }
}

#[cfg(unix)]
fn create_symlink(_home: &Path, path: &Path, target: &Path) -> Result<()> {
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent).with_context(|| format!("create {}", parent.display()))?;
    }
    std::os::unix::fs::symlink(target, path)
        .with_context(|| format!("symlink {} -> {}", path.display(), target.display()))
}

#[cfg(not(unix))]
fn create_symlink(_home: &Path, _path: &Path, _target: &Path) -> Result<()> {
    bail!("roster sync's skill/doctrine symlink farm is only supported on unix targets")
}

struct ManifestInfo {
    files: BTreeSet<String>,
    symlinks: BTreeSet<String>,
}

fn read_manifest(home: &Path) -> Result<Option<ManifestInfo>> {
    let manifest_path = home.join(MANIFEST_REL);
    if !manifest_path.exists() {
        return Ok(None);
    }
    let text = fs::read_to_string(&manifest_path)
        .with_context(|| format!("read {}", manifest_path.display()))?;
    let value: Value = serde_json::from_str(&text)
        .with_context(|| format!("parse {}", manifest_path.display()))?;
    let files = manifest_file_paths(&value)?.into_iter().collect();
    let symlinks = value
        .get("symlinks")
        .and_then(Value::as_array)
        .map(|items| {
            items
                .iter()
                .filter_map(Value::as_str)
                .map(ToOwned::to_owned)
                .collect()
        })
        .unwrap_or_default();
    Ok(Some(ManifestInfo { files, symlinks }))
}

/// Removes anything the previous run managed that this run did not
/// re-plan — a symlink whose target left the current catalog (curated
/// shrank, or a skill was renamed), or a doctrine link this run skipped
/// for a different reason. Only ever removes paths this run recognizes as
/// still roster-owned (symlink-kind self-check), matching the same guard
/// `sync_symlink` and `disable_sync` apply.
fn cleanup_stale(
    home: &Path,
    prior: Option<&ManifestInfo>,
    written_files: &[String],
    written_symlinks: &[String],
) -> Result<()> {
    let Some(prior) = prior else {
        return Ok(());
    };
    let still_current: BTreeSet<&str> = written_files
        .iter()
        .chain(written_symlinks.iter())
        .map(String::as_str)
        .collect();

    for relative_path in prior.files.iter().chain(prior.symlinks.iter()) {
        if still_current.contains(relative_path.as_str()) || is_roster_state_path(relative_path) {
            continue;
        }
        let path = safe_home_path(home, relative_path)?;
        let Ok(meta) = fs::symlink_metadata(&path) else {
            continue;
        };
        if prior.symlinks.contains(relative_path) {
            if meta.file_type().is_symlink() {
                fs::remove_file(&path)
                    .with_context(|| format!("remove stale managed symlink {}", path.display()))?;
            }
        } else if meta.is_file() {
            let existing = fs::read_to_string(&path).unwrap_or_default();
            if existing.contains(SYNC_MARKER) {
                fs::remove_file(&path)
                    .with_context(|| format!("remove stale managed file {}", path.display()))?;
            }
        }
    }
    Ok(())
}

fn disable_sync(home: &Path) -> Result<()> {
    let manifest_path = home.join(MANIFEST_REL);
    if !manifest_path.exists() {
        println!(
            "No roster orchestrator sync manifest at {}; nothing to disable.",
            manifest_path.display()
        );
        return Ok(());
    }

    let manifest_text = fs::read_to_string(&manifest_path)
        .with_context(|| format!("read {}", manifest_path.display()))?;
    let manifest: Value = serde_json::from_str(&manifest_text)
        .with_context(|| format!("parse {}", manifest_path.display()))?;
    let files = manifest_file_paths(&manifest)?;
    let symlink_set: BTreeSet<String> = manifest
        .get("symlinks")
        .and_then(Value::as_array)
        .map(|items| {
            items
                .iter()
                .filter_map(Value::as_str)
                .map(ToOwned::to_owned)
                .collect()
        })
        .unwrap_or_default();

    let mut removed = Vec::new();
    let mut skipped = Vec::new();
    for relative_path in files
        .iter()
        .filter(|relative_path| !is_roster_state_path(relative_path))
    {
        let path = safe_home_path(home, relative_path)?;
        let Ok(meta) = fs::symlink_metadata(&path) else {
            continue;
        };

        if symlink_set.contains(relative_path) {
            if meta.file_type().is_symlink() {
                fs::remove_file(&path).with_context(|| format!("remove {}", path.display()))?;
                removed.push(relative_path.clone());
            } else {
                skipped.push(format!(
                    "{relative_path} (expected roster-managed symlink, found real path; left in place)"
                ));
            }
            continue;
        }

        if !meta.is_file() {
            skipped.push(format!(
                "{relative_path} (expected roster-managed file, found other path type; left in place)"
            ));
            continue;
        }

        let existing = fs::read_to_string(&path)
            .with_context(|| format!("inspect existing {}", path.display()))?;
        if existing.contains(SYNC_MARKER) {
            fs::remove_file(&path).with_context(|| format!("remove {}", path.display()))?;
            removed.push(relative_path.clone());
        } else {
            skipped.push(format!("{relative_path} (not roster-managed)"));
        }
    }

    let sync_dir = home.join(SYNC_DIR_REL);
    if sync_dir.exists() {
        fs::remove_dir_all(&sync_dir).with_context(|| format!("remove {}", sync_dir.display()))?;
        removed.push(SYNC_DIR_REL.to_string());
    }

    println!("Disabled roster orchestrator sync.");
    if !removed.is_empty() {
        println!("Removed:");
        for relative_path in removed {
            println!("- {relative_path}");
        }
    }
    if !skipped.is_empty() {
        println!("Skipped:");
        for relative_path in skipped {
            println!("- {relative_path}");
        }
    }

    Ok(())
}

fn manifest_file_paths(manifest: &Value) -> Result<Vec<String>> {
    let files = manifest
        .get("files")
        .and_then(Value::as_array)
        .ok_or_else(|| anyhow!("sync manifest missing files array"))?;
    files
        .iter()
        .map(|value| {
            value
                .as_str()
                .map(ToOwned::to_owned)
                .ok_or_else(|| anyhow!("sync manifest contains a non-string file path"))
        })
        .collect()
}

fn safe_home_path(home: &Path, relative_path: &str) -> Result<PathBuf> {
    let path = Path::new(relative_path);
    if path.is_absolute()
        || path.components().any(|component| {
            matches!(
                component,
                Component::ParentDir | Component::Prefix(_) | Component::RootDir
            )
        })
    {
        bail!("refusing unsafe sync path {relative_path:?}");
    }
    Ok(home.join(path))
}

fn is_roster_state_path(relative_path: &str) -> bool {
    relative_path == SYNC_DIR_REL || relative_path.starts_with(".roster/orchestrator/")
}
