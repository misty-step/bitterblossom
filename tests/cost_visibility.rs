//! bitterblossom-969: per-harness cost visibility. Every plane spend control
//! (per-run cap, global and per-repo daily ceilings, in-flight overrun
//! monitor) reads parsed attempt `cost_usd`, so this suite pins the truth
//! table documented in docs/spine.md ("Per-harness cost reporting"):
//!
//! - every harness that claims cost reporting (claude, pi, omp, opencode)
//!   must land a non-null cost_usd in the ledger after a stubbed dispatch;
//! - codex and a silent command wrapper are cost-blind by construction;
//! - an API-auth agent holding the metered OPENROUTER_API_KEY on a
//!   cost-blind harness is refused admission (`cost_blind_harness`) unless
//!   it declares a provider-side child-key cap, in which case the capped
//!   child key is what actually reaches the run.

use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::Path;

use bitterblossom::dispatch;
use bitterblossom::ledger::{IngressRequest, Ledger};
use bitterblossom::spec::Plane;

const CLAUDE_STUB: &str = r#"#!/bin/sh
cat > /dev/null
echo '{"type":"result","result":"BB-CLAUDE-OK","total_cost_usd":0.0123,"num_turns":2,"usage":{"input_tokens":50,"output_tokens":20}}'
"#;

const CODEX_STUB: &str = r#"#!/bin/sh
cat > /dev/null
printf '{"type":"turn.completed","usage":{"input_tokens":50,"output_tokens":20}}\n'
printf '{"type":"item.completed","item":{"type":"agent_message","text":"BB-CODEX-OK"}}\n'
"#;

/// pi and omp share one JSONL contract: assistant `message_end` carries
/// `usage.cost.total` (OpenRouter per-response usage accounting).
const AGENT_JSONL_STUB: &str = r#"#!/bin/sh
cat > /dev/null
printf '{"type":"turn_end"}\n'
printf '{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"BB-JSONL-OK"}],"usage":{"input":100,"output":10,"cost":{"total":0.0031}}}}\n'
"#;

const OPENCODE_STUB: &str = r#"#!/bin/sh
cat > /dev/null
printf '{"type":"text","part":{"type":"text","text":"BB-OC-OK"}}\n'
printf '{"type":"step_finish","part":{"type":"step-finish","cost":0.0021,"tokens":{"input":100,"output":10}}}\n'
"#;

const COMMAND_COST_STUB: &str = r#"#!/bin/sh
cat > /dev/null
echo '{"schema_version":"bb.command_result.v1","result":"BB-CMD-OK","tokens_in":10,"tokens_out":5,"turns":1,"cost_usd":0.0042}'
"#;

/// A wrapper that reports no usage at all — the shape that produced the
/// live NULL-cost review run this card was born from.
const COMMAND_SILENT_STUB: &str = r#"#!/bin/sh
cat > /dev/null
echo 'BB-CMD-SILENT-OK'
"#;

fn write_executable(path: &Path, content: &str) {
    fs::write(path, content).unwrap();
    fs::set_permissions(path, fs::Permissions::from_mode(0o755)).unwrap();
}

fn setup(root: &Path, stub: &str, agent_toml_tail: &str) -> Plane {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    let stub_path = root.join("stub.sh");
    write_executable(&stub_path, stub);
    fs::write(
        root.join("agents/a.toml"),
        format!(
            "version = 1\nbin = \"{}\"\n{agent_toml_tail}",
            stub_path.display()
        ),
    )
    .unwrap();
    fs::write(root.join("tasks/demo/card.md"), "card\n").unwrap();
    fs::write(
        root.join("tasks/demo/task.toml"),
        "agent = \"a\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
    Plane::load(root).unwrap()
}

fn dispatch_demo(plane: &Plane) -> (bitterblossom::ledger::RunRow, Ledger) {
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
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
    let run = dispatch::dispatch_run(plane, &mut ledger, &id).unwrap();
    (run, ledger)
}

fn assert_stubbed_run_reports_cost(harness: &str, stub: &str, expected_cost: f64) {
    let dir = tempfile::tempdir().unwrap();
    let plane = setup(
        dir.path(),
        stub,
        &format!("harness = \"{harness}\"\nmodel = \"m\"\n"),
    );
    let (run, ledger) = dispatch_demo(&plane);
    assert_eq!(run.state, "success", "{harness}: {:?}", run.state_reason);
    assert_eq!(
        run.cost_usd,
        Some(expected_cost),
        "{harness}: run cost_usd must be non-null after a stubbed run"
    );
    let attempts = ledger.attempts(&run.id).unwrap();
    assert_eq!(
        attempts.last().unwrap().cost_usd,
        Some(expected_cost),
        "{harness}: attempt cost_usd must be non-null after a stubbed run"
    );
}

#[test]
fn claude_stubbed_run_lands_nonnull_cost() {
    assert_stubbed_run_reports_cost("claude", CLAUDE_STUB, 0.0123);
}

#[test]
fn pi_stubbed_run_lands_nonnull_cost() {
    assert_stubbed_run_reports_cost("pi", AGENT_JSONL_STUB, 0.0031);
}

#[test]
fn omp_stubbed_run_lands_nonnull_cost() {
    assert_stubbed_run_reports_cost("omp", AGENT_JSONL_STUB, 0.0031);
}

#[test]
fn opencode_stubbed_run_lands_nonnull_cost() {
    assert_stubbed_run_reports_cost("opencode", OPENCODE_STUB, 0.0021);
}

#[test]
fn command_wrapper_self_report_lands_nonnull_cost() {
    assert_stubbed_run_reports_cost("command", COMMAND_COST_STUB, 0.0042);
}

/// codex JSONL carries token usage but no dollar figure — the table in
/// docs/spine.md claims "never", and this pins it: a successful codex run
/// records NULL cost. codex is subscription-auth (API auth is forbidden at
/// load), so admission does not refuse it — there is no metered spend.
#[test]
fn codex_stubbed_run_is_cost_blind() {
    let dir = tempfile::tempdir().unwrap();
    let plane = setup(
        dir.path(),
        CODEX_STUB,
        "harness = \"codex\"\nmodel = \"m\"\n",
    );
    let (run, ledger) = dispatch_demo(&plane);
    assert_eq!(run.state, "success", "{:?}", run.state_reason);
    assert_eq!(run.cost_usd, None);
    let attempt = ledger.attempts(&run.id).unwrap().pop().unwrap();
    assert_eq!(attempt.cost_usd, None);
    assert_eq!(attempt.tokens_in, Some(50));
}

/// The live incident shape: an api-auth command wrapper holding the metered
/// parent OPENROUTER_API_KEY, with no child-key cap. Every spend control
/// would be blind, so admission refuses it by name.
#[test]
fn metered_agent_on_cost_blind_harness_is_refused_without_child_key_cap() {
    let dir = tempfile::tempdir().unwrap();
    let plane = setup(
        dir.path(),
        COMMAND_SILENT_STUB,
        "harness = \"command\"\nauth = \"api\"\nsecrets = [\"OPENROUTER_API_KEY\"]\n",
    );
    let (run, ledger) = dispatch_demo(&plane);
    assert_eq!(run.state, "blocked_budget");
    let reason = run.state_reason.unwrap();
    assert!(
        reason.contains("cannot report cost_usd") && reason.contains("command"),
        "refusal must name the blind harness: {reason}"
    );
    assert_eq!(
        ledger
            .budget_events_today_count("demo", "cost_blind_harness")
            .unwrap(),
        1
    );
    assert!(ledger.attempts(&run.id).unwrap().is_empty());
}

/// Bypass 1 (PR #1005 review, executed): the refusal must key on the metered
/// secret, not the auth label. A command agent labelled `auth =
/// "subscription"` still loads (spec only forbids subscription auth on
/// pi/omp/opencode) and still receives the parent OPENROUTER_API_KEY — the
/// label changes nothing about the spend, so it must not change admission.
#[test]
fn subscription_label_does_not_bypass_cost_blind_refusal() {
    let dir = tempfile::tempdir().unwrap();
    let plane = setup(
        dir.path(),
        COMMAND_SILENT_STUB,
        "harness = \"command\"\nauth = \"subscription\"\nsecrets = [\"OPENROUTER_API_KEY\"]\n",
    );
    let (run, ledger) = dispatch_demo(&plane);
    assert_eq!(run.state, "blocked_budget", "{:?}", run.state_reason);
    assert_eq!(
        ledger
            .budget_events_today_count("demo", "cost_blind_harness")
            .unwrap(),
        1
    );
}

/// Bypass 2 (PR #1005 review, executed): `provider` is a free-form string,
/// so any non-exact value (here a trailing space) must not disable the
/// refusal — the parent OPENROUTER_API_KEY still resolves from env while
/// the child-key swap can never happen for a provider bb cannot mint for.
#[test]
fn malformed_provider_does_not_bypass_cost_blind_refusal() {
    let dir = tempfile::tempdir().unwrap();
    let plane = setup(
        dir.path(),
        COMMAND_SILENT_STUB,
        "harness = \"command\"\nauth = \"api\"\nprovider = \"openrouter \"\nsecrets = [\"OPENROUTER_API_KEY\"]\n",
    );
    let (run, ledger) = dispatch_demo(&plane);
    assert_eq!(run.state, "blocked_budget", "{:?}", run.state_reason);
    assert_eq!(
        ledger
            .budget_events_today_count("demo", "cost_blind_harness")
            .unwrap(),
        1
    );
}

/// A declared cap only exempts when it can actually take effect: child keys
/// are minted for provider "openrouter" exactly, so a declared cap on any
/// other provider string is a dead letter and must not admit the run.
#[test]
fn declared_cap_on_unmintable_provider_does_not_admit() {
    let dir = tempfile::tempdir().unwrap();
    let plane = setup(
        dir.path(),
        COMMAND_SILENT_STUB,
        "harness = \"command\"\nauth = \"api\"\nprovider = \"openrouter \"\nsecrets = [\"OPENROUTER_API_KEY\"]\n\
         [policy]\nprovider_key_name = \"demo-key\"\nprovider_spend_cap_usd = 1.0\n",
    );
    let (run, _ledger) = dispatch_demo(&plane);
    assert_eq!(run.state, "blocked_budget", "{:?}", run.state_reason);
}

/// Same shape with a declared child-key cap and a minted (stored) key: the
/// run is admitted and the capped child key — not the parent env key — is
/// what reaches the wrapper.
#[test]
fn declared_child_key_cap_admits_metered_agent_on_cost_blind_harness() {
    let dir = tempfile::tempdir().unwrap();
    // Wrapper proves which key it received by echoing a marker when the
    // child key (never exported to this test's env) is present.
    let stub = r#"#!/bin/sh
cat > /dev/null
if [ "$OPENROUTER_API_KEY" = "sk-or-v1-test-child-key-sentinel" ]; then
  echo '{"schema_version":"bb.command_result.v1","result":"BB-CHILD-KEY-OK","cost_usd":0.0042}'
else
  echo '{"schema_version":"bb.command_result.v1","result":"BB-WRONG-KEY"}'
fi
"#;
    let plane = setup(
        dir.path(),
        stub,
        "harness = \"command\"\nauth = \"api\"\nsecrets = [\"OPENROUTER_API_KEY\"]\n\
         [policy]\nprovider_key_name = \"demo-key\"\nprovider_spend_cap_usd = 1.0\n",
    );
    let store = dir.path().join(".bb/provider-keys/openrouter");
    fs::create_dir_all(&store).unwrap();
    fs::write(
        store.join("a.json"),
        r#"{
  "schema_version": 1,
  "provider": "openrouter",
  "agent": "a",
  "provider_key_name": "demo-key",
  "name": "bb:test:a:demo-key",
  "hash": "hash-a",
  "label": "test",
  "spend_cap_usd": 1.0,
  "limit_remaining_usd": null,
  "limit_reset": null,
  "usage_usd": 0.0,
  "disabled": false,
  "created_at": "2026-07-14T00:00:00Z",
  "updated_at": null,
  "minted_at": "2026-07-14T00:00:00Z",
  "revoked_at": null,
  "api_key": "sk-or-v1-test-child-key-sentinel"
}"#,
    )
    .unwrap();
    let (run, ledger) = dispatch_demo(&plane);
    assert_eq!(run.state, "success", "{:?}", run.state_reason);
    assert_eq!(run.cost_usd, Some(0.0042));
    assert_eq!(
        ledger
            .budget_events_today_count("demo", "cost_blind_harness")
            .unwrap(),
        0
    );
}

/// The refusal is scoped to metered-key holders: a command agent with no
/// OPENROUTER_API_KEY cannot spend metered dollars, so it is admitted — and
/// its NULL cost stays an honest NULL, never a fabricated zero.
#[test]
fn unmetered_command_agent_is_admitted_and_records_null_cost() {
    let dir = tempfile::tempdir().unwrap();
    let plane = setup(dir.path(), COMMAND_SILENT_STUB, "harness = \"command\"\n");
    let (run, ledger) = dispatch_demo(&plane);
    assert_eq!(run.state, "success", "{:?}", run.state_reason);
    assert_eq!(run.cost_usd, None);
    assert_eq!(
        ledger.attempts(&run.id).unwrap().pop().unwrap().cost_usd,
        None
    );
}
