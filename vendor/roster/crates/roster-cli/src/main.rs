use anyhow::{Context, Result, anyhow, bail};
use clap::{Parser, Subcommand, ValueEnum};
use roster_core::{
    CardContext, Providers, Roster, render_bb_agent, render_brief, render_claude_agent, render_show,
};
use serde_json::{Value, json};
use std::{
    fs,
    path::{Component, Path, PathBuf},
};

const DEFAULT_AGENT: &str = "orchestrator";
const SYNC_MARKER: &str = "<!-- roster-sync:orchestrator:v1 -->";
const SYNC_DIR_REL: &str = ".roster/orchestrator";
const MANIFEST_REL: &str = ".roster/orchestrator/manifest.json";

#[derive(Debug, Parser)]
#[command(name = "roster")]
#[command(about = "Materialize roster agent declarations for harnesses")]
struct Cli {
    #[arg(long, default_value = ".")]
    root: PathBuf,
    #[command(subcommand)]
    command: Command,
}

#[derive(Debug, Subcommand)]
enum Command {
    List,
    Show {
        agent: String,
    },
    Materialize {
        agent: String,
        #[arg(long, value_enum)]
        harness: Harness,
    },
    Brief {
        agent: String,
        #[arg(long)]
        card: Option<String>,
        #[arg(long = "add-skill")]
        add_skills: Vec<String>,
        #[arg(long = "add-mcp")]
        add_mcps: Vec<String>,
    },
    Sync {
        #[arg(long)]
        home: Option<PathBuf>,
        #[arg(long)]
        disable: bool,
    },
}

#[derive(Clone, Debug, ValueEnum)]
enum Harness {
    Claude,
    Codex,
    Bb,
}

fn main() -> Result<()> {
    let cli = Cli::parse();

    match cli.command {
        Command::List => {
            let roster = Roster::load(&cli.root)?;
            for agent in roster.agents() {
                println!(
                    "{}\t{}\t{}\t{}",
                    agent.role.name,
                    agent.role.model_policy.preferred,
                    agent.role.model_policy.reasoning,
                    agent.role.description
                );
            }
        }
        Command::Show { agent } => {
            let roster = Roster::load(&cli.root)?;
            let agent = find_agent(&roster, &agent)?;
            print!("{}", render_show(agent));
        }
        Command::Materialize { agent, harness } => {
            let roster = Roster::load(&cli.root)?;
            let agent = find_agent(&roster, &agent)?;
            match harness {
                Harness::Claude => {
                    let providers = Providers::load(&cli.root)?;
                    print!("{}", render_claude_agent(agent, &providers));
                }
                Harness::Codex => print!("{}", render_brief(agent, &[], &[], None)),
                Harness::Bb => {
                    let providers = Providers::load(&cli.root)?;
                    print!(
                        "{}",
                        render_bb_agent(agent, &providers).map_err(|error| anyhow!(error))?
                    );
                }
            }
        }
        Command::Brief {
            agent,
            card,
            add_skills,
            add_mcps,
        } => {
            let roster = Roster::load(&cli.root)?;
            let agent = find_agent(&roster, &agent)?;
            let card = card
                .as_deref()
                .map(fetch_powder_card)
                .transpose()
                .context("failed to fetch Powder card")?;
            print!(
                "{}",
                render_brief(agent, &add_skills, &add_mcps, card.as_ref())
            );
        }
        Command::Sync { home, disable } => run_sync(&cli.root, home, disable)?,
    }

    Ok(())
}

fn find_agent<'a>(roster: &'a Roster, name: &str) -> Result<&'a roster_core::Agent> {
    roster
        .agent(name)
        .ok_or_else(|| anyhow!("unknown agent {name:?}"))
}

fn fetch_powder_card(id: &str) -> Result<CardContext> {
    let base_url = std::env::var("POWDER_API_BASE_URL")
        .context("POWDER_API_BASE_URL is required for --card")?;
    let api_key =
        std::env::var("POWDER_API_KEY").context("POWDER_API_KEY is required for --card")?;
    let url = format!("{}/api/v1/cards/{}", base_url.trim_end_matches('/'), id);
    let response: Value = ureq::get(&url)
        .set("Authorization", &format!("Bearer {api_key}"))
        .call()
        .with_context(|| format!("GET {url}"))?
        .into_json()
        .with_context(|| format!("decode Powder response for {id}"))?;

    let card = response.get("card").unwrap_or(&response);
    let title = card
        .get("title")
        .and_then(Value::as_str)
        .unwrap_or("")
        .to_string();
    let body = card
        .get("body")
        .and_then(Value::as_str)
        .unwrap_or("")
        .to_string();
    let acceptance = card
        .get("acceptance")
        .and_then(Value::as_array)
        .map(|items| {
            items
                .iter()
                .filter_map(Value::as_str)
                .map(ToOwned::to_owned)
                .collect::<Vec<_>>()
        })
        .unwrap_or_default();

    Ok(CardContext {
        id: id.to_string(),
        title,
        body,
        acceptance,
    })
}

fn run_sync(root: &Path, home: Option<PathBuf>, disable: bool) -> Result<()> {
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
        install_sync(root, &home)
    }
}

fn install_sync(root: &Path, home: &Path) -> Result<()> {
    let roster = Roster::load(root)?;
    let agent = find_agent(&roster, DEFAULT_AGENT)?;
    let providers = Providers::load(root)?;
    let files = sync_files(agent, &providers)?;

    for (relative_path, contents) in &files {
        write_managed_file(home, relative_path, contents)?;
    }

    println!(
        "Installed roster orchestrator sync at {}",
        home.join(SYNC_DIR_REL).display()
    );
    println!("Manifest: {}", home.join(MANIFEST_REL).display());
    println!("Disable: roster sync --disable");
    println!("Harness-kit globals were not overwritten.");

    Ok(())
}

fn sync_files(agent: &roster_core::Agent, providers: &Providers) -> Result<Vec<(String, String)>> {
    let brief = render_brief(agent, &[], &[], None);
    let claude_agent = render_claude_agent(agent, providers);
    let skills_index = skills_index_json(agent)?;
    let rollback = rollback_doc();

    let mut files = vec![
        (
            ".roster/orchestrator/brief.md".to_string(),
            managed_markdown(&brief),
        ),
        (
            ".roster/orchestrator/claude.md".to_string(),
            managed_markdown(&claude_agent),
        ),
        (
            ".roster/orchestrator/primitives/skills-index.json".to_string(),
            skills_index,
        ),
        (".roster/orchestrator/ROLLBACK.md".to_string(), rollback),
        (
            ".codex/agents/orchestrator.md".to_string(),
            managed_markdown(&brief),
        ),
        (
            ".claude/agents/orchestrator.md".to_string(),
            managed_markdown(&claude_agent),
        ),
        (
            ".pi/agents/orchestrator.md".to_string(),
            managed_markdown(&brief),
        ),
    ];

    let mut managed_paths = files
        .iter()
        .map(|(relative_path, _)| relative_path.clone())
        .collect::<Vec<_>>();
    managed_paths.push(MANIFEST_REL.to_string());

    let manifest = serde_json::to_string_pretty(&json!({
        "schema_version": "roster.sync.v1",
        "managed_by": "roster sync",
        "agent": agent.role.name,
        "mode": "parallel-run",
        "disable_command": "roster sync --disable",
        "files": managed_paths,
        "preserved_harness_kit_files": [
            ".codex/AGENTS.md",
            ".codex/CLAUDE.md",
            ".claude/CLAUDE.md",
            ".pi/settings.json"
        ]
    }))?;
    files.push((MANIFEST_REL.to_string(), format!("{manifest}\n")));

    Ok(files)
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

The disable path removes only files recorded in `.roster/orchestrator/manifest.json`
and carrying the roster sync marker outside `.roster/orchestrator`.
It leaves harness-kit bootstrap files untouched.
"#
    )
}

fn managed_markdown(contents: &str) -> String {
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

    let mut removed = Vec::new();
    let mut skipped = Vec::new();
    for relative_path in files
        .iter()
        .filter(|relative_path| !is_roster_state_path(relative_path))
    {
        let path = safe_home_path(home, relative_path)?;
        if !path.exists() {
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
