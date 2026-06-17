//! Budget enforcement tiers and agent-swap visibility. Enforced:
//! runs/day and the global daily ceiling, pre-dispatch. Advisory: per-run
//! cost — the run completes, the task parks, the operator gets a webhook.

use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::Path;

use bitterblossom::dispatch;
use bitterblossom::ledger::{IngressRequest, Ledger};
use bitterblossom::spec::Plane;

static NOTIFY_ENV_LOCK: std::sync::Mutex<()> = std::sync::Mutex::new(());

const CLAUDE_STUB: &str = r#"#!/bin/sh
cat > /dev/null
echo '{"type":"result","result":"ok","total_cost_usd":0.0123,"num_turns":2,"usage":{"input_tokens":50,"output_tokens":20}}'
"#;

/// Notify transport stub: swallow curl-style args, append the JSON body
/// (stdin) to $BB_NOTIFY_LOG.
const NOTIFY_STUB: &str = r#"#!/bin/sh
cat >> "$BB_NOTIFY_LOG"
echo >> "$BB_NOTIFY_LOG"
"#;

const SLOW_NOTIFY_STUB: &str = r#"#!/bin/sh
cat > /dev/null
sleep 0.1
echo done >> "$BB_NOTIFY_LOG"
"#;

fn write_executable(path: &Path, content: &str) {
    fs::write(path, content).unwrap();
    fs::set_permissions(path, fs::Permissions::from_mode(0o755)).unwrap();
}

fn setup(root: &Path, plane_toml: &str, budget_toml: &str) -> Plane {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    let stub = root.join("stub.sh");
    write_executable(&stub, CLAUDE_STUB);
    fs::write(
        root.join("agents/a.toml"),
        format!(
            "version = 1\nharness = \"claude\"\nmodel = \"m\"\nbin = \"{}\"\n",
            stub.display()
        ),
    )
    .unwrap();
    fs::write(
        root.join("agents/b.toml"),
        format!(
            "version = 5\nharness = \"claude\"\nmodel = \"m2\"\nbin = \"{}\"\n",
            stub.display()
        ),
    )
    .unwrap();
    fs::write(root.join("tasks/demo/card.md"), "card\n").unwrap();
    fs::write(
        root.join("tasks/demo/task.toml"),
        format!(
            "agent = \"a\"\nsubstrate = \"local\"\n{budget_toml}\n[[trigger]]\nkind = \"manual\"\n"
        ),
    )
    .unwrap();
    fs::write(root.join("plane.toml"), format!("dev = true\n{plane_toml}")).unwrap();
    Plane::load(root).unwrap()
}

fn run_task(plane: &Plane, ledger: &mut Ledger) -> bitterblossom::ledger::RunRow {
    let id = ledger
        .ingest(IngressRequest {
            task: "demo",
            trigger_kind: "manual",
            idempotency_key: None,
            source_event_id: None,
            payload: None,
            parent_run_id: None,
        })
        .unwrap()
        .run_id;
    dispatch::dispatch_run(plane, ledger, &id).unwrap()
}

fn with_notify_stub<T>(root: &Path, f: impl FnOnce() -> T) -> (T, String) {
    // Env vars are process-global; tests touching BB_NOTIFY_BIN must not
    // overlap.
    let _guard = NOTIFY_ENV_LOCK.lock().unwrap();
    let notify_stub = root.join("notify-stub.sh");
    write_executable(&notify_stub, NOTIFY_STUB);
    let log = root.join("notify.log");
    std::env::set_var("BB_NOTIFY_BIN", &notify_stub);
    std::env::set_var("BB_NOTIFY_LOG", &log);
    let out = f();
    // Notifications are synchronous, so the log should be present when the
    // triggering work returns. Keep a short poll for filesystem latency.
    let mut text = String::new();
    for _ in 0..40 {
        text = fs::read_to_string(&log).unwrap_or_default();
        if !text.is_empty() {
            break;
        }
        std::thread::sleep(std::time::Duration::from_millis(50));
    }
    std::env::remove_var("BB_NOTIFY_BIN");
    std::env::remove_var("BB_NOTIFY_LOG");
    (out, text)
}

#[test]
fn notification_storm_is_synchronously_accounted() {
    let _guard = NOTIFY_ENV_LOCK.lock().unwrap();
    let dir = tempfile::tempdir().unwrap();
    let plane = setup(
        dir.path(),
        "[notify]\nwebhook_url = \"http://example.invalid/hook\"\n",
        "",
    );
    let notify_stub = dir.path().join("slow-notify-stub.sh");
    write_executable(&notify_stub, SLOW_NOTIFY_STUB);
    let log = dir.path().join("notify-storm.log");
    std::env::set_var("BB_NOTIFY_BIN", &notify_stub);
    std::env::set_var("BB_NOTIFY_LOG", &log);

    for i in 0..8 {
        bitterblossom::notify::notify(&plane, "storm_probe", &serde_json::json!({ "sequence": i }));
    }

    let finished = fs::read_to_string(&log).unwrap_or_default();
    std::env::remove_var("BB_NOTIFY_BIN");
    std::env::remove_var("BB_NOTIFY_LOG");
    assert_eq!(
        finished.lines().count(),
        8,
        "notify() returned before all curl waiters were accounted"
    );
}

#[test]
fn global_daily_ceiling_blocks_pre_dispatch() {
    let dir = tempfile::tempdir().unwrap();
    let plane = setup(dir.path(), "[budget]\nmax_cost_per_day_usd = 0.005\n", "");
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    // First run costs 0.0123, blowing the 0.005 daily ceiling.
    assert_eq!(run_task(&plane, &mut ledger).state, "success");
    let blocked = run_task(&plane, &mut ledger);
    assert_eq!(blocked.state, "blocked_budget");
    assert!(blocked.state_reason.unwrap().contains("ceiling"));
    // The ceiling does not park the task (a new UTC day clears it).
    assert!(ledger.parked_reason("demo").unwrap().is_none());
}

#[test]
fn advisory_per_run_cost_breach_parks_task_and_notifies() {
    let dir = tempfile::tempdir().unwrap();
    let plane = setup(
        dir.path(),
        "[notify]\nwebhook_url = \"http://example.invalid/hook\"\n",
        "[budget]\nmax_cost_per_run_usd = 0.001\n",
    );
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let (run, notify_log) = with_notify_stub(dir.path(), || run_task(&plane, &mut ledger));
    // Advisory: the run itself still completes — parking bounds the damage
    // to one run, it does not undo it.
    assert_eq!(run.state, "success");
    assert_eq!(run.cost_usd, Some(0.0123));
    let parked = ledger.parked_reason("demo").unwrap();
    assert!(parked.is_some(), "cost breach must park the task");
    assert!(notify_log.contains("budget_breach_parked"), "{notify_log}");

    // Next ingress is recorded but blocked.
    let blocked = ledger
        .ingest(IngressRequest {
            task: "demo",
            trigger_kind: "manual",
            idempotency_key: None,
            source_event_id: None,
            payload: None,
            parent_run_id: None,
        })
        .unwrap();
    assert_eq!(blocked.state, "blocked_budget");
}

#[test]
fn runs_per_day_park_fires_notification() {
    let dir = tempfile::tempdir().unwrap();
    let plane = setup(
        dir.path(),
        "[notify]\nwebhook_url = \"http://example.invalid/hook\"\n",
        "[budget]\nmax_runs_per_day = 1\n",
    );
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let ((), notify_log) = with_notify_stub(dir.path(), || {
        assert_eq!(run_task(&plane, &mut ledger).state, "success");
        assert_eq!(run_task(&plane, &mut ledger).state, "blocked_budget");
    });
    assert!(notify_log.contains("budget_blocked"), "{notify_log}");
    assert!(ledger.parked_reason("demo").unwrap().is_some());
}

#[test]
fn dead_letter_fires_notification_webhook() {
    let dir = tempfile::tempdir().unwrap();
    let plane = setup(
        dir.path(),
        "[notify]\nwebhook_url = \"http://example.invalid/hook\"\n",
        "pre_command = \"exit 9\"\n",
    );
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let (run, notify_log) = with_notify_stub(dir.path(), || run_task(&plane, &mut ledger));
    assert_eq!(run.state, "failure");
    assert!(notify_log.contains("run_dead_lettered"), "{notify_log}");
}

#[test]
fn agent_swap_is_one_config_edit_and_visible_in_ledger() {
    let dir = tempfile::tempdir().unwrap();
    let plane = setup(dir.path(), "", "");
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let first = run_task(&plane, &mut ledger);
    assert_eq!(first.agent_name.as_deref(), Some("a"));
    assert_eq!(first.agent_version, Some(1));

    // The swap: one line in task.toml. Nothing else changes.
    let task_toml = dir.path().join("tasks/demo/task.toml");
    let swapped = fs::read_to_string(&task_toml)
        .unwrap()
        .replace("agent = \"a\"", "agent = \"b\"");
    fs::write(&task_toml, swapped).unwrap();
    let plane = Plane::load(dir.path()).unwrap();

    let second = run_task(&plane, &mut ledger);
    assert_eq!(second.agent_name.as_deref(), Some("b"));
    assert_eq!(second.agent_version, Some(5));

    // Both bindings visible side by side in the ledger.
    let attempts: Vec<_> = ledger
        .list_runs(Some("demo"), None)
        .unwrap()
        .into_iter()
        .map(|r| (r.agent_name.unwrap(), r.agent_version.unwrap()))
        .collect();
    assert!(attempts.contains(&("a".to_string(), 1)));
    assert!(attempts.contains(&("b".to_string(), 5)));
}

#[test]
fn release_requeues_one_blocked_run_and_leaves_the_rest() {
    let dir = tempfile::tempdir().unwrap();
    let plane = setup(dir.path(), "", "");
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    // Park the task, then three deliveries pile up as blocked_budget.
    ledger.park_task("demo", "cost outlier").unwrap();
    let blocked: Vec<_> = (0..3).map(|_| run_task(&plane, &mut ledger)).collect();
    for r in &blocked {
        assert_eq!(r.state, "blocked_budget");
    }

    // Release exactly one: it returns to pending, the park clears, the rest stay blocked.
    ledger
        .release_blocked_run(&blocked[1].id, "current PR")
        .unwrap();
    assert_eq!(ledger.run(&blocked[1].id).unwrap().state, "pending");
    assert_eq!(ledger.run(&blocked[0].id).unwrap().state, "blocked_budget");
    assert_eq!(ledger.run(&blocked[2].id).unwrap().state, "blocked_budget");
    assert!(
        ledger.parked_reason("demo").unwrap().is_none(),
        "release must clear the park so the run is not immediately re-blocked"
    );

    // The released run now actually dispatches instead of bouncing back to blocked.
    let rerun = dispatch::dispatch_run(&plane, &mut ledger, &blocked[1].id).unwrap();
    assert_eq!(rerun.state, "success");
}

#[test]
fn retire_marks_blocked_run_terminal_and_keeps_history() {
    let dir = tempfile::tempdir().unwrap();
    let plane = setup(dir.path(), "", "");
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    ledger.park_task("demo", "cost outlier").unwrap();
    let blocked = run_task(&plane, &mut ledger);
    assert_eq!(blocked.state, "blocked_budget");

    ledger
        .retire_blocked_run(&blocked.id, "superseded by a newer PR")
        .unwrap();
    let row = ledger.run(&blocked.id).unwrap();
    assert_eq!(row.state, "retired");
    assert_eq!(
        row.state_reason.as_deref(),
        Some("superseded by a newer PR")
    );
    // Retiring one run does not resume the task; the park is a separate decision.
    assert!(ledger.parked_reason("demo").unwrap().is_some());

    // Only blocked_budget runs can be released or retired.
    assert!(ledger.retire_blocked_run(&blocked.id, "again").is_err());
    assert!(ledger.release_blocked_run(&blocked.id, "again").is_err());
}

#[test]
fn release_refuses_a_run_held_by_a_ceiling_not_a_park() {
    let dir = tempfile::tempdir().unwrap();
    let plane = setup(dir.path(), "[budget]\nmax_cost_per_day_usd = 0.005\n", "");
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    // First run blows the daily ceiling; the ceiling blocks without parking.
    assert_eq!(run_task(&plane, &mut ledger).state, "success");
    let blocked = run_task(&plane, &mut ledger);
    assert_eq!(blocked.state, "blocked_budget");
    assert!(ledger.parked_reason("demo").unwrap().is_none());

    // release cannot clear a ceiling, so it refuses instead of bouncing with a
    // false "released" success; retire still closes the row.
    assert!(ledger.release_blocked_run(&blocked.id, "try").is_err());
    ledger
        .retire_blocked_run(&blocked.id, "over budget today")
        .unwrap();
    assert_eq!(ledger.run(&blocked.id).unwrap().state, "retired");
}
