use std::fs;
use std::process::Command;

use bitterblossom::harness::parse_output;
use bitterblossom::spec::{AuthClass, Plane, TriggerSpec};
use serde_json::Value;

fn repo_root() -> std::path::PathBuf {
    std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR"))
}

fn public_plane_root() -> std::path::PathBuf {
    repo_root().join("tests/fixtures/public-plane")
}

fn git(repo: &std::path::Path, args: &[&str]) {
    let status = Command::new("git")
        .current_dir(repo)
        .args(args)
        .status()
        .unwrap();
    assert!(status.success(), "git {args:?} failed in {repo:?}");
}

fn write_json(path: &std::path::Path, value: &Value) {
    fs::write(path, serde_json::to_string_pretty(value).unwrap()).unwrap();
}

#[test]
fn branch_prune_task_is_report_first_cron_reflex_contract() {
    let plane = Plane::load(&public_plane_root()).unwrap();
    let task = plane.task("branch-prune").unwrap();

    assert_eq!(task.agent_name, "branch-pruner");
    assert_eq!(task.agent.harness, "command");
    assert_eq!(task.agent.auth_class().unwrap(), AuthClass::Api);
    assert_eq!(task.agent.role.as_deref(), Some("branch-pruner"));
    assert!(task.agent.secrets.contains(&"GH_TOKEN".to_string()));
    assert!(task
        .agent
        .args
        .iter()
        .any(|arg| arg.contains("hygiene-reflex.py")));
    assert!(task.agent.args.iter().any(|arg| arg == "branch-prune"));
    assert_eq!(task.spec.substrate, "sprites");
    assert_eq!(task.spec.required_artifacts, vec!["REPORT.json"]);
    assert_eq!(task.spec.budget.timeout_minutes, Some(30));
    assert_eq!(task.spec.budget.max_runs_per_day, Some(4));
    assert_eq!(task.spec.budget.max_cost_per_run_usd, Some(0.01));

    let has_manual = task
        .spec
        .triggers
        .iter()
        .any(|trigger| matches!(trigger, TriggerSpec::Manual));
    let cron = task
        .spec
        .triggers
        .iter()
        .find_map(|trigger| match trigger {
            TriggerSpec::Cron { schedule } => Some(schedule.as_str()),
            TriggerSpec::Manual | TriggerSpec::Webhook { .. } => None,
        })
        .expect("branch-prune cron trigger");
    assert!(has_manual);
    assert_eq!(cron, "0 14 * * 6");

    for required in [
        "REPORT mode is the default",
        "DRY-RUN FIRST",
        "git push --delete",
        "Never force-push",
        "default branch",
        "unmerged",
        "open PR",
        "delete_enabled",
        "BRANCH_PRUNE_ENABLE_DELETE",
        "bb.branch_prune_report.v1",
        "\"would_delete\"",
        "\"artifact_paths\": [\"REPORT.json\"]",
    ] {
        assert!(task.card.contains(required), "card missing {required}");
    }
}

#[test]
fn dependabot_triage_task_is_report_first_cron_reflex_contract() {
    let plane = Plane::load(&public_plane_root()).unwrap();
    let task = plane.task("dependabot-triage").unwrap();

    assert_eq!(task.agent_name, "dependabot-triager");
    assert_eq!(task.agent.harness, "command");
    assert_eq!(task.agent.auth_class().unwrap(), AuthClass::Api);
    assert_eq!(task.agent.role.as_deref(), Some("dependabot-triager"));
    assert!(task.agent.secrets.contains(&"GH_TOKEN".to_string()));
    assert!(task
        .agent
        .args
        .iter()
        .any(|arg| arg.contains("hygiene-reflex.py")));
    assert!(task.agent.args.iter().any(|arg| arg == "dependabot-triage"));
    assert_eq!(task.spec.substrate, "sprites");
    assert_eq!(task.spec.required_artifacts, vec!["REPORT.json"]);
    assert_eq!(task.spec.budget.timeout_minutes, Some(20));
    assert_eq!(task.spec.budget.max_runs_per_day, Some(8));
    assert_eq!(task.spec.budget.max_cost_per_run_usd, Some(0.01));

    let has_manual = task
        .spec
        .triggers
        .iter()
        .any(|trigger| matches!(trigger, TriggerSpec::Manual));
    let cron = task
        .spec
        .triggers
        .iter()
        .find_map(|trigger| match trigger {
            TriggerSpec::Cron { schedule } => Some(schedule.as_str()),
            TriggerSpec::Manual | TriggerSpec::Webhook { .. } => None,
        })
        .expect("dependabot triage cron trigger");
    assert!(has_manual);
    assert_eq!(cron, "0 15 * * *");

    for required in [
        "REPORT mode is the default",
        "patch",
        "minor",
        "major",
        "merge-on-green",
        "dev/CI deps only",
        "never majors",
        "never runtime deps",
        "DEPENDABOT_TRIAGE_ENABLE_MERGE",
        "bb.dependabot_triage_report.v1",
        "\"merge_candidates\"",
        "\"artifact_paths\": [\"REPORT.json\"]",
    ] {
        assert!(task.card.contains(required), "card missing {required}");
    }
}

#[test]
fn branch_prune_report_fixture_preserves_never_delete_rules() {
    let report: Value = serde_json::from_str(include_str!(
        "fixtures/contracts/bb.branch_prune_report.v1.valid.json"
    ))
    .unwrap();

    assert_eq!(report["schema_version"], "bb.branch_prune_report.v1");
    assert_eq!(report["mode"], "report");
    assert_eq!(report["authority"]["current"], "report-only");
    assert_eq!(report["authority"]["delete_enabled"], false);
    assert_eq!(report["artifact_paths"], serde_json::json!(["REPORT.json"]));
    assert_eq!(report["summary"]["total_would_delete"], 2);

    let repo = &report["repos"][0];
    assert_eq!(repo["default_branch"], "master");
    assert!(repo["would_delete"]
        .as_array()
        .unwrap()
        .contains(&serde_json::json!("old/merged-1")));
    assert!(repo["would_delete"]
        .as_array()
        .unwrap()
        .contains(&serde_json::json!("old/merged-2")));

    let kept = repo["kept"].as_array().unwrap();
    for (branch, reason) in [
        ("master", "default_branch"),
        ("feature/unmerged", "unmerged"),
        ("dependabot/npm/foo-1.2.3", "open_pr"),
        ("release/keep", "explicit_never"),
    ] {
        let entry = kept
            .iter()
            .find(|entry| entry["branch"] == branch)
            .unwrap_or_else(|| panic!("missing kept branch {branch}"));
        assert!(
            entry["reasons"]
                .as_array()
                .unwrap()
                .contains(&serde_json::json!(reason)),
            "{branch} missing reason {reason}"
        );
    }
}

#[test]
fn dependabot_triage_report_fixture_is_conservative_floor() {
    let report: Value = serde_json::from_str(include_str!(
        "fixtures/contracts/bb.dependabot_triage_report.v1.valid.json"
    ))
    .unwrap();

    assert_eq!(report["schema_version"], "bb.dependabot_triage_report.v1");
    assert_eq!(report["mode"], "report");
    assert_eq!(report["authority"]["current"], "report-only");
    assert_eq!(report["authority"]["merge_enabled"], false);
    assert_eq!(report["artifact_paths"], serde_json::json!(["REPORT.json"]));
    assert_eq!(report["summary"]["open_dependabot_prs"], 4);
    assert_eq!(report["summary"]["merge_candidates"], 1);

    let prs = report["repos"][0]["prs"].as_array().unwrap();
    let patch_ci = prs
        .iter()
        .find(|pr| pr["number"] == 10)
        .expect("patch CI PR");
    assert_eq!(patch_ci["version_class"], "patch");
    assert_eq!(patch_ci["dependency_scope"], "ci");
    assert_eq!(patch_ci["ci_state"], "green");
    assert_eq!(patch_ci["decision"], "merge_candidate");

    for (number, decision) in [
        (11, "needs_human_runtime_dependency"),
        (12, "needs_human_major"),
        (13, "wait_for_green"),
    ] {
        let pr = prs
            .iter()
            .find(|pr| pr["number"] == number)
            .unwrap_or_else(|| panic!("missing PR {number}"));
        assert_eq!(pr["decision"], decision);
        assert_eq!(pr["would_merge"], false);
    }
}

#[test]
fn branch_prune_wrapper_reports_local_merged_branches_without_deleting() {
    let dir = tempfile::tempdir().unwrap();
    let remote = dir.path().join("remote.git");
    let work = dir.path().join("work");
    fs::create_dir(&work).unwrap();
    git(&work, &["init", "-q", "-b", "master"]);
    git(&work, &["config", "user.name", "Fixture"]);
    git(&work, &["config", "user.email", "fixture@example.com"]);
    fs::write(work.join("README.md"), "base\n").unwrap();
    git(&work, &["add", "README.md"]);
    git(&work, &["commit", "-q", "-m", "base"]);
    git(&work, &["branch", "old/merged"]);
    git(&work, &["branch", "dependabot/npm/open-pr"]);
    git(&work, &["branch", "release/keep"]);
    git(&work, &["checkout", "-q", "-b", "feature/unmerged"]);
    fs::write(work.join("README.md"), "base\nunmerged\n").unwrap();
    git(&work, &["commit", "-am", "unmerged"]);
    git(&work, &["checkout", "-q", "master"]);
    let status = Command::new("git")
        .args(["init", "--bare", "-q"])
        .arg(&remote)
        .status()
        .unwrap();
    assert!(status.success());
    git(
        &work,
        &["remote", "add", "origin", remote.to_str().unwrap()],
    );
    git(&work, &["push", "-q", "origin", "master"]);
    git(&work, &["push", "-q", "origin", "old/merged"]);
    git(&work, &["push", "-q", "origin", "dependabot/npm/open-pr"]);
    git(&work, &["push", "-q", "origin", "release/keep"]);
    git(&work, &["push", "-q", "origin", "feature/unmerged"]);

    let run_dir = dir.path().join("run");
    fs::create_dir(&run_dir).unwrap();
    write_json(
        &run_dir.join("EVENT.json"),
        &serde_json::json!({
            "mode": "report",
            "repos": [{
                "repo": "fixture/local",
                "remote": remote,
                "default_branch": "master",
                "never": ["release/*"],
                "open_pr_branches": ["dependabot/npm/open-pr"]
            }]
        }),
    );
    fs::write(
        run_dir.join("RUN.json"),
        r#"{"run_id":"run-hygiene","task":"branch-prune","trigger":{"kind":"manual","idempotency_key":"test"}}"#,
    )
    .unwrap();

    let script = repo_root().join("scripts/hygiene-reflex.py");
    let output = Command::new("python3")
        .current_dir(&run_dir)
        .arg(&script)
        .arg("branch-prune")
        .output()
        .unwrap();
    assert!(
        output.status.success(),
        "stdout={}\nstderr={}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );

    let report: Value =
        serde_json::from_str(&fs::read_to_string(run_dir.join("REPORT.json")).unwrap()).unwrap();
    assert_eq!(report["summary"]["total_would_delete"], 1);
    assert_eq!(
        report["repos"][0]["would_delete"],
        serde_json::json!(["old/merged"])
    );
    assert_eq!(report["repos"][0]["deleted"], serde_json::json!([]));

    let parsed = parse_output("command", &String::from_utf8(output.stdout).unwrap()).unwrap();
    assert_eq!(
        parsed.result,
        "branch-prune report: 1 repos, 1 branches would delete"
    );
}

#[test]
fn dependabot_triage_wrapper_reports_sample_prs_without_merging() {
    let dir = tempfile::tempdir().unwrap();
    write_json(
        &dir.path().join("EVENT.json"),
        &serde_json::json!({
            "mode": "report",
            "repos": [{
                "repo": "fixture/dependabot",
                "dependabot_prs": [
                    {
                        "number": 10,
                        "title": "Bump actions/checkout from 4.1.0 to 4.1.1",
                        "url": "https://github.com/fixture/dependabot/pull/10",
                        "createdAt": "2026-07-01T00:00:00Z",
                        "headRefName": "dependabot/github_actions/actions/checkout-4.1.1",
                        "files": [{"path": ".github/workflows/ci.yml"}],
                        "statusCheckRollup": [{"conclusion": "SUCCESS"}]
                    },
                    {
                        "number": 12,
                        "title": "Bump rails from 7.2.1 to 8.0.0",
                        "url": "https://github.com/fixture/dependabot/pull/12",
                        "createdAt": "2026-06-20T00:00:00Z",
                        "headRefName": "dependabot/bundler/rails-8.0.0",
                        "files": [{"path": "Gemfile.lock"}],
                        "statusCheckRollup": [{"conclusion": "SUCCESS"}]
                    },
                    {
                        "number": 14,
                        "title": "chore(deps-dev): bump vitest from 3.2.4 to 3.2.6",
                        "url": "https://github.com/fixture/dependabot/pull/14",
                        "createdAt": "2026-07-02T00:00:00Z",
                        "headRefName": "dependabot/npm_and_yarn/vitest-3.2.6",
                        "files": [{"path": "package-lock.json"}],
                        "statusCheckRollup": [{"conclusion": "SUCCESS"}]
                    }
                ]
            }]
        }),
    );
    fs::write(
        dir.path().join("RUN.json"),
        r#"{"run_id":"run-dependabot","task":"dependabot-triage","trigger":{"kind":"manual","idempotency_key":"test"}}"#,
    )
    .unwrap();

    let output = Command::new("python3")
        .current_dir(dir.path())
        .arg(repo_root().join("scripts/hygiene-reflex.py"))
        .arg("dependabot-triage")
        .output()
        .unwrap();
    assert!(
        output.status.success(),
        "stdout={}\nstderr={}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );

    let report: Value =
        serde_json::from_str(&fs::read_to_string(dir.path().join("REPORT.json")).unwrap()).unwrap();
    assert_eq!(report["summary"]["open_dependabot_prs"], 3);
    assert_eq!(report["summary"]["merge_candidates"], 2);
    assert_eq!(report["repos"][0]["prs"][0]["decision"], "merge_candidate");
    assert_eq!(
        report["repos"][0]["prs"][1]["decision"],
        "needs_human_major"
    );
    assert_eq!(report["repos"][0]["prs"][2]["decision"], "merge_candidate");

    let parsed = parse_output("command", &String::from_utf8(output.stdout).unwrap()).unwrap();
    assert_eq!(
        parsed.result,
        "dependabot-triage report: 1 repos, 2 merge candidates"
    );
}
