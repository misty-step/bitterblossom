//! bitterblossom-933: the run plane's glass emitter. `deliver()` shells to
//! curl exactly like notify.rs/canary.rs, so tests point `BB_GLASS_BIN` at a
//! stub that logs the request body and returns a glass-shaped JSON response
//! (proven live against the real instance: an unrecognized `session_id` is
//! a 404, so the stub must hand back a session id the way glass really
//! does, or the "reuse across posts in one lineage" behavior can't be
//! exercised at all).

use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::Path;
use std::sync::Mutex;
use std::time::Duration;

use bitterblossom::dispatch;
use bitterblossom::ledger::{IngressRequest, Ledger};
use bitterblossom::spec::Plane;

static GLASS_ENV_LOCK: Mutex<()> = Mutex::new(());

/// Logs the request body (one line, `\n---\n` separated) then answers with a
/// glass-shaped `PublishOutcome`: a *new* session_id if the request had none
/// (mimicking `ensure_session`'s auto-create), or the caller's own
/// session_id echoed back (mimicking a reused, already-valid session).
const GLASS_STUB: &str = r#"#!/bin/sh
body="$(cat)"
printf '%s\n---\n' "$body" >> "$BB_GLASS_LOG"
session=$(printf '%s' "$body" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("session_id") or "")')
if [ -z "$session" ]; then
  session="ses-stub-$(date +%s%N)"
fi
printf '{"post":{"id":"post-stub","session_id":"%s"},"url":"https://glass.invalid/p/post-stub"}' "$session"
"#;

fn write_executable(path: &Path, content: &str) {
    fs::write(path, content).unwrap();
    fs::set_permissions(path, fs::Permissions::from_mode(0o755)).unwrap();
}

fn with_glass_stub<T>(root: &Path, f: impl FnOnce() -> T) -> (T, String) {
    let _guard = GLASS_ENV_LOCK.lock().unwrap();
    let stub = root.join("glass-stub.sh");
    write_executable(&stub, GLASS_STUB);
    let log = root.join("glass.log");
    std::env::set_var("BB_GLASS_BIN", &stub);
    std::env::set_var("BB_GLASS_LOG", &log);
    let out = f();
    let mut text = String::new();
    for _ in 0..40 {
        text = fs::read_to_string(&log).unwrap_or_default();
        if !text.is_empty() {
            break;
        }
        std::thread::sleep(Duration::from_millis(50));
    }
    std::env::remove_var("BB_GLASS_BIN");
    std::env::remove_var("BB_GLASS_LOG");
    (out, text)
}

fn make_plane(root: &Path, glass_base_url: Option<&str>) -> Plane {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    let glass_toml = glass_base_url
        .map(|url| format!("[glass]\nbase_url = \"{url}\"\n"))
        .unwrap_or_default();
    fs::write(root.join("plane.toml"), format!("dev = true\n{glass_toml}")).unwrap();
    let stub_path = root.join("stub-harness.sh");
    write_executable(
        &stub_path,
        "#!/bin/sh\ncat > /dev/null\necho '{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"ok\",\"total_cost_usd\":0.01,\"num_turns\":1,\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}'\n",
    );
    fs::write(
        root.join("agents/stub.toml"),
        format!(
            "harness = \"claude\"\nmodel = \"m\"\nbin = \"{}\"\n",
            stub_path.display()
        ),
    )
    .unwrap();
    fs::write(root.join("tasks/demo/card.md"), "card\n").unwrap();
    fs::write(
        root.join("tasks/demo/task.toml"),
        "agent = \"stub\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
    Plane::load(root).unwrap()
}

fn manual_run(ledger: &mut Ledger, task: &str) -> String {
    ledger
        .ingest(IngressRequest {
            task,
            trigger_kind: "manual",
            idempotency_key: None,
            source_event_id: None,
            payload: None,
            parent_run_id: None,
        })
        .unwrap()
        .run_id
}

#[test]
fn dispatch_posts_dispatched_and_completed_reusing_one_glass_session() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path(), Some("http://glass.invalid"));
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let run_id = manual_run(&mut ledger, "demo");

    let (run, log) = with_glass_stub(dir.path(), || {
        dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap()
    });

    assert_eq!(run.state, "success");
    assert!(log.contains("dispatched"), "{log}");
    assert!(log.contains("success"), "{log}");
    assert!(log.contains(&run_id), "{log}");
    assert!(log.contains("\"agent\":\"stub\""), "{log}");

    // The first post (dispatched) had no session_id; the second (completed)
    // must carry the SAME session_id the stub handed back for the first --
    // proof that bb persisted and reused glass's own session, not its own.
    let requests: Vec<&str> = log.split("---").filter(|s| !s.trim().is_empty()).collect();
    assert_eq!(requests.len(), 2, "expected dispatched + completed: {log}");
    assert!(
        !requests[0].contains("session_id"),
        "first post must omit session_id so glass creates one: {}",
        requests[0]
    );
    assert!(
        requests[1].contains("\"session_id\":\"ses-stub-"),
        "second post must reuse the created session: {}",
        requests[1]
    );

    let stored = ledger.run_glass_session(&run_id).unwrap();
    assert!(stored.is_some(), "glass_session_id was never persisted");
    assert!(requests[1].contains(stored.as_deref().unwrap()));
}

#[test]
fn a_resumed_run_reuses_its_parked_parents_glass_session() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path(), Some("http://glass.invalid"));
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let run_id = manual_run(&mut ledger, "demo");
    ledger.transition(&run_id, "running", None).unwrap();

    let (_, log) = with_glass_stub(dir.path(), || {
        bitterblossom::glass::post_asked(
            &plane, &ledger, &run_id, "demo", "stub", "ask-1", "approval", "deploy?",
        );
    });
    assert!(
        !log.contains("session_id"),
        "first post must omit it: {log}"
    );
    let session = ledger.run_glass_session(&run_id).unwrap().unwrap();

    ledger.transition(&run_id, "parked_on_ask", None).unwrap();
    let resumed_run = ledger
        .ingest(IngressRequest {
            task: "demo",
            trigger_kind: "resume",
            idempotency_key: Some("resume:ask-1"),
            source_event_id: None,
            payload: None,
            parent_run_id: Some(&run_id),
        })
        .unwrap()
        .run_id;

    let (_, log) = with_glass_stub(dir.path(), || {
        bitterblossom::glass::post_resumed(
            &plane,
            &ledger,
            &run_id,
            &resumed_run,
            "demo",
            "stub",
            "ask-1",
        );
    });
    // The resume run's own session lookup walks its parent_run_id back to
    // the parked run, so this post reuses THAT session, not a new one --
    // this is what makes park -> resume look like one continuous glass
    // session instead of two unrelated posts.
    assert!(
        log.contains(&format!("\"session_id\":\"{session}\"")),
        "{log}"
    );
    assert!(log.contains(&resumed_run), "{log}");

    // Reused, not overwritten: the root run's stored session is unchanged.
    assert_eq!(ledger.run_glass_session(&run_id).unwrap(), Some(session));
}

#[test]
fn glass_posting_is_a_no_op_without_a_configured_base_url() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path(), None);
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let run_id = manual_run(&mut ledger, "demo");

    let _guard = GLASS_ENV_LOCK.lock().unwrap();
    let stub = dir.path().join("glass-stub.sh");
    write_executable(&stub, GLASS_STUB);
    let log = dir.path().join("glass.log");
    std::env::set_var("BB_GLASS_BIN", &stub);
    std::env::set_var("BB_GLASS_LOG", &log);

    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();
    assert_eq!(run.state, "success");

    std::env::remove_var("BB_GLASS_BIN");
    std::env::remove_var("BB_GLASS_LOG");
    assert!(
        !log.exists(),
        "glass stub ran despite no configured base_url"
    );
}
