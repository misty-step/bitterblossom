use std::path::PathBuf;

use anyhow::{bail, Result};
use bitterblossom::{dispatch, ledger, recovery, serve, spec};
use clap::{Parser, Subcommand};

use ledger::{IngressRequest, Ledger};
use spec::Plane;

#[derive(Parser)]
#[command(
    name = "bb",
    version,
    about = "Bitterblossom event plane: tasks + agents + triggers as files"
)]
struct Cli {
    /// Plane config directory (contains plane.toml). Defaults to
    /// $BB_PLANE_DIR or the current directory.
    #[arg(long, global = true)]
    config: Option<PathBuf>,
    #[command(subcommand)]
    command: Command,
}

#[derive(Subcommand)]
enum Command {
    /// Trigger a task manually (the degenerate trigger).
    Run {
        task: String,
        /// Dedupe key: a second run with the same key records an ingress
        /// event but creates no second run.
        #[arg(long)]
        idempotency_key: Option<String>,
        #[arg(long)]
        json: bool,
    },
    /// Inspect the run ledger.
    Runs {
        #[command(subcommand)]
        command: RunsCommand,
    },
    /// Dead letters: dispatches that failed before executing.
    Dlq {
        #[command(subcommand)]
        command: DlqCommand,
    },
    /// Park/unpark tasks (budget breaches park; unpark is explicit).
    Task {
        #[command(subcommand)]
        command: TaskCommand,
    },
    /// Validate the plane config and print what's loaded.
    Check {
        #[arg(long)]
        json: bool,
    },
    /// Classify runs inherited in `running` state (host probe + attempt
    /// phase), instead of blindly orphaning them. Also runs at serve boot.
    Recover {
        #[arg(long)]
        json: bool,
    },
    /// Run the plane: boot recovery, webhook ingress, cron scheduler,
    /// dispatch loop.
    Serve,
}

#[derive(Subcommand)]
enum RunsCommand {
    List {
        #[arg(long)]
        task: Option<String>,
        #[arg(long)]
        state: Option<String>,
        #[arg(long)]
        json: bool,
    },
    Show {
        run_id: String,
        #[arg(long)]
        json: bool,
    },
    /// Flat JSONL dump (run + attempts per line) for downstream analysis.
    Export,
    /// Resolve an `awaiting_recovery` run after inspecting its side
    /// effects (an operator judgment the plane refuses to make).
    Resolve {
        run_id: String,
        #[arg(value_parser = ["success", "failure"])]
        outcome: String,
        #[arg(long, default_value = "resolved by operator")]
        reason: String,
    },
}

#[derive(Subcommand)]
enum DlqCommand {
    List {
        #[arg(long)]
        json: bool,
    },
    /// Mint a new run linked to the dead-lettered one via parent_run_id.
    Replay { id: i64 },
}

#[derive(Subcommand)]
enum TaskCommand {
    Park {
        task: String,
        #[arg(long, default_value = "parked by operator")]
        reason: String,
    },
    Unpark {
        task: String,
    },
}

fn main() {
    // Die quietly on SIGPIPE (e.g. `bb runs export | head`) like other
    // Unix CLIs instead of panicking on a broken-pipe write.
    unsafe {
        libc::signal(libc::SIGPIPE, libc::SIG_DFL);
    }
    if let Err(e) = run() {
        eprintln!("error: {e:#}");
        std::process::exit(1);
    }
}

fn run() -> Result<()> {
    let cli = Cli::parse();
    let root = cli
        .config
        .or_else(|| std::env::var_os("BB_PLANE_DIR").map(PathBuf::from))
        .unwrap_or_else(|| PathBuf::from("."));
    let plane = Plane::load(&root)?;
    let mut ledger = Ledger::open(&plane.db_path())?;

    match cli.command {
        Command::Run {
            task,
            idempotency_key,
            json,
        } => {
            plane.task(&task)?;
            let outcome = ledger.ingest(IngressRequest {
                task: &task,
                trigger_kind: "manual",
                idempotency_key: idempotency_key.as_deref(),
                source_event_id: None,
                payload: None,
                parent_run_id: None,
            })?;
            if outcome.duplicate {
                eprintln!(
                    "duplicate idempotency key; existing run {} ({})",
                    outcome.run_id, outcome.state
                );
                print_run(&ledger, &outcome.run_id, json)?;
                return Ok(());
            }
            if outcome.state == "blocked_budget" {
                eprintln!("run {} blocked: task is parked", outcome.run_id);
                print_run(&ledger, &outcome.run_id, json)?;
                return Ok(());
            }
            let run = dispatch::dispatch_run(&plane, &mut ledger, &outcome.run_id)?;
            if json {
                print_run(&ledger, &run.id, true)?;
            } else {
                println!(
                    "run {} {} (task={} agent={}@v{} cost={} duration_ms={})",
                    run.id,
                    run.state,
                    run.task,
                    run.agent_name.as_deref().unwrap_or("-"),
                    run.agent_version.unwrap_or(0),
                    run.cost_usd
                        .map(|c| format!("${c:.4}"))
                        .unwrap_or_else(|| "-".into()),
                    run.duration_ms.unwrap_or(0),
                );
                if run.state != "success" {
                    eprintln!("reason: {}", run.state_reason.as_deref().unwrap_or("-"));
                }
            }
            if run.state == "failure" {
                std::process::exit(2);
            }
        }
        Command::Runs { command } => match command {
            RunsCommand::List { task, state, json } => {
                let runs = ledger.list_runs(task.as_deref(), state.as_deref())?;
                if json {
                    println!("{}", serde_json::to_string_pretty(&runs)?);
                } else {
                    for r in runs {
                        let agent = format!(
                            "{}@v{}",
                            r.agent_name.as_deref().unwrap_or("-"),
                            r.agent_version.unwrap_or(0)
                        );
                        println!(
                            "{}  {:<18} {:<10} {:<14} {}  {}",
                            r.created_at, r.task, r.trigger_kind, r.state, agent, r.id,
                        );
                    }
                }
            }
            RunsCommand::Show { run_id, json } => {
                print_run(&ledger, &run_id, json)?;
            }
            RunsCommand::Resolve {
                run_id,
                outcome,
                reason,
            } => {
                ledger.transition(&run_id, &outcome, Some(&reason))?;
                // An Unknown boot probe keeps the host lease until the
                // operator resolves; free it now.
                ledger.release_leases_for_run(&run_id)?;
                println!("run {run_id} resolved: {outcome}");
            }
            RunsCommand::Export => {
                for r in ledger.list_runs(None, None)? {
                    let attempts = ledger.attempts(&r.id)?;
                    let line = serde_json::json!({ "run": r, "attempts": attempts });
                    println!("{line}");
                }
            }
        },
        Command::Dlq { command } => match command {
            DlqCommand::List { json } => {
                let rows = ledger.list_dead_letters()?;
                if json {
                    println!("{}", serde_json::to_string_pretty(&rows)?);
                } else {
                    for d in rows {
                        println!(
                            "{}  #{} run={} task={} replayed={}  {}",
                            d.created_at,
                            d.id,
                            d.run_id,
                            d.task,
                            d.replayed_run_id.as_deref().unwrap_or("-"),
                            d.error,
                        );
                    }
                }
            }
            DlqCommand::Replay { id } => {
                let dl = ledger.dead_letter(id)?;
                if let Some(prev) = &dl.replayed_run_id {
                    bail!("dead letter {id} already replayed as run {prev}");
                }
                // Deterministic key: concurrent replays of the same dead
                // letter dedupe at ingress instead of minting twin runs.
                let replay_key = format!("replay:dl-{id}");
                let outcome = ledger.ingest(IngressRequest {
                    task: &dl.task,
                    trigger_kind: "replay",
                    idempotency_key: Some(&replay_key),
                    source_event_id: None,
                    payload: dl.payload.as_deref(),
                    parent_run_id: Some(&dl.run_id),
                })?;
                if !ledger.mark_dead_letter_replayed(id, &outcome.run_id)? {
                    bail!("dead letter {id} was claimed by a concurrent replay");
                }
                println!("replaying dead letter {id} as run {}", outcome.run_id);
                // Always attempt dispatch — if a previous replay crashed
                // after minting the run, this picks the pending run up; if
                // it is already running/terminal, the atomic claim inside
                // dispatch_run walks away without a second attempt.
                let run = dispatch::dispatch_run(&plane, &mut ledger, &outcome.run_id)?;
                println!("run {} {}", run.id, run.state);
            }
        },
        Command::Task { command } => match command {
            TaskCommand::Park { task, reason } => {
                plane.task(&task)?;
                ledger.park_task(&task, &reason)?;
                ledger.record_budget_event(Some(&task), "parked", &reason)?;
                println!("parked {task}");
            }
            TaskCommand::Unpark { task } => {
                let released = ledger.unpark_task(&task)?;
                ledger.record_budget_event(Some(&task), "unparked", "operator unpark")?;
                println!(
                    "unparked {task}; {} blocked run(s) now pending",
                    released.len()
                );
                for id in released {
                    println!("  {id}");
                }
            }
        },
        Command::Recover { json } => {
            let reports = recovery::recover_inherited_runs(&plane, &mut ledger)?;
            if json {
                println!("{}", serde_json::to_string_pretty(&reports)?);
            } else if reports.is_empty() {
                println!("no inherited running runs");
            } else {
                for r in &reports {
                    println!(
                        "run {} task={} phase={} probe={} -> {}",
                        r.run_id,
                        r.task,
                        r.attempt_phase.as_deref().unwrap_or("-"),
                        r.probe.as_deref().unwrap_or("-"),
                        r.disposition,
                    );
                }
            }
        }
        Command::Serve => {
            drop(ledger);
            serve::serve(&plane.root)?;
        }
        Command::Check { json } => {
            if json {
                let summary = serde_json::json!({
                    "root": plane.root,
                    "db_path": plane.db_path(),
                    "agents": plane.agents.keys().collect::<Vec<_>>(),
                    "tasks": plane.tasks.keys().collect::<Vec<_>>(),
                });
                println!("{}", serde_json::to_string_pretty(&summary)?);
            } else {
                println!("plane root: {}", plane.root.display());
                println!("db: {}", plane.db_path().display());
                for (name, a) in &plane.agents {
                    println!("agent {name}@v{}: {} {}", a.version, a.harness, a.model);
                }
                for (name, t) in &plane.tasks {
                    println!(
                        "task {name}: agent={} substrate={} triggers={}",
                        t.agent_name,
                        t.spec.substrate,
                        t.spec.triggers.len(),
                    );
                }
            }
        }
    }
    Ok(())
}

fn print_run(ledger: &Ledger, run_id: &str, json: bool) -> Result<()> {
    let run = ledger.run(run_id)?;
    let attempts = ledger.attempts(run_id)?;
    let events = ledger.events(run_id)?;
    if json {
        let doc = serde_json::json!({ "run": run, "attempts": attempts, "events": events });
        println!("{}", serde_json::to_string_pretty(&doc)?);
    } else {
        println!(
            "run {}  task={}  state={}  trigger={}",
            run.id, run.task, run.state, run.trigger_kind
        );
        if let Some(reason) = &run.state_reason {
            println!("reason: {reason}");
        }
        println!(
            "trace={}  parent={}  cost={}  duration_ms={}",
            run.trace_id,
            run.parent_run_id.as_deref().unwrap_or("-"),
            run.cost_usd
                .map(|c| format!("${c:.4}"))
                .unwrap_or_else(|| "-".into()),
            run.duration_ms.unwrap_or(0),
        );
        for a in attempts {
            println!(
                "  attempt {} {}@v{} {} {} phase={} outcome={} cost={} artifacts={}",
                a.n,
                a.agent_name,
                a.agent_version,
                a.harness,
                a.model,
                a.phase,
                a.outcome.as_deref().unwrap_or("-"),
                a.cost_usd
                    .map(|c| format!("${c:.4}"))
                    .unwrap_or_else(|| "-".into()),
                a.artifact_dir.as_deref().unwrap_or("-"),
            );
            if let Some(err) = a.error {
                println!("    error: {err}");
            }
        }
        for e in events {
            println!("  {} {} {}", e.at, e.kind, e.data.as_deref().unwrap_or(""));
        }
    }
    Ok(())
}
