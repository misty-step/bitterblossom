use std::fs;
use std::os::unix::fs::PermissionsExt;

use bitterblossom::ledger::{IngressRequest, Ledger};
use bitterblossom::spec::Plane;

fn write_executable(path: &std::path::Path, content: &str) {
    fs::write(path, content).unwrap();
    fs::set_permissions(path, fs::Permissions::from_mode(0o755)).unwrap();
}

fn write_plane(root: &std::path::Path) {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/review")).unwrap();
    fs::create_dir_all(root.join("vendor/roster")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();

    let agent = root.join("agent.sh");
    write_executable(
        &agent,
        r#"#!/bin/sh
cat >/dev/null
printf '{"schema_version":"bb.command_result.v1","result":"roster review ok","turns":1,"cost_usd":0.01}\n'
"#,
    );

    let roster = root.join("roster-stub.sh");
    write_executable(
        &roster,
        &format!(
            r#"#!/bin/sh
case "$*" in
  *"materialize cerberus --harness bb"*)
    cat <<'TOML'
version = 3
harness = "command"
model = ""
role = "cerberus"
auth = "api"
skills = ["code-review", "ci"]
bin = "{agent}"
TOML
    ;;
  *"brief cerberus"*)
    cat <<'BRIEF'
# Roster Brief: cerberus

## Evidence Contract

- Every finding must cite concrete evidence.
BRIEF
    ;;
  *)
    echo "unexpected roster args: $*" >&2
    exit 7
    ;;
esac
"#,
            agent = agent.display()
        ),
    );

    fs::write(
        root.join("agents/cerberus.toml"),
        format!(
            r#"[roster]
root = "vendor/roster"
agent = "cerberus"
bin = "{roster}"
"#,
            roster = roster.display()
        ),
    )
    .unwrap();
    fs::write(
        root.join("tasks/review/task.toml"),
        format!(
            r#"agent = "cerberus"
substrate = "local"

[roster_brief]
root = "vendor/roster"
agent = "cerberus"
bin = "{roster}"

[[trigger]]
kind = "manual"
"#,
            roster = roster.display()
        ),
    )
    .unwrap();
    fs::write(
        root.join("tasks/review/card.md"),
        "Task-specific review target.\n",
    )
    .unwrap();
}

#[test]
fn roster_sources_materialize_agent_and_prepend_task_brief() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let plane = Plane::load(dir.path()).unwrap();
    let task = plane.task("review").unwrap();

    assert_eq!(task.agent_name, "cerberus");
    assert_eq!(task.agent.version, 3);
    assert_eq!(task.agent.harness, "command");
    assert_eq!(task.agent.role.as_deref(), Some("cerberus"));
    assert_eq!(task.agent.skills, ["code-review", "ci"]);
    assert!(task.card.contains("# Roster Brief: cerberus"));
    assert!(task
        .card
        .contains("Every finding must cite concrete evidence."));
    assert!(task.card.contains("## Bitterblossom Task Commission"));
    assert!(task.card.contains("Task-specific review target."));
    assert_eq!(task.roster.agent.as_ref().unwrap().agent, "cerberus");
    assert_eq!(task.roster.brief.as_ref().unwrap().agent, "cerberus");

    let ledger = Ledger::open(&plane.db_path()).unwrap();
    let rows = bitterblossom::serve::tasks_view(&plane, &ledger).unwrap();
    assert_eq!(rows[0]["roster"]["agent"]["agent"], "cerberus");
    assert_eq!(rows[0]["roster"]["brief"]["agent"], "cerberus");
}

#[test]
fn roster_provenance_is_recorded_on_native_run() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let plane = Plane::load(dir.path()).unwrap();
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let outcome = ledger
        .ingest(IngressRequest {
            task: "review",
            trigger_kind: "manual",
            idempotency_key: Some("roster-test"),
            source_event_id: None,
            payload: Some(r#"{"repo":"misty-step/example","pr":1}"#),
            parent_run_id: None,
        })
        .unwrap();

    let run = bitterblossom::dispatch::dispatch_run(&plane, &mut ledger, &outcome.run_id).unwrap();
    assert_eq!(run.state, "success");

    let events = ledger.events(&run.id).unwrap();
    let provenance = events
        .iter()
        .find(|event| event.kind == "roster_provenance")
        .expect("roster provenance event");
    let data = provenance.data.as_deref().unwrap();
    assert!(data.contains("\"agent\":\"cerberus\""), "{data}");
}
