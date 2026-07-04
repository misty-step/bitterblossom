use assert_cmd::Command;
use predicates::prelude::*;
use std::path::PathBuf;

fn workspace_root() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .parent()
        .and_then(|path| path.parent())
        .expect("workspace root")
        .to_path_buf()
}

fn roster_cmd() -> Command {
    let mut command = Command::cargo_bin("roster").expect("roster binary");
    command.current_dir(workspace_root());
    command
}

#[test]
fn list_prints_seed_agents() {
    roster_cmd()
        .arg("list")
        .assert()
        .success()
        .stdout(predicate::str::contains("cerberus\tcodex-class\txhigh"))
        .stdout(predicate::str::contains("lead\tfable-class\tlow"))
        .stdout(predicate::str::contains("sweep\topenrouter-class\tmedium"));
}

#[test]
fn materialize_codex_prints_brief_header() {
    roster_cmd()
        .args(["materialize", "cerberus", "--harness", "codex"])
        .assert()
        .success()
        .stdout(predicate::str::contains("# Roster Brief: cerberus"))
        .stdout(predicate::str::contains("Read:"))
        .stdout(predicate::str::contains("Code-review master"));
}

#[test]
fn materialize_bb_prints_agent_binding() {
    roster_cmd()
        .args(["materialize", "cerberus", "--harness", "bb"])
        .assert()
        .success()
        .stdout(predicate::str::contains(
            "# Generated from roster agent cerberus",
        ))
        .stdout(predicate::str::contains("harness = \"pi\""))
        .stdout(predicate::str::contains(
            "model = \"moonshotai/kimi-k2.7-code\"",
        ))
        .stdout(predicate::str::contains("role = \"cerberus\""))
        .stdout(predicate::str::contains("output_bytes_cap = 120000"))
        .stdout(predicate::str::contains("side_effect_policy = \"kill\""));
}

#[test]
fn sync_is_p2_stub() {
    roster_cmd()
        .arg("sync")
        .assert()
        .failure()
        .stderr(predicate::str::contains("P2"));
}
