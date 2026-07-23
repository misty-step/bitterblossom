//! Local substrate concurrency proof: host leases, workflow CAS, and
//! attempt-scoped WorkspacePlan directories are exercised through real
//! dispatch/runtime entry points with a command-harness stub.

use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::{Path, PathBuf};
use std::process::{Command, Output};
use std::thread;
use std::time::{Duration, Instant};

use bitterblossom::dispatch;
use bitterblossom::ledger::{IngressRequest, Ledger};
use bitterblossom::spec::Plane;

fn bb(root: &Path, args: &[&str]) -> Output {
    Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", root.to_str().unwrap()])
        .args(args)
        .output()
        .unwrap()
}

fn bb_ok(root: &Path, args: &[&str]) -> String {
    let output = bb(root, args);
    assert!(
        output.status.success(),
        "bb {args:?} failed\nstdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );
    String::from_utf8_lossy(&output.stdout).to_string()
}

fn write_executable(path: &Path, text: &str) {
    fs::write(path, text).unwrap();
    let mut permissions = fs::metadata(path).unwrap().permissions();
    permissions.set_mode(0o755);
    fs::set_permissions(path, permissions).unwrap();
}

fn write_plane(root: &Path, stub: &Path) {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/legacy")).unwrap();
    fs::write(
        root.join("plane.toml"),
        "allow_local_substrate = true\ndb_path = \".bb/plane.db\"\n",
    )
    .unwrap();
    fs::write(
        root.join("agents/stub.toml"),
        format!(
            "version = 1\nharness = \"command\"\nmodel = \"stub\"\nbin = \"{}\"\n",
            stub.display()
        ),
    )
    .unwrap();
    fs::write(
        root.join("tasks/legacy/card.md"),
        "# Legacy concurrency lane\n",
    )
    .unwrap();
    fs::write(
        root.join("tasks/legacy/task.toml"),
        "agent = \"stub\"\nsubstrate = \"local\"\n[workspace]\nhost = \"single-host\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
}

fn wait_for_marker(root: &Path, prefix: &str) {
    let deadline = Instant::now() + Duration::from_secs(10);
    loop {
        let found = fs::read_dir(root)
            .unwrap()
            .flatten()
            .any(|entry| entry.file_name().to_string_lossy().starts_with(prefix));
        if found {
            return;
        }
        assert!(
            Instant::now() < deadline,
            "timed out waiting for {prefix} marker"
        );
        thread::sleep(Duration::from_millis(10));
    }
}

fn marker_paths(root: &Path, prefix: &str) -> Vec<PathBuf> {
    let mut paths: Vec<_> = fs::read_dir(root)
        .unwrap()
        .flatten()
        .filter_map(|entry| {
            entry
                .file_name()
                .to_string_lossy()
                .starts_with(prefix)
                .then_some(entry.path())
        })
        .collect();
    paths.sort();
    paths
}

fn workflow_run(root: &Path, workflow: &str) -> String {
    let value: serde_json::Value = serde_json::from_str(&bb_ok(
        root,
        &[
            "workflow",
            "accept",
            workflow,
            "--trigger",
            "test",
            "--json",
        ],
    ))
    .unwrap();
    assert_eq!(value["disposition"], "accepted", "{value}");
    value["run"]["id"].as_str().unwrap().to_string()
}

#[test]
fn local_substrate_concurrency_keeps_workspaces_distinct_and_lease_exclusive() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    let stub = root.join("stub.sh");
    // The child records its actual cwd. Legacy attempts hold the SQLite host
    // lease while sleeping; workflow attempts run concurrently in their own
    // WorkspacePlan directories. The marker mkdir is atomic, so the test can
    // observe the live overlap without a test-only synchronization primitive.
    write_executable(
        &stub,
        r#"#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname "$0")" && pwd -P)
printf '%s\n' "$PWD" > WORKDIR.txt
case "$PWD" in
  */.bb/runs/*)
    mkdir "$root/.legacy-entered-$$"
    sleep 0.5
    ;;
  */.bb/workflow-runs/*)
    mkdir "$root/.workflow-entered-$$"
    sleep 0.2
    ;;
esac
printf '%s\n' '{"schema_version":"bb.command_result.v1","result":"isolated local run"}'
"#,
    );
    write_plane(root, &stub);
    let plane = Plane::load(root).unwrap();

    let workflow_doc = root.join("parallel.toml");
    fs::write(
        &workflow_doc,
        format!(
            r#"name = "parallel"
goal = "Prove local workspace isolation."
[[trigger]]
kind = "test"

[[step]]
name = "work"
goal = "Record the local workspace."
host = "single-host"
[step.agent]
name = "stub"
version = 1
harness = "command"
model = "stub"
bin = "{}"

[policies]
substrate = "local"
"#,
            stub.display()
        ),
    )
    .unwrap();
    bb_ok(
        root,
        &["workflow", "create", workflow_doc.to_str().unwrap()],
    );
    bb_ok(root, &["workflow", "activate", "parallel"]);

    // Two independent workflow runs execute concurrently. Their claimed run
    // rows and attempt paths are separate, even though both name one host.
    let workflow_a = workflow_run(root, "parallel");
    let workflow_b = workflow_run(root, "parallel");
    let root_a = root.to_path_buf();
    let root_b = root.to_path_buf();
    let id_a = workflow_a.clone();
    let id_b = workflow_b.clone();
    let thread_a = thread::spawn(move || bb(&root_a, &["workflow", "execute", &id_a, "--json"]));
    let thread_b = thread::spawn(move || bb(&root_b, &["workflow", "execute", &id_b, "--json"]));
    wait_for_marker(root, ".workflow-entered-");
    let output_a = thread_a.join().unwrap();
    let output_b = thread_b.join().unwrap();
    assert!(
        output_a.status.success(),
        "workflow A: {}",
        String::from_utf8_lossy(&output_a.stderr)
    );
    assert!(
        output_b.status.success(),
        "workflow B: {}",
        String::from_utf8_lossy(&output_b.stderr)
    );
    assert_eq!(marker_paths(root, ".workflow-entered-").len(), 2);

    let work_a = root
        .join(".bb/workflow-runs")
        .join(&workflow_a)
        .join("attempt-1-work/workspace/WORKDIR.txt");
    let work_b = root
        .join(".bb/workflow-runs")
        .join(&workflow_b)
        .join("attempt-1-work/workspace/WORKDIR.txt");
    let path_a = fs::read_to_string(work_a).unwrap();
    let path_b = fs::read_to_string(work_b).unwrap();
    assert_ne!(path_a, path_b, "WorkspacePlan paths collided");
    assert!(path_a.contains(&format!("/workflow-runs/{workflow_a}/")));
    assert!(path_b.contains(&format!("/workflow-runs/{workflow_b}/")));

    // The legacy dispatch path still takes the SQLite host lease. Start one
    // run, observe its lease while the stub is in-flight, then start a peer on
    // the same host. The second run cannot enter until the first releases it.
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let legacy_a = ledger
        .ingest(IngressRequest {
            task: "legacy",
            trigger_kind: "manual",
            idempotency_key: Some("legacy-a"),
            source_event_id: None,
            payload: None,
            parent_run_id: None,
        })
        .unwrap()
        .run_id;
    let legacy_b = ledger
        .ingest(IngressRequest {
            task: "legacy",
            trigger_kind: "manual",
            idempotency_key: Some("legacy-b"),
            source_event_id: None,
            payload: None,
            parent_run_id: None,
        })
        .unwrap()
        .run_id;
    drop(ledger);
    let plane_a = plane.clone();
    let plane_b = plane.clone();
    let id_a = legacy_a.clone();
    let id_b = legacy_b.clone();
    let thread_a = thread::spawn(move || {
        let mut ledger = Ledger::open(&plane_a.db_path()).unwrap();
        dispatch::dispatch_run(&plane_a, &mut ledger, &id_a)
    });
    wait_for_marker(root, ".legacy-entered-");
    let lease_ledger = Ledger::open(&plane.db_path()).unwrap();
    assert_eq!(
        lease_ledger.lease_holder("single-host").unwrap(),
        Some(legacy_a.clone())
    );
    drop(lease_ledger);
    let thread_b = thread::spawn(move || {
        let mut ledger = Ledger::open(&plane_b.db_path()).unwrap();
        dispatch::dispatch_run(&plane_b, &mut ledger, &id_b)
    });
    let result_a = thread_a.join().unwrap().unwrap();
    let result_b = thread_b.join().unwrap().unwrap();
    assert_eq!(result_a.state, "success");
    assert_eq!(result_b.state, "success");
    assert!(Ledger::open(&plane.db_path())
        .unwrap()
        .lease_holder("single-host")
        .unwrap()
        .is_none());
    assert_eq!(marker_paths(root, ".legacy-entered-").len(), 2);
    let legacy_paths = [legacy_a, legacy_b].map(|id| {
        let attempt = Ledger::open(&plane.db_path())
            .unwrap()
            .attempts(&id)
            .unwrap();
        assert_eq!(attempt.len(), 1);
        fs::read_to_string(
            Path::new(attempt[0].artifact_dir.as_deref().unwrap()).join("workspace/WORKDIR.txt"),
        )
        .unwrap()
    });
    assert_ne!(
        legacy_paths[0], legacy_paths[1],
        "legacy WorkspacePlan paths collided"
    );
}

#[test]
fn workflow_claim_is_compare_and_swap_under_concurrent_execute() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    let stub = root.join("stub.sh");
    write_executable(
        &stub,
        r#"#!/bin/sh
set -eu
printf '%s\n' '{"schema_version":"bb.command_result.v1","result":"one execution"}'
"#,
    );
    write_plane(root, &stub);
    let workflow_doc = root.join("cas.toml");
    fs::write(
        &workflow_doc,
        format!(
            r#"name = "cas"
goal = "Prove one queued run has one executor."
[[trigger]]
kind = "test"
[[step]]
name = "work"
goal = "Execute once."
[step.agent]
name = "stub"
version = 1
harness = "command"
model = "stub"
bin = "{}"
[policies]
substrate = "local"
"#,
            stub.display()
        ),
    )
    .unwrap();
    bb_ok(
        root,
        &["workflow", "create", workflow_doc.to_str().unwrap()],
    );
    bb_ok(root, &["workflow", "activate", "cas"]);
    let run_id = workflow_run(root, "cas");
    let root_a = root.to_path_buf();
    let root_b = root.to_path_buf();
    let id_a = run_id.clone();
    let id_b = run_id.clone();
    let a = thread::spawn(move || bb(&root_a, &["workflow", "execute", &id_a, "--json"]));
    let b = thread::spawn(move || bb(&root_b, &["workflow", "execute", &id_b, "--json"]));
    let a = a.join().unwrap();
    let b = b.join().unwrap();
    let successful = usize::from(a.status.success()) + usize::from(b.status.success());
    assert_eq!(
        successful,
        1,
        "exactly one executor must win CAS\nA status={:?}\nA stdout={}\nA stderr={}\nB status={:?}\nB stdout={}\nB stderr={}",
        a.status,
        String::from_utf8_lossy(&a.stdout),
        String::from_utf8_lossy(&a.stderr),
        b.status,
        String::from_utf8_lossy(&b.stdout),
        String::from_utf8_lossy(&b.stderr),
    );
    let ledger = Ledger::open(&Plane::load(root).unwrap().db_path()).unwrap();
    assert_eq!(ledger.workflow_step_runs(&run_id).unwrap().len(), 1);
}
