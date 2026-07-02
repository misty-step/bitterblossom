//! Repo-owned workload config: a target repo may version `.bb/tasks/*`,
//! but the plane keeps agent, substrate, workspace, and budget authority.

use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::Path;
use std::process::Command;

use bitterblossom::dispatch;
use bitterblossom::ledger::{IngressRequest, Ledger};
use bitterblossom::spec::Plane;

const STUB: &str = r#"#!/bin/sh
cat > /dev/null
echo 'repo workload ok'
"#;

fn write_executable(path: &Path, content: &str) {
    fs::write(path, content).unwrap();
    fs::set_permissions(path, fs::Permissions::from_mode(0o755)).unwrap();
}

fn git(root: &Path, args: &[&str]) {
    let output = Command::new("git")
        .args(args)
        .current_dir(root)
        .output()
        .unwrap();
    assert!(
        output.status.success(),
        "git {args:?}\nstdout={}\nstderr={}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );
}

fn target_repo(root: &Path, task_toml: &str, card: &str) {
    fs::create_dir_all(root.join(".bb/tasks/review")).unwrap();
    fs::write(root.join(".bb/tasks/review/card.md"), card).unwrap();
    fs::write(root.join(".bb/tasks/review/task.toml"), task_toml).unwrap();
    git(root, &["init"]);
    git(root, &["checkout", "-B", "main"]);
    git(root, &["config", "user.email", "bb-test@example.com"]);
    git(root, &["config", "user.name", "Bitterblossom Test"]);
    git(root, &["add", "."]);
    git(root, &["commit", "-m", "seed repo workload"]);
}

fn write_plane(root: &Path, target: &Path, workload_extra: &str) {
    write_plane_with_max_runs(root, target, workload_extra, 5);
}

fn write_plane_with_max_runs(
    root: &Path,
    target: &Path,
    workload_extra: &str,
    max_runs_per_day: u32,
) {
    fs::create_dir_all(root.join("agents")).unwrap();
    let stub_path = root.join("stub.sh");
    write_executable(&stub_path, STUB);
    fs::write(
        root.join("agents/stub.toml"),
        format!("harness = \"command\"\nbin = \"{}\"\n", stub_path.display()),
    )
    .unwrap();
    fs::write(
        root.join("plane.toml"),
        format!(
            r#"dev = true

[[workload_repo]]
name = "target"
path = "{}"
ref = "main"
agent = "stub"
substrate = "local"
{workload_extra}

[workload_repo.workspace]
host = "local"

[workload_repo.budget_caps]
timeout_minutes = 10
max_runs_per_day = {max_runs_per_day}
max_cost_per_run_usd = 1.0
"#,
            target.display(),
        ),
    )
    .unwrap();
}

fn write_plane_with_budget(root: &Path, target: &Path, max_runs_per_day: u32) {
    write_plane_with_max_runs(root, target, "", max_runs_per_day);
}

fn repo_task_toml(extra: &str) -> String {
    format!(
        r#"{extra}
[budget]
timeout_minutes = 5
max_runs_per_day = 2

[[trigger]]
kind = "manual"
"#
    )
}

#[test]
fn allowlisted_repo_tasks_load_with_plane_owned_binding_and_check_source() {
    let dir = tempfile::tempdir().unwrap();
    let target = tempfile::tempdir().unwrap();
    target_repo(target.path(), &repo_task_toml(""), "# Repo card v1\n");
    write_plane(dir.path(), target.path(), "");

    let plane = Plane::load(dir.path()).unwrap();
    let task = plane.tasks.get("target/review").unwrap();
    let target_path = target.path().canonicalize().unwrap().display().to_string();
    assert_eq!(task.agent_name, "stub");
    assert_eq!(task.spec.substrate, "local");
    assert_eq!(task.spec.budget.timeout_minutes, Some(5));
    assert_eq!(task.spec.budget.max_runs_per_day, Some(2));
    assert_eq!(task.spec.budget.max_cost_per_run_usd, Some(1.0));
    assert_eq!(task.spec.workspace.repos.len(), 1);
    assert_eq!(task.spec.workspace.repos[0].url, target_path);
    assert_eq!(task.source.as_ref().unwrap().repo, target_path);
    assert_eq!(task.source.as_ref().unwrap().r#ref, "main");

    let output = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", dir.path().to_str().unwrap(), "check"])
        .output()
        .unwrap();
    assert!(output.status.success());
    let stdout = String::from_utf8_lossy(&output.stdout);
    assert!(stdout.contains("task target/review"), "{stdout}");
    assert!(stdout.contains("source="), "{stdout}");
    assert!(stdout.contains(&target_path), "{stdout}");
    assert!(stdout.contains("@main"), "{stdout}");
}

#[test]
fn repo_url_overrides_workspace_clone_without_changing_config_source() {
    let dir = tempfile::tempdir().unwrap();
    let target = tempfile::tempdir().unwrap();
    target_repo(target.path(), &repo_task_toml(""), "# Repo card\n");
    write_plane(
        dir.path(),
        target.path(),
        "repo_url = \"https://github.com/example/repo.git\"\n",
    );

    let plane = Plane::load(dir.path()).unwrap();
    let task = plane.tasks.get("target/review").unwrap();
    assert_eq!(
        task.spec.workspace.repos[0].url,
        "https://github.com/example/repo.git"
    );
    assert_eq!(
        task.source.as_ref().unwrap().repo,
        target.path().canonicalize().unwrap().display().to_string()
    );
}

#[test]
fn repo_tasks_cannot_take_over_agent_substrate_workspace_or_budget_authority() {
    let cases = [
        ("agent = \"missing\"\n", "unknown agent 'missing'"),
        (
            "[budget]\ntimeout_minutes = 30\n[[trigger]]\nkind = \"manual\"\n",
            "timeout_minutes 30 exceeds plane cap 10",
        ),
        (
            "substrate = \"sprites\"\n",
            "requests substrate 'sprites' but plane grants 'local'",
        ),
        (
            "[workspace]\nhost = \"other\"\n",
            "declares workspace authority",
        ),
    ];

    for (extra, want) in cases {
        let dir = tempfile::tempdir().unwrap();
        let target = tempfile::tempdir().unwrap();
        let task_toml = if extra.starts_with("[budget]") {
            extra.to_string()
        } else {
            repo_task_toml(extra)
        };
        target_repo(target.path(), &task_toml, "# Repo card\n");
        write_plane(dir.path(), target.path(), "");
        let err = Plane::load(dir.path()).unwrap_err();
        let msg = err.to_string();
        assert!(msg.contains("workload repo 'target'"), "{msg}");
        assert!(msg.contains(want), "{msg}");
    }
}

#[test]
fn workload_repo_url_is_rejected_in_v1_instead_of_fetching_on_hot_path() {
    let dir = tempfile::tempdir().unwrap();
    fs::create_dir_all(dir.path().join("agents")).unwrap();
    fs::write(
        dir.path().join("agents/stub.toml"),
        "harness = \"command\"\nbin = \"/bin/echo\"\n",
    )
    .unwrap();
    fs::write(
        dir.path().join("plane.toml"),
        r#"dev = true

[[workload_repo]]
name = "target"
url = "https://github.com/example/repo.git"
ref = "main"
agent = "stub"
substrate = "local"

[workload_repo.workspace]
host = "local"
"#,
    )
    .unwrap();

    let err = Plane::load(dir.path()).unwrap_err();
    assert!(
        err.to_string().contains("url checkout is not in v1"),
        "{err}"
    );
}

#[test]
fn repo_task_changes_refresh_on_load_and_allowlist_removal_removes_tasks() {
    let dir = tempfile::tempdir().unwrap();
    let target = tempfile::tempdir().unwrap();
    target_repo(target.path(), &repo_task_toml(""), "# Repo card v1\n");
    write_plane(dir.path(), target.path(), "");

    let first = Plane::load(dir.path()).unwrap();
    assert!(first.tasks["target/review"].card.contains("v1"));

    fs::write(
        target.path().join(".bb/tasks/review/card.md"),
        "# Repo card v2\n",
    )
    .unwrap();
    let second = Plane::load(dir.path()).unwrap();
    assert!(second.tasks["target/review"].card.contains("v2"));

    fs::write(dir.path().join("plane.toml"), "dev = true\n").unwrap();
    let without_repo = Plane::load(dir.path()).unwrap();
    assert!(!without_repo.tasks.contains_key("target/review"));
}

#[test]
fn budget_blocked_repo_run_records_config_source() {
    let dir = tempfile::tempdir().unwrap();
    let target = tempfile::tempdir().unwrap();
    target_repo(
        target.path(),
        "[budget]\ntimeout_minutes = 5\n[[trigger]]\nkind = \"manual\"\n",
        "# Repo card\n",
    );
    write_plane_with_budget(dir.path(), target.path(), 0);

    let plane = Plane::load(dir.path()).unwrap();
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let run_id = ledger
        .ingest(IngressRequest {
            task: "target/review",
            trigger_kind: "manual",
            idempotency_key: None,
            source_event_id: None,
            payload: None,
            parent_run_id: None,
        })
        .unwrap()
        .run_id;
    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();

    assert_eq!(run.state, "blocked_budget");
    let target_path = target.path().canonicalize().unwrap().display().to_string();
    assert_eq!(
        run.config_source_repo.as_deref(),
        Some(target_path.as_str())
    );
    assert_eq!(run.config_source_ref.as_deref(), Some("main"));
}

#[test]
fn dispatch_records_repo_config_source_on_run_row() {
    let dir = tempfile::tempdir().unwrap();
    let target = tempfile::tempdir().unwrap();
    target_repo(target.path(), &repo_task_toml(""), "# Repo card\n");
    write_plane(dir.path(), target.path(), "");

    let plane = Plane::load(dir.path()).unwrap();
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let run_id = ledger
        .ingest(IngressRequest {
            task: "target/review",
            trigger_kind: "manual",
            idempotency_key: None,
            source_event_id: None,
            payload: None,
            parent_run_id: None,
        })
        .unwrap()
        .run_id;
    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();

    assert_eq!(run.state, "success");
    let target_path = target.path().canonicalize().unwrap().display().to_string();
    assert_eq!(
        run.config_source_repo.as_deref(),
        Some(target_path.as_str())
    );
    assert_eq!(run.config_source_ref.as_deref(), Some("main"));
}
