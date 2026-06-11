//! Submission-loop oracle: CAS lifecycle, plane-owned rounds and report
//! snapshots, verdict parsing, command-harness verdicts, and every gate
//! rule from the 039 packet. Stub harnesses stand in for the external CLI
//! boundary only — the spine under test is real.

use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::Path;
use std::sync::Mutex;

use bitterblossom::dispatch;
use bitterblossom::ledger::{IngressRequest, Ledger};
use bitterblossom::spec::Plane;
use bitterblossom::submit::{self, Finding, VerdictDoc};

/// BB_NOTIFY_BIN is process-global; tests that set it serialize here.
static ENV_LOCK: Mutex<()> = Mutex::new(());

const REQUIRED: &[&str] = &["correctness", "security"];

fn write_executable(path: &Path, content: &str) {
    fs::write(path, content).unwrap();
    fs::set_permissions(path, fs::Permissions::from_mode(0o755)).unwrap();
}

/// A dev plane with verdict tasks for each required kind plus an arbiter.
/// `notify` adds a webhook so escalations can be asserted via BB_NOTIFY_BIN.
fn make_gate_plane(root: &Path, max_rounds: u32, notify: bool) -> Plane {
    fs::create_dir_all(root.join("agents")).unwrap();
    let stub = root.join("stub.sh");
    write_executable(
        &stub,
        "#!/bin/sh\ncat > /dev/null\necho '{\"type\":\"result\",\"result\":\"unused\"}'\n",
    );
    fs::write(
        root.join("agents/stub.toml"),
        format!(
            "harness = \"claude\"\nmodel = \"m\"\nbin = \"{}\"\n",
            stub.display()
        ),
    )
    .unwrap();
    let webhook = if notify {
        "[notify]\nwebhook_url = \"http://localhost:1/x\"\n"
    } else {
        ""
    };
    fs::write(
        root.join("plane.toml"),
        format!(
            "dev = true\n{webhook}[gate]\nrequired = [\"correctness\", \"security\"]\nmax_rounds = {max_rounds}\n"
        ),
    )
    .unwrap();
    for kind in REQUIRED.iter().chain(["arbiter"].iter()) {
        let dir = root.join("tasks").join(kind);
        fs::create_dir_all(&dir).unwrap();
        fs::write(dir.join("card.md"), "verdict card\n").unwrap();
        fs::write(
            dir.join("task.toml"),
            format!(
                "agent = \"stub\"\nsubstrate = \"local\"\nverdict = \"{kind}\"\n[[trigger]]\nkind = \"manual\"\n"
            ),
        )
        .unwrap();
    }
    Plane::load(root).unwrap()
}

fn pass() -> VerdictDoc {
    VerdictDoc {
        verdict: "pass".into(),
        findings: vec![],
    }
}

fn finding(severity: &str, claim: &str) -> Finding {
    Finding {
        severity: severity.into(),
        file: Some("src/x.rs".into()),
        line: Some(1),
        claim: claim.into(),
        evidence: None,
        fingerprint: Some(submit::fingerprint("t", Some("src/x.rs"), claim)),
    }
}

fn with_findings(verdict: &str, findings: Vec<Finding>) -> VerdictDoc {
    VerdictDoc {
        verdict: verdict.into(),
        findings,
    }
}

/// Mint the canonical storm run for a member and drive it to `success` —
/// the gate only honors verdicts bound to this run.
fn canonical_run(ledger: &mut Ledger, sub: &str, kind: &str) -> String {
    let run = ledger
        .ingest(IngressRequest {
            task: kind,
            trigger_kind: "manual",
            idempotency_key: Some(&format!("storm:{sub}:{kind}")),
            source_event_id: None,
            payload: Some(&format!("{{\"submission\":\"{sub}\"}}")),
            parent_run_id: None,
        })
        .unwrap()
        .run_id;
    ledger.transition(&run, "running", None).unwrap();
    ledger.transition(&run, "success", None).unwrap();
    run
}

/// Complete a round: canonical run + verdict per required kind
/// (`overrides` wins, others pass).
fn fill_round(ledger: &mut Ledger, sub: &str, overrides: &[(&str, &VerdictDoc)]) {
    for kind in REQUIRED {
        let doc = overrides
            .iter()
            .find(|(k, _)| k == kind)
            .map(|(_, d)| (*d).clone())
            .unwrap_or_else(pass);
        let run = canonical_run(ledger, sub, kind);
        ledger.record_verdict(sub, &run, kind, &doc).unwrap();
    }
}

#[test]
fn cas_at_most_one_open_submission_per_change() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_gate_plane(dir.path(), 3, false);
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let sub = ledger.open_submission("feat/x", "sha1", None).unwrap();
    assert_eq!((sub.round, sub.state.as_str()), (1, "open"));
    assert!(ledger.open_submission("feat/x", "sha2", None).is_err());
    // A different change is unaffected.
    ledger.open_submission("feat/y", "sha1", None).unwrap();
    // Terminal state frees the change key; non-blocked terminals reset
    // the chain to round 1.
    assert!(ledger
        .settle_submission(&sub.id, "abandoned", "{}")
        .unwrap());
    let again = ledger.open_submission("feat/x", "sha3", None).unwrap();
    assert_eq!(again.round, 1);
    assert!(again.prior_report_json.is_none());
}

#[test]
fn blocked_round_increments_plane_side_and_snapshots_report() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_gate_plane(dir.path(), 3, false);
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let r1 = ledger.open_submission("feat/x", "sha1", None).unwrap();
    fill_round(
        &mut ledger,
        &r1.id,
        &[(
            "correctness",
            &with_findings("blocking", vec![finding("blocking", "planted bug")]),
        )],
    );
    let report = submit::evaluate(&plane, &ledger, &r1.id).unwrap();
    assert_eq!(report.decision, "blocked");
    assert_eq!(ledger.submission(&r1.id).unwrap().state, "blocked");

    let r2 = ledger.open_submission("feat/x", "sha2", None).unwrap();
    assert_eq!(r2.round, 2);
    let snapshot = r2.prior_report_json.expect("prior report snapshotted");
    assert!(snapshot.contains("planted bug"), "{snapshot}");
}

#[test]
fn gate_is_pending_with_member_states_until_round_is_complete() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_gate_plane(dir.path(), 3, false);
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let sub = ledger.open_submission("feat/x", "sha1", None).unwrap();

    // One verdict in, one member not even started: pending, never clear.
    let run_a = canonical_run(&mut ledger, &sub.id, "correctness");
    ledger
        .record_verdict(&sub.id, &run_a, "correctness", &pass())
        .unwrap();
    let report = submit::evaluate(&plane, &ledger, &sub.id).unwrap();
    assert_eq!(report.decision, "pending");
    let by_kind: std::collections::BTreeMap<_, _> = report
        .members
        .iter()
        .map(|m| (m.kind.as_str(), m.status.as_str()))
        .collect();
    assert_eq!(by_kind["correctness"], "verdict:pass");
    assert_eq!(by_kind["security"], "not_started");
    assert_eq!(ledger.submission(&sub.id).unwrap().state, "open");

    // A storm run in flight shows its run state.
    let run = ledger
        .ingest(IngressRequest {
            task: "security",
            trigger_kind: "manual",
            idempotency_key: Some(&format!("storm:{}:security", sub.id)),
            source_event_id: None,
            payload: Some(&format!("{{\"submission\":\"{}\"}}", sub.id)),
            parent_run_id: None,
        })
        .unwrap()
        .run_id;
    let report = submit::evaluate(&plane, &ledger, &sub.id).unwrap();
    assert_eq!(report.decision, "pending");
    assert!(report
        .members
        .iter()
        .any(|m| m.kind == "security" && m.status == "run:pending"));
    let _ = run;
}

#[test]
fn required_member_terminal_failure_escalates_with_one_notify() {
    let _guard = ENV_LOCK.lock().unwrap();
    let dir = tempfile::tempdir().unwrap();
    let plane = make_gate_plane(dir.path(), 3, true);
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let sub = ledger.open_submission("feat/x", "sha1", None).unwrap();

    let log = dir.path().join("notify.log");
    let notify_stub = dir.path().join("notify.sh");
    write_executable(
        &notify_stub,
        &format!("#!/bin/sh\ncat >> {}\n", log.display()),
    );
    std::env::set_var("BB_NOTIFY_BIN", &notify_stub);

    let run_a = canonical_run(&mut ledger, &sub.id, "correctness");
    ledger
        .record_verdict(&sub.id, &run_a, "correctness", &pass())
        .unwrap();
    let run = ledger
        .ingest(IngressRequest {
            task: "security",
            trigger_kind: "manual",
            idempotency_key: Some(&format!("storm:{}:security", sub.id)),
            source_event_id: None,
            payload: None,
            parent_run_id: None,
        })
        .unwrap()
        .run_id;
    ledger.transition(&run, "running", None).unwrap();
    ledger.transition(&run, "failure", Some("boom")).unwrap();

    let report = submit::evaluate(&plane, &ledger, &sub.id).unwrap();
    std::env::remove_var("BB_NOTIFY_BIN");
    assert_eq!(report.decision, "escalated");
    assert_eq!(ledger.submission(&sub.id).unwrap().state, "escalated");

    // Notify is async fire-and-forget; the `>>` redirect creates the log
    // before cat copies stdin, so poll for content, not existence.
    let deadline = std::time::Instant::now() + std::time::Duration::from_secs(5);
    let mut body = String::new();
    while body.is_empty() && std::time::Instant::now() < deadline {
        std::thread::sleep(std::time::Duration::from_millis(50));
        body = fs::read_to_string(&log).unwrap_or_default();
    }
    assert!(body.contains("submission_escalated"), "{body}");
    assert_eq!(body.matches("submission_escalated").count(), 1);

    // Re-evaluating a settled submission neither re-settles nor re-notifies.
    submit::evaluate(&plane, &ledger, &sub.id).unwrap();
    std::thread::sleep(std::time::Duration::from_millis(200));
    let body = fs::read_to_string(&log).unwrap();
    assert_eq!(body.matches("submission_escalated").count(), 1);
}

#[test]
fn fresh_blocking_finding_blocks_in_any_round() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_gate_plane(dir.path(), 3, false);
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    // Round 1 blocked on one planting.
    let r1 = ledger.open_submission("feat/x", "sha1", None).unwrap();
    fill_round(
        &mut ledger,
        &r1.id,
        &[(
            "correctness",
            &with_findings("blocking", vec![finding("blocking", "bug A")]),
        )],
    );
    assert_eq!(
        submit::evaluate(&plane, &ledger, &r1.id).unwrap().decision,
        "blocked"
    );

    // Round 2: bug A fixed, but a FRESH blocker surfaces late — it blocks.
    let r2 = ledger.open_submission("feat/x", "sha2", None).unwrap();
    fill_round(
        &mut ledger,
        &r2.id,
        &[(
            "security",
            &with_findings("blocking", vec![finding("blocking", "fresh fatal bug B")]),
        )],
    );
    let report = submit::evaluate(&plane, &ledger, &r2.id).unwrap();
    assert_eq!(report.decision, "blocked");
    assert!(report.blocking.iter().any(|f| f.claim.contains("bug B")));
}

#[test]
fn serious_and_minor_findings_never_block() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_gate_plane(dir.path(), 3, false);
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let sub = ledger.open_submission("feat/x", "sha1", None).unwrap();
    fill_round(
        &mut ledger,
        &sub.id,
        &[(
            "correctness",
            &with_findings(
                "advisory",
                vec![finding("serious", "naming nit"), finding("minor", "style")],
            ),
        )],
    );
    let report = submit::evaluate(&plane, &ledger, &sub.id).unwrap();
    assert_eq!(report.decision, "clear");
    assert_eq!(report.advisory.len(), 2);
    assert_eq!(ledger.submission(&sub.id).unwrap().state, "clear");
}

#[test]
fn rejected_blocking_finding_blocks_until_arbiter_sustains() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_gate_plane(dir.path(), 3, false);
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let sub = ledger.open_submission("feat/x", "sha1", None).unwrap();

    let f = finding("blocking", "disputed claim");
    let fp = f.fingerprint.clone().unwrap();
    fill_round(
        &mut ledger,
        &sub.id,
        &[("correctness", &with_findings("blocking", vec![f.clone()]))],
    );

    // Driver rejection alone does NOT unblock a blocking finding.
    ledger
        .reject_finding("feat/x", &fp, "false positive: covered by test")
        .unwrap();
    let report = submit::evaluate(&plane, &ledger, &sub.id).unwrap();
    assert_eq!(report.decision, "blocked");

    // Arbiter sustains the rejection on the next round → no longer blocks.
    let r2 = ledger.open_submission("feat/x", "sha2", None).unwrap();
    fill_round(
        &mut ledger,
        &r2.id,
        &[("correctness", &with_findings("blocking", vec![f.clone()]))],
    );
    ledger
        .record_verdict(
            &r2.id,
            "run-arb",
            "arbiter",
            &with_findings("pass", vec![f.clone()]),
        )
        .unwrap();
    let report = submit::evaluate(&plane, &ledger, &r2.id).unwrap();
    assert_eq!(report.decision, "clear");
    assert_eq!(report.rejected.len(), 1);
    assert!(report.rejected[0].1.contains("false positive"));

    // A rejected NON-blocking finding needs no arbiter: it just stops
    // appearing as advisory.
    let r2 = ledger.submission(&r2.id).unwrap();
    assert_eq!(r2.state, "clear");
}

#[test]
fn rejected_non_blocking_finding_moves_to_rejected_without_arbiter() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_gate_plane(dir.path(), 3, false);
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let sub = ledger.open_submission("feat/x", "sha1", None).unwrap();
    let f = finding("serious", "tabs vs spaces");
    let fp = f.fingerprint.clone().unwrap();
    ledger
        .reject_finding("feat/x", &fp, "not our style rule")
        .unwrap();
    fill_round(
        &mut ledger,
        &sub.id,
        &[("correctness", &with_findings("advisory", vec![f]))],
    );
    let report = submit::evaluate(&plane, &ledger, &sub.id).unwrap();
    assert_eq!(report.decision, "clear");
    assert!(report.advisory.is_empty());
    assert_eq!(report.rejected.len(), 1);
}

#[test]
fn blocked_at_max_rounds_escalates() {
    let _guard = ENV_LOCK.lock().unwrap();
    let dir = tempfile::tempdir().unwrap();
    let plane = make_gate_plane(dir.path(), 2, true);
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let log = dir.path().join("notify.log");
    let notify_stub = dir.path().join("notify.sh");
    write_executable(
        &notify_stub,
        &format!("#!/bin/sh\ncat >> {}\n", log.display()),
    );
    std::env::set_var("BB_NOTIFY_BIN", &notify_stub);

    let block = with_findings("blocking", vec![finding("blocking", "persistent bug")]);
    let r1 = ledger.open_submission("feat/x", "sha1", None).unwrap();
    fill_round(&mut ledger, &r1.id, &[("correctness", &block)]);
    assert_eq!(
        submit::evaluate(&plane, &ledger, &r1.id).unwrap().decision,
        "blocked"
    );

    // Round 2 == max_rounds: still blocking → escalated, not round 3.
    let r2 = ledger.open_submission("feat/x", "sha2", None).unwrap();
    assert_eq!(r2.round, 2);
    fill_round(&mut ledger, &r2.id, &[("correctness", &block)]);
    let report = submit::evaluate(&plane, &ledger, &r2.id).unwrap();
    std::env::remove_var("BB_NOTIFY_BIN");
    assert_eq!(report.decision, "escalated");
    assert_eq!(ledger.submission(&r2.id).unwrap().state, "escalated");
    // No round 3: the chain ended escalated, reopening starts fresh.
    let r3 = ledger.open_submission("feat/x", "sha3", None).unwrap();
    assert_eq!(r3.round, 1);
}

#[test]
fn parse_verdict_accepts_prose_wrapped_json_and_rejects_garbage() {
    let doc = submit::parse_verdict(
        "correctness",
        "Here is my verdict:\n```json\n{\"verdict\":\"blocking\",\"findings\":[{\"severity\":\"blocking\",\"file\":\"a.rs\",\"claim\":\"off by one\"}]}\n```\nDone.",
    )
    .unwrap();
    assert_eq!(doc.verdict, "blocking");
    // Plane computes the fingerprint when absent.
    assert!(doc.findings[0].fingerprint.is_some());

    assert!(submit::parse_verdict("k", "no json here").is_err());
    assert!(submit::parse_verdict("k", "{\"verdict\":\"maybe\"}").is_err());
    assert!(submit::parse_verdict(
        "k",
        "{\"verdict\":\"pass\",\"findings\":[{\"severity\":\"catastrophic\",\"claim\":\"x\"}]}"
    )
    .is_err());
}

// ---- e2e through dispatch (local substrate, stub harness CLIs) ----------

fn make_storm_plane(root: &Path, agent_toml: &str, kind: &str) -> Plane {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks").join(kind)).unwrap();
    fs::write(
        root.join("plane.toml"),
        format!("dev = true\n[gate]\nrequired = [\"{kind}\"]\n"),
    )
    .unwrap();
    fs::write(root.join("agents/member.toml"), agent_toml).unwrap();
    fs::write(root.join(format!("tasks/{kind}/card.md")), "card\n").unwrap();
    fs::write(
        root.join(format!("tasks/{kind}/task.toml")),
        format!(
            "agent = \"member\"\nsubstrate = \"local\"\nverdict = \"{kind}\"\n[[trigger]]\nkind = \"manual\"\n"
        ),
    )
    .unwrap();
    Plane::load(root).unwrap()
}

fn storm_dispatch(plane: &Plane, ledger: &mut Ledger, sub_id: &str, kind: &str) -> String {
    let run_id = ledger
        .ingest(IngressRequest {
            task: kind,
            trigger_kind: "manual",
            idempotency_key: Some(&format!("storm:{sub_id}:{kind}")),
            source_event_id: None,
            payload: Some(&format!("{{\"submission\":\"{sub_id}\"}}")),
            parent_run_id: None,
        })
        .unwrap()
        .run_id;
    dispatch::dispatch_run(plane, ledger, &run_id).unwrap();
    run_id
}

#[test]
fn command_harness_maps_exit_codes_to_verdicts_with_no_model() {
    let dir = tempfile::tempdir().unwrap();
    let ok = dir.path().join("ok.sh");
    write_executable(&ok, "#!/bin/sh\ncat > /dev/null\necho checks pass\n");
    let plane = make_storm_plane(
        dir.path(),
        &format!("harness = \"command\"\nbin = \"{}\"\n", ok.display()),
        "verify",
    );
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let sub = ledger.open_submission("feat/x", "sha1", None).unwrap();

    let run = storm_dispatch(&plane, &mut ledger, &sub.id, "verify");
    assert_eq!(ledger.run(&run).unwrap().state, "success");
    let report = submit::evaluate(&plane, &ledger, &sub.id).unwrap();
    assert_eq!(report.decision, "clear");

    // Failing command → blocking verdict carrying stderr, run still succeeds.
    let dir2 = tempfile::tempdir().unwrap();
    let bad = dir2.path().join("bad.sh");
    write_executable(
        &bad,
        "#!/bin/sh\ncat > /dev/null\necho 'clippy: 3 errors' >&2\nexit 1\n",
    );
    let plane2 = make_storm_plane(
        dir2.path(),
        &format!("harness = \"command\"\nbin = \"{}\"\n", bad.display()),
        "verify",
    );
    let mut ledger2 = Ledger::open(&plane2.db_path()).unwrap();
    let sub2 = ledger2.open_submission("feat/x", "sha1", None).unwrap();
    let run2 = storm_dispatch(&plane2, &mut ledger2, &sub2.id, "verify");
    assert_eq!(ledger2.run(&run2).unwrap().state, "success");
    let report = submit::evaluate(&plane2, &ledger2, &sub2.id).unwrap();
    assert_eq!(report.decision, "blocked");
    assert!(report.blocking[0]
        .evidence
        .as_deref()
        .unwrap()
        .contains("clippy: 3 errors"));
}

#[test]
fn llm_verdict_json_records_a_row_and_invalid_json_fails_the_run() {
    let dir = tempfile::tempdir().unwrap();
    // Claude-shaped stub whose result is verdict JSON.
    let stub = dir.path().join("stub.sh");
    write_executable(
        &stub,
        r#"#!/bin/sh
cat > /dev/null
echo '{"type":"result","result":"{\"verdict\":\"pass\",\"findings\":[]}","total_cost_usd":0.002,"usage":{"input_tokens":10,"output_tokens":5}}'
"#,
    );
    let plane = make_storm_plane(
        dir.path(),
        &format!(
            "harness = \"claude\"\nmodel = \"m\"\nbin = \"{}\"\n",
            stub.display()
        ),
        "correctness",
    );
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let sub = ledger.open_submission("feat/x", "sha1", None).unwrap();
    let run = storm_dispatch(&plane, &mut ledger, &sub.id, "correctness");
    assert_eq!(ledger.run(&run).unwrap().state, "success");
    let verdicts = ledger.verdicts(&sub.id).unwrap();
    assert_eq!(verdicts.len(), 1);
    assert_eq!(verdicts[0].verdict, "pass");

    // Same shape, non-verdict result → run failure, raw output preserved.
    let dir2 = tempfile::tempdir().unwrap();
    let bad = dir2.path().join("bad.sh");
    write_executable(
        &bad,
        r#"#!/bin/sh
cat > /dev/null
echo '{"type":"result","result":"looks good to me!","total_cost_usd":0.002}'
"#,
    );
    let plane2 = make_storm_plane(
        dir2.path(),
        &format!(
            "harness = \"claude\"\nmodel = \"m\"\nbin = \"{}\"\n",
            bad.display()
        ),
        "correctness",
    );
    let mut ledger2 = Ledger::open(&plane2.db_path()).unwrap();
    let sub2 = ledger2.open_submission("feat/x", "sha1", None).unwrap();
    let run2 = storm_dispatch(&plane2, &mut ledger2, &sub2.id, "correctness");
    let row = ledger2.run(&run2).unwrap();
    assert_eq!(row.state, "failure");
    assert!(row
        .state_reason
        .as_deref()
        .unwrap()
        .contains("invalid verdict JSON"));
    assert!(ledger2.verdicts(&sub2.id).unwrap().is_empty());
    let attempts = ledger2.attempts(&run2).unwrap();
    let raw = fs::read_to_string(
        Path::new(attempts[0].artifact_dir.as_deref().unwrap()).join("stdout.txt"),
    )
    .unwrap();
    assert!(raw.contains("looks good to me"), "{raw}");
}

#[test]
fn report_json_materializes_in_round_two_workspaces() {
    let dir = tempfile::tempdir().unwrap();
    let stub = dir.path().join("stub.sh");
    write_executable(
        &stub,
        r#"#!/bin/sh
cat > /dev/null
echo '{"type":"result","result":"{\"verdict\":\"pass\",\"findings\":[]}"}'
"#,
    );
    let plane = make_storm_plane(
        dir.path(),
        &format!(
            "harness = \"claude\"\nmodel = \"m\"\nbin = \"{}\"\n",
            stub.display()
        ),
        "correctness",
    );
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    // Round 1 blocked with a named planting.
    let r1 = ledger.open_submission("feat/x", "sha1", None).unwrap();
    let run_a = canonical_run(&mut ledger, &r1.id, "correctness");
    ledger
        .record_verdict(
            &r1.id,
            &run_a,
            "correctness",
            &with_findings("blocking", vec![finding("blocking", "planted flaw")]),
        )
        .unwrap();
    assert_eq!(
        submit::evaluate(&plane, &ledger, &r1.id).unwrap().decision,
        "blocked"
    );

    // Round 2 dispatch: REPORT.json lands in the workspace verbatim.
    let r2 = ledger.open_submission("feat/x", "sha2", None).unwrap();
    let run = storm_dispatch(&plane, &mut ledger, &r2.id, "correctness");
    assert_eq!(ledger.run(&run).unwrap().state, "success");
    let attempts = ledger.attempts(&run).unwrap();
    let report = fs::read_to_string(
        Path::new(attempts[0].artifact_dir.as_deref().unwrap()).join("workspace/REPORT.json"),
    )
    .unwrap();
    assert!(report.contains("planted flaw"), "{report}");
    assert_eq!(report, r2.prior_report_json.unwrap());
}

#[test]
fn verdict_task_without_submission_payload_fails_before_executing() {
    let dir = tempfile::tempdir().unwrap();
    let stub = dir.path().join("stub.sh");
    write_executable(&stub, "#!/bin/sh\ncat > /dev/null\necho x\n");
    let plane = make_storm_plane(
        dir.path(),
        &format!(
            "harness = \"claude\"\nmodel = \"m\"\nbin = \"{}\"\n",
            stub.display()
        ),
        "correctness",
    );
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let run_id = ledger
        .ingest(IngressRequest {
            task: "correctness",
            trigger_kind: "manual",
            idempotency_key: None,
            source_event_id: None,
            payload: None,
            parent_run_id: None,
        })
        .unwrap()
        .run_id;
    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();
    assert_eq!(run.state, "failure");
    assert!(run.state_reason.as_deref().unwrap().contains("payload"));
}

// ---- regressions from the 2026-06-11 codex adversarial review ----------

#[test]
fn non_canonical_run_verdict_never_counts_for_the_gate() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_gate_plane(dir.path(), 3, false);
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let sub = ledger.open_submission("feat/x", "sha1", None).unwrap();
    fill_round(&mut ledger, &sub.id, &[]);

    // A rogue re-run (different key) recording `pass` over a member whose
    // canonical verdict was blocking must not flip the gate.
    let sub2 = ledger.open_submission("feat/y", "sha1", None).unwrap();
    let canonical = canonical_run(&mut ledger, &sub2.id, "correctness");
    ledger
        .record_verdict(
            &sub2.id,
            &canonical,
            "correctness",
            &with_findings("blocking", vec![finding("blocking", "real bug")]),
        )
        .unwrap();
    let rogue = ledger
        .ingest(IngressRequest {
            task: "correctness",
            trigger_kind: "manual",
            idempotency_key: Some("rogue-rerun"),
            source_event_id: None,
            payload: Some(&format!("{{\"submission\":\"{}\"}}", sub2.id)),
            parent_run_id: None,
        })
        .unwrap()
        .run_id;
    ledger
        .record_verdict(&sub2.id, &rogue, "correctness", &pass())
        .unwrap();
    let run_b = canonical_run(&mut ledger, &sub2.id, "security");
    ledger
        .record_verdict(&sub2.id, &run_b, "security", &pass())
        .unwrap();
    let report = submit::evaluate(&plane, &ledger, &sub2.id).unwrap();
    assert_eq!(report.decision, "blocked");
    assert!(report.blocking.iter().any(|f| f.claim == "real bug"));
}

#[test]
fn unknown_supplied_fingerprints_are_recomputed() {
    // A reviewer reusing a rejected/sustained fingerprint on a FRESH
    // finding cannot inherit its rejection: the plane recomputes any
    // fingerprint it has never seen for this submission.
    let mut doc = submit::parse_verdict(
        "correctness",
        r#"{"verdict":"blocking","findings":[{"severity":"blocking","file":"a.rs","claim":"new bug","fingerprint":"stolen-rejected-fp"}]}"#,
    )
    .unwrap();
    let known: std::collections::BTreeSet<String> = ["legit-fp".to_string()].into();
    submit::enforce_fingerprints(&mut doc, "correctness", &known);
    assert_ne!(
        doc.findings[0].fingerprint.as_deref(),
        Some("stolen-rejected-fp")
    );

    // A known fingerprint (a true re-raise) survives.
    let mut doc = submit::parse_verdict(
        "correctness",
        r#"{"verdict":"blocking","findings":[{"severity":"blocking","file":"a.rs","claim":"same bug","fingerprint":"legit-fp"}]}"#,
    )
    .unwrap();
    submit::enforce_fingerprints(&mut doc, "correctness", &known);
    assert_eq!(doc.findings[0].fingerprint.as_deref(), Some("legit-fp"));
}

#[test]
fn latest_submission_is_insertion_order_not_round_order() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_gate_plane(dir.path(), 3, false);
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    // Chain 1 reaches round 2 and clears.
    let r1 = ledger.open_submission("feat/x", "sha1", None).unwrap();
    fill_round(
        &mut ledger,
        &r1.id,
        &[(
            "correctness",
            &with_findings("blocking", vec![finding("blocking", "bug")]),
        )],
    );
    submit::evaluate(&plane, &ledger, &r1.id).unwrap();
    let r2 = ledger.open_submission("feat/x", "sha2", None).unwrap();
    assert_eq!(r2.round, 2);
    fill_round(&mut ledger, &r2.id, &[]);
    assert_eq!(
        submit::evaluate(&plane, &ledger, &r2.id).unwrap().decision,
        "clear"
    );

    // A fresh chain opens at round 1 — and IS the latest submission,
    // despite the older round-2 row.
    let r3 = ledger.open_submission("feat/x", "sha3", None).unwrap();
    assert_eq!(r3.round, 1);
    let latest = ledger.latest_submission("feat/x").unwrap().unwrap();
    assert_eq!(latest.id, r3.id);
}

#[test]
fn plane_forces_submission_rev_into_event_json() {
    let dir = tempfile::tempdir().unwrap();
    let stub = dir.path().join("stub.sh");
    write_executable(
        &stub,
        r#"#!/bin/sh
cat > /dev/null
echo '{"type":"result","result":"{\"verdict\":\"pass\",\"findings\":[]}"}'
"#,
    );
    let plane = make_storm_plane(
        dir.path(),
        &format!(
            "harness = \"claude\"\nmodel = \"m\"\nbin = \"{}\"\n",
            stub.display()
        ),
        "correctness",
    );
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let sub = ledger
        .open_submission("feat/x", "the-real-sha", None)
        .unwrap();

    // The driver lies about the rev in the payload; the workspace's
    // EVENT.json carries the submission's rev anyway.
    let run_id = ledger
        .ingest(IngressRequest {
            task: "correctness",
            trigger_kind: "manual",
            idempotency_key: None,
            source_event_id: None,
            payload: Some(&format!(
                "{{\"submission\":\"{}\",\"repo\":\"o/r\",\"rev\":\"a-known-good-sha\"}}",
                sub.id
            )),
            parent_run_id: None,
        })
        .unwrap()
        .run_id;
    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();
    assert_eq!(run.state, "success");
    let attempts = ledger.attempts(&run_id).unwrap();
    let event = fs::read_to_string(
        Path::new(attempts[0].artifact_dir.as_deref().unwrap()).join("workspace/EVENT.json"),
    )
    .unwrap();
    assert!(event.contains("the-real-sha"), "{event}");
    assert!(!event.contains("a-known-good-sha"), "{event}");
    assert!(
        event.contains("\"repo\":\"o/r\""),
        "driver extras survive: {event}"
    );
}
