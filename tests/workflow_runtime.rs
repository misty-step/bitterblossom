//! bitterblossom-workflow-runtime-v1: behavior-first drills for the workflow
//! runtime — trigger -> step (pinned agent + goal) -> result -> route —
//! through the public CLI and HTTP surfaces with stub (command-harness)
//! agents.
//!
//! Card oracle coverage:
//! 1. trigger + two agent steps + named outcome route + terminal result,
//!    end to end
//! 2. single-route steps need no result schema; branching steps must use one
//!    declared completion outcome
//! 3. dynamic child agents inherit authority under their parent and never
//!    become catalog entries
//! 4. a cycle stops on each supported selected guard (stop signal, rounds,
//!    elapsed, spend) recording every attempt in one run group
//! 5. external, schedule, internal, and synthetic-test trigger sources share
//!    one normalized acceptance contract (incl. dedupe parity)
//! 6. the outcome vocabulary is workflow config — the spine carries no
//!    workload keywords (arbitrary vocabularies route identically)
//!
//! Proof-plan extras: serve SIGKILL restart/recovery classification plus
//! webhook redelivery dedupe across the restart; Roster v0.2 resolved-bundle
//! consumption with digest provenance.

use std::fs;
use std::io::{Read, Write};
use std::net::TcpStream;
use std::os::unix::fs::PermissionsExt;
use std::path::{Path, PathBuf};
use std::process::{Command, Output};
use std::time::{Duration, Instant};

use bitterblossom::ledger::Ledger;

fn write_plane(root: &Path) {
    fs::write(
        root.join("plane.toml"),
        "dev = true\n[ingress]\nbind = \"127.0.0.1:0\"\n",
    )
    .unwrap();
}

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

fn bb_json(root: &Path, args: &[&str]) -> serde_json::Value {
    serde_json::from_str(&bb_ok(root, args)).unwrap()
}

/// Write an executable stub agent script and return its absolute path.
fn write_stub(root: &Path, name: &str, script: &str) -> PathBuf {
    let path = root.join(name);
    fs::write(&path, format!("#!/bin/sh\n{script}\n")).unwrap();
    let mut perms = fs::metadata(&path).unwrap().permissions();
    perms.set_mode(0o755);
    fs::set_permissions(&path, perms).unwrap();
    path
}

fn write_doc(root: &Path, name: &str, text: &str) -> String {
    let path = root.join(name);
    fs::write(&path, text).unwrap();
    path.to_str().unwrap().to_string()
}

/// Create + activate a workflow from TOML, returning nothing; panics on error.
fn create_and_activate(root: &Path, doc_file: &str, workflow: &str) {
    bb_ok(root, &["workflow", "create", doc_file]);
    bb_ok(root, &["workflow", "activate", workflow]);
}

/// Accept a synthetic test event and return the run id.
fn accept_test_run(root: &Path, workflow: &str) -> String {
    let accepted = bb_json(
        root,
        &[
            "workflow",
            "accept",
            workflow,
            "--trigger",
            "test",
            "--json",
        ],
    );
    assert_eq!(accepted["disposition"], "accepted", "{accepted}");
    accepted["run"]["id"].as_str().unwrap().to_string()
}

fn execute(root: &Path, run_id: &str) -> serde_json::Value {
    bb_json(root, &["workflow", "execute", run_id, "--json"])
}

#[test]
fn metered_workflow_run_stops_in_flight_on_per_run_ceiling() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let stub = write_stub(
        root,
        "metered.sh",
        r#"printf '%s\n' '{"part":{"type":"step-finish","cost":0.50,"tokens":{"input":1,"output":1}}}'
sleep 2
printf '%s\n' '{"part":{"type":"text","text":"done"}}'"#,
    );
    let doc = write_doc(
        root,
        "metered.toml",
        &format!(
            r#"
name = "metered"
goal = "Stop when the metered step exceeds its run budget."

[[trigger]]
kind = "test"

[[step]]
name = "work"
goal = "Emit a metered result."
[step.agent]
name = "metered"
version = 1
harness = "opencode"
model = "stub"
bin = "{}"

[policies]
max_cost_per_run_usd = 0.01
side_effect_policy = "kill"
"#,
            stub.display()
        ),
    );
    create_and_activate(root, &doc, "metered");
    let run_id = accept_test_run(root, "metered");
    let view = execute(root, &run_id);
    assert_eq!(view["status"]["state"], "stopped", "{view}");
    assert!(view["status"]["detail"]
        .as_str()
        .unwrap()
        .contains("workflow in-flight cost cap"));
    let ledger = Ledger::open(&root.join(".bb/plane.db")).unwrap();
    assert!(ledger
        .list_guard_events(100)
        .unwrap()
        .iter()
        .any(|event| event.kind == "workflow_guard_spend_in_flight"));
}

// --- criterion 1: trigger -> two agent steps -> named outcome route -> done --

#[test]
fn two_step_workflow_with_named_outcome_route_runs_end_to_end() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let triage = write_stub(
        root,
        "triage.sh",
        r#"cat > OUTCOME.json <<'EOF'
{"outcome": "fix", "summary": "seeded flaw found in module x"}
EOF
echo "triage complete""#,
    );
    let act = write_stub(root, "act.sh", r#"echo "applied the fix and verified""#);
    let doc = write_doc(
        root,
        "ship.toml",
        &format!(
            r#"
name = "ship"
goal = "Investigate the event and act on what triage finds."

[[trigger]]
kind = "test"

[[step]]
name = "triage"
goal = "Diagnose the event and decide whether action is needed."
[step.agent]
name = "stub-triage"
version = 1
harness = "command"
model = "stub"
bin = "{}"
[step.routes]
fix = "act"
ok = "done"

[[step]]
name = "act"
goal = "Apply the fix triage identified."
[step.agent]
name = "stub-act"
version = 1
harness = "command"
model = "stub"
bin = "{}"
"#,
            triage.display(),
            act.display()
        ),
    );
    create_and_activate(root, &doc, "ship");
    let run_id = accept_test_run(root, "ship");

    let view = execute(root, &run_id);
    assert_eq!(view["status"]["state"], "succeeded", "{view}");
    // terminal result carried through: the last step's result text
    assert_eq!(view["status"]["detail"], "applied the fix and verified");
    let steps = view["steps"].as_array().unwrap();
    assert_eq!(steps.len(), 2);
    assert_eq!(steps[0]["step"], "triage");
    assert_eq!(steps[0]["attempt"], 1);
    assert_eq!(steps[0]["state"], "succeeded");
    assert_eq!(steps[0]["outcome"], "fix");
    assert_eq!(steps[0]["summary"], "seeded flaw found in module x");
    assert_eq!(steps[0]["agent"]["name"], "stub-triage");
    assert_eq!(steps[1]["step"], "act");
    assert_eq!(steps[1]["attempt"], 2);
    assert_eq!(steps[1]["state"], "succeeded");
    // single-route terminal step: no outcome required or recorded
    assert_eq!(steps[1]["outcome"], serde_json::Value::Null);

    // the same projection reads back over the CLI run-show surface
    let shown = bb_json(root, &["workflow", "run-show", &run_id, "--json"]);
    assert_eq!(shown["status"]["state"], "succeeded");
    assert_eq!(shown["steps"].as_array().unwrap().len(), 2);
    // executing again is refused: the run group is terminal, not re-runnable
    let rerun = bb(root, &["workflow", "execute", &run_id]);
    assert!(!rerun.status.success());
    assert!(String::from_utf8_lossy(&rerun.stderr).contains("succeeded"));
}

// --- criterion 2: result schemas only where routing needs them ---------------

#[test]
fn single_route_step_completes_without_result_schema() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let stub = write_stub(root, "work.sh", r#"echo "work finished""#);
    let doc = write_doc(
        root,
        "simple.toml",
        &format!(
            r#"
name = "simple"
goal = "Do the work."
[[step]]
name = "work"
goal = "Do the work and finish."
[step.agent]
name = "stub"
version = 1
harness = "command"
model = "stub"
bin = "{}"
"#,
            stub.display()
        ),
    );
    create_and_activate(root, &doc, "simple");
    let run_id = accept_test_run(root, "simple");
    let view = execute(root, &run_id);
    // no OUTCOME.json was written and none was needed
    assert_eq!(view["status"]["state"], "succeeded", "{view}");
    assert_eq!(view["steps"][0]["outcome"], serde_json::Value::Null);
}

#[test]
fn branching_step_requires_one_declared_completion_outcome() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    // stub 1: branching step that never writes OUTCOME.json
    let silent = write_stub(root, "silent.sh", r#"echo "did things, said nothing""#);
    // stub 2: writes an outcome outside the declared vocabulary
    let rogue = write_stub(
        root,
        "rogue.sh",
        r#"printf '%s' '{"outcome": "maybe", "summary": "hedging"}' > OUTCOME.json
echo done"#,
    );
    for (idx, (workflow, stub)) in [("silent", &silent), ("rogue", &rogue)].iter().enumerate() {
        let doc = write_doc(
            root,
            &format!("wf-{workflow}.toml"),
            &format!(
                r#"
name = "{workflow}"
goal = "Branch on a declared outcome."
[[step]]
name = "decide"
goal = "Decide yes or no."
[step.agent]
name = "stub"
version = 1
harness = "command"
model = "stub"
bin = "{}"
[step.routes]
yes = "done"
no = "done"
"#,
                stub.display()
            ),
        );
        create_and_activate(root, &doc, workflow);
        let run_id = accept_test_run(root, workflow);
        let view = execute(root, &run_id);
        // the harness exited 0 with prose — the plane never guesses an outcome
        assert_eq!(view["status"]["state"], "incomplete", "{view}");
        assert_eq!(view["steps"][0]["state"], "incomplete");
        let detail = view["status"]["detail"].as_str().unwrap();
        if idx == 0 {
            assert!(detail.contains("supplied none"), "{detail}");
            assert!(detail.contains("yes") && detail.contains("no"), "{detail}");
        } else {
            assert!(detail.contains("undeclared outcome 'maybe'"), "{detail}");
        }
    }
}

// --- criterion 3: dynamic children inherit authority, never catalog entries --

#[test]
fn dynamic_children_inherit_authority_under_parent_without_catalog_entries() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let parent = write_stub(
        root,
        "parent.sh",
        r#"cat > CHILD_AGENTS.json <<'EOF'
[
  {"name": "qa-child", "goal": "verify the change"},
  {"name": "sec-child", "harness": "command", "model": "tiny",
   "goal": "scan for leaks", "authority": ["repo:read"],
   "cost_usd": 0.05, "result": "no leaks found"}
]
EOF
echo "composed two children""#,
    );
    let doc = write_doc(
        root,
        "compose.toml",
        &format!(
            r#"
name = "compose"
goal = "Compose children to get the work done."
[[step]]
name = "lead"
goal = "Decompose and verify."
authority = ["repo:read", "pr:comment"]
[step.agent]
name = "stub-lead"
version = 1
harness = "command"
model = "stub"
bin = "{}"
"#,
            parent.display()
        ),
    );
    create_and_activate(root, &doc, "compose");
    let run_id = accept_test_run(root, "compose");
    let view = execute(root, &run_id);
    assert_eq!(view["status"]["state"], "succeeded", "{view}");
    let children = view["steps"][0]["children"].as_array().unwrap();
    assert_eq!(children.len(), 2);
    // no declared grant -> inherits the parent grant verbatim
    assert_eq!(children[0]["name"], "qa-child");
    assert_eq!(children[0]["inherited"], true);
    assert_eq!(
        children[0]["authority"],
        serde_json::json!(["repo:read", "pr:comment"])
    );
    // declared narrower grant -> recorded as its own subset
    assert_eq!(children[1]["name"], "sec-child");
    assert_eq!(children[1]["inherited"], false);
    assert_eq!(children[1]["authority"], serde_json::json!(["repo:read"]));
    assert_eq!(children[1]["cost_usd"], 0.05);
    assert_eq!(children[1]["result"], "no leaks found");

    // children never became catalog entries: still exactly one workflow, and
    // the child names resolve to nothing
    let workflows = bb_json(root, &["workflow", "list", "--json"]);
    assert_eq!(workflows.as_array().unwrap().len(), 1);
    let missing = bb(root, &["workflow", "show", "qa-child"]);
    assert!(!missing.status.success());
}

#[test]
fn child_declaring_broader_authority_than_parent_fails_the_step() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let greedy = write_stub(
        root,
        "greedy.sh",
        r#"cat > CHILD_AGENTS.json <<'EOF'
[{"name": "escalator", "authority": ["repo:read", "repo:write"]}]
EOF
echo tried"#,
    );
    let doc = write_doc(
        root,
        "narrow.toml",
        &format!(
            r#"
name = "narrow"
goal = "Prove authority narrows monotonically."
[[step]]
name = "lead"
goal = "Try to escalate."
authority = ["repo:read"]
[step.agent]
name = "stub"
version = 1
harness = "command"
model = "stub"
bin = "{}"
"#,
            greedy.display()
        ),
    );
    create_and_activate(root, &doc, "narrow");
    let run_id = accept_test_run(root, "narrow");
    let view = execute(root, &run_id);
    assert_eq!(view["status"]["state"], "failed", "{view}");
    let detail = view["status"]["detail"].as_str().unwrap();
    assert!(detail.contains("beyond its parent step grant"), "{detail}");
    assert!(detail.contains("repo:write"), "{detail}");
}

// --- criterion 4: cycles stop on every supported selected guard --------------

fn cycle_doc(root: &Path, workflow: &str, stub: &Path, policies: &str) -> String {
    write_doc(
        root,
        &format!("cycle-{workflow}.toml"),
        &format!(
            r#"
name = "{workflow}"
goal = "Loop until a guard stops the run group."
[[step]]
name = "spin"
goal = "Do one round and route again."
[step.agent]
name = "stub-spin"
version = 1
harness = "command"
model = "stub"
bin = "{}"
[step.routes]
again = "spin"
finish = "done"

[policies]
{policies}
"#,
            stub.display()
        ),
    )
}

fn spin_stub(root: &Path, name: &str, extra: &str) -> PathBuf {
    write_stub(
        root,
        name,
        &format!(
            r#"{extra}
cat > OUTCOME.json <<'EOF'
{{"outcome": "again", "summary": "one more round"}}
EOF
printf '%s' '{{"schema_version": "bb.command_result.v1", "result": "spun", "cost_usd": 0.6}}'"#
        ),
    )
}

#[test]
fn cycle_without_any_enforceable_guard_is_rejected_at_validation() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let stub = spin_stub(root, "spin.sh", "");
    let doc = cycle_doc(root, "unbounded", &stub, "");
    let refused = bb(root, &["workflow", "create", &doc]);
    assert!(!refused.status.success());
    let stderr = String::from_utf8_lossy(&refused.stderr);
    assert!(stderr.contains("cycle"), "{stderr}");
    assert!(stderr.contains("max_rounds"), "{stderr}");
}

#[test]
fn cycle_stops_on_rounds_guard_recording_every_attempt_in_one_run_group() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let stub = spin_stub(root, "spin.sh", "");
    let doc = cycle_doc(root, "rounds", &stub, "max_rounds = 2");
    create_and_activate(root, &doc, "rounds");
    let run_id = accept_test_run(root, "rounds");
    let view = execute(root, &run_id);
    assert_eq!(view["status"]["state"], "stopped", "{view}");
    let detail = view["status"]["detail"].as_str().unwrap();
    assert!(detail.contains("rounds guard"), "{detail}");
    let steps = view["steps"].as_array().unwrap();
    assert_eq!(steps.len(), 2, "every attempt recorded in one run group");
    for (i, step) in steps.iter().enumerate() {
        assert_eq!(step["run_id"], view["run"]["id"]);
        assert_eq!(step["step"], "spin");
        assert_eq!(step["attempt"], (i + 1) as i64);
        assert_eq!(step["state"], "succeeded");
        assert_eq!(step["outcome"], "again");
    }
}

#[test]
fn cycle_stops_on_elapsed_guard() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let stub = spin_stub(root, "slow-spin.sh", "sleep 2");
    let doc = cycle_doc(root, "elapsed", &stub, "max_elapsed_seconds = 1");
    create_and_activate(root, &doc, "elapsed");
    let run_id = accept_test_run(root, "elapsed");
    let view = execute(root, &run_id);
    assert_eq!(view["status"]["state"], "stopped", "{view}");
    assert!(
        view["status"]["detail"]
            .as_str()
            .unwrap()
            .contains("elapsed guard"),
        "{view}"
    );
    assert_eq!(view["steps"].as_array().unwrap().len(), 1);
}

#[test]
fn cycle_stops_on_spend_guard_from_observed_step_costs() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let stub = spin_stub(root, "spin.sh", "");
    // command is cost-blind by declaration (it MAY self-report, nothing
    // guarantees it), so spend cannot be the SOLE cycle guard here: rounds
    // bounds the cycle and the spend cap still fires first from observed
    // self-reported costs.
    let doc = cycle_doc(
        root,
        "spend",
        &stub,
        "max_cost_per_run_usd = 1.0\nmax_rounds = 100",
    );
    create_and_activate(root, &doc, "spend");
    let run_id = accept_test_run(root, "spend");
    let view = execute(root, &run_id);
    assert_eq!(view["status"]["state"], "stopped", "{view}");
    let detail = view["status"]["detail"].as_str().unwrap();
    assert!(detail.contains("spend guard"), "{detail}");
    // 0.6 + 0.6 = 1.2 observed > 1.0 cap, caught before attempt 3
    assert_eq!(view["steps"].as_array().unwrap().len(), 2);
    assert!(view["status"]["cost_usd"].as_f64().unwrap() > 1.0);
}

#[test]
fn cycle_stops_on_external_stop_signal() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    // the stub stops its own run group mid-cycle, like an external authority
    // would; the stub script is rewritten in place once the run id is known
    let stub = spin_stub(root, "halt-spin.sh", "");
    let doc = cycle_doc(root, "halt", &stub, "max_rounds = 100");
    create_and_activate(root, &doc, "halt");
    let run_id = accept_test_run(root, "halt");
    // rewrite the stub in place to send the stop signal during attempt 1
    let bb_bin = env!("CARGO_BIN_EXE_bb");
    let stop_cmd = format!(
        "{} --config {} workflow stop {} --reason drill-stop",
        bb_bin,
        root.display(),
        run_id
    );
    spin_stub(root, "halt-spin.sh", &stop_cmd);

    let view = execute(root, &run_id);
    assert_eq!(view["status"]["state"], "stopped", "{view}");
    let detail = view["status"]["detail"].as_str().unwrap();
    assert!(detail.contains("stop signal: drill-stop"), "{detail}");
    // attempt 1 completed (never killed mid-flight); the stop applied before
    // attempt 2
    assert_eq!(view["steps"].as_array().unwrap().len(), 1);
    assert_eq!(view["steps"][0]["state"], "succeeded");
}

// --- criterion 6 support: the outcome vocabulary is config, not spine --------

#[test]
fn arbitrary_outcome_vocabulary_routes_identically() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let stub = write_stub(
        root,
        "oracle.sh",
        r#"printf '%s' '{"outcome": "descry", "summary": "vocabulary is config"}' > OUTCOME.json
echo scried"#,
    );
    let follow = write_stub(root, "follow.sh", r#"echo "followed the route""#);
    let doc = write_doc(
        root,
        "vocab.toml",
        &format!(
            r#"
name = "vocab"
goal = "Prove no keyword lives in the spine."
[[step]]
name = "scry"
goal = "Emit a domain-specific outcome."
[step.agent]
name = "stub"
version = 1
harness = "command"
model = "stub"
bin = "{}"
[step.routes]
frobnicate = "done"
descry = "follow"

[[step]]
name = "follow"
goal = "Prove the declared route was followed."
[step.agent]
name = "stub"
version = 1
harness = "command"
model = "stub"
bin = "{}"
"#,
            stub.display(),
            follow.display()
        ),
    );
    create_and_activate(root, &doc, "vocab");
    let run_id = accept_test_run(root, "vocab");
    let view = execute(root, &run_id);
    assert_eq!(view["status"]["state"], "succeeded", "{view}");
    let steps = view["steps"].as_array().unwrap();
    assert_eq!(steps[0]["outcome"], "descry");
    assert_eq!(steps[1]["step"], "follow");
}

// --- Roster v0.2 resolved bundles -------------------------------------------

#[test]
fn step_agent_consumes_roster_bundle_with_digest_provenance() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let bundle = root.join("bundle");
    fs::create_dir_all(&bundle).unwrap();
    fs::write(
        bundle.join("AGENTS.md"),
        "# Cerberus\nYou are Cerberus, the review gatekeeper.\n",
    )
    .unwrap();
    fs::write(bundle.join("manifest.yaml"), "agent: cerberus\n").unwrap();
    let stub = write_stub(root, "bundled.sh", r#"echo "ran with bundle""#);
    let doc = write_doc(
        root,
        "bundled.toml",
        &format!(
            r#"
name = "bundled"
goal = "Run a step from a resolved Roster bundle."
[[step]]
name = "review"
goal = "Review with the bundled identity."
[step.agent]
name = "cerberus"
version = 1
harness = "command"
model = "stub"
bin = "{}"
bundle = "{}"
"#,
            stub.display(),
            bundle.display()
        ),
    );
    create_and_activate(root, &doc, "bundled");
    let run_id = accept_test_run(root, "bundled");
    let view = execute(root, &run_id);
    assert_eq!(view["status"]["state"], "succeeded", "{view}");
    // the bundle's AGENTS.md joined the commission the agent received
    let artifact_dir = view["steps"][0]["artifact_dir"].as_str().unwrap();
    let card = fs::read_to_string(Path::new(artifact_dir).join("workspace/LANE_CARD.md")).unwrap();
    assert!(card.contains("You are Cerberus"), "{card}");
    // digest provenance recorded in the run context the step saw
    let run_ctx = fs::read_to_string(Path::new(artifact_dir).join("workspace/RUN.json")).unwrap();
    let run_ctx: serde_json::Value = serde_json::from_str(&run_ctx).unwrap();
    assert_eq!(
        run_ctx["bundle_agents_md_sha256"].as_str().unwrap().len(),
        64
    );

    // a declared-but-missing bundle fails honestly, never launches
    fs::remove_dir_all(&bundle).unwrap();
    let run2 = accept_test_run(root, "bundled");
    let view2 = execute(root, &run2);
    assert_eq!(view2["status"]["state"], "failed", "{view2}");
    assert!(
        view2["status"]["detail"]
            .as_str()
            .unwrap()
            .contains("AGENTS.md"),
        "{view2}"
    );
}

// --- criterion 5: one normalized acceptance contract -------------------------

struct ServeGuard {
    child: std::process::Child,
}

impl ServeGuard {
    fn stop(mut self) {
        unsafe {
            libc::kill(self.child.id() as libc::pid_t, libc::SIGTERM);
        }
        let deadline = Instant::now() + Duration::from_secs(5);
        while Instant::now() < deadline {
            if self.child.try_wait().unwrap().is_some() {
                return;
            }
            std::thread::sleep(Duration::from_millis(20));
        }
        let _ = self.child.kill();
        let _ = self.child.wait();
    }

    fn kill_hard(mut self) {
        let _ = self.child.kill();
        let _ = self.child.wait();
    }
}

impl Drop for ServeGuard {
    fn drop(&mut self) {
        let _ = self.child.kill();
        let _ = self.child.wait();
    }
}

fn spawn_serve(root: &Path, envs: &[(&str, &str)]) -> (ServeGuard, u16) {
    let port_file = root.join("bb-serve-port");
    let _ = fs::remove_file(&port_file);
    let stderr_log = fs::File::create(root.join("serve-stderr.log")).unwrap();
    let mut cmd = Command::new(env!("CARGO_BIN_EXE_bb"));
    cmd.args(["--config", root.to_str().unwrap(), "serve"])
        .env("BB_INGRESS_REPORT_PORT_FILE", &port_file)
        .stdout(std::process::Stdio::null())
        .stderr(stderr_log);
    for (k, v) in envs {
        cmd.env(k, v);
    }
    let child = cmd.spawn().unwrap();
    let deadline = Instant::now() + Duration::from_secs(10);
    let port = loop {
        if let Ok(text) = fs::read_to_string(&port_file) {
            if let Ok(port) = text.trim().parse::<u16>() {
                break port;
            }
        }
        assert!(
            Instant::now() < deadline,
            "bb serve never reported its port"
        );
        std::thread::sleep(Duration::from_millis(20));
    };
    (ServeGuard { child }, port)
}

fn http(
    port: u16,
    method: &str,
    path: &str,
    headers: &[(String, String)],
    body: Option<&str>,
) -> (u16, serde_json::Value) {
    let extra: String = headers
        .iter()
        .map(|(k, v)| format!("{k}: {v}\r\n"))
        .collect();
    let body = body.unwrap_or("");
    let content = if body.is_empty() {
        String::new()
    } else {
        format!(
            "Content-Type: application/json\r\nContent-Length: {}\r\n",
            body.len()
        )
    };
    let request = format!(
        "{method} {path} HTTP/1.1\r\nHost: 127.0.0.1\r\n{extra}{content}Connection: close\r\n\r\n{body}"
    );
    let deadline = Instant::now() + Duration::from_secs(5);
    loop {
        let response = TcpStream::connect(("127.0.0.1", port)).and_then(|mut stream| {
            stream.write_all(request.as_bytes())?;
            let mut response = String::new();
            stream.read_to_string(&mut response)?;
            Ok(response)
        });
        if let Ok(response) = response {
            if response.starts_with("HTTP/1.1") {
                let status: u16 = response
                    .lines()
                    .next()
                    .unwrap()
                    .split_whitespace()
                    .nth(1)
                    .unwrap()
                    .parse()
                    .unwrap();
                let body = response.split("\r\n\r\n").nth(1).unwrap_or("").trim();
                let json = if body.is_empty() {
                    serde_json::Value::Null
                } else {
                    serde_json::from_str(body).unwrap_or(serde_json::Value::Null)
                };
                return (status, json);
            }
        }
        assert!(
            Instant::now() < deadline,
            "{method} {path}: no HTTP response"
        );
        std::thread::sleep(Duration::from_millis(20));
    }
}

fn signed_headers(secret: &str, body: &str) -> Vec<(String, String)> {
    vec![(
        "X-Hub-Signature-256".to_string(),
        bitterblossom::ingress::sign_hmac(secret, body.as_bytes()),
    )]
}

const PIPELINE_SECRET: &str = "wf-hook-test-secret";

fn pipeline_doc(root: &Path) -> String {
    let stub = write_stub(root, "noop.sh", r#"echo ok"#);
    write_doc(
        root,
        "pipeline.toml",
        &format!(
            r#"
name = "pipeline"
goal = "Accept events from every source through one contract."

[[trigger]]
kind = "webhook"
route = "wfhook"
secret_env = "BB_HOOK_WFTEST"
dedupe_key = "json:/id"

[[trigger]]
kind = "cron"
schedule = "*/5 * * * *"

[[step]]
name = "work"
goal = "Acknowledge the event."
[step.agent]
name = "stub"
version = 1
harness = "command"
model = "stub"
bin = "{}"
"#,
            stub.display()
        ),
    )
}

#[test]
fn all_trigger_sources_share_one_normalized_acceptance_contract() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let doc = pipeline_doc(root);
    create_and_activate(root, &doc, "pipeline");
    let (serve, port) = spawn_serve(root, &[("BB_HOOK_WFTEST", PIPELINE_SECRET)]);

    // external: signed webhook delivery, deduped on the payload id
    let event = r#"{"id": "evt-1", "kind": "external"}"#;
    let (status, first) = http(
        port,
        "POST",
        "/hooks/wfhook",
        &signed_headers(PIPELINE_SECRET, event),
        Some(event),
    );
    assert_eq!(status, 202, "{first}");
    let webhook_run = first["workflow_run_id"].as_str().unwrap().to_string();
    assert_eq!(first["duplicate"], false);
    // a bad signature is refused before acceptance
    let (status, _) = http(
        port,
        "POST",
        "/hooks/wfhook",
        &signed_headers("wrong-secret", event),
        Some(event),
    );
    assert_eq!(status, 401);
    // redelivery: same id, duplicate disposition, same run
    let (status, again) = http(
        port,
        "POST",
        "/hooks/wfhook",
        &signed_headers(PIPELINE_SECRET, event),
        Some(event),
    );
    assert_eq!(status, 202);
    assert_eq!(again["duplicate"], true);
    assert_eq!(again["workflow_run_id"].as_str().unwrap(), webhook_run);

    // internal: another workflow/agent posts through the same contract
    let internal_body = serde_json::json!({
        "trigger_kind": "internal",
        "payload": {"from": "other-workflow"},
        "dedupe_key": "emit:other-workflow:1",
    })
    .to_string();
    let (status, internal) = http(
        port,
        "POST",
        "/api/workflows/pipeline/runs",
        &[],
        Some(&internal_body),
    );
    assert_eq!(status, 201, "{internal}");
    assert_eq!(internal["disposition"], "accepted");
    let (status, internal_dup) = http(
        port,
        "POST",
        "/api/workflows/pipeline/runs",
        &[],
        Some(&internal_body),
    );
    assert_eq!(status, 200);
    assert_eq!(internal_dup["disposition"], "duplicate");
    assert_eq!(internal_dup["run"]["id"], internal["run"]["id"]);

    // synthetic test: CLI acceptance, same dedupe semantics
    let test_accept = bb_json(
        root,
        &[
            "workflow",
            "accept",
            "pipeline",
            "--trigger",
            "test",
            "--dedupe-key",
            "fixture:pr-42",
            "--json",
        ],
    );
    assert_eq!(test_accept["disposition"], "accepted");
    let test_dup = bb_json(
        root,
        &[
            "workflow",
            "accept",
            "pipeline",
            "--trigger",
            "test",
            "--dedupe-key",
            "fixture:pr-42",
            "--json",
        ],
    );
    assert_eq!(test_dup["disposition"], "duplicate");
    assert_eq!(test_dup["run"]["id"], test_accept["run"]["id"]);

    // schedule: the same tick function `bb serve` runs, driven directly with
    // a deterministic window; dedupe key is the scheduled timestamp
    use chrono::TimeZone;
    let ledger = Ledger::open(&root.join(".bb/plane.db")).unwrap();
    let mut lasts = std::collections::HashMap::new();
    let window_start = chrono::Utc.with_ymd_and_hms(2026, 7, 14, 12, 0, 1).unwrap();
    let window_end = chrono::Utc.with_ymd_and_hms(2026, 7, 14, 12, 6, 0).unwrap();
    let accepted = bitterblossom::workflow_runtime::workflow_cron_tick(
        &ledger,
        &mut lasts,
        window_start,
        window_end,
        3,
    )
    .unwrap();
    assert_eq!(accepted.len(), 1, "{accepted:?}");
    assert!(!accepted[0].duplicate);
    // overlapping window re-derives the same scheduled fire: duplicate
    let mut lasts_again = std::collections::HashMap::new();
    let replay = bitterblossom::workflow_runtime::workflow_cron_tick(
        &ledger,
        &mut lasts_again,
        window_start,
        window_end,
        3,
    )
    .unwrap();
    assert_eq!(replay.len(), 1);
    assert!(replay[0].duplicate);
    drop(ledger);

    // one contract: every source landed in the same acceptance table with
    // the same normalized row shape — trigger kind + dedupe key + pinned
    // revision — and nothing else distinguishes them
    let runs = bb_json(root, &["workflow", "runs", "pipeline", "--json"]);
    let runs = runs.as_array().unwrap();
    let mut kinds: Vec<&str> = runs
        .iter()
        .map(|r| r["trigger_kind"].as_str().unwrap())
        .collect();
    kinds.sort_unstable();
    assert_eq!(kinds, vec!["cron", "internal", "test", "webhook"]);
    for run in runs {
        assert!(run["dedupe_key"].as_str().is_some(), "{run}");
        assert_eq!(run["revision"], 1);
        assert!(run["id"].as_str().unwrap().starts_with("wfr-"));
    }

    serve.stop();
}

// --- proof plan: restart/recovery + dedupe drill ------------------------------

#[test]
fn restart_classifies_inflight_run_and_dedupe_survives_recovery() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let sleeper = write_stub(root, "sleeper.sh", "sleep 300\necho never");
    let doc = write_doc(
        root,
        "sleepy.toml",
        &format!(
            r#"
name = "sleepy"
goal = "Sleep long enough to be interrupted."

[[trigger]]
kind = "webhook"
route = "sleepy"
secret_env = "BB_HOOK_WFTEST"
dedupe_key = "json:/id"

[[step]]
name = "nap"
goal = "Sleep."
[step.agent]
name = "stub"
version = 1
harness = "command"
model = "stub"
bin = "{}"
"#,
            sleeper.display()
        ),
    );
    create_and_activate(root, &doc, "sleepy");
    let (serve, port) = spawn_serve(root, &[("BB_HOOK_WFTEST", PIPELINE_SECRET)]);

    let event = r#"{"id": "boom-1"}"#;
    let (status, accepted) = http(
        port,
        "POST",
        "/hooks/sleepy",
        &signed_headers(PIPELINE_SECRET, event),
        Some(event),
    );
    assert_eq!(status, 202, "{accepted}");
    let run_id = accepted["workflow_run_id"].as_str().unwrap().to_string();

    // wait until the serve workflow runner has claimed and started the step
    let deadline = Instant::now() + Duration::from_secs(20);
    loop {
        let (status, view) = http(
            port,
            "GET",
            &format!("/api/workflow-runs/{run_id}"),
            &[],
            None,
        );
        assert_eq!(status, 200);
        if view["status"]["state"] == "running" && !view["steps"].as_array().unwrap().is_empty() {
            break;
        }
        assert!(
            Instant::now() < deadline,
            "workflow runner never started the step: {view}"
        );
        std::thread::sleep(Duration::from_millis(100));
    }

    // hard-kill the plane mid-execution (SIGKILL: no graceful shutdown)
    serve.kill_hard();

    // restart: boot classification must mark the inherited run
    // needs_attention naming the in-flight step — never blindly re-execute
    let (serve2, port2) = spawn_serve(root, &[("BB_HOOK_WFTEST", PIPELINE_SECRET)]);
    let deadline = Instant::now() + Duration::from_secs(10);
    loop {
        let (status, view) = http(
            port2,
            "GET",
            &format!("/api/workflow-runs/{run_id}"),
            &[],
            None,
        );
        assert_eq!(status, 200);
        if view["status"]["state"] == "needs_attention" {
            let detail = view["status"]["detail"].as_str().unwrap();
            assert!(detail.contains("step 'nap'"), "{detail}");
            assert!(detail.contains("side effects unknown"), "{detail}");
            break;
        }
        assert!(
            Instant::now() < deadline,
            "inherited run was not classified: {view}"
        );
        std::thread::sleep(Duration::from_millis(100));
    }

    // dedupe survives the restart: redelivery returns the original run
    let (status, redelivered) = http(
        port2,
        "POST",
        "/hooks/sleepy",
        &signed_headers(PIPELINE_SECRET, event),
        Some(event),
    );
    assert_eq!(status, 202);
    assert_eq!(redelivered["duplicate"], true);
    assert_eq!(redelivered["workflow_run_id"].as_str().unwrap(), run_id);
    // and the recovered run stays needs_attention: nothing re-queued it
    let (_, view) = http(
        port2,
        "GET",
        &format!("/api/workflow-runs/{run_id}"),
        &[],
        None,
    );
    assert_eq!(view["status"]["state"], "needs_attention");

    serve2.stop();
}

// --- review blockers (PR #1006): spend-guard honesty + pinned-doc validation --

/// BLOCKER 1 (validation door): `command` never guarantees cost_usd, so with
/// max_cost_per_run_usd as the only cycle guard the cap could never fire —
/// NULL cost must not be laundered into zero spend. The critic's repro
/// (cost-blind stub + $0.01 cap) ran unbounded; creation must refuse it.
#[test]
fn spend_only_cycle_with_cost_blind_harness_is_refused() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let stub = spin_stub(root, "spin.sh", "");
    let doc = cycle_doc(root, "blindspend", &stub, "max_cost_per_run_usd = 0.01");
    let refused = bb(root, &["workflow", "create", &doc]);
    assert!(
        !refused.status.success(),
        "spend-only cycle on a cost-blind harness must not be creatable"
    );
    let stderr = String::from_utf8_lossy(&refused.stderr);
    assert!(stderr.contains("cost-blind"), "{stderr}");
    assert!(stderr.contains("command"), "{stderr}");
}

/// BLOCKER 1 (runtime door): `pi` reports cost in general so validation
/// accepts the doc, but THIS attempt yields no usage events -> attempt cost
/// NULL. A spend-only cycle with an unmetered attempt is guard-indeterminate:
/// the run stops naming that, never continuing on "unknown = zero".
#[test]
fn spend_only_cycle_stops_when_attempt_reports_no_cost() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let count = root.join("spin-count");
    // Self-limiting so the pre-fix behavior terminates instead of hanging:
    // outcomes "again" x3 then "finish". Post-fix the guard stops attempt 2.
    let stub = write_stub(
        root,
        "pi-spin.sh",
        &format!(
            r#"c=$(cat {count} 2>/dev/null || echo 0)
c=$((c+1))
printf '%s' "$c" > {count}
if [ "$c" -ge 4 ]; then out=finish; else out=again; fi
printf '{{"outcome": "%s"}}\n' "$out" > OUTCOME.json
printf '{{"type":"message_end","message":{{"role":"assistant","content":[{{"type":"text","text":"looped"}}]}}}}\n'"#,
            count = count.display()
        ),
    );
    let doc = write_doc(
        root,
        "unmetered.toml",
        &format!(
            r#"
name = "unmetered"
goal = "Spend-only cycle on a cost-reporting harness."
[[step]]
name = "spin"
goal = "Do one round and route again."
[step.agent]
name = "pi-stub"
version = 1
harness = "pi"
model = "stub"
bin = "{}"
[step.routes]
again = "spin"
finish = "done"

[policies]
max_cost_per_run_usd = 1.0
"#,
            stub.display()
        ),
    );
    create_and_activate(root, &doc, "unmetered");
    let run_id = accept_test_run(root, "unmetered");
    let view = execute(root, &run_id);
    assert_eq!(view["status"]["state"], "stopped", "{view}");
    let detail = view["status"]["detail"].as_str().unwrap();
    assert!(detail.contains("indeterminate"), "{detail}");
    assert!(detail.contains("no cost"), "{detail}");
    // attempt 1 completed; the indeterminate guard applied before attempt 2
    assert_eq!(view["steps"].as_array().unwrap().len(), 1, "{view}");
}

/// Simulate a revision stored by the #1001 store-era binary (which had no
/// cycle rule): insert the snapshot directly, exactly as that binary would
/// have, bypassing current validation.
fn insert_prerule_revision(root: &Path, workflow: &str, document: &str) -> i64 {
    let conn = rusqlite::Connection::open(root.join(".bb/plane.db")).unwrap();
    let wf_id: String = conn
        .query_row(
            "SELECT id FROM workflows WHERE name = ?1",
            [workflow],
            |r| r.get(0),
        )
        .unwrap();
    let rev: i64 = conn
        .query_row(
            "SELECT COALESCE(MAX(revision),0)+1 FROM workflow_revisions WHERE workflow_id = ?1",
            [&wf_id],
            |r| r.get(0),
        )
        .unwrap();
    conn.execute(
        "INSERT INTO workflow_revisions (workflow_id, revision, document, source, note, created_at)
         VALUES (?1, ?2, ?3, 'import', NULL, '2026-01-01T00:00:00Z')",
        rusqlite::params![wf_id, rev, document],
    )
    .unwrap();
    rev
}

fn prerule_cyclic_document(name: &str, stub: &Path) -> String {
    format!(
        r#"{{"name":"{name}","goal":"pre-rule cyclic snapshot","step":[{{"name":"spin","agent":{{"name":"stub","version":1,"harness":"command","model":"stub","bin":"{}"}},"goal":"route again forever","routes":{{"again":"spin","finish":"done"}}}}]}}"#,
        stub.display()
    )
}

/// BLOCKER 2 (rollback door): a stored snapshot that fails CURRENT validation
/// must not be re-activated by rollback.
#[test]
fn rollback_to_snapshot_failing_current_validation_is_refused() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let stub = spin_stub(root, "spin.sh", "");
    let doc = cycle_doc(root, "legacy", &stub, "max_rounds = 2");
    create_and_activate(root, &doc, "legacy");
    let rev = insert_prerule_revision(root, "legacy", &prerule_cyclic_document("legacy", &stub));

    let refused = bb(
        root,
        &["workflow", "rollback", "legacy", "--to", &rev.to_string()],
    );
    assert!(
        !refused.status.success(),
        "rollback re-activated a snapshot that fails current validation"
    );
    let stderr = String::from_utf8_lossy(&refused.stderr);
    assert!(stderr.contains("validation"), "{stderr}");
    assert!(stderr.contains("cycle"), "{stderr}");
}

/// BLOCKER 2 (execution door): a pinned pre-rule revision that fails CURRENT
/// validation must refuse to execute — named error, zero step attempts, run
/// `failed` — instead of running with zero guards.
#[test]
fn pinned_prerule_document_failing_current_validation_never_executes() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let count = root.join("prerule-count");
    // Self-limiting (3 rounds then finish) so pre-fix behavior terminates:
    // the red assertion is zero attempts, not a hang.
    let stub = write_stub(
        root,
        "prerule-spin.sh",
        &format!(
            r#"c=$(cat {count} 2>/dev/null || echo 0)
c=$((c+1))
printf '%s' "$c" > {count}
if [ "$c" -ge 4 ]; then out=finish; else out=again; fi
printf '{{"outcome": "%s"}}\n' "$out" > OUTCOME.json
echo spun"#,
            count = count.display()
        ),
    );
    let seed = cycle_doc(root, "prerule", &stub, "max_rounds = 2");
    create_and_activate(root, &seed, "prerule");
    let rev = insert_prerule_revision(root, "prerule", &prerule_cyclic_document("prerule", &stub));
    {
        let conn = rusqlite::Connection::open(root.join(".bb/plane.db")).unwrap();
        conn.execute(
            "UPDATE workflows SET active_revision = ?1 WHERE name = 'prerule'",
            [rev],
        )
        .unwrap();
    }
    let run_id = accept_test_run(root, "prerule");

    let refused = bb(root, &["workflow", "execute", &run_id]);
    assert!(
        !refused.status.success(),
        "a pinned revision failing current validation executed anyway"
    );
    let view = bb_json(root, &["workflow", "run-show", &run_id, "--json"]);
    assert_eq!(view["status"]["state"], "failed", "{view}");
    let detail = view["status"]["detail"].as_str().unwrap();
    assert!(detail.contains("validation"), "{detail}");
    assert_eq!(
        view["steps"].as_array().unwrap().len(),
        0,
        "no step may execute under a doc failing current validation: {view}"
    );
}

/// BLOCKER 2 (defense-in-depth): even when every declared guard is huge, the
/// executor's absolute attempt ceiling backstops one run group.
#[test]
fn absolute_attempt_ceiling_backstops_run_groups() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let stub = spin_stub(root, "spin.sh", "");
    let doc = cycle_doc(root, "ceiling", &stub, "max_rounds = 100000");
    create_and_activate(root, &doc, "ceiling");
    let run_id = accept_test_run(root, "ceiling");
    let view = execute(root, &run_id);
    assert_eq!(view["status"]["state"], "stopped", "{view}");
    let detail = view["status"]["detail"].as_str().unwrap();
    assert!(detail.contains("attempt ceiling"), "{detail}");
    assert_eq!(view["steps"].as_array().unwrap().len(), 256, "{view}");
}

/// Agent-controlled contract files are size-capped: the substrate release
/// cap fires first (attempt-scoped step failure naming the artifact and the
/// limit), and the executor's read-side cap backstops layouts release never
/// collected. Either way the run terminates naming the size — never an
/// unbounded read, never a stranded `running`.
#[test]
fn oversize_outcome_file_is_refused_by_size() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let stub = write_stub(
        root,
        "bloat.sh",
        r#"head -c 2000000 /dev/zero | tr '\0' 'x' > OUTCOME.json
echo done"#,
    );
    let doc = write_doc(
        root,
        "bloat.toml",
        &format!(
            r#"
name = "bloat"
goal = "Emit an oversize completion file."
[[step]]
name = "work"
goal = "Do the work."
[step.agent]
name = "stub"
version = 1
harness = "command"
model = "stub"
bin = "{}"
"#,
            stub.display()
        ),
    );
    create_and_activate(root, &doc, "bloat");
    let run_id = accept_test_run(root, "bloat");
    let view = execute(root, &run_id);
    assert_eq!(view["status"]["state"], "failed", "{view}");
    let detail = view["status"]["detail"].as_str().unwrap();
    assert!(detail.contains("OUTCOME.json"), "{detail}");
    assert!(detail.contains("exceeds"), "{detail}");
    assert!(detail.contains("bytes"), "{detail}");
    // attempt-scoped: the step row closed with the same named reason
    assert_eq!(view["steps"][0]["state"], "failed", "{view}");
}

/// An oversize CHILD_AGENTS.json fails the step naming the size.
#[test]
fn oversize_child_agents_file_fails_the_step() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let stub = write_stub(
        root,
        "bloat-children.sh",
        r#"head -c 2000000 /dev/zero | tr '\0' 'x' > CHILD_AGENTS.json
echo done"#,
    );
    let doc = write_doc(
        root,
        "bloatkids.toml",
        &format!(
            r#"
name = "bloatkids"
goal = "Emit an oversize child evidence file."
[[step]]
name = "work"
goal = "Do the work."
[step.agent]
name = "stub"
version = 1
harness = "command"
model = "stub"
bin = "{}"
"#,
            stub.display()
        ),
    );
    create_and_activate(root, &doc, "bloatkids");
    let run_id = accept_test_run(root, "bloatkids");
    let view = execute(root, &run_id);
    assert_eq!(view["status"]["state"], "failed", "{view}");
    let detail = view["status"]["detail"].as_str().unwrap();
    assert!(detail.contains("CHILD_AGENTS.json"), "{detail}");
    assert!(detail.contains("exceeds"), "{detail}");
    assert!(detail.contains("bytes"), "{detail}");
    assert_eq!(view["steps"][0]["state"], "failed", "{view}");
}

/// Child-declared costs are summed into the run group's observed cost so the
/// enforced spend cap sees delegated spend, not just parent-harness spend.
#[test]
fn child_declared_costs_count_toward_the_spend_cap() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let stub = spin_stub(
        root,
        "delegate-spin.sh",
        r#"cat > CHILD_AGENTS.json <<'EOF'
[{"name": "helper", "cost_usd": 0.5}]
EOF"#,
    );
    // 0.6 parent + 0.5 child = 1.1 observed per round vs $2 cap: rounds alone
    // would allow 100 attempts, but summed child spend trips the cap first.
    let doc = cycle_doc(
        root,
        "delegated",
        &stub,
        "max_cost_per_run_usd = 2.0\nmax_rounds = 100",
    );
    create_and_activate(root, &doc, "delegated");
    let run_id = accept_test_run(root, "delegated");
    let view = execute(root, &run_id);
    assert_eq!(view["status"]["state"], "stopped", "{view}");
    let detail = view["status"]["detail"].as_str().unwrap();
    assert!(detail.contains("spend guard"), "{detail}");
    // 1.1 + 1.1 = 2.2 > 2.0 after two rounds; caught before attempt 3
    assert_eq!(view["steps"].as_array().unwrap().len(), 2, "{view}");
    assert!(view["status"]["cost_usd"].as_f64().unwrap() > 2.0, "{view}");
}

// --- re-review round 2: poisoned-doc isolation + guard precision -------------

/// REQUIRED regression fix (round 2): one workflow whose ACTIVE revision
/// fails current validation (a pre-rule snapshot) must never block other
/// workflows' ingress. 'aaa-legacy' enumerates before 'zzz-hook'; before the
/// fix its load error 500'd zzz-hook's correctly-signed webhook and halted
/// the whole workflow cron tick. Skip-and-canary per workflow; the hard
/// refusal stays at the execute/rollback doors.
#[test]
fn poisoned_workflow_never_blocks_other_workflows_ingress() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);

    // the poisoned workflow: valid at creation, then its active revision is
    // replaced by a pre-rule cyclic snapshot exactly as #1001 stored them
    let spin = spin_stub(root, "aaa-spin.sh", "");
    let seed = cycle_doc(root, "aaa-legacy", &spin, "max_rounds = 2");
    create_and_activate(root, &seed, "aaa-legacy");
    let rev = insert_prerule_revision(
        root,
        "aaa-legacy",
        &prerule_cyclic_document("aaa-legacy", &spin),
    );
    {
        let conn = rusqlite::Connection::open(root.join(".bb/plane.db")).unwrap();
        conn.execute(
            "UPDATE workflows SET active_revision = ?1 WHERE name = 'aaa-legacy'",
            [rev],
        )
        .unwrap();
    }

    // the healthy workflow, enumerated after the poisoned one
    let ok = write_stub(root, "zzz-ok.sh", r#"echo handled"#);
    let doc = write_doc(
        root,
        "zzz-hook.toml",
        &format!(
            r#"
name = "zzz-hook"
goal = "Stay reachable while a sibling workflow is poisoned."

[[trigger]]
kind = "webhook"
route = "zzzhook"
secret_env = "BB_HOOK_WFTEST"

[[trigger]]
kind = "cron"
schedule = "*/5 * * * *"

[[step]]
name = "work"
goal = "Acknowledge the event."
[step.agent]
name = "stub"
version = 1
harness = "command"
model = "stub"
bin = "{}"
"#,
            ok.display()
        ),
    );
    create_and_activate(root, &doc, "zzz-hook");

    // webhook door: signed delivery to the healthy route must accept and run
    let (serve, port) = spawn_serve(root, &[("BB_HOOK_WFTEST", PIPELINE_SECRET)]);
    let event = r#"{"id": "poison-drill-1"}"#;
    let (status, accepted) = http(
        port,
        "POST",
        "/hooks/zzzhook",
        &signed_headers(PIPELINE_SECRET, event),
        Some(event),
    );
    assert_eq!(status, 202, "healthy webhook must accept: {accepted}");
    let run_id = accepted["workflow_run_id"].as_str().unwrap().to_string();
    let deadline = Instant::now() + Duration::from_secs(15);
    loop {
        let (_, view) = http(
            port,
            "GET",
            &format!("/api/workflow-runs/{run_id}"),
            &[],
            None,
        );
        if view["status"]["state"] == "succeeded" {
            break;
        }
        assert!(
            Instant::now() < deadline,
            "healthy run never reached succeeded: {view}"
        );
        std::thread::sleep(Duration::from_millis(100));
    }
    serve.stop();

    // cron door: the tick must skip the poisoned workflow and still accept
    // the healthy workflow's due fire
    let ledger = Ledger::open(&root.join(".bb/plane.db")).unwrap();
    let mut last = std::collections::HashMap::new();
    let now = chrono::Utc::now();
    let accepted = bitterblossom::workflow_runtime::workflow_cron_tick(
        &ledger,
        &mut last,
        now - chrono::Duration::minutes(6),
        now,
        1,
    )
    .expect("cron tick must survive a poisoned sibling workflow");
    assert!(
        accepted.iter().any(|a| a.workflow == "zzz-hook"),
        "healthy cron fire missing: {accepted:?}"
    );
    assert!(accepted.iter().all(|a| a.workflow != "aaa-legacy"));
}

/// Round 2 precision fix: a cost-blind entry step OFF the cycle is admitted
/// by validation, so the runtime indeterminate rule must read only attempts
/// of steps ON cycles — otherwise every run of the admitted shape is dead on
/// arrival after attempt 1.
#[test]
fn blind_off_cycle_entry_step_does_not_trip_the_indeterminate_guard() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let prep = write_stub(root, "prep.sh", r#"echo prepared"#);
    // pi stub that reports usage WITH cost: the cycle itself is fully metered
    let spin = write_stub(
        root,
        "metered-spin.sh",
        r#"printf '{"outcome": "again"}\n' > OUTCOME.json
printf '{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"looped"}],"usage":{"input":1,"output":1,"cost":{"total":0.6}}}}\n'"#,
    );
    let doc = write_doc(
        root,
        "mixed.toml",
        &format!(
            r#"
name = "mixed"
goal = "Blind entry step, metered spend-only cycle."
[[step]]
name = "prep"
goal = "Prepare once, off the cycle."
[step.agent]
name = "prep-stub"
version = 1
harness = "command"
model = "stub"
bin = "{}"
[step.routes]
ready = "spin"

[[step]]
name = "spin"
goal = "Do one round and route again."
[step.agent]
name = "pi-stub"
version = 1
harness = "pi"
model = "stub"
bin = "{}"
[step.routes]
again = "spin"
finish = "done"

[policies]
max_cost_per_run_usd = 1.0
"#,
            prep.display(),
            spin.display()
        ),
    );
    create_and_activate(root, &doc, "mixed");
    let run_id = accept_test_run(root, "mixed");
    let view = execute(root, &run_id);
    assert_eq!(view["status"]["state"], "stopped", "{view}");
    let detail = view["status"]["detail"].as_str().unwrap();
    assert!(
        detail.contains("spend guard: observed"),
        "must stop on the real spend cap, not indeterminate: {detail}"
    );
    // prep (unmetered, off-cycle) + two metered spins (0.6 + 0.6 > 1.0)
    assert_eq!(view["steps"].as_array().unwrap().len(), 3, "{view}");
}

/// Round 2: a negative child-declared cost must not offset siblings' spend.
#[test]
fn negative_child_declared_cost_fails_the_step() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let stub = write_stub(
        root,
        "refund.sh",
        r#"cat > CHILD_AGENTS.json <<'EOF'
[{"name": "big-spender", "cost_usd": 0.9}, {"name": "helper", "cost_usd": -0.5}]
EOF
echo done"#,
    );
    let doc = write_doc(
        root,
        "refund.toml",
        &format!(
            r#"
name = "refund"
goal = "Declare a negative child cost."
[[step]]
name = "work"
goal = "Do the work."
[step.agent]
name = "stub"
version = 1
harness = "command"
model = "stub"
bin = "{}"
"#,
            stub.display()
        ),
    );
    create_and_activate(root, &doc, "refund");
    let run_id = accept_test_run(root, "refund");
    let view = execute(root, &run_id);
    assert_eq!(view["status"]["state"], "failed", "{view}");
    let detail = view["status"]["detail"].as_str().unwrap();
    assert!(detail.contains("negative"), "{detail}");
    assert!(detail.contains("helper"), "{detail}");
}

// --- bitterblossom-workflow-step-host: substrate host + repos binding --------

/// bb invocation with extra env (the sprites stub needs BB_SPRITE_BIN etc. to
/// reach the `sprite` subprocess bb spawns).
fn bb_env(root: &Path, args: &[&str], envs: &[(&str, &std::ffi::OsStr)]) -> Output {
    let mut cmd = Command::new(env!("CARGO_BIN_EXE_bb"));
    cmd.args(["--config", root.to_str().unwrap()]).args(args);
    for (k, v) in envs {
        cmd.env(k, v);
    }
    cmd.output().unwrap()
}

/// The stub `sprite` CLI from tests/e2e_sprites.rs, trimmed to what a
/// workflow step drill needs: it logs every invocation (including the
/// `-s <host>` selector) and executes the remote command locally with
/// /home/sprite mapped onto SPRITE_FAKE_HOME.
const WF_SPRITE_STUB: &str = r#"#!/bin/bash
log="$SPRITE_STUB_LOG"
home="$(cd "$SPRITE_FAKE_HOME" && pwd -P)"
cmd="$1"; shift
echo "$cmd $*" >> "$log"
case "$cmd" in
  restore)
    exit 0;;
  exec)
    declare -a rest
    while [ $# -gt 0 ]; do
      case "$1" in
        -s|--dir|--env|-o) shift 2;;
        --) shift; break;;
        --file)
          spec="$2"; shift 2
          src="${spec%%:*}"; dest="${spec#*:}"
          dest="${dest//\/home\/sprite/$home}"
          mkdir -p "$(dirname "$dest")"
          cp "$src" "$dest";;
        *) break;;
      esac
    done
    while [ $# -gt 0 ]; do
      rest+=("${1//\/home\/sprite/$home}"); shift
    done
    if [ "${rest[0]}" = "setsid" ]; then
      rest=("${rest[@]:1}")
      [ "${rest[0]}" = "-w" ] && rest=("${rest[@]:1}")
    fi
    if [ "${rest[0]}" = "sh" ] && [ "${#rest[@]}" -eq 1 ]; then
      sed "s|/home/sprite|$home|g" | sh
      exit $?
    fi
    exec "${rest[@]}";;
  *)
    echo "stub: unknown command $cmd" >&2; exit 64;;
esac
"#;

/// bitterblossom-workflow-step-host criterion 1+2 (red-first): a step's
/// declared host must reach substrate acquire() — and a dev=false plane
/// (the public-plane posture) must accept and run a host-bound step on a
/// non-local substrate end to end.
#[test]
fn step_host_reaches_the_substrate_and_runs_on_a_prod_plane() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    // dev defaults to false: this is the public-plane posture where the
    // local substrate is refused and every workload runs on a remote host.
    fs::write(
        root.join("plane.toml"),
        "[ingress]\nbind = \"127.0.0.1:0\"\n",
    )
    .unwrap();

    let sprite_stub = root.join("sprite-stub");
    fs::write(&sprite_stub, WF_SPRITE_STUB).unwrap();
    let mut perms = fs::metadata(&sprite_stub).unwrap().permissions();
    perms.set_mode(0o755);
    fs::set_permissions(&sprite_stub, perms).unwrap();
    let fake_home = root.join("sprite-home");
    fs::create_dir_all(&fake_home).unwrap();
    let log = root.join("sprite-log");
    fs::write(&log, "").unwrap();

    let stub = write_stub(root, "work.sh", r#"echo "ran on the sprite""#);
    let doc = write_doc(
        root,
        "hosted.toml",
        &format!(
            r#"
name = "hosted"
goal = "Run one step on a named sprite host."
[[step]]
name = "work"
goal = "Do the work on the sprite."
host = "misty-step/lane-1"
[step.agent]
name = "stub"
version = 1
harness = "command"
model = "stub"
bin = "{}"

[policies]
substrate = "sprites"
"#,
            stub.display()
        ),
    );
    let envs: Vec<(&str, &std::ffi::OsStr)> = vec![
        ("BB_SPRITE_BIN", sprite_stub.as_os_str()),
        ("SPRITE_STUB_LOG", log.as_os_str()),
        ("SPRITE_FAKE_HOME", fake_home.as_os_str()),
    ];
    let create = bb_env(root, &["workflow", "create", doc.as_str()], &envs);
    assert!(
        create.status.success(),
        "create: {}",
        String::from_utf8_lossy(&create.stderr)
    );
    assert!(bb_env(root, &["workflow", "activate", "hosted"], &envs)
        .status
        .success());
    let accepted = bb_env(
        root,
        &[
            "workflow",
            "accept",
            "hosted",
            "--trigger",
            "test",
            "--json",
        ],
        &envs,
    );
    assert!(accepted.status.success());
    let accepted: serde_json::Value = serde_json::from_slice(&accepted.stdout).unwrap();
    let run_id = accepted["run"]["id"].as_str().unwrap();

    let exec = bb_env(root, &["workflow", "execute", run_id, "--json"], &envs);
    assert!(
        exec.status.success(),
        "execute on a dev=false plane must accept a host-bound step\nstderr: {}",
        String::from_utf8_lossy(&exec.stderr)
    );
    let view: serde_json::Value = serde_json::from_slice(&exec.stdout).unwrap();
    assert_eq!(view["status"]["state"], "succeeded", "{view}");
    assert_eq!(view["status"]["detail"], "ran on the sprite");

    // the declared step host reached the substrate: the adapter pins the
    // org from `org/name` syntax, so every sprite CLI call carries
    // `-o misty-step -s lane-1` — never the old hardcoded wf-<runid> name
    let calls = fs::read_to_string(&log).unwrap();
    assert!(
        calls.contains("-o misty-step -s lane-1"),
        "step host never reached the sprite CLI:\n{calls}"
    );
    assert!(
        !calls.contains("-s wf-"),
        "substrate still addressed by the hardcoded run-id name:\n{calls}"
    );
}

/// bitterblossom-workflow-step-host criterion 3 (red-first): a host-requiring
/// substrate with a hostless step is refused at the config door with a named
/// error — never a silent local fallback.
#[test]
fn hostless_step_on_host_requiring_substrate_is_refused_at_validation() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let stub = write_stub(root, "work.sh", r#"echo ok"#);
    let doc = write_doc(
        root,
        "hostless.toml",
        &format!(
            r#"
name = "hostless"
goal = "Declare a remote substrate but no step host."
[[step]]
name = "work"
goal = "Do the work."
[step.agent]
name = "stub"
version = 1
harness = "command"
model = "stub"
bin = "{}"

[policies]
substrate = "sprites"
"#,
            stub.display()
        ),
    );
    let refused = bb(root, &["workflow", "create", &doc]);
    assert!(
        !refused.status.success(),
        "hostless step on sprites must not be creatable"
    );
    let stderr = String::from_utf8_lossy(&refused.stderr);
    assert!(stderr.contains("work"), "{stderr}");
    assert!(stderr.contains("sprites"), "{stderr}");
    assert!(stderr.contains("host"), "{stderr}");
}

/// Additive-fields contract: a pinned snapshot stored BEFORE host/repos
/// existed (no such keys in its JSON) still validates and executes
/// unchanged on the substrate it declared.
#[test]
fn pre_host_field_pinned_snapshot_stays_valid_and_executes() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let stub = write_stub(root, "old.sh", r#"echo "still valid""#);
    let seed = write_doc(
        root,
        "elder-seed.toml",
        &format!(
            r#"
name = "elder"
goal = "Seed workflow."
[[step]]
name = "work"
goal = "Do the work."
[step.agent]
name = "stub"
version = 1
harness = "command"
model = "stub"
bin = "{}"
"#,
            stub.display()
        ),
    );
    create_and_activate(root, &seed, "elder");
    // a snapshot exactly as the pre-host-field binary stored it: no host,
    // no repos keys anywhere
    let old_doc = format!(
        r#"{{"name":"elder","goal":"Pre-host-field snapshot.","step":[{{"name":"work","agent":{{"name":"stub","version":1,"harness":"command","model":"stub","bin":"{}"}},"goal":"Do the work."}}]}}"#,
        stub.display()
    );
    let rev = insert_prerule_revision(root, "elder", &old_doc);
    {
        let conn = rusqlite::Connection::open(root.join(".bb/plane.db")).unwrap();
        conn.execute(
            "UPDATE workflows SET active_revision = ?1 WHERE name = 'elder'",
            [rev],
        )
        .unwrap();
    }
    let run_id = accept_test_run(root, "elder");
    let view = execute(root, &run_id);
    assert_eq!(view["status"]["state"], "succeeded", "{view}");
    assert_eq!(view["status"]["detail"], "still valid");
}
