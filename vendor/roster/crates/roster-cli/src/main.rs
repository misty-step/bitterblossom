mod check;
mod sync;

use anyhow::{Context, Result, anyhow};
use clap::{Parser, Subcommand, ValueEnum};
use roster_core::{
    CardContext, Models, Roster, render_bb_agent, render_brief, render_claude_agent, render_show,
};
use serde_json::Value;
use std::path::PathBuf;
use sync::Catalog;

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
        #[arg(long, value_enum, default_value = "full")]
        catalog: Catalog,
        #[arg(long = "all-agents")]
        all_agents: bool,
    },
    /// Gate the primitives catalog: frontmatter shape, referenced-path
    /// existence, skills-index/disk parity, conflict markers.
    Check,
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
                    agent.role.model_policy.preferred.model,
                    agent.role.model_policy.preferred.reasoning,
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
                    let models = Models::load(&cli.root)?;
                    print!("{}", render_claude_agent(agent, &models));
                }
                Harness::Codex => print!("{}", render_brief(agent, &[], &[], None)),
                Harness::Bb => {
                    let models = Models::load(&cli.root)?;
                    print!(
                        "{}",
                        render_bb_agent(agent, &models).map_err(|error| anyhow!(error))?
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
        Command::Sync {
            home,
            disable,
            catalog,
            all_agents,
        } => sync::run(&cli.root, home, disable, catalog, all_agents)?,
        Command::Check => {
            if !check::run(&cli.root)? {
                std::process::exit(1);
            }
        }
    }

    Ok(())
}

pub(crate) fn find_agent<'a>(roster: &'a Roster, name: &str) -> Result<&'a roster_core::Agent> {
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
