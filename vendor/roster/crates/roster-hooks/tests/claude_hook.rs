//! Smoke tests for the `claude-hook <name>` stdin/stdout protocol: feed a
//! sample hook event on stdin, assert the output shape a real Claude Code
//! hook config would parse.

use assert_cmd::Command;
use predicates::prelude::*;

fn hooks_cmd() -> Command {
    Command::cargo_bin("roster-hooks").expect("roster-hooks binary")
}

#[test]
fn permission_auto_approve_smoke() {
    hooks_cmd()
        .args(["claude-hook", "permission-auto-approve"])
        .write_stdin(r#"{"tool_name":"Read","tool_input":{"file_path":"README.md"}}"#)
        .assert()
        .success()
        .stdout(predicate::str::contains("\"permissionDecision\":\"allow\""))
        .stdout(predicate::str::contains("\"hookEventName\":\"PreToolUse\""));
}

#[test]
fn time_context_smoke() {
    hooks_cmd()
        .args(["claude-hook", "time-context"])
        .write_stdin("{}")
        .assert()
        .success()
        .stdout(predicate::str::contains("\"result\":\"continue\""))
        .stdout(predicate::str::contains("Current time:"));
}

#[test]
fn destructive_command_guard_smoke() {
    hooks_cmd()
        .args(["claude-hook", "destructive-command-guard"])
        .write_stdin(r#"{"tool_name":"Bash","tool_input":{"command":"git reset --hard"}}"#)
        .assert()
        .success()
        .stdout(predicate::str::contains("\"permissionDecision\":\"deny\""))
        .stdout(predicate::str::contains("BLOCKED"));
}

#[test]
fn skill_invocation_tracker_smoke() {
    let temp = tempfile::tempdir().expect("temp dir");
    let log = temp.path().join("skill-invocations.jsonl");

    hooks_cmd()
        .args(["claude-hook", "skill-invocation-tracker"])
        .env("SKILL_TRACKER_LOG_PATH", &log)
        .write_stdin(
            r#"{"tool_name":"Skill","tool_input":{"skill":"orient","args":""},"session_id":"smoke","cwd":"/tmp/proj"}"#,
        )
        .assert()
        .success();

    let logged = std::fs::read_to_string(&log).expect("log file written");
    assert!(logged.contains("\"skill\":\"orient\""));
    assert!(logged.contains("\"session_id\":\"smoke\""));
}

#[test]
fn unknown_hook_name_fails_with_usage_message() {
    hooks_cmd()
        .args(["claude-hook", "not-a-real-hook"])
        .write_stdin("{}")
        .assert()
        .failure()
        .stderr(predicate::str::contains("unknown claude-hook"));
}

#[test]
fn no_command_prints_usage_and_fails() {
    hooks_cmd()
        .assert()
        .failure()
        .stderr(predicate::str::contains("usage: roster-hooks claude-hook"));
}

#[test]
fn unknown_top_level_command_fails_with_message() {
    hooks_cmd()
        .arg("not-a-command")
        .assert()
        .failure()
        .stderr(predicate::str::contains("unknown command"));
}

#[test]
fn claude_hook_without_name_prints_usage_and_fails() {
    hooks_cmd()
        .arg("claude-hook")
        .assert()
        .failure()
        .stderr(predicate::str::contains("usage: roster-hooks claude-hook"));
}
