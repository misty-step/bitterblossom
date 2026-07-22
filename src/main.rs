use std::{
    collections::{BTreeMap, BTreeSet},
    path::{Path, PathBuf},
    thread,
    time::Duration,
};

use anyhow::{bail, Context, Result};
use bitterblossom::{
    artifacts, ask, budget, canary, dispatch, ingress, ledger, mcp, provider_keys, reap, recovery,
    serve, spec,
};
use clap::{Parser, Subcommand};
use time::{format_description::well_known::Rfc3339, OffsetDateTime};

use ledger::{IngressRequest, Ledger};
use spec::Plane;

const TASK_UNPARK_CONFIRMATION_LIMIT: usize = 1;

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
    /// Enqueue one operator-dispatch job from a repo path and brief file.
    /// The selected task defaults to `BB_DISPATCH_TASK`, then `dispatch`,
    /// then `build`, then a single manual task. Prints the accepted run id
    /// and exits; a running `bb serve` drains the pending run.
    Dispatch {
        #[arg(long)]
        repo: PathBuf,
        #[arg(long)]
        brief: PathBuf,
        #[arg(long)]
        model: Option<String>,
        #[arg(long)]
        label: Option<String>,
    },
    /// Print or follow one run's ledger events and released text artifacts.
    Logs {
        #[arg(short = 'f', long)]
        follow: bool,
        run_id: String,
    },
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
    /// Notification outbox: list retryable/delivered rows, retry pending or
    /// failed webhook deliveries, and acknowledge rows that should not retry.
    Notify {
        #[command(subcommand)]
        command: NotifyCommand,
    },
    /// Scoped provider keys for API-auth agents. OpenRouter management calls
    /// read `OPENROUTER_MANAGEMENT_KEY` from the environment and never print
    /// child key material; child keys are stored under the configured plane's
    /// `.bb/` state and injected per run as the declared provider secret.
    Keys {
        #[command(subcommand)]
        command: KeysCommand,
    },
    /// Task inventory and budget parking. `list --json` is the agent-facing
    /// task surface (agent, substrate, triggers, verdict, parked, caps).
    Task {
        #[command(subcommand)]
        command: TaskCommand,
    },
    /// HITL: raise a question/decision/approval from a running attempt (any
    /// harness, via its own Bash tool) and answer one operator-side.
    Ask {
        #[command(subcommand)]
        command: AskCommand,
    },
    /// Submission-loop merge gate: list/open changes, reject findings, waive
    /// required members, abandon.
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
    /// Reconcile runs inherited from a dead plane (probe, evidence, and
    /// bounded recovery): transitions or dead-letters inherited rows, closes
    /// uncertain steps, and releases leases when recovery proves it is safe.
    Recover {
        #[arg(long)]
        json: bool,
    },
    /// Run the plane: webhook ingress, cron scheduler, queue, dispatch.
    Serve,
    /// Pause reflex dispatch for the whole plane (backlog 083): the
    /// autonomous dispatch loop stops claiming pending runs until `resume`.
    /// Distinct from per-task parking — manual `bb run` still dispatches.
    /// Reason is recorded and visible in `status`.
    Pause {
        #[arg(long, default_value = "paused by operator")]
        reason: String,
    },
    /// Resume reflex dispatch after a `pause`. No-op if not paused.
    Resume,
    /// Report missing declared secrets, unspawnable command-harness binaries,
    /// and subscription-auth readiness before dispatch creates run rows.
    /// Targets one task or the submission-storm member set (the gate-required
    /// verdict tasks). Read-only report; non-zero exit when findings exist.
    Preflight {
        task: Option<String>,
        #[arg(long)]
        storm: bool,
        #[arg(long)]
        json: bool,
    },
    /// Verified-live onboarding gate (application floor, bitterblossom-123):
    /// proves the configured plane isn't just installed but actually works
    /// -- config loads, the ledger is reachable and schema-current, every
    /// task's declared secrets/binaries preflight clean, and (best-effort,
    /// or required with `--expect-serve`) the running `bb serve`'s `/health`
    /// and `/` routes answer. Read-only; exits non-zero on any failing check.
    Doctor {
        #[arg(long)]
        json: bool,
        /// Require the serve/dashboard probes to succeed instead of treating
        /// an unreachable address as informational.
        #[arg(long)]
        expect_serve: bool,
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
    /// read-only tools for status, check, tasks, runs, DLQ, preflight, and gate
    /// evaluation. No mutating tools in this slice.
    Mcp {
        #[command(subcommand)]
        command: McpCommand,
    },
    /// Lane-checkout lifecycle (bitterblossom-921): sweep finished-lane
    /// worktrees/clones that leave full repo checkouts with no reaper.
    Janitor {
        #[command(subcommand)]
        command: JanitorCommand,
    },
    /// Revisioned workflow configuration store: the plane database is
    /// authoritative for active workflow config. Draft, revise, diff,
    /// activate, pause, archive, and roll back immutable revisions; import/
    /// export declarative TOML documents; accepted runs pin the revision
    /// active at acceptance. Same store the `/api/workflows` routes use.
    Workflow {
        #[command(subcommand)]
        command: WorkflowCommand,
    },
}

#[derive(Subcommand)]
enum WorkflowCommand {
    /// Create a draft workflow from a declarative TOML document (revision 1).
    Create {
        /// Path to the workflow TOML document.
        file: PathBuf,
        #[arg(long)]
        note: Option<String>,
        #[arg(long)]
        json: bool,
    },
    /// List workflows with lifecycle state and active revision.
    List {
        #[arg(long)]
        json: bool,
    },
    /// Show one workflow: state, revision history, and the active document.
    Show {
        name: String,
        #[arg(long)]
        json: bool,
    },
    /// Append a new immutable revision from a TOML document. Refuses a
    /// document identical to the latest revision.
    Revise {
        name: String,
        file: PathBuf,
        #[arg(long)]
        note: Option<String>,
        #[arg(long)]
        json: bool,
    },
    /// Line diff between two stored revisions (canonical JSON).
    Diff {
        name: String,
        #[arg(long)]
        from: i64,
        #[arg(long)]
        to: i64,
        #[arg(long)]
        json: bool,
    },
    /// Activate one revision (default: latest). New acceptances pin the
    /// newly active revision; existing runs keep the one they pinned.
    Activate {
        name: String,
        #[arg(long)]
        revision: Option<i64>,
        #[arg(long)]
        json: bool,
    },
    /// Pause an active workflow: new events are suppressed (disposition
    /// recorded in the audit trail), nothing is replayed on resume.
    Pause {
        name: String,
        #[arg(long, default_value = "paused by operator")]
        reason: String,
        #[arg(long)]
        json: bool,
    },
    /// Resume a paused workflow on its already-active revision.
    Resume {
        name: String,
        #[arg(long)]
        json: bool,
    },
    /// Archive a workflow: frozen, never deleted — historical runs keep
    /// their revision referents readable.
    Archive {
        name: String,
        #[arg(long)]
        json: bool,
    },
    /// Re-activate an earlier snapshot as a NEW revision (history is never
    /// rewritten).
    Rollback {
        name: String,
        #[arg(long)]
        to: i64,
        #[arg(long)]
        json: bool,
    },
    /// Import a declarative TOML document: creates a new workflow, revises a
    /// changed one, or no-ops when identical to the latest revision (files
    /// stay interchange, never a second live authority).
    Import {
        file: PathBuf,
        #[arg(long)]
        note: Option<String>,
        #[arg(long)]
        json: bool,
    },
    /// Export one revision (default: active, else latest) as declarative
    /// TOML on stdout.
    Export {
        name: String,
        #[arg(long)]
        revision: Option<i64>,
    },
    /// Migration: convert one currently-loaded file-defined task (task.toml
    /// plus card plus bound agent) into a workflow document and import it.
    /// Files are the migration source; nothing is written back to them.
    ImportTask {
        task: String,
        /// Also activate the imported revision.
        #[arg(long)]
        activate: bool,
        #[arg(long)]
        json: bool,
    },
    /// Accept one triggering event for an active workflow: creates a
    /// workflow run pinned to the revision active right now. Paused
    /// workflows suppress with a recorded disposition (exit 3). All trigger
    /// sources share one normalized acceptance contract; a repeated
    /// --dedupe-key returns the original run as a duplicate.
    Accept {
        name: String,
        #[arg(long, default_value = "manual")]
        trigger: String,
        /// Inline JSON payload, validated before acceptance.
        #[arg(long)]
        payload: Option<String>,
        /// Idempotency key: a repeat acceptance returns the original run.
        #[arg(long)]
        dedupe_key: Option<String>,
        #[arg(long)]
        json: bool,
    },
    /// Resolve a recovered workflow run after inspecting side effects. This
    /// explicit path releases any retained host lease before recording the
    /// operator-selected terminal disposition.
    Resolve {
        run_id: String,
        /// Terminal disposition: succeeded, failed, or stopped.
        #[arg(long)]
        state: String,
        #[arg(long, default_value = "resolved by operator")]
        reason: String,
        #[arg(long)]
        json: bool,
    },
    /// Execute one accepted (queued) workflow run to a terminal state:
    /// steps commission their pinned agents, results route, guards bound
    /// cycles. Same executor the `bb serve` workflow runner uses.
    Execute {
        run_id: String,
        #[arg(long)]
        json: bool,
    },
    /// Request an external stop signal for a run group. Takes effect before
    /// the next step attempt; the in-flight attempt is never killed blindly.
    Stop {
        run_id: String,
        #[arg(long, default_value = "stopped by operator")]
        reason: String,
        #[arg(long)]
        json: bool,
    },
    /// List accepted runs with their pinned revisions.
    Runs {
        name: String,
        #[arg(long)]
        json: bool,
    },
    /// Show one accepted run with the exact document it pinned.
    RunShow {
        run_id: String,
        #[arg(long)]
        json: bool,
    },
    /// Show current-day observed spend and the workflow daily ceiling.
    Spend {
        name: String,
        #[arg(long)]
        json: bool,
    },
    /// Audit trail: every lifecycle act, acceptance, and suppression.
    Events {
        name: String,
        #[arg(long)]
        json: bool,
    },
}

#[derive(Subcommand)]
enum JanitorCommand {
    /// Evaluate (and, with `--apply`, remove) reap-eligible checkouts: every
    /// non-primary worktree of each `--root` repo, plus every terminal bb
    /// run with a registered `checkout_path`. Eligible means clean tree, HEAD
    /// reachable from a remote branch, and idle at least `--grace-hours`.
    /// Dry-run (report only, no deletion) unless `--apply` is passed.
    Sweep {
        /// Primary repo checkout to scan via `git worktree list`. Repeatable.
        #[arg(long = "root")]
        roots: Vec<PathBuf>,
        #[arg(long, default_value_t = 6.0)]
        grace_hours: f64,
        #[arg(long)]
        apply: bool,
        #[arg(long)]
        json: bool,
        /// Skip any candidate whose path contains this substring -- never
        /// evaluated, never touched. Repeatable. For worktrees that belong
        /// to a different workstream/operator scope even though they are
        /// registered against a repo in `--root`.
        #[arg(long = "exclude")]
        excludes: Vec<String>,
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
    /// Record the lane checkout/worktree path a run's own agent created for
    /// isolation (bitterblossom-921), so `bb janitor sweep` can find it once
    /// the run reaches a terminal state. The dispatched agent calls this
    /// itself; the plane never creates worktrees on its behalf.
    SetCheckoutPath { run_id: String, path: PathBuf },
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
enum NotifyCommand {
    /// List notification outbox rows. `--json` emits the versioned row shape.
    List {
        #[arg(long, default_value_t = 50)]
        limit: i64,
        #[arg(long)]
        json: bool,
    },
    /// Retry pending and failed notification rows using the configured webhook.
    Retry {
        #[arg(long, default_value_t = 20)]
        limit: i64,
        #[arg(long)]
        json: bool,
    },
    /// Acknowledge a pending or failed notification as handled out-of-band.
    Ack {
        id: i64,
        #[arg(long)]
        reason: String,
        #[arg(long)]
        json: bool,
    },
}

#[derive(Subcommand)]
enum KeysCommand {
    /// Mint an OpenRouter child key for one policy-bound agent, or for every
    /// eligible agent with `--all`. The provider `limit` is the agent policy's
    /// `provider_spend_cap_usd`.
    Mint {
        agent: Option<String>,
        #[arg(long)]
        all: bool,
        #[arg(long)]
        json: bool,
    },
    /// Rotate one agent's OpenRouter child key: create a replacement with the
    /// current policy cap, store it plane-side, then revoke the old key.
    Rotate {
        agent: String,
        #[arg(long)]
        json: bool,
    },
    /// Revoke one agent's stored OpenRouter child key and remove local key
    /// material while preserving metadata for audit.
    Revoke {
        agent: String,
        #[arg(long)]
        json: bool,
    },
    /// List local plane-side key metadata, or `--remote` to read OpenRouter's
    /// management list endpoint. No plaintext key material is printed.
    List {
        #[arg(long)]
        remote: bool,
        #[arg(long)]
        include_disabled: bool,
        #[arg(long)]
        json: bool,
    },
    /// Sync local non-secret metadata from OpenRouter's key list and report
    /// drift between agent policy caps, stored records, and remote limits.
    /// With `--check`, exits non-zero when any selected key is missing or
    /// drifted after printing the report.
    Sync {
        agent: Option<String>,
        #[arg(long)]
        all: bool,
        #[arg(long)]
        check: bool,
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
    /// Waive a required gate member (backlog 088) so the storm does not need
    /// to run it for this specific rev. `reason` must be `risk-tier:<tier>`
    /// naming an explicit skip rule (e.g. `risk-tier:docs-only`), not a free
    /// excuse. Scoped to `--rev`: a later rev of the same change needs its
    /// own waiver, and a waiver never overrides a verdict already recorded.
    Waive {
        #[arg(long)]
        change: String,
        #[arg(long)]
        rev: String,
        #[arg(long)]
        kind: String,
        #[arg(long)]
        reason: String,
    },
    Abandon {
        #[arg(long)]
        change: String,
    },
}

#[derive(Subcommand)]
enum AskCommand {
    /// Raise an ask and wait: reads run_id/task/ask_token from RUN.json in
    /// the current directory (the workspace dispatch already provides),
    /// POSTs to the plane, then polls. Exit 0 with the answer on stdout if
    /// answered within `--window-seconds`; exits 75 (documented parked exit
    /// code) if the window elapses first -- the caller must then write its
    /// own `ASK_PACKET.json` episodic handoff packet and stop, not retry.
    Raise {
        question: String,
        #[arg(long, default_value = "question")]
        kind: String,
        /// "blocking" (park + escalate on timeout) or "advisory" (proceed on
        /// a stated assumption; a late answer folds in later).
        #[arg(long)]
        semantics: String,
        #[arg(long)]
        window_seconds: i64,
        #[arg(long)]
        context: Option<String>,
        /// Defaults to the `BB_API_BASE_URL` env var if omitted.
        #[arg(long)]
        api_base_url: Option<String>,
        #[arg(long, default_value = "RUN.json")]
        run_json: PathBuf,
    },
    /// Operator-facing: deliver an answer. Uses `BB_API_TOKEN` like the rest
    /// of the read API. If the ask's run already parked, this creates a
    /// lineage-linked resume run (same mechanism `dlq replay` uses); if the
    /// run is still running, the raising attempt's next poll sees it.
    Answer {
        ask_id: String,
        answer: String,
        #[arg(long, default_value = "operator")]
        answered_by: String,
        /// Defaults to the `BB_API_BASE_URL` env var if omitted.
        #[arg(long)]
        api_base_url: Option<String>,
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
    /// Unpark a task and re-queue selected blocked-budget runs.
    Unpark {
        task: String,
        /// Confirm release when more than one blocked-budget run is selected.
        #[arg(long)]
        yes: bool,
        /// Release only blocked-budget runs created at or after this RFC3339 timestamp.
        #[arg(long)]
        since: Option<String>,
        /// Release only this blocked-budget run id. Repeat for a bounded batch.
        #[arg(long = "run-id")]
        run_ids: Vec<String>,
    },
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
    /// Export a portable artifact bundle directory with manifest.json.
    Bundle {
        run_id: String,
        #[arg(long)]
        out: PathBuf,
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
        let msg = format!("{e:#}");
        eprintln!("error: {msg}");
        canary::report_error("bb.startup", &msg);
        std::process::exit(1);
    }
}

fn select_task_unpark_runs(
    blocked: &[ledger::RunRow],
    since: Option<&str>,
    run_ids: &[String],
) -> Result<Vec<String>> {
    let since = since
        .map(|value| {
            OffsetDateTime::parse(value, &Rfc3339)
                .with_context(|| format!("--since must be an RFC3339 timestamp: {value}"))
        })
        .transpose()?;
    let requested: BTreeSet<&str> = run_ids.iter().map(String::as_str).collect();
    if !requested.is_empty() {
        let blocked_ids: BTreeSet<&str> = blocked.iter().map(|run| run.id.as_str()).collect();
        let missing: Vec<&str> = requested.difference(&blocked_ids).copied().collect();
        if !missing.is_empty() {
            bail!(
                "run id(s) not blocked_budget for this task: {}",
                missing.join(", ")
            );
        }
    }

    let mut selected = Vec::new();
    for run in blocked {
        if let Some(since) = &since {
            let created = OffsetDateTime::parse(&run.created_at, &Rfc3339)
                .with_context(|| format!("run {} created_at is not RFC3339", run.id))?;
            if created < *since {
                continue;
            }
        }
        if !requested.is_empty() && !requested.contains(run.id.as_str()) {
            continue;
        }
        selected.push(run.id.clone());
    }
    Ok(selected)
}

fn print_task_unpark_preview(task: &str, blocked: &[ledger::RunRow], selected_count: usize) {
    match (blocked.first(), blocked.last()) {
        (Some(oldest), Some(newest)) => println!(
            "{task}: {} blocked_budget run(s); oldest {}; newest {}",
            blocked.len(),
            oldest.created_at,
            newest.created_at
        ),
        _ => println!("{task}: 0 blocked_budget run(s)"),
    }
    println!("{selected_count} selected for release");
}

fn task_unpark_budget_detail(
    total_blocked: usize,
    released: usize,
    since: Option<&str>,
    run_ids: &[String],
) -> String {
    let mut detail =
        format!("operator unpark; released {released}/{total_blocked} blocked_budget run(s)");
    if let Some(since) = since {
        detail.push_str(&format!("; since {since}"));
    }
    if !run_ids.is_empty() {
        detail.push_str(&format!("; run_ids {}", run_ids.join(",")));
    }
    detail
}

fn run() -> Result<()> {
    let cli = Cli::parse();
    let root = cli
        .config
        .or_else(|| std::env::var_os("BB_PLANE_DIR").map(PathBuf::from))
        .unwrap_or_else(|| PathBuf::from("."));

    // Doctor owns its own error handling for exactly the failures every
    // other command would otherwise propagate as a raw anyhow bail (invalid
    // config, unreachable ledger) -- it must run before the eager
    // Plane::load/Ledger::open below, not through them.
    if let Command::Doctor { json, expect_serve } = &cli.command {
        let report = bitterblossom::doctor::run(&root, *expect_serve);
        if *json {
            println!("{}", serde_json::to_string_pretty(&report)?);
        } else {
            for check in &report.checks {
                println!("[{}] {}: {}", check.status, check.name, check.detail);
                if let Some(remediation) = &check.remediation {
                    println!("    remediation: {remediation}");
                }
            }
            println!(
                "{}",
                if report.ok {
                    "doctor: ok"
                } else {
                    "doctor: FAILED"
                }
            );
        }
        if !report.ok {
            std::process::exit(2);
        }
        return Ok(());
    }

    let plane = Plane::load(&root)?;
    let mut ledger = Ledger::open(&plane.db_path())?;

    match cli.command {
        Command::Dispatch {
            repo,
            brief,
            model,
            label,
        } => {
            let task = dispatch::default_dispatch_task(&plane)?;
            let payload = dispatch_payload(&repo, &brief, model, label)?;
            let outcome = ledger.ingest(IngressRequest {
                task: &task,
                trigger_kind: "manual",
                idempotency_key: None,
                source_event_id: None,
                payload: Some(&payload),
                parent_run_id: None,
            })?;
            if outcome.state == "blocked_budget" {
                eprintln!("run {} blocked: task is parked", outcome.run_id);
            }
            eprintln!(
                "queued run {} task={}; follow with `bb logs -f {}`",
                outcome.run_id, task, outcome.run_id
            );
            println!("{}", outcome.run_id);
        }
        Command::Logs { follow, run_id } => {
            follow_logs(&ledger, &run_id, follow)?;
        }
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
                    ledger.resolve_run(&run_id, &outcome, &reason)?;
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
                RunsCommand::SetCheckoutPath { run_id, path } => {
                    let canonical = path
                        .canonicalize()
                        .with_context(|| format!("checkout path {}", path.display()))?;
                    if !canonical.is_dir() {
                        bail!("checkout path {} is not a directory", canonical.display());
                    }
                    ledger.set_run_checkout_path(&run_id, &canonical.to_string_lossy())?;
                    println!(
                        "recorded checkout path for {run_id}: {}",
                        canonical.display()
                    );
                }
                RunsCommand::Export => {
                    for line in bitterblossom::telemetry::export_all(&plane, &ledger)? {
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
            ArtifactsCommand::Bundle { run_id, out } => {
                let manifest = artifacts::bundle(&ledger, &run_id, &out)?;
                println!(
                    "wrote artifact bundle for run {run_id}: {} ({} entries)",
                    out.join("manifest.json").display(),
                    manifest.entries.len()
                );
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
        Command::Notify { command } => match command {
            NotifyCommand::List { limit, json } => {
                let rows = ledger.list_notification_outbox(limit)?;
                if json {
                    println!("{}", serde_json::to_string_pretty(&rows)?);
                } else {
                    for n in rows {
                        println!(
                            "{}  #{} {:<12} attempts={} event={}",
                            n.created_at, n.id, n.status, n.attempts, n.event
                        );
                        if let Some(err) = &n.last_error {
                            println!("    last_error: {err}");
                        }
                        if let Some(code) = n.last_status_code {
                            println!("    last_status_code: {code}");
                        }
                        if let Some(response) = &n.last_response {
                            println!("    last_response: {response}");
                        }
                        if let Some(reason) = &n.acknowledged_reason {
                            println!("    acknowledged: {reason}");
                        }
                    }
                }
            }
            NotifyCommand::Retry { limit, json } => {
                let report = bitterblossom::notify::retry_pending(&plane, &ledger, limit)?;
                if json {
                    println!("{}", serde_json::to_string_pretty(&report)?);
                } else {
                    println!(
                        "notification retry: attempted={} delivered={} failed={}",
                        report.attempted, report.delivered, report.failed
                    );
                    for row in report.rows {
                        println!("  #{} {:<9} {}", row.id, row.status, row.event);
                        if let Some(err) = row.error {
                            println!("    {err}");
                        }
                    }
                }
            }
            NotifyCommand::Ack { id, reason, json } => {
                let row = ledger.acknowledge_notification(id, &reason)?;
                if json {
                    println!("{}", serde_json::to_string_pretty(&row)?);
                } else {
                    println!(
                        "acknowledged notification {id}: {}",
                        row.acknowledged_reason.as_deref().unwrap_or(&reason)
                    );
                }
            }
        },
        Command::Keys { command } => match command {
            KeysCommand::Mint { agent, all, json } => {
                let agents = selected_key_agents(&plane, agent, all)?;
                let mut keys = Vec::new();
                for agent in agents {
                    keys.push(provider_keys::mint_agent(&plane, &agent)?);
                }
                let report = provider_keys::KeyOperationReport {
                    operation: "mint".into(),
                    keys,
                };
                if json {
                    println!("{}", serde_json::to_string_pretty(&report)?);
                } else {
                    println!("minted {} provider key(s)", report.keys.len());
                    for key in report.keys {
                        println!(
                            "  {} {} cap=${:.2} hash={}",
                            key.agent, key.provider_key_name, key.spend_cap_usd, key.hash
                        );
                    }
                }
            }
            KeysCommand::Rotate { agent, json } => {
                let report = provider_keys::rotate_agent(&plane, &agent)?;
                if json {
                    println!("{}", serde_json::to_string_pretty(&report)?);
                } else {
                    for key in report.keys {
                        println!(
                            "rotated {} {} cap=${:.2} hash={}",
                            key.agent, key.provider_key_name, key.spend_cap_usd, key.hash
                        );
                    }
                }
            }
            KeysCommand::Revoke { agent, json } => {
                let key = provider_keys::revoke_agent(&plane, &agent)?;
                if json {
                    println!("{}", serde_json::to_string_pretty(&key)?);
                } else {
                    println!(
                        "revoked {} {} hash={}",
                        key.agent, key.provider_key_name, key.hash
                    );
                }
            }
            KeysCommand::List {
                remote,
                include_disabled,
                json,
            } => {
                if remote {
                    let rows = provider_keys::list_remote(include_disabled)?;
                    if json {
                        println!("{}", serde_json::to_string_pretty(&rows)?);
                    } else {
                        for key in rows {
                            println!(
                                "{:<36} cap={} remaining={} disabled={} hash={}",
                                key.name,
                                key.limit
                                    .map(|v| format!("${v:.2}"))
                                    .unwrap_or_else(|| "-".into()),
                                key.limit_remaining
                                    .map(|v| format!("${v:.2}"))
                                    .unwrap_or_else(|| "-".into()),
                                key.disabled,
                                key.hash
                            );
                        }
                    }
                } else {
                    let rows = provider_keys::list_local(&plane)?;
                    if json {
                        println!("{}", serde_json::to_string_pretty(&rows)?);
                    } else {
                        for key in rows {
                            println!(
                                "{:<24} {:<24} cap=${:.2} active={} hash={}",
                                key.agent,
                                key.provider_key_name,
                                key.spend_cap_usd,
                                key.secret_available && !key.revoked,
                                key.hash
                            );
                        }
                    }
                }
            }
            KeysCommand::Sync {
                agent,
                all,
                check,
                json,
            } => {
                let agents = selected_key_agents(&plane, agent, all)?;
                let report = provider_keys::sync_agents(&plane, &agents)?;
                if json {
                    println!("{}", serde_json::to_string_pretty(&report)?);
                } else {
                    println!(
                        "synced {} provider key(s); ok={}",
                        report.keys.len(),
                        report.ok
                    );
                    for key in &report.keys {
                        println!(
                            "{:<24} {:<14} configured=${:.2} remote={} hash={}",
                            key.local.agent,
                            key.local.status,
                            key.local.configured_spend_cap_usd,
                            key.local
                                .remote_limit_usd
                                .map(|v| format!("${v:.2}"))
                                .unwrap_or_else(|| "-".into()),
                            key.local.stored_hash.as_deref().unwrap_or("-")
                        );
                        for drift in &key.local.drift {
                            println!("  drift: {drift}");
                        }
                    }
                }
                if check && !report.ok {
                    bail!("provider key drift detected");
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
            TaskCommand::Unpark {
                task,
                yes,
                since,
                run_ids,
            } => {
                plane.task(&task)?;
                let blocked = ledger.blocked_budget_runs_for_task(&task)?;
                let selected = select_task_unpark_runs(&blocked, since.as_deref(), &run_ids)?;
                print_task_unpark_preview(&task, &blocked, selected.len());
                if selected.len() > TASK_UNPARK_CONFIRMATION_LIMIT && !yes {
                    bail!(
                        "refusing to release {} blocked_budget run(s) for {task}; pass --yes to confirm",
                        selected.len()
                    );
                }

                let released = ledger.unpark_task_runs(&task, &selected, "unparked")?;
                let detail = task_unpark_budget_detail(
                    blocked.len(),
                    released.len(),
                    since.as_deref(),
                    &run_ids,
                );
                ledger.record_budget_event(Some(&task), "unparked", &detail)?;
                println!(
                    "unparked {task}; {} blocked run(s) now pending; {} left blocked_budget",
                    released.len(),
                    blocked.len().saturating_sub(released.len())
                );
                for id in released {
                    println!("  {id}");
                }
            }
        },
        Command::Ask { command } => match command {
            AskCommand::Raise {
                question,
                kind,
                semantics,
                window_seconds,
                context,
                api_base_url,
                run_json,
            } => {
                let blocking = match semantics.as_str() {
                    "blocking" => true,
                    "advisory" => false,
                    other => bail!("--semantics must be 'blocking' or 'advisory', got '{other}'"),
                };
                let api_base_url = api_base_url
                    .or_else(|| std::env::var("BB_API_BASE_URL").ok())
                    .context("--api-base-url or BB_API_BASE_URL required")?;
                match ask::raise(
                    &api_base_url,
                    &run_json,
                    &kind,
                    &question,
                    context.as_deref(),
                    blocking,
                    window_seconds,
                )? {
                    ask::RaiseOutcome::Answered(answer) => println!("{answer}"),
                    ask::RaiseOutcome::Parked => std::process::exit(ask::PARKED_EXIT_CODE),
                }
            }
            AskCommand::Answer {
                ask_id,
                answer,
                answered_by,
                api_base_url,
            } => {
                let api_token = std::env::var("BB_API_TOKEN").context("BB_API_TOKEN not set")?;
                let api_base_url = api_base_url
                    .or_else(|| std::env::var("BB_API_BASE_URL").ok())
                    .context("--api-base-url or BB_API_BASE_URL required")?;
                let response =
                    ask::answer(&api_base_url, &api_token, &ask_id, &answer, &answered_by)?;
                println!("{response}");
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
            SubmitCommand::Waive {
                change,
                rev,
                kind,
                reason,
            } => {
                ledger.waive_member(&change, &rev, &kind, &reason)?;
                println!("waived {kind} on {change}@{rev}: {reason}");
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
            let report =
                serve::gate_view(&plane, &ledger, submission.as_deref(), change.as_deref())
                    .context("evaluate gate")?;
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
                    if let Some(reason) = &m.waiver_reason {
                        println!("    waived: {reason}");
                    }
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
            let workflow_reports =
                bitterblossom::workflow_runtime::recover_inherited_workflow_runs(&plane, &ledger)?;
            let output = if json {
                let mut all = Vec::with_capacity(reports.len() + workflow_reports.len());
                all.extend(
                    reports
                        .iter()
                        .map(serde_json::to_value)
                        .collect::<std::result::Result<Vec<_>, _>>()?,
                );
                all.extend(
                    workflow_reports
                        .iter()
                        .map(serde_json::to_value)
                        .collect::<std::result::Result<Vec<_>, _>>()?,
                );
                serde_json::to_string_pretty(&all)?
            } else {
                format!(
                    "recovered {} task run(s), {} workflow run(s)",
                    reports.len(),
                    workflow_reports.len()
                )
            };
            println!("{output}");
        }
        Command::Serve => {
            drop(ledger);
            serve::serve(&plane.root)?;
        }
        Command::Pause { reason } => {
            ledger.pause_plane(&reason)?;
            ledger.record_guard_event("plane_paused", None, &reason, 1)?;
            println!("paused reflex dispatch: {reason}");
            eprintln!("note: manual `bb run` still dispatches; autonomous loop halted");
        }
        Command::Resume => {
            if ledger.resume_plane()? {
                ledger.record_guard_event("plane_resumed", None, "operator resume", 1)?;
                println!("resumed reflex dispatch");
            } else {
                println!("plane was not paused");
            }
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
                    "preflight ok: {} task(s) checked, no pre-dispatch findings",
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
                    if let Some(remediation) = &f.remediation {
                        println!("    remediation: {remediation}");
                    }
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
        Command::Janitor { command } => match command {
            JanitorCommand::Sweep {
                roots,
                grace_hours,
                apply,
                json,
                excludes,
            } => {
                let candidates = reap::sweep(&ledger, &roots, grace_hours, apply, &excludes)?;
                if json {
                    println!("{}", serde_json::to_string_pretty(&candidates)?);
                } else {
                    let removed = candidates.iter().filter(|c| c.removed).count();
                    let eligible = candidates.iter().filter(|c| c.eligible).count();
                    for c in &candidates {
                        let mark = if c.removed {
                            "removed"
                        } else if c.eligible {
                            "eligible"
                        } else {
                            "refused"
                        };
                        println!("{mark:8} [{}] {} -- {}", c.source, c.path, c.reason);
                    }
                    println!(
                        "{} candidate(s), {} eligible, {} removed{}",
                        candidates.len(),
                        eligible,
                        removed,
                        if apply {
                            ""
                        } else {
                            " (dry run, pass --apply to remove)"
                        }
                    );
                }
            }
        },
        Command::Workflow { command } => workflow_command(&plane, &ledger, command)?,
        Command::Doctor { .. } => unreachable!("Command::Doctor returns early above"),
    }
    Ok(())
}

/// `bb workflow ...`: thin projections over src/workflow.rs store functions —
/// the same functions the `/api/workflows` HTTP routes call, so the two
/// surfaces create/read/diff/activate the same immutable revisions.
fn workflow_command(plane: &Plane, ledger: &Ledger, command: WorkflowCommand) -> Result<()> {
    use bitterblossom::workflow::{self, AcceptOutcome, WorkflowDoc};

    fn read_doc(file: &Path) -> Result<WorkflowDoc> {
        let text = std::fs::read_to_string(file)
            .with_context(|| format!("read workflow document {}", file.display()))?;
        WorkflowDoc::from_toml(&text)
    }
    fn print_workflow(wf: &workflow::WorkflowRow, json: bool) -> Result<()> {
        if json {
            println!("{}", serde_json::to_string_pretty(wf)?);
        } else {
            println!(
                "{} {} active_revision={}",
                wf.name,
                wf.state,
                wf.active_revision
                    .map(|r| r.to_string())
                    .unwrap_or_else(|| "-".into())
            );
        }
        Ok(())
    }
    fn print_revision(
        wf: &workflow::WorkflowRow,
        revision: i64,
        verb: &str,
        json: bool,
    ) -> Result<()> {
        if json {
            println!(
                "{}",
                serde_json::to_string_pretty(
                    &serde_json::json!({ "workflow": wf, "revision": revision })
                )?
            );
        } else {
            println!("{verb} '{}' revision {revision} ({})", wf.name, wf.state);
        }
        Ok(())
    }

    match command {
        WorkflowCommand::Create { file, note, json } => {
            let doc = read_doc(&file)?;
            let (wf, revision) = ledger.create_workflow(&doc, "cli", note.as_deref())?;
            print_revision(&wf, revision, "created", json)?;
        }
        WorkflowCommand::List { json } => {
            let workflows = ledger.list_workflows()?;
            if json {
                println!("{}", serde_json::to_string_pretty(&workflows)?);
            } else {
                for wf in workflows {
                    print_workflow(&wf, false)?;
                }
            }
        }
        WorkflowCommand::Show { name, json } => {
            let view = workflow::workflow_view(ledger, &name)?;
            if json {
                println!("{}", serde_json::to_string_pretty(&view)?);
            } else {
                let wf = ledger.workflow_by_name(&name)?;
                print_workflow(&wf, false)?;
                for r in ledger.workflow_revisions(&name)? {
                    println!(
                        "  revision {} [{}] {} {}",
                        r.revision,
                        r.source,
                        r.created_at,
                        r.note.as_deref().unwrap_or("")
                    );
                }
            }
        }
        WorkflowCommand::Revise {
            name,
            file,
            note,
            json,
        } => {
            let doc = read_doc(&file)?;
            let (wf, revision) = ledger.revise_workflow(&name, &doc, "cli", note.as_deref())?;
            print_revision(&wf, revision, "revised", json)?;
        }
        WorkflowCommand::Diff {
            name,
            from,
            to,
            json,
        } => {
            let view = workflow::diff_view(ledger, &name, from, to)?;
            if json {
                println!("{}", serde_json::to_string_pretty(&view)?);
            } else if view["identical"].as_bool() == Some(true) {
                println!("revisions {from} and {to} are identical");
            } else {
                for change in view["changes"].as_array().into_iter().flatten() {
                    println!(
                        "{}{}",
                        change["op"].as_str().unwrap_or("?"),
                        change["line"].as_str().unwrap_or("")
                    );
                }
            }
        }
        WorkflowCommand::Activate {
            name,
            revision,
            json,
        } => {
            let routes = ingress::task_webhook_routes(plane);
            let wf = ledger.activate_workflow_with_reserved_routes(&name, revision, &routes)?;
            print_workflow(&wf, json)?;
        }
        WorkflowCommand::Pause { name, reason, json } => {
            let wf = ledger.pause_workflow(&name, &reason)?;
            print_workflow(&wf, json)?;
        }
        WorkflowCommand::Resume { name, json } => {
            let routes = ingress::task_webhook_routes(plane);
            let wf = ledger.resume_workflow_with_reserved_routes(&name, &routes)?;
            print_workflow(&wf, json)?;
        }
        WorkflowCommand::Archive { name, json } => {
            let wf = ledger.archive_workflow(&name)?;
            print_workflow(&wf, json)?;
        }
        WorkflowCommand::Rollback { name, to, json } => {
            let routes = ingress::task_webhook_routes(plane);
            let (wf, revision) =
                ledger.rollback_workflow_with_reserved_routes(&name, to, &routes)?;
            print_revision(&wf, revision, "rolled back to snapshot as", json)?;
        }
        WorkflowCommand::Import { file, note, json } => {
            let doc = read_doc(&file)?;
            let (wf, revision, outcome) =
                ledger.import_workflow(&doc, "import", note.as_deref())?;
            if json {
                println!(
                    "{}",
                    serde_json::to_string_pretty(&serde_json::json!({
                        "workflow": wf,
                        "revision": revision,
                        "outcome": outcome,
                    }))?
                );
            } else {
                println!("import {:?}: '{}' revision {revision}", outcome, wf.name);
            }
        }
        WorkflowCommand::Export { name, revision } => {
            let (revision, toml) = workflow::export_toml(ledger, &name, revision)?;
            eprintln!("exporting '{name}' revision {revision}");
            print!("{toml}");
        }
        WorkflowCommand::ImportTask {
            task,
            activate,
            json,
        } => {
            let task = plane.task(&task)?;
            let doc = WorkflowDoc::from_task(task)?;
            let (wf, revision, outcome) = ledger.import_workflow(
                &doc,
                "import-task",
                Some(&format!("from task '{}'", task.name)),
            )?;
            let wf = if activate {
                {
                    let routes = ingress::task_webhook_routes(plane);
                    ledger.activate_workflow_with_reserved_routes(
                        &wf.name,
                        Some(revision),
                        &routes,
                    )?
                }
            } else {
                wf
            };
            if json {
                println!(
                    "{}",
                    serde_json::to_string_pretty(&serde_json::json!({
                        "workflow": wf,
                        "revision": revision,
                        "outcome": outcome,
                    }))?
                );
            } else {
                println!(
                    "import-task {:?}: '{}' revision {revision} ({})",
                    outcome, wf.name, wf.state
                );
            }
        }
        WorkflowCommand::Accept {
            name,
            trigger,
            payload,
            dedupe_key,
            json,
        } => {
            let envelope = bitterblossom::workflow_runtime::TriggerEnvelope {
                workflow: name,
                source: bitterblossom::workflow_runtime::TriggerSource::from_kind(&trigger)?,
                payload,
                dedupe_key,
            };
            let outcome = bitterblossom::workflow_runtime::accept(plane, ledger, &envelope)?;
            if json {
                println!("{}", serde_json::to_string_pretty(&outcome)?);
            } else {
                match &outcome {
                    AcceptOutcome::Accepted { run } => println!(
                        "accepted run {} pinned to '{}' revision {}",
                        run.id, run.workflow, run.revision
                    ),
                    AcceptOutcome::Duplicate { run } => println!(
                        "duplicate: dedupe key already accepted run {} on '{}'",
                        run.id, run.workflow
                    ),
                    AcceptOutcome::Denied { workflow, reason } => {
                        println!("denied on '{workflow}': {reason}");
                    }
                    AcceptOutcome::Suppressed { workflow, reason } => {
                        println!("suppressed on '{workflow}': {reason}")
                    }
                    AcceptOutcome::Denied {
                        workflow,
                        kind,
                        reason,
                    } => {
                        println!("denied on '{workflow}' ({kind}): {reason}")
                    }
                }
            }
            if matches!(
                outcome,
                AcceptOutcome::Suppressed { .. } | AcceptOutcome::Denied { .. }
            ) {
                std::process::exit(3);
            }
        }
        WorkflowCommand::Resolve {
            run_id,
            state,
            reason,
            json,
        } => {
            let status = bitterblossom::workflow_runtime::resolve_workflow_run(
                ledger, &run_id, &state, &reason,
            )?;
            if json {
                println!("{}", serde_json::to_string_pretty(&status)?);
            } else {
                println!("workflow run {run_id} resolved to {state}: {reason}");
            }
        }
        WorkflowCommand::Execute { run_id, json } => {
            let view = bitterblossom::workflow_runtime::execute_run(plane, ledger, &run_id)?;
            if json {
                println!("{}", serde_json::to_string_pretty(&view)?);
            } else {
                let status = &view["status"];
                println!(
                    "run {} {} detail={} steps={}",
                    run_id,
                    status["state"].as_str().unwrap_or("-"),
                    status["detail"].as_str().unwrap_or("-"),
                    view["steps"].as_array().map(Vec::len).unwrap_or(0),
                );
            }
        }
        WorkflowCommand::Stop {
            run_id,
            reason,
            json,
        } => {
            let status = ledger.request_workflow_run_stop(&run_id, &reason)?;
            if json {
                println!("{}", serde_json::to_string_pretty(&status)?);
            } else {
                println!("stop requested for run {run_id}: {reason}");
            }
        }
        WorkflowCommand::Runs { name, json } => {
            let runs = ledger.workflow_runs(&name)?;
            if json {
                println!("{}", serde_json::to_string_pretty(&runs)?);
            } else {
                for run in runs {
                    println!(
                        "{}  {:<10} revision {:<4} {}",
                        run.created_at, run.trigger_kind, run.revision, run.id
                    );
                }
            }
        }
        WorkflowCommand::RunShow { run_id, json } => {
            let view = bitterblossom::workflow_runtime::run_detail_view(ledger, &run_id)?;
            if json {
                println!("{}", serde_json::to_string_pretty(&view)?);
            } else {
                let run = &view["run"];
                println!(
                    "run {} workflow={} revision={} trigger={} state={} created_at={}",
                    run["id"].as_str().unwrap_or("-"),
                    run["workflow"].as_str().unwrap_or("-"),
                    run["revision"],
                    run["trigger_kind"].as_str().unwrap_or("-"),
                    view["status"]["state"].as_str().unwrap_or("-"),
                    run["created_at"].as_str().unwrap_or("-"),
                );
                println!("{}", serde_json::to_string_pretty(&view["document"])?);
            }
        }
        WorkflowCommand::Spend { name, json } => {
            let view = workflow::workflow_spend_view(ledger, &name)?;
            if json {
                println!("{}", serde_json::to_string_pretty(&view)?);
            } else {
                println!(
                    "workflow {} spend_today_usd={} max_cost_per_day_usd={}",
                    name, view["spend_today_usd"], view["max_cost_per_day_usd"]
                );
            }
        }
        WorkflowCommand::Events { name, json } => {
            let events = ledger.workflow_events(&name)?;
            if json {
                println!("{}", serde_json::to_string_pretty(&events)?);
            } else {
                for event in events {
                    println!(
                        "{}  {:<16} {}",
                        event.at,
                        event.kind,
                        event.data.as_deref().unwrap_or("")
                    );
                }
            }
        }
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
        let _ = ledger.record_progress(&run_id, &format!("heartbeat state={}", run.state));
        if !matches!(run.state.as_str(), "pending" | "running") {
            break;
        }
    });
}

/// CLI-specific wrapper over `dispatch::build_dispatch_job_payload`: reads
/// the brief file (rejecting an oversized one before ever reading it into
/// memory) and defaults `label` from the brief's file stem. The shared
/// builder in the `dispatch` module is what actually assembles the
/// `bb.dispatch_job.v1` JSON, so the MCP `bb_dispatch` tool produces the
/// exact same payload shape from its own inline `prompt` argument.
fn dispatch_payload(
    repo: &Path,
    brief: &Path,
    model: Option<String>,
    label: Option<String>,
) -> Result<String> {
    let brief_path = brief
        .canonicalize()
        .with_context(|| format!("brief file {}", brief.display()))?;
    let brief_size = std::fs::metadata(&brief_path)
        .with_context(|| format!("stat brief {}", brief_path.display()))?
        .len();
    if brief_size > dispatch::DISPATCH_BRIEF_MAX_BYTES {
        bail!(
            "brief {} is {} bytes; max is {}",
            brief_path.display(),
            brief_size,
            dispatch::DISPATCH_BRIEF_MAX_BYTES
        );
    }
    let brief_text = std::fs::read_to_string(&brief_path)
        .with_context(|| format!("read brief {}", brief_path.display()))?;
    let label = label.unwrap_or_else(|| {
        brief_path
            .file_stem()
            .and_then(|s| s.to_str())
            .filter(|s| !s.is_empty())
            .unwrap_or("dispatch")
            .to_string()
    });
    let branch_slug = dispatch::slugify_label(&label);
    dispatch::build_dispatch_job_payload(repo, &brief_text, model, label, branch_slug, None)
}

fn follow_logs(ledger: &Ledger, run_id: &str, follow: bool) -> Result<()> {
    ledger.run(run_id)?;
    let poll = std::env::var("BB_LOGS_POLL_MS")
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(500);
    let mut seen_events = 0usize;
    let mut artifact_offsets = BTreeMap::<String, usize>::new();
    loop {
        print_new_log_lines(ledger, run_id, &mut seen_events, &mut artifact_offsets)?;
        let run = ledger.run(run_id)?;
        if is_terminal_state(&run.state) {
            println!("terminal state={}", run.state);
            if let Some(reason) = run.state_reason {
                println!("reason: {reason}");
            }
            break;
        }
        if !follow {
            break;
        }
        thread::sleep(Duration::from_millis(poll));
    }
    Ok(())
}

fn print_new_log_lines(
    ledger: &Ledger,
    run_id: &str,
    seen_events: &mut usize,
    artifact_offsets: &mut BTreeMap<String, usize>,
) -> Result<()> {
    let events = ledger.events(run_id)?;
    for event in events.iter().skip(*seen_events) {
        match &event.data {
            Some(data) if !data.is_empty() => println!("{} {} {}", event.at, event.kind, data),
            _ => println!("{} {}", event.at, event.kind),
        }
    }
    *seen_events = events.len();

    let entries = match artifacts::list(ledger, run_id) {
        Ok(entries) => entries,
        Err(e) => {
            eprintln!("warning: could not list artifacts for run {run_id}: {e:#}");
            return Ok(());
        }
    };
    for entry in entries {
        if entry.binary || !matches!(entry.path.as_str(), "stdout.txt" | "stderr.txt") {
            continue;
        }
        let outcome = match artifacts::read(ledger, run_id, &entry.path) {
            Ok(outcome) => outcome,
            Err(e) => {
                eprintln!(
                    "warning: could not read artifact {} for run {run_id}: {e:#}",
                    entry.path
                );
                continue;
            }
        };
        let artifacts::ReadOutcome::Text { content, .. } = outcome else {
            continue;
        };
        let key = format!("{}:{}", entry.attempt, entry.path);
        let offset = artifact_offsets.entry(key).or_insert(0);
        if *offset == 0 && !content.is_empty() {
            println!("--- attempt {} {} ---", entry.attempt, entry.path);
        }
        if content.len() > *offset {
            print!("{}", &content[*offset..]);
            if !content.ends_with('\n') {
                println!();
            }
            *offset = content.len();
        }
    }
    Ok(())
}

fn is_terminal_state(state: &str) -> bool {
    !matches!(state, "pending" | "running")
}

fn selected_key_agents(plane: &Plane, agent: Option<String>, all: bool) -> Result<Vec<String>> {
    match (agent, all) {
        (Some(_), true) => bail!("pass an agent or --all, not both"),
        (Some(agent), false) => {
            plane
                .agents
                .get(&agent)
                .with_context(|| format!("unknown agent '{agent}'"))?;
            Ok(vec![agent])
        }
        (None, true) => {
            let agents = provider_keys::eligible_agents(plane);
            if agents.is_empty() {
                bail!("no agents declare policy.provider_key_name");
            }
            Ok(agents)
        }
        (None, false) => bail!("pass an agent or --all"),
    }
}

fn print_run(ledger: &Ledger, run_id: &str, json: bool) -> Result<()> {
    if json {
        println!(
            "{}",
            serde_json::to_string_pretty(&serve::run_view(ledger, run_id)?)?
        );
        return Ok(());
    }
    let run = ledger.run(run_id)?;
    let attempts = ledger.attempts(run_id)?;
    let events = ledger.events(run_id)?;
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
