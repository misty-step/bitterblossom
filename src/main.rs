use std::{path::PathBuf, thread, time::Duration};

use anyhow::{bail, Context, Result};
use bitterblossom::{artifacts, budget, dispatch, ledger, mcp, recovery, serve, spec};
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
    #[arg(long, global = true)]
    config: Option<PathBuf>,
    #[command(subcommand)]
    command: Command,
}

#[derive(Subcommand)]
enum Command {
    /// Dispatch a task manually from a terminal. Ingests the event (idempotent
    /// per `--idempotency-key`), then runs it to completion. `--payload` is
    /// validated as JSON *before* a run row is created; invalid JSON exits
    /// non-zero and leaves the ledger untouched. `--json` prints only the
    /// final run bundle; human mode prints an early run id plus stderr
    /// heartbeats while dispatch is in progress.
    Run {
        task: String,
        #[arg(long)]
        idempotency_key: Option<String>,
        /// Inline JSON payload for the run. Validated as JSON before ingest.
        #[arg(long)]
        payload: Option<String>,
        /// Path to a file holding the JSON payload. Read and validated as
        /// JSON before ingest. Mutually exclusive with `--payload`.
        #[arg(long, conflicts_with = "payload")]
        payload_file: Option<PathBuf>,
        #[arg(long)]
        json: bool,
    },
    /// Durable run ledger: list, show, export, cancel, resolve, release, retire.
    Runs {
        #[command(subcommand)]
        command: RunsCommand,
    },
    /// Dead-letter queue: list, replay, acknowledge. Pre-execute failures only;
    /// post-execute failures are operator-resolved via `runs resolve`.
    Dlq {
        #[command(subcommand)]
        command: DlqCommand,
    },
    /// Task inventory and budget parking. `list --json` is the agent-facing
    /// task surface (agent, substrate, triggers, verdict, parked, caps).
    Task {
        #[command(subcommand)]
        command: TaskCommand,
    },
    /// Submission-loop merge gate: list/open changes, reject findings, abandon.
    Submit {
        #[command(subcommand)]
        command: SubmitCommand,
    },
    /// Evaluate the submission gate for a change or submission id. Pure
    /// arithmetic over recorded verdicts: `clear` proceeds, `blocked` names
    /// the fix prompt, `escalated` fires past max_rounds. Read-only.
    Gate {
        #[arg(long)]
        submission: Option<String>,
        #[arg(long)]
        change: Option<String>,
        #[arg(long)]
        json: bool,
    },
    /// Operator truth: tasks, runs by state, parked tasks, open DLQs, and the
    /// safe next action for each. Read-only; the stale-recovery surface.
    Status {
        #[arg(long)]
        json: bool,
    },
    /// Validate the config surface: agents, tasks, substrates, budget caps,
    /// and auth policy. Read-only. `--json` emits agent/task lists and the
    /// task view.
    Check {
        #[arg(long)]
        json: bool,
    },
    /// Classify runs inherited from a dead plane (probe, no orphaning):
    /// transitions `running`/`pending` into `awaiting_recovery` for operator
    /// resolution. Read-only inspection.
    Recover {
        #[arg(long)]
        json: bool,
    },
    /// Run the plane: webhook ingress, cron scheduler, queue, dispatch.
    Serve,
    /// Report missing declared secrets and unspawnable command-harness
    /// binaries before dispatch creates run rows. Targets one task or the
    /// submission-storm member set (the gate-required verdict tasks).
    /// Read-only report; non-zero exit when findings exist.
    Preflight {
        task: Option<String>,
        #[arg(long)]
        storm: bool,
        #[arg(long)]
        json: bool,
    },
    /// Inspect run artifacts without spelunking attempt directories. `list`
    /// enumerates artifact files across a run's attempts; `read` prints a
    /// safe text/JSON artifact such as REPORT.json. Binary and oversized
    /// artifacts are rejected with structured errors; unsafe paths fail.
    Artifacts {
        #[command(subcommand)]
        command: ArtifactsCommand,
    },
    /// Read-only MCP (Model Context Protocol) stdio server. `serve` speaks
    /// JSON-RPC 2.0 over stdin/stdout with no network listener, exposing
    /// read-only tools (`bb_status`, `bb_check`) backed by the same view
    /// helpers as the CLI/API. No mutating tools in this slice.
    Mcp {
        #[command(subcommand)]
        command: McpCommand,
    },
}

#[derive(Subcommand)]
enum RunsCommand {
    /// List runs, optionally filtered by task or state. `--json` emits the
    /// versioned RunRow shape (state, agent@version, cost, duration, timestamps).
    List {
        #[arg(long)]
        task: Option<String>,
        #[arg(long)]
        state: Option<String>,
        #[arg(long)]
        json: bool,
    },
    /// Show one run with its attempts and event timeline. `--json` emits the
    /// run bundle `{ run, attempts, events }`.
    Show {
        run_id: String,
        #[arg(long)]
        json: bool,
    },
    /// Emit bb.run_telemetry.v1 JSONL for every run (Daedalus/OTel adapters).
    Export,
    /// Cancel one pending run. Refused once a dispatcher has claimed it.
    Cancel {
        run_id: String,
        #[arg(long, default_value = "canceled by operator")]
        reason: String,
    },
    /// Resolve `awaiting_recovery` after operator side-effect inspection.
    Resolve {
        run_id: String,
        #[arg(value_parser = ["success", "failure"])]
        outcome: String,
        #[arg(long, default_value = "resolved by operator")]
        reason: String,
    },
    /// Re-queue one budget-blocked run; clears the task park, leaves other blocked runs.
    Release {
        run_id: String,
        #[arg(long, default_value = "released by operator")]
        reason: String,
    },
    /// Retire one budget-blocked run as intentionally not-to-run; keeps ledger history.
    Retire {
        run_id: String,
        #[arg(long)]
        reason: String,
    },
}

#[derive(Subcommand)]
enum DlqCommand {
    /// List dead letters with status (`open`, `replayed`, `acknowledged`),
    /// replay lineage, and acknowledgement reason/timestamp. `--json` emits
    /// the versioned DeadLetterRow shape.
    List {
        #[arg(long)]
        json: bool,
    },
    /// Replay a dead letter as a new run with lineage (`parent_run_id` set).
    /// Refused once replayed or acknowledged — the two are mutually exclusive.
    Replay {
        id: i64,
        #[arg(long)]
        json: bool,
    },
    /// Acknowledge a pre-execute dead letter as superseded without replaying
    /// it. Replay history is preserved; an acknowledged dead letter cannot be
    /// replayed. Requires an explicit operator reason.
    Ack {
        id: i64,
        #[arg(long)]
        reason: String,
        #[arg(long)]
        json: bool,
    },
}

#[derive(Subcommand)]
enum SubmitCommand {
    /// List recent submissions with verdict rows and rejections. `--json`
    /// emits the versioned submission bundle used by agents/supervisors.
    List {
        #[arg(long, default_value_t = 20)]
        limit: u32,
        #[arg(long)]
        json: bool,
    },
    Open {
        #[arg(long)]
        change: String,
        #[arg(long)]
        rev: String,
        #[arg(long)]
        context: Option<String>,
        #[arg(long)]
        json: bool,
    },
    Reject {
        #[arg(long)]
        change: String,
        #[arg(long)]
        fingerprint: String,
        #[arg(long)]
        reason: String,
    },
    Abandon {
        #[arg(long)]
        change: String,
    },
}

#[derive(Subcommand)]
enum TaskCommand {
    /// List tasks with agent, substrate, trigger count, verdict, and parked
    /// state. `--json` is the agent-facing task inventory surface.
    List {
        #[arg(long)]
        json: bool,
    },
    /// Park a task (budget breach or operator pause). Blocks new runs;
    /// existing blocked runs stay queued.
    Park {
        task: String,
        #[arg(long, default_value = "parked by operator")]
        reason: String,
    },
    /// Unpark a task and re-queue its blocked-budget runs.
    Unpark { task: String },
}

#[derive(Subcommand)]
enum ArtifactsCommand {
    /// List artifact files (attempt, path, size, content type, binary flag)
    /// across a run's attempts. `--json` emits the versioned ArtifactEntry
    /// array; human mode prints a compact table.
    List {
        run_id: String,
        #[arg(long)]
        json: bool,
    },
    /// Print a safe text/JSON artifact from the newest attempt that has it.
    /// Binary and oversized artifacts are rejected; unsafe paths fail.
    /// Default prints the raw artifact to stdout; `--json` emits a
    /// `{kind, ...}` envelope and exits non-zero on non-text outcomes.
    Read {
        run_id: String,
        path: String,
        #[arg(long)]
        json: bool,
    },
}
#[derive(Subcommand)]
enum McpCommand {
    /// Run the read-only MCP stdio server. Reads newline-delimited JSON-RPC
    /// from stdin and writes one response per request to stdout.
    Serve,
}

fn main() {
    unsafe { libc::signal(libc::SIGPIPE, libc::SIG_DFL) };
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
            payload,
            payload_file,
            json,
        } => {
            plane.task(&task)?;
            // Resolve payload from --payload or --payload-file (clap enforces
            // mutual exclusion), then validate as JSON *before* ingest so a
            // malformed payload never creates a run row.
            let (payload, payload_source) = match (payload, payload_file) {
                (Some(p), None) => (Some(p), "--payload"),
                (None, Some(path)) => (
                    Some(
                        std::fs::read_to_string(&path)
                            .with_context(|| format!("read payload file {}", path.display()))?,
                    ),
                    "--payload-file",
                ),
                (None, None) => (None, "payload"),
                (Some(_), Some(_)) => {
                    unreachable!("clap enforces --payload/--payload-file exclusion")
                }
            };
            if let Some(p) = &payload {
                serde_json::from_str::<serde_json::Value>(p)
                    .with_context(|| format!("{payload_source} is not valid JSON"))?;
            }
            let outcome = ledger.ingest(IngressRequest {
                task: &task,
                trigger_kind: "manual",
                idempotency_key: idempotency_key.as_deref(),
                source_event_id: None,
                payload: payload.as_deref(),
                parent_run_id: None,
            })?;
            if outcome.duplicate && outcome.state != "pending" {
                eprintln!(
                    "duplicate idempotency key; existing run {} ({})",
                    outcome.run_id, outcome.state
                );
                print_run(&ledger, &outcome.run_id, json)?;
                return Ok(());
            }
            if outcome.duplicate {
                eprintln!(
                    "duplicate idempotency key; dispatching pending run {}",
                    outcome.run_id
                );
            }
            if outcome.state == "blocked_budget" {
                eprintln!("run {} blocked: task is parked", outcome.run_id);
                print_run(&ledger, &outcome.run_id, json)?;
                return Ok(());
            }
            if !json {
                start_run_progress(plane.db_path(), outcome.run_id.clone());
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
        Command::Runs { command } => {
            match command {
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
                RunsCommand::Cancel { run_id, reason } => {
                    let state = ledger.run(&run_id)?.state;
                    if state != "pending" {
                        bail!("run {run_id} is {state}; only pending runs can be canceled");
                    }
                    if !ledger.try_transition(
                        &run_id,
                        "failure",
                        Some(&format!("canceled: {reason}")),
                    )? {
                        bail!("run {run_id} was claimed by a dispatcher before cancel");
                    }
                    println!("run {run_id} canceled");
                }
                RunsCommand::Resolve {
                    run_id,
                    outcome,
                    reason,
                } => {
                    ledger.transition(&run_id, &outcome, Some(&reason))?;
                    ledger.release_leases_for_run(&run_id)?;
                    println!("run {run_id} resolved: {outcome}");
                }
                RunsCommand::Release { run_id, reason } => {
                    let task = plane.task(&ledger.run(&run_id)?.task)?;
                    if let Some(v) = budget::budget_limits(&plane, &ledger, task)? {
                        bail!("cannot release {run_id}: {} — retire it or wait for the limit to reset", v.detail);
                    }
                    ledger.release_blocked_run(&run_id, &reason)?;
                    println!(
                        "released {run_id} -> pending; task unparked, other blocked runs unchanged"
                    );
                }
                RunsCommand::Retire { run_id, reason } => {
                    ledger.retire_blocked_run(&run_id, &reason)?;
                    println!("retired {run_id}; kept in ledger history");
                }
                RunsCommand::Export => {
                    let dead_letters = ledger.list_dead_letters()?;
                    for r in ledger.list_runs(None, None)? {
                        let attempts = ledger.attempts(&r.id)?;
                        let dlq = dead_letters.iter().find(|d| d.run_id == r.id);
                        let line = export_run_telemetry(&plane, r, attempts, dlq);
                        println!("{line}");
                    }
                }
            }
        }
        Command::Artifacts { command } => match command {
            ArtifactsCommand::List { run_id, json } => {
                let entries = match artifacts::list(&ledger, &run_id) {
                    Ok(entries) => entries,
                    Err(e) if json => {
                        print_artifact_json_error(&run_id, None, &e)?;
                        std::process::exit(1);
                    }
                    Err(e) => return Err(e),
                };
                if json {
                    println!("{}", serde_json::to_string_pretty(&entries)?);
                } else if entries.is_empty() {
                    println!("no artifacts recorded for run {run_id}");
                } else {
                    println!("attempt  size      type                 binary  path");
                    for e in &entries {
                        println!(
                            "{:>7}  {:>8}  {:<20} {:<6}  {}",
                            e.attempt, e.size, e.content_type, e.binary, e.path
                        );
                    }
                }
            }
            ArtifactsCommand::Read { run_id, path, json } => {
                let outcome = match artifacts::read(&ledger, &run_id, &path) {
                    Ok(outcome) => outcome,
                    Err(e) if json => {
                        print_artifact_json_error(&run_id, Some(&path), &e)?;
                        std::process::exit(1);
                    }
                    Err(e) => return Err(e),
                };
                if json {
                    println!("{}", serde_json::to_string_pretty(&outcome)?);
                }
                match outcome {
                    artifacts::ReadOutcome::Text { content, .. } => {
                        if !json {
                            print!("{content}");
                        }
                    }
                    artifacts::ReadOutcome::Binary { attempt, size, .. } => {
                        if json {
                            std::process::exit(1);
                        }
                        bail!("artifact {path:?} (attempt {attempt}, {size} bytes) is binary; refused");
                    }
                    artifacts::ReadOutcome::Oversized {
                        attempt,
                        size,
                        limit,
                        ..
                    } => {
                        if json {
                            std::process::exit(1);
                        }
                        bail!(
                            "artifact {path:?} (attempt {attempt}, {size} bytes) exceeds read limit {limit}; refused"
                        );
                    }
                    artifacts::ReadOutcome::Missing { .. } => {
                        if json {
                            std::process::exit(1);
                        }
                        bail!("no artifact {path:?} found in any attempt of run {run_id}");
                    }
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
                            "{}  #{} {} run={} task={}  {}",
                            d.created_at, d.id, d.status, d.run_id, d.task, d.error,
                        );
                        if let Some(r) = &d.replayed_run_id {
                            println!("    replayed as run {r}");
                        }
                        if let Some(reason) = &d.acknowledged_reason {
                            println!("    acknowledged: {reason}");
                        }
                    }
                }
            }
            DlqCommand::Replay { id, json } => {
                let dl = ledger.dead_letter(id)?;
                if let Some(prev) = &dl.replayed_run_id {
                    bail!("dead letter {id} already replayed as run {prev}");
                }
                if let Some(reason) = &dl.acknowledged_reason {
                    bail!(
                        "dead letter {id} acknowledged ({}): replay rejected — acknowledge vs replay are mutually exclusive",
                        reason
                    );
                }
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
                let run = dispatch::dispatch_run(&plane, &mut ledger, &outcome.run_id)?;
                if json {
                    print_run(&ledger, &run.id, true)?;
                } else {
                    println!("replaying dead letter {id} as run {}", outcome.run_id);
                    println!("run {} {}", run.id, run.state);
                }
            }
            DlqCommand::Ack { id, reason, json } => {
                let dl = ledger.acknowledge_dead_letter(id, &reason)?;
                if json {
                    println!("{}", serde_json::to_string_pretty(&dl)?);
                } else {
                    println!(
                        "acknowledged dead letter {id} (run {}): {}",
                        dl.run_id,
                        dl.acknowledged_reason.as_deref().unwrap_or(&reason)
                    );
                }
            }
        },
        Command::Task { command } => match command {
            TaskCommand::List { json } => {
                let rows = serve::tasks_view(&plane, &ledger)?;
                if json {
                    println!("{}", serde_json::to_string_pretty(&rows)?);
                } else {
                    for row in rows {
                        println!("{row}");
                    }
                }
            }
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
        Command::Submit { command } => match command {
            SubmitCommand::List { limit, json } => {
                let submissions = ledger.list_submissions(
                    bitterblossom::submit::clamp_submission_list_limit(i64::from(limit)),
                )?;
                if json {
                    println!("{}", serde_json::to_string_pretty(&submissions)?);
                } else {
                    for bundle in submissions {
                        let sub = bundle.submission;
                        println!(
                            "{}  {:<10} round={} verdicts={} change={} rev={}",
                            sub.id,
                            sub.state,
                            sub.round,
                            bundle.verdicts.len(),
                            sub.change_key,
                            sub.rev
                        );
                    }
                }
            }
            SubmitCommand::Open {
                change,
                rev,
                context,
                json,
            } => {
                let sub = ledger.open_submission(&change, &rev, context.as_deref())?;
                if json {
                    println!("{}", serde_json::to_string_pretty(&sub)?);
                } else {
                    println!(
                        "submission {} change={} rev={} round={}",
                        sub.id, sub.change_key, sub.rev, sub.round
                    );
                }
            }
            SubmitCommand::Reject {
                change,
                fingerprint,
                reason,
            } => {
                ledger.reject_finding(&change, &fingerprint, &reason)?;
                println!("rejected {fingerprint} on {change} (blocking findings stay blocking until an arbiter sustains)");
            }
            SubmitCommand::Abandon { change } => {
                let sub = ledger
                    .latest_submission(&change)?
                    .ok_or_else(|| anyhow::anyhow!("no submissions for change '{change}'"))?;
                if !ledger.settle_submission(&sub.id, "abandoned", "{}")? {
                    bail!("submission {} is {}, not open", sub.id, sub.state);
                }
                println!("submission {} abandoned", sub.id);
            }
        },
        Command::Gate {
            submission,
            change,
            json,
        } => {
            let id = match (submission, change) {
                (Some(id), _) => id,
                (None, Some(change)) => {
                    ledger
                        .latest_submission(&change)?
                        .ok_or_else(|| anyhow::anyhow!("no submissions for change '{change}'"))?
                        .id
                }
                (None, None) => bail!("pass --submission or --change"),
            };
            let report = bitterblossom::submit::evaluate(&plane, &ledger, &id)?;
            if json {
                println!("{}", serde_json::to_string_pretty(&report)?);
            } else {
                println!(
                    "gate {}: {} (round {}/{})",
                    report.change_key, report.decision, report.round, report.max_rounds
                );
                for m in &report.members {
                    println!(
                        "  {:<16} {:<20} cost={}",
                        m.kind,
                        m.status,
                        m.cost_usd
                            .map(|c| format!("${c:.4}"))
                            .unwrap_or_else(|| "-".into()),
                    );
                }
                for f in &report.blocking {
                    println!(
                        "  BLOCKING [{}] {} ({})",
                        f.fingerprint.as_deref().unwrap_or("-"),
                        f.claim,
                        f.file.as_deref().unwrap_or("-"),
                    );
                }
                for f in &report.advisory {
                    println!(
                        "  advisory [{}] {}: {}",
                        f.fingerprint.as_deref().unwrap_or("-"),
                        f.severity,
                        f.claim,
                    );
                }
                for (f, reason) in &report.rejected {
                    println!(
                        "  rejected [{}] {} — {}",
                        f.fingerprint.as_deref().unwrap_or("-"),
                        f.claim,
                        reason,
                    );
                }
            }
        }
        Command::Status { json } => {
            let doc = bitterblossom::health::status_view(&plane, &ledger)?;
            let output = if json {
                serde_json::to_string_pretty(&doc)?
            } else {
                doc["summary"].to_string()
            };
            println!("{output}");
        }
        Command::Recover { json } => {
            let reports = recovery::recover_inherited_runs(&plane, &mut ledger)?;
            let output = if json {
                serde_json::to_string_pretty(&reports)?
            } else {
                format!("recovered {}", reports.len())
            };
            println!("{output}");
        }
        Command::Serve => {
            drop(ledger);
            serve::serve(&plane.root)?;
        }
        Command::Check { json } => {
            if json {
                let summary = serve::check_view(&plane, &ledger)?;
                println!("{}", serde_json::to_string_pretty(&summary)?);
            } else {
                for (name, t) in &plane.tasks {
                    let source = t
                        .source
                        .as_ref()
                        .map(|s| format!(" source={}@{}", s.repo, s.r#ref))
                        .unwrap_or_default();
                    println!("task {name}: agent={}{}", t.agent_name, source);
                }
            }
        }
        Command::Preflight { task, storm, json } => {
            let report = bitterblossom::preflight::run(&plane, task.as_deref(), storm)?;
            if json {
                println!("{}", serde_json::to_string_pretty(&report)?);
            } else if report.findings.is_empty() {
                println!(
                    "preflight ok: {} task(s) checked, no missing secrets or unspawnable binaries",
                    report.tasks_checked.len()
                );
            } else {
                println!(
                    "preflight found {} problem(s) across {} task(s):",
                    report.findings.len(),
                    report.tasks_checked.len()
                );
                for f in &report.findings {
                    println!("  {} [{}] {}", f.task, f.kind, f.detail);
                }
            }
            if !report.findings.is_empty() {
                std::process::exit(2);
            }
        }
        Command::Mcp { command } => match command {
            McpCommand::Serve => {
                drop(ledger);
                mcp::serve_stdio(&plane)?;
            }
        },
    }
    Ok(())
}

fn start_run_progress(db_path: PathBuf, run_id: String) {
    eprintln!("run {run_id} accepted; inspect with `bb runs show {run_id} --json`");
    let ms = std::env::var("BB_RUN_HEARTBEAT_MS")
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(30_000);
    let _ = thread::spawn(move || loop {
        thread::sleep(Duration::from_millis(ms));
        let Ok(ledger) = Ledger::open(&db_path) else {
            continue;
        };
        let Ok(run) = ledger.run(&run_id) else {
            continue;
        };
        eprintln!("run {run_id} heartbeat state={}", run.state);
        if !matches!(run.state.as_str(), "pending" | "running") {
            break;
        }
    });
}

fn export_run_telemetry(
    plane: &Plane,
    r: ledger::RunRow,
    attempts: Vec<ledger::AttemptRow>,
    dlq: Option<&ledger::DeadLetterRow>,
) -> serde_json::Value {
    let dead_status = dlq.map(|d| d.status.as_str()).unwrap_or("none");
    let provider = |name: &str| {
        plane
            .agents
            .get(name)
            .map(|a| a.provider())
            .unwrap_or("unknown")
    };
    let mut attempt_docs = Vec::with_capacity(attempts.len());
    let mut agent_configs = Vec::with_capacity(attempts.len());
    let mut artifacts = Vec::new();
    let mut spans = Vec::with_capacity(attempts.len());
    let (mut input_total, mut output_total) = (0, 0);
    let (mut has_input, mut has_output) = (false, false);
    for a in &attempts {
        let provider = provider(&a.agent_name);
        has_input |= a.tokens_in.is_some();
        has_output |= a.tokens_out.is_some();
        input_total += a.tokens_in.unwrap_or(0);
        output_total += a.tokens_out.unwrap_or(0);
        attempt_docs.push(serde_json::json!({
            "n": a.n, "phase": &a.phase, "outcome": &a.outcome, "error": &a.error,
            "exit_code": a.exit_code,
            "agent": {"name": &a.agent_name, "version": a.agent_version, "harness": &a.harness,
                "provider": provider, "model": &a.model},
            "tokens": {"input": a.tokens_in, "output": a.tokens_out}, "turns": a.turns,
            "cost_usd": a.cost_usd, "artifact_dir": &a.artifact_dir,
            "started_at": &a.started_at, "ended_at": &a.ended_at,
        }));
        agent_configs.push(
            serde_json::json!({"name": &a.agent_name, "version": a.agent_version,
            "harness": &a.harness, "provider": provider, "model": &a.model,
            "outcome": &a.outcome, "cost_usd": a.cost_usd,
            "tokens": {"input": a.tokens_in, "output": a.tokens_out}}),
        );
        if let Some(path) = &a.artifact_dir {
            artifacts.push(
                serde_json::json!({"kind": "attempt_artifact_dir", "attempt": a.n, "path": path}),
            );
        }
        spans.push(serde_json::json!({
            "name": format!("bb.{}.attempt.{}", r.task, a.n), "kind": "internal",
            "start_time": &a.started_at, "end_time": &a.ended_at,
            "attributes": {
                "gen_ai.operation.name": &r.task,
                "gen_ai.provider.name": provider,
                "gen_ai.request.model": &a.model, "gen_ai.response.model": &a.model,
                "gen_ai.agent.name": &a.agent_name,
                "gen_ai.agent.version": a.agent_version.to_string(),
                "gen_ai.usage.input_tokens": a.tokens_in,
                "gen_ai.usage.output_tokens": a.tokens_out,
                "bb.run.id": &r.id, "bb.attempt.n": a.n, "bb.harness": &a.harness
            }
        }));
    }
    let input_tokens = has_input.then_some(input_total);
    let output_tokens = has_output.then_some(output_total);
    let dead_letter = dlq.map_or_else(
        || serde_json::json!({"status": "none"}),
        |d| {
            serde_json::json!({"status": dead_status, "id": d.id, "error": &d.error,
            "created_at": &d.created_at, "replayed_run_id": &d.replayed_run_id,
            "acknowledged_reason": &d.acknowledged_reason, "acknowledged_at": &d.acknowledged_at})
        },
    );
    serde_json::json!({
        "schema": "bb.run_telemetry.v1", "schema_version": 1, "exported_at": ledger::now(),
        "run": {"id": &r.id, "task": &r.task, "state": &r.state, "state_reason": &r.state_reason,
            "trigger": {"kind": &r.trigger_kind, "idempotency_key": &r.idempotency_key},
            "trace_id": &r.trace_id, "parent_run_id": &r.parent_run_id,
            "agent": {"name": &r.agent_name, "version": r.agent_version},
            "config_source": {"repo": &r.config_source_repo, "ref": &r.config_source_ref},
            "cost_usd": r.cost_usd, "tokens": {"input": input_tokens, "output": output_tokens},
            "duration_ms": r.duration_ms, "created_at": &r.created_at, "updated_at": &r.updated_at},
        "attempts": attempt_docs,
        "retry": {"attempt_count": attempts.len(), "mechanical_retry_count": attempts.len().saturating_sub(1),
            "final_phase": attempts.last().map(|a| a.phase.as_str())},
        "dead_letter": dead_letter, "artifacts": artifacts,
        "daedalus": {"source": "bitterblossom", "run_id": &r.id, "task_key": &r.task,
            "trace_id": &r.trace_id, "agent_configs": agent_configs,
            "result": {"state": &r.state, "state_reason": &r.state_reason,
                "duration_ms": r.duration_ms, "dead_letter_status": dead_status}},
        "otel": {"trace_id": &r.trace_id, "spans": spans,
            "metrics": [{"name": "gen_ai.client.operation.duration", "unit": "ms",
                "value": r.duration_ms, "attributes": {"gen_ai.operation.name": &r.task, "bb.run.id": &r.id}}]},
    })
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
        if let Some(repo) = &run.config_source_repo {
            println!(
                "source={}@{}",
                repo,
                run.config_source_ref.as_deref().unwrap_or("-")
            );
        }
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

fn print_artifact_json_error(run_id: &str, path: Option<&str>, err: &anyhow::Error) -> Result<()> {
    let mut doc = serde_json::Map::new();
    doc.insert(
        "kind".into(),
        serde_json::Value::String(artifact_json_error_kind(err).into()),
    );
    doc.insert("run_id".into(), serde_json::Value::String(run_id.into()));
    if let Some(path) = path {
        doc.insert("path".into(), serde_json::Value::String(path.into()));
    }
    doc.insert(
        "message".into(),
        serde_json::Value::String(format!("{err:#}")),
    );
    println!(
        "{}",
        serde_json::to_string_pretty(&serde_json::Value::Object(doc))?
    );
    Ok(())
}

fn artifact_json_error_kind(err: &anyhow::Error) -> &'static str {
    err.downcast_ref::<artifacts::ArtifactError>()
        .map(artifacts::ArtifactError::json_kind)
        .unwrap_or("io_error")
}
