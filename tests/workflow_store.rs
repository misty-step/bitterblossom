//! bitterblossom-workflow-store: behavior-first drills for the revisioned
//! workflow configuration store, through the public CLI and HTTP surfaces.
//!
//! Card oracle coverage:
//! 1. draft/activate/pause/archive/revise/rollback in one database-backed API
//! 2. CLI and HTTP create/read/diff/activate the same immutable revisions
//! 3. declarative import/export round-trips without a second live authority
//! 4. runs accepted before and after activation keep their pinned config
//! 5. migration fixture: file-defined workload import + active readback
//!
//! Proof plan extras: SQLite restart/readback and a revision concurrency drill.

use std::fs;
use std::io::{Read, Write};
use std::net::TcpStream;
use std::path::{Path, PathBuf};
use std::process::{Command, Output};
use std::time::{Duration, Instant};

use bitterblossom::ledger::Ledger;
use bitterblossom::workflow::{AcceptOutcome, WorkflowDoc};

const DOC: &str = r#"
name = "pr-review"
goal = "Review every reviewable pull-request head and post one formal review."

[[trigger]]
kind = "webhook"
route = "review"
secret_env = "BB_HOOK_REVIEW"

[[step]]
name = "review"
goal = "Review the pull request against the goal contract."

[step.agent]
name = "cerberus"
version = 3
harness = "opencode"
model = "moonshotai/kimi-k2.6"

[step.routes]
clear = "done"
blocked = "done"

[policies]
max_runs_per_day = 20
"#;

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

fn write_doc(root: &Path, name: &str, text: &str) -> PathBuf {
    let path = root.join(name);
    fs::write(&path, text).unwrap();
    path
}

// --- criterion 1: one database-backed lifecycle API -------------------------

#[test]
fn lifecycle_revise_rollback_and_audit_in_one_store() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let doc = write_doc(root, "wf.toml", DOC);
    let doc = doc.to_str().unwrap();

    // draft
    let created = bb_json(root, &["workflow", "create", doc, "--json"]);
    assert_eq!(created["workflow"]["state"], "draft");
    assert_eq!(created["revision"], 1);
    assert_eq!(
        created["workflow"]["active_revision"],
        serde_json::Value::Null
    );

    // a draft accepts nothing
    let refused = bb(root, &["workflow", "accept", "pr-review"]);
    assert!(!refused.status.success());
    assert!(String::from_utf8_lossy(&refused.stderr).contains("draft"));

    // activate (defaults to latest revision)
    let active = bb_json(root, &["workflow", "activate", "pr-review", "--json"]);
    assert_eq!(active["state"], "active");
    assert_eq!(active["active_revision"], 1);

    // revise: identical document refused, changed document appends revision 2
    let unchanged = bb(root, &["workflow", "revise", "pr-review", doc]);
    assert!(!unchanged.status.success());
    assert!(String::from_utf8_lossy(&unchanged.stderr).contains("identical to revision 1"));
    let doc2 = write_doc(
        root,
        "wf2.toml",
        &DOC.replace("Review every", "Re-review every"),
    );
    let revised = bb_json(
        root,
        &[
            "workflow",
            "revise",
            "pr-review",
            doc2.to_str().unwrap(),
            "--json",
        ],
    );
    assert_eq!(revised["revision"], 2);
    // revising does NOT activate: the active revision is still 1
    assert_eq!(revised["workflow"]["active_revision"], 1);

    bb_ok(
        root,
        &["workflow", "activate", "pr-review", "--revision", "2"],
    );

    // rollback re-activates the old snapshot as a NEW revision 3
    let rolled = bb_json(
        root,
        &["workflow", "rollback", "pr-review", "--to", "1", "--json"],
    );
    assert_eq!(rolled["revision"], 3);
    assert_eq!(rolled["workflow"]["active_revision"], 3);
    let diff = bb_json(
        root,
        &[
            "workflow",
            "diff",
            "pr-review",
            "--from",
            "1",
            "--to",
            "3",
            "--json",
        ],
    );
    assert_eq!(
        diff["identical"], true,
        "rollback snapshot must equal its source"
    );
    let history = bb_json(root, &["workflow", "show", "pr-review", "--json"]);
    assert_eq!(history["revisions"].as_array().unwrap().len(), 3);

    // pause suppresses acceptance with a recorded disposition; resume never replays
    bb_ok(
        root,
        &["workflow", "pause", "pr-review", "--reason", "drill"],
    );
    let suppressed = bb(root, &["workflow", "accept", "pr-review", "--json"]);
    assert_eq!(suppressed.status.code(), Some(3));
    let body: serde_json::Value =
        serde_json::from_slice(&suppressed.stdout).expect("suppressed acceptance emits JSON");
    assert_eq!(body["disposition"], "suppressed");
    bb_ok(root, &["workflow", "resume", "pr-review"]);
    assert!(bb_json(root, &["workflow", "runs", "pr-review", "--json"])
        .as_array()
        .unwrap()
        .is_empty());

    // archive freezes: no new revisions, no acceptance, history stays readable
    let archived = bb_json(root, &["workflow", "archive", "pr-review", "--json"]);
    assert_eq!(archived["state"], "archived");
    let frozen = bb(
        root,
        &["workflow", "revise", "pr-review", doc2.to_str().unwrap()],
    );
    assert!(!frozen.status.success());
    let refused = bb(root, &["workflow", "accept", "pr-review"]);
    assert!(!refused.status.success());
    let readback = bb_json(root, &["workflow", "show", "pr-review", "--json"]);
    assert_eq!(readback["workflow"]["state"], "archived");
    assert_eq!(readback["active_document"]["name"], "pr-review");

    // the audit trail names every act
    let events = bb_json(root, &["workflow", "events", "pr-review", "--json"]);
    let kinds: Vec<&str> = events
        .as_array()
        .unwrap()
        .iter()
        .map(|e| e["kind"].as_str().unwrap())
        .collect();
    for expected in [
        "created",
        "activated",
        "revised",
        "rolled_back",
        "paused",
        "event_suppressed",
        "resumed",
        "archived",
    ] {
        assert!(
            kinds.contains(&expected),
            "audit trail missing {expected}: {kinds:?}"
        );
    }
}

// Review fix 1 (PR #1001): a `/` in a workflow name would make every
// name-scoped HTTP route unaddressable (tiny_http does no percent-decoding
// and the routes parse the name as one path segment), so names must reject
// `/` at create/import — on every surface, because CLI and HTTP share one
// store.
#[test]
fn slash_names_are_rejected_at_create_and_import() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let doc = write_doc(
        root,
        "slashed.toml",
        &DOC.replace("name = \"pr-review\"", "name = \"team/pr-review\""),
    );
    let doc = doc.to_str().unwrap();

    let created = bb(root, &["workflow", "create", doc]);
    assert!(!created.status.success(), "create accepted a '/' name");
    assert!(String::from_utf8_lossy(&created.stderr).contains("team/pr-review"));
    let imported = bb(root, &["workflow", "import", doc]);
    assert!(!imported.status.success(), "import accepted a '/' name");
    assert!(
        bb_json(root, &["workflow", "list", "--json"])
            .as_array()
            .unwrap()
            .is_empty(),
        "a rejected name must not leave a workflow row behind"
    );

    // and over HTTP, where the name would be unaddressable
    let token = "wf-slash-token";
    let (serve, port) = spawn_serve(root, token);
    let mut doc = WorkflowDoc::from_toml(DOC).unwrap();
    doc.name = "team/pr-review".into();
    let body = serde_json::json!({ "document": doc }).to_string();
    let (status, response) = http(port, "POST", "/api/workflows", Some(token), Some(&body));
    assert_eq!(status, 400, "{response}");
    serve.stop();
}

// --- criterion 2: CLI and HTTP share the same immutable revisions -----------

struct ServeGuard {
    child: std::process::Child,
}

impl ServeGuard {
    fn stop(mut self) {
        #[cfg(unix)]
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
}

impl Drop for ServeGuard {
    fn drop(&mut self) {
        let _ = self.child.kill();
        let _ = self.child.wait();
    }
}

fn spawn_serve(root: &Path, token: &str) -> (ServeGuard, u16) {
    let port_file = root.join("bb-serve-port");
    let _ = fs::remove_file(&port_file);
    // stderr goes to a log file so tests can assert what the server did NOT
    // do (e.g. fire a runtime-error report for a plain 404).
    let stderr_log = fs::File::create(root.join("serve-stderr.log")).unwrap();
    let child = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", root.to_str().unwrap(), "serve"])
        .env("BB_INGRESS_REPORT_PORT_FILE", &port_file)
        .env("BB_API_TOKEN", token)
        .stdout(std::process::Stdio::null())
        .stderr(stderr_log)
        .spawn()
        .unwrap();
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
    token: Option<&str>,
    body: Option<&str>,
) -> (u16, serde_json::Value) {
    let auth = token
        .map(|t| format!("Authorization: Bearer {t}\r\n"))
        .unwrap_or_default();
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
        "{method} {path} HTTP/1.1\r\nHost: 127.0.0.1\r\n{auth}{content}Connection: close\r\n\r\n{body}"
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

#[test]
fn cli_and_http_create_read_diff_activate_the_same_revisions() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let token = "wf-test-token";
    let (serve, port) = spawn_serve(root, token);

    // token required on every workflow route
    assert_eq!(http(port, "GET", "/api/workflows", None, None).0, 401);
    assert_eq!(
        http(port, "POST", "/api/workflows/x/activate", None, Some("{}")).0,
        401
    );

    // create over HTTP, revision 1
    let doc = WorkflowDoc::from_toml(DOC).unwrap();
    let create_body = serde_json::json!({ "document": doc, "note": "via http" }).to_string();
    let (status, created) = http(
        port,
        "POST",
        "/api/workflows",
        Some(token),
        Some(&create_body),
    );
    assert_eq!(status, 201, "{created}");
    assert_eq!(created["revision"], 1);
    assert_eq!(created["workflow"]["state"], "draft");

    // the CLI sees the identical stored revision (same canonical document)
    let cli_rev = bb_json(root, &["workflow", "show", "pr-review", "--json"]);
    assert_eq!(cli_rev["revisions"].as_array().unwrap().len(), 1);

    // revise over CLI; HTTP reads revision 2 and diffs 1 -> 2
    let doc2 = write_doc(
        root,
        "wf2.toml",
        &DOC.replace("Review every", "Re-review every"),
    );
    bb_ok(
        root,
        &["workflow", "revise", "pr-review", doc2.to_str().unwrap()],
    );
    let (status, revision) = http(
        port,
        "GET",
        "/api/workflows/pr-review/revisions/2",
        Some(token),
        None,
    );
    assert_eq!(status, 200);
    assert!(revision["document"]["goal"]
        .as_str()
        .unwrap()
        .starts_with("Re-review"));
    let (status, diff) = http(
        port,
        "GET",
        "/api/workflows/pr-review/diff?from=1&to=2",
        Some(token),
        None,
    );
    assert_eq!(status, 200);
    assert_eq!(diff["identical"], false);
    let cli_diff = bb_json(
        root,
        &[
            "workflow",
            "diff",
            "pr-review",
            "--from",
            "1",
            "--to",
            "2",
            "--json",
        ],
    );
    assert_eq!(
        diff["changes"], cli_diff["changes"],
        "CLI and HTTP diff the same revisions"
    );

    // activate over HTTP; CLI observes it (and vice versa)
    let (status, activated) = http(
        port,
        "POST",
        "/api/workflows/pr-review/activate",
        Some(token),
        Some(&serde_json::json!({ "revision": 2 }).to_string()),
    );
    assert_eq!(status, 200, "{activated}");
    assert_eq!(activated["active_revision"], 2);
    let cli_wf = bb_json(root, &["workflow", "show", "pr-review", "--json"]);
    assert_eq!(cli_wf["workflow"]["active_revision"], 2);
    assert_eq!(cli_wf["workflow"]["state"], "active");

    // accept over HTTP pins revision 2; pause over CLI suppresses HTTP accepts
    let (status, accepted) = http(
        port,
        "POST",
        "/api/workflows/pr-review/runs",
        Some(token),
        Some(&serde_json::json!({ "trigger_kind": "webhook", "payload": {"pr": 7} }).to_string()),
    );
    assert_eq!(status, 201, "{accepted}");
    assert_eq!(accepted["run"]["revision"], 2);
    bb_ok(root, &["workflow", "pause", "pr-review"]);
    let (status, suppressed) = http(
        port,
        "POST",
        "/api/workflows/pr-review/runs",
        Some(token),
        Some("{}"),
    );
    assert_eq!(status, 202);
    assert_eq!(suppressed["disposition"], "suppressed");

    // run readback over HTTP names the pinned document
    let run_id = accepted["run"]["id"].as_str().unwrap();
    let (status, run_view) = http(
        port,
        "GET",
        &format!("/api/workflow-runs/{run_id}"),
        Some(token),
        None,
    );
    assert_eq!(status, 200);
    assert!(run_view["document"]["goal"]
        .as_str()
        .unwrap()
        .starts_with("Re-review"));

    serve.stop();
}

// Review fixes 2 + 4 (PR #1001): unknown names on the runs/events routes
// must be plain 404 client errors — not 500s that fire runtime-error
// telemetry — and a malformed export revision must be a 400, not a silent
// fallback to the default revision.
#[test]
fn unknown_and_malformed_workflow_requests_are_client_errors_not_500s() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let token = "wf-404-token";
    let (serve, port) = spawn_serve(root, token);

    for path in [
        "/api/workflows/nope/runs",
        "/api/workflows/nope/events",
        "/api/workflows/nope",
    ] {
        let (status, body) = http(port, "GET", path, Some(token), None);
        assert_eq!(status, 404, "{path}: {body}");
        assert!(
            body["error"].as_str().unwrap().contains("not found"),
            "{path}: {body}"
        );
    }

    let doc = WorkflowDoc::from_toml(DOC).unwrap();
    let body = serde_json::json!({ "document": doc }).to_string();
    assert_eq!(
        http(port, "POST", "/api/workflows", Some(token), Some(&body)).0,
        201
    );
    let (status, body) = http(
        port,
        "GET",
        "/api/workflows/pr-review/export?revision=garbage",
        Some(token),
        None,
    );
    assert_eq!(
        status, 400,
        "malformed revision must not silently export: {body}"
    );
    let (status, _) = http(
        port,
        "GET",
        "/api/workflows/pr-review/export?revision=99",
        Some(token),
        None,
    );
    assert_eq!(status, 404, "missing revision is a 404");
    let (status, _) = http(
        port,
        "GET",
        "/api/workflows/pr-review/export?revision=1",
        Some(token),
        None,
    );
    assert_eq!(status, 200);

    serve.stop();
    let stderr = fs::read_to_string(root.join("serve-stderr.log")).unwrap();
    assert!(
        !stderr.contains("http request"),
        "client errors fired runtime-error telemetry:\n{stderr}"
    );
}

// --- criterion 3 + restart/readback -----------------------------------------

#[test]
fn import_export_round_trips_and_survives_serve_restart() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let token = "wf-restart-token";

    // create + activate over HTTP on the first serve process
    let (serve, port) = spawn_serve(root, token);
    let doc = WorkflowDoc::from_toml(DOC).unwrap();
    let body = serde_json::json!({ "document": doc }).to_string();
    assert_eq!(
        http(port, "POST", "/api/workflows", Some(token), Some(&body)).0,
        201
    );
    assert_eq!(
        http(
            port,
            "POST",
            "/api/workflows/pr-review/activate",
            Some(token),
            Some("{}")
        )
        .0,
        200
    );
    serve.stop();

    // restart: the database is the authority; everything reads back
    let (serve, port) = spawn_serve(root, token);
    let (status, wf) = http(port, "GET", "/api/workflows/pr-review", Some(token), None);
    assert_eq!(status, 200);
    assert_eq!(wf["workflow"]["state"], "active");
    assert_eq!(wf["workflow"]["active_revision"], 1);
    assert_eq!(
        wf["active_document"]["step"][0]["agent"]["name"],
        "cerberus"
    );

    // export over HTTP == export over CLI; importing it back is a no-op
    let (status, exported) = http(
        port,
        "GET",
        "/api/workflows/pr-review/export",
        Some(token),
        None,
    );
    assert_eq!(status, 200);
    let toml_text = exported["toml"].as_str().unwrap();
    let cli_export = bb_ok(root, &["workflow", "export", "pr-review"]);
    assert_eq!(toml_text, cli_export, "one export shape across surfaces");
    let reimport = write_doc(root, "reimport.toml", toml_text);
    let outcome = bb_json(
        root,
        &["workflow", "import", reimport.to_str().unwrap(), "--json"],
    );
    assert_eq!(outcome["outcome"], "unchanged", "{outcome}");
    assert_eq!(outcome["revision"], 1);
    assert_eq!(
        bb_json(root, &["workflow", "show", "pr-review", "--json"])["revisions"]
            .as_array()
            .unwrap()
            .len(),
        1,
        "round-trip import must not mint a shadow revision"
    );

    // a changed document imports as a new revision; importing it twice does not
    let changed = write_doc(
        root,
        "changed.toml",
        &toml_text.replace("Review every", "Re-review every"),
    );
    let outcome = bb_json(
        root,
        &["workflow", "import", changed.to_str().unwrap(), "--json"],
    );
    assert_eq!(outcome["outcome"], "revised");
    assert_eq!(outcome["revision"], 2);
    let outcome = bb_json(
        root,
        &["workflow", "import", changed.to_str().unwrap(), "--json"],
    );
    assert_eq!(outcome["outcome"], "unchanged");
    // import never activates by itself: the live authority stays revision 1
    assert_eq!(
        bb_json(root, &["workflow", "show", "pr-review", "--json"])["workflow"]["active_revision"],
        1
    );

    // importing a brand-new name creates a draft
    let fresh = write_doc(root, "fresh.toml", &DOC.replace("pr-review", "ci-audit"));
    let outcome = bb_json(
        root,
        &["workflow", "import", fresh.to_str().unwrap(), "--json"],
    );
    assert_eq!(outcome["outcome"], "created");
    assert_eq!(outcome["workflow"]["state"], "draft");

    serve.stop();
}

// --- criterion 4: pinned run configuration ----------------------------------

#[test]
fn runs_accepted_before_and_after_activation_keep_their_pinned_config() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let doc = write_doc(root, "wf.toml", DOC);
    bb_ok(root, &["workflow", "create", doc.to_str().unwrap()]);
    bb_ok(root, &["workflow", "activate", "pr-review"]);

    let before = bb_json(
        root,
        &[
            "workflow",
            "accept",
            "pr-review",
            "--payload",
            r#"{"pr":1}"#,
            "--json",
        ],
    );
    assert_eq!(before["run"]["revision"], 1);
    let before_id = before["run"]["id"].as_str().unwrap().to_string();

    // new activation affects new events only
    let doc2 = write_doc(
        root,
        "wf2.toml",
        &DOC.replace("moonshotai/kimi-k2.6", "deepseek/deepseek-v4-flash"),
    );
    bb_ok(
        root,
        &["workflow", "revise", "pr-review", doc2.to_str().unwrap()],
    );
    bb_ok(
        root,
        &["workflow", "activate", "pr-review", "--revision", "2"],
    );
    let after = bb_json(
        root,
        &[
            "workflow",
            "accept",
            "pr-review",
            "--payload",
            r#"{"pr":2}"#,
            "--json",
        ],
    );
    assert_eq!(after["run"]["revision"], 2);
    let after_id = after["run"]["id"].as_str().unwrap().to_string();

    // both runs read back their exact acceptance-time documents — including
    // after the workflow rolls back and archives (fresh CLI process per read
    // is itself a SQLite reopen/readback proof).
    bb_ok(root, &["workflow", "rollback", "pr-review", "--to", "1"]);
    bb_ok(root, &["workflow", "archive", "pr-review"]);
    let view = bb_json(root, &["workflow", "run-show", &before_id, "--json"]);
    assert_eq!(view["run"]["revision"], 1);
    assert_eq!(
        view["document"]["step"][0]["agent"]["model"],
        "moonshotai/kimi-k2.6"
    );
    let view = bb_json(root, &["workflow", "run-show", &after_id, "--json"]);
    assert_eq!(view["run"]["revision"], 2);
    assert_eq!(
        view["document"]["step"][0]["agent"]["model"],
        "deepseek/deepseek-v4-flash"
    );
}

#[test]
fn workflow_daily_ceiling_denies_and_exposes_ledger_spend() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let text = DOC.replace(
        "max_runs_per_day = 20",
        "max_runs_per_day = 20\nmax_cost_per_day_usd = 0.50\nestimated_cost_per_run_usd = 0.50",
    );
    let doc = write_doc(root, "budget.toml", &text);
    bb_ok(root, &["workflow", "create", doc.to_str().unwrap()]);
    bb_ok(root, &["workflow", "activate", "pr-review"]);

    let accepted = bb_json(root, &["workflow", "accept", "pr-review", "--json"]);
    let run_id = accepted["run"]["id"].as_str().unwrap();
    // Seed the immutable run's mutable status with observed spend, as a
    // completed runtime would, then read it through the public CLI projection.
    let db = root.join(".bb/plane.db");
    let conn = rusqlite::Connection::open(&db).unwrap();
    conn.execute(
        "UPDATE workflow_run_status SET state = 'succeeded', cost_usd = 0.50 WHERE run_id = ?1",
        [run_id],
    )
    .unwrap();
    let spend = bb_json(root, &["workflow", "spend", "pr-review", "--json"]);
    assert_eq!(spend["spend_today_usd"], 0.5);
    assert_eq!(spend["max_cost_per_day_usd"], 0.5);

    let denied = bb(root, &["workflow", "accept", "pr-review", "--json"]);
    assert_eq!(denied.status.code(), Some(3));
    let denied_json: serde_json::Value = serde_json::from_slice(&denied.stdout).unwrap();
    assert_eq!(denied_json["disposition"], "denied");
    assert!(denied_json["reason"]
        .as_str()
        .unwrap()
        .contains("workflow daily ceiling"));
    let events = bb_json(root, &["workflow", "events", "pr-review", "--json"]);
    assert!(events.as_array().unwrap().iter().any(|event| {
        event["kind"] == "workflow_daily_ceiling"
            && event["data"]
                .as_str()
                .unwrap_or("")
                .contains("max_cost_per_day_usd")
    }));

    let exported = bb_ok(root, &["workflow", "export", "pr-review"]);
    assert!(exported.contains("max_cost_per_day_usd = 0.5"));
    assert!(exported.contains("estimated_cost_per_run_usd = 0.5"));
}

#[test]
fn workflow_run_count_policy_denies_second_same_day_acceptance() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let text = DOC.replace("max_runs_per_day = 20", "max_runs_per_day = 1");
    let doc = write_doc(root, "run-count.toml", &text);
    bb_ok(root, &["workflow", "create", doc.to_str().unwrap()]);
    bb_ok(root, &["workflow", "activate", "pr-review"]);

    let accepted = bb_json(root, &["workflow", "accept", "pr-review", "--json"]);
    assert_eq!(accepted["disposition"], "accepted");
    let denied = bb(root, &["workflow", "accept", "pr-review", "--json"]);
    assert_eq!(denied.status.code(), Some(3));
    let denied: serde_json::Value = serde_json::from_slice(&denied.stdout).unwrap();
    assert_eq!(denied["disposition"], "denied");
    assert!(denied["reason"]
        .as_str()
        .unwrap()
        .contains("max_runs_per_day"));
}

#[test]
fn plane_daily_budget_counts_workflow_reservations_and_standard_spend() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    fs::write(
        root.join("plane.toml"),
        "dev = true\n[budget]\nmax_cost_per_day_usd = 1.0\n",
    )
    .unwrap();
    let doc = write_doc(
        root,
        "plane-budget.toml",
        &DOC.replace(
            "max_runs_per_day = 20",
            "max_runs_per_day = 20\nmax_cost_per_day_usd = 10.0\nestimated_cost_per_run_usd = 1.0",
        ),
    );
    bb_ok(root, &["workflow", "create", doc.to_str().unwrap()]);
    bb_ok(root, &["workflow", "activate", "pr-review"]);
    let first = bb_json(root, &["workflow", "accept", "pr-review", "--json"]);
    assert_eq!(first["disposition"], "accepted");
    let second = bb(root, &["workflow", "accept", "pr-review", "--json"]);
    assert_eq!(second.status.code(), Some(3));
    assert!(
        String::from_utf8_lossy(&second.stdout).contains("global_daily_ceiling")
            || String::from_utf8_lossy(&second.stdout).contains("plane daily ceiling")
    );
}

#[test]
fn plane_budget_combines_standard_observed_with_workflow_reservation() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    fs::write(
        root.join("plane.toml"),
        "dev = true\n[budget]\nmax_cost_per_day_usd = 1.0\n",
    )
    .unwrap();
    let doc = write_doc(
        root,
        "combined.toml",
        &DOC.replace(
            "max_runs_per_day = 20",
            "max_runs_per_day = 20\nmax_cost_per_day_usd = 10.0\nestimated_cost_per_run_usd = 0.5",
        ),
    );
    bb_ok(root, &["workflow", "create", doc.to_str().unwrap()]);
    bb_ok(root, &["workflow", "activate", "pr-review"]);
    let db = root.join(".bb/plane.db");
    let conn = rusqlite::Connection::open(&db).unwrap();
    conn.execute("INSERT INTO runs (id, task, trigger_kind, state, trace_id, created_at, updated_at) VALUES ('std-1', 'standard', 'test', 'running', 'trace', datetime('now'), datetime('now'))", []).unwrap();
    conn.execute("INSERT INTO attempts (run_id, n, agent_name, agent_version, harness, model, phase, cost_usd, started_at) VALUES ('std-1', 1, 'agent', 1, 'opencode', 'stub', 'executing', 0.75, datetime('now'))", []).unwrap();
    let denied = bb(root, &["workflow", "accept", "pr-review", "--json"]);
    assert_eq!(denied.status.code(), Some(3));
    assert!(String::from_utf8_lossy(&denied.stdout).contains("plane daily ceiling"));
}

#[test]
fn workflow_metered_parent_key_is_refused_at_acceptance() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let doc = write_doc(
        root,
        "metered-key.toml",
        &DOC.replace(
            "harness = \"opencode\"",
            "harness = \"command\"\nsecrets = [\"OPENROUTER_API_KEY\"]",
        ),
    );
    bb_ok(root, &["workflow", "create", doc.to_str().unwrap()]);
    bb_ok(root, &["workflow", "activate", "pr-review"]);
    let denied = bb(root, &["workflow", "accept", "pr-review", "--json"]);
    assert_eq!(denied.status.code(), Some(3));
    assert!(String::from_utf8_lossy(&denied.stdout).contains("cannot report cost_usd"));
}

#[test]
fn queued_workflow_reservation_stays_pinned_after_revision_policy_change() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let first_doc = write_doc(
        root,
        "pinned-10.toml",
        &DOC.replace(
            "max_runs_per_day = 20",
            "max_runs_per_day = 20\nmax_cost_per_day_usd = 10.0\nestimated_cost_per_run_usd = 10.0",
        ),
    );
    bb_ok(root, &["workflow", "create", first_doc.to_str().unwrap()]);
    bb_ok(root, &["workflow", "activate", "pr-review"]);
    let first = bb_json(root, &["workflow", "accept", "pr-review", "--json"]);
    assert_eq!(first["run"]["estimated_cost_usd"], 10.0);
    let second_text = DOC.replace(
        "max_runs_per_day = 20",
        "max_runs_per_day = 20\nmax_cost_per_day_usd = 10.0\nestimated_cost_per_run_usd = 1.0",
    );
    let second_doc = write_doc(root, "pinned-1.toml", &second_text);
    bb_ok(
        root,
        &[
            "workflow",
            "revise",
            "pr-review",
            second_doc.to_str().unwrap(),
        ],
    );
    bb_ok(
        root,
        &["workflow", "activate", "pr-review", "--revision", "2"],
    );
    let denied = bb(root, &["workflow", "accept", "pr-review", "--json"]);
    assert_eq!(denied.status.code(), Some(3));
    assert!(String::from_utf8_lossy(&denied.stdout).contains("workflow daily ceiling"));
}

#[test]
fn workflow_realized_spend_uses_step_occurrence_date() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let stub = root.join("dated.sh");
    fs::write(&stub, "#!/bin/sh\nprintf '%s\\n' '{\\\"part\\\":{\\\"type\\\":\\\"step-finish\\\",\\\"cost\\\":0.25,\\\"tokens\\\":{\\\"input\\\":1,\\\"output\\\":1}}}'\nprintf '%s\\n' '{\\\"part\\\":{\\\"type\\\":\\\"text\\\",\\\"text\\\":\\\"done\\\"}}'\n").unwrap();
    let mut perms = fs::metadata(&stub).unwrap().permissions();
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        perms.set_mode(0o755);
        fs::set_permissions(&stub, perms).unwrap();
    }
    let doc_text = DOC.replace("bin =", &format!("bin = \\\"{}\\\"\n#", stub.display()));
    let doc = write_doc(root, "dated.toml", &doc_text);
    bb_ok(root, &["workflow", "create", doc.to_str().unwrap()]);
    bb_ok(root, &["workflow", "activate", "pr-review"]);
    let accepted = bb_json(root, &["workflow", "accept", "pr-review", "--json"]);
    let run_id = accepted["run"]["id"].as_str().unwrap();
    let queued = bb_json(root, &["workflow", "spend", "pr-review", "--json"]);
    assert_eq!(queued["spend_today_usd"], 0.0);
    assert_eq!(queued["reserved_usd"], 1.0);
    bb_ok(root, &["workflow", "execute", run_id, "--json"]);
    let conn = rusqlite::Connection::open(root.join(".bb/plane.db")).unwrap();
    conn.execute(
        "UPDATE workflow_step_runs SET started_at = '2000-01-01T00:00:00Z' WHERE run_id = ?1",
        [run_id],
    )
    .unwrap();
    let spend = bb_json(root, &["workflow", "spend", "pr-review", "--json"]);
    assert_eq!(spend["observed_usd"], 0.0);
}

#[test]
fn active_workflow_spend_counts_observed_cost_before_reservation() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let doc = write_doc(root, "active-spend.toml", DOC);
    bb_ok(root, &["workflow", "create", doc.to_str().unwrap()]);
    bb_ok(root, &["workflow", "activate", "pr-review"]);
    let accepted = bb_json(root, &["workflow", "accept", "pr-review", "--json"]);
    let run_id = accepted["run"]["id"].as_str().unwrap();
    let conn = rusqlite::Connection::open(root.join(".bb/plane.db")).unwrap();
    conn.execute(
        "UPDATE workflow_run_status SET state = 'running' WHERE run_id = ?1",
        [run_id],
    )
    .unwrap();
    conn.execute(
        "INSERT INTO workflow_step_runs
         (id, run_id, step, attempt, agent_json, goal, state, cost_usd,
          authority_json, started_at)
         VALUES ('wfs-active', ?1, 'review', 1, '{}', 'review', 'running',
                 1.5, '{}', datetime('now'))",
        [run_id],
    )
    .unwrap();
    drop(conn);
    let spend = bb_json(root, &["workflow", "spend", "pr-review", "--json"]);
    assert_eq!(spend["observed_usd"], 1.5, "{spend}");
    assert_eq!(spend["reserved_usd"], 0.0, "{spend}");
    assert_eq!(spend["spend_today_usd"], 1.5, "{spend}");
}

#[test]
fn terminal_mixed_metered_and_unpriced_attempts_keep_estimate_reservation() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let doc = write_doc(root, "mixed-spend.toml", DOC);
    bb_ok(root, &["workflow", "create", doc.to_str().unwrap()]);
    bb_ok(root, &["workflow", "activate", "pr-review"]);
    let accepted = bb_json(root, &["workflow", "accept", "pr-review", "--json"]);
    let run_id = accepted["run"]["id"].as_str().unwrap();
    let conn = rusqlite::Connection::open(root.join(".bb/plane.db")).unwrap();
    conn.execute(
        "UPDATE workflow_run_status
         SET state = 'succeeded', cost_usd = 0.10, updated_at = datetime('now')
         WHERE run_id = ?1",
        [run_id],
    )
    .unwrap();
    conn.execute(
        "INSERT INTO workflow_step_runs
         (id, run_id, step, attempt, agent_json, goal, state, cost_usd,
          authority_json, started_at)
         VALUES ('wfs-priced', ?1, 'review', 1, '{}', 'review', 'succeeded',
                 0.10, '{}', datetime('now')),
                ('wfs-unpriced', ?1, 'review', 2, '{}', 'review', 'succeeded',
                 NULL, '{}', datetime('now'))",
        [run_id],
    )
    .unwrap();
    drop(conn);

    let spend = bb_json(root, &["workflow", "spend", "pr-review", "--json"]);
    assert_eq!(spend["observed_usd"], 0.10, "{spend}");
    assert_eq!(spend["estimated_usd"], 0.90, "{spend}");
    assert_eq!(spend["spend_today_usd"], 1.0, "{spend}");

    let conn = rusqlite::Connection::open(root.join(".bb/plane.db")).unwrap();
    conn.execute(
        "UPDATE workflow_step_runs SET cost_usd = 0.20 WHERE id = 'wfs-unpriced'",
        [],
    )
    .unwrap();
    conn.execute(
        "INSERT INTO workflow_child_agents
         (step_run_id, name, authority_json, inherited, cost_usd, recorded_at)
         VALUES ('wfs-priced', 'unpriced-child', '{}', 1, NULL, datetime('now'))",
        [],
    )
    .unwrap();
    drop(conn);
    let child_spend = bb_json(root, &["workflow", "spend", "pr-review", "--json"]);
    assert!(
        (child_spend["observed_usd"].as_f64().unwrap() - 0.30).abs() < 1e-9,
        "{child_spend}"
    );
    assert_eq!(child_spend["estimated_usd"], 0.70, "{child_spend}");
    assert_eq!(child_spend["spend_today_usd"], 1.0, "{child_spend}");
}

// --- criterion 5: migration fixture -----------------------------------------

#[test]
fn migration_fixture_imports_demo_plane_task_and_reads_back_active_state() {
    // The fixture is the checked-in demo plane: a complete current
    // file-defined workload (task.toml + card + bound agent). Copy it so the
    // ledger lands in a tempdir, not the repo.
    let fixture = Path::new(env!("CARGO_MANIFEST_DIR")).join("examples/demo-plane");
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    copy_dir(&fixture, root);

    let imported = bb_json(
        root,
        &["workflow", "import-task", "demo", "--activate", "--json"],
    );
    assert_eq!(imported["outcome"], "created");
    assert_eq!(imported["workflow"]["state"], "active");
    assert_eq!(imported["workflow"]["active_revision"], 1);

    // active-state readback: the database revision carries the task's real
    // pinned configuration (agent snapshot, triggers, budgets, card goal)
    let view = bb_json(root, &["workflow", "show", "demo", "--json"]);
    let doc = &view["active_document"];
    assert_eq!(doc["name"], "demo");
    assert!(doc["goal"].as_str().unwrap().contains("BB-DEMO-OK"));
    let agent = &doc["step"][0]["agent"];
    assert_eq!(agent["name"], "opencode");
    assert_eq!(agent["version"], 1);
    assert_eq!(agent["harness"], "opencode");
    assert_eq!(agent["model"], "deepseek/deepseek-v4-flash");
    let kinds: Vec<&str> = doc["trigger"]
        .as_array()
        .unwrap()
        .iter()
        .map(|t| t["kind"].as_str().unwrap())
        .collect();
    assert_eq!(kinds, ["manual", "cron", "webhook"]);
    assert_eq!(doc["trigger"][2]["route"], "demo");
    assert_eq!(doc["policies"]["max_runs_per_day"], 24);

    // the imported workflow is immediately live: accept pins revision 1
    let accepted = bb_json(root, &["workflow", "accept", "demo", "--json"]);
    assert_eq!(accepted["run"]["revision"], 1);

    // re-import is a no-op: the file stayed a migration source, the database
    // stayed the authority
    let again = bb_json(root, &["workflow", "import-task", "demo", "--json"]);
    assert_eq!(again["outcome"], "unchanged");
}

fn copy_dir(from: &Path, to: &Path) {
    for entry in fs::read_dir(from).unwrap() {
        let entry = entry.unwrap();
        let target = to.join(entry.file_name());
        if entry.file_type().unwrap().is_dir() {
            fs::create_dir_all(&target).unwrap();
            copy_dir(&entry.path(), &target);
        } else {
            fs::copy(entry.path(), &target).unwrap();
        }
    }
}

// --- proof plan: revision concurrency drill ----------------------------------

#[test]
fn concurrent_revisions_activations_and_accepts_never_lose_or_fork_history() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let db = root.join(".bb/plane.db");
    {
        let ledger = Ledger::open(&db).unwrap();
        let doc = WorkflowDoc::from_toml(DOC).unwrap();
        ledger.create_workflow(&doc, "cli", None).unwrap();
        ledger.activate_workflow("pr-review", None).unwrap();
    }

    // 4 writers x 5 revisions each, racing 2 activators and 2 acceptors on
    // their own connections. BEGIN IMMEDIATE + the (workflow_id, revision)
    // primary key must serialize them: dense unique revision history, every
    // accepted run pinned to a revision that was genuinely active.
    const WRITERS: usize = 4;
    const PER_WRITER: usize = 5;
    let mut handles = Vec::new();
    for writer in 0..WRITERS {
        let db = db.clone();
        handles.push(std::thread::spawn(move || {
            let ledger = Ledger::open(&db).unwrap();
            for n in 0..PER_WRITER {
                let text = DOC.replace(
                    "Review every",
                    &format!("Review (writer {writer} pass {n}) every"),
                );
                let doc = WorkflowDoc::from_toml(&text).unwrap();
                ledger
                    .revise_workflow("pr-review", &doc, "cli", None)
                    .unwrap();
            }
        }));
    }
    for _ in 0..2 {
        let db = db.clone();
        handles.push(std::thread::spawn(move || {
            let ledger = Ledger::open(&db).unwrap();
            for _ in 0..5 {
                ledger.activate_workflow("pr-review", None).unwrap();
            }
        }));
    }
    let accept_handles: Vec<_> = (0..2)
        .map(|_| {
            let db = db.clone();
            std::thread::spawn(move || {
                let ledger = Ledger::open(&db).unwrap();
                let plane =
                    bitterblossom::spec::Plane::load(db.parent().unwrap().parent().unwrap())
                        .unwrap();
                let mut accepted = Vec::new();
                for _ in 0..5 {
                    match ledger
                        .accept_workflow_run(&plane, "pr-review", "test", None, None)
                        .unwrap()
                    {
                        AcceptOutcome::Accepted { run } => accepted.push((run.id, run.revision)),
                        AcceptOutcome::Duplicate { .. } => unreachable!("no dedupe key supplied"),
                        AcceptOutcome::Denied { .. } => {
                            unreachable!("daily ceiling is not configured")
                        }
                        AcceptOutcome::Suppressed { .. } => unreachable!("never paused"),
                    }
                }
                accepted
            })
        })
        .collect();
    for handle in handles {
        handle.join().unwrap();
    }
    let mut accepted = Vec::new();
    for handle in accept_handles {
        accepted.extend(handle.join().unwrap());
    }

    let ledger = Ledger::open(&db).unwrap();
    let revisions = ledger.workflow_revisions("pr-review").unwrap();
    let expected = 1 + WRITERS as i64 * PER_WRITER as i64;
    assert_eq!(revisions.len() as i64, expected, "no lost revision");
    let numbers: Vec<i64> = revisions.iter().map(|r| r.revision).collect();
    assert_eq!(
        numbers,
        (1..=expected).collect::<Vec<_>>(),
        "revision history is dense and monotonic"
    );
    // every stored document is intact canonical JSON
    for revision in &revisions {
        WorkflowDoc::from_canonical_json(&revision.document).unwrap();
    }
    // every accepted run pinned an existing revision and reads back exactly
    assert_eq!(accepted.len(), 10);
    for (run_id, revision) in accepted {
        assert!(
            (1..=expected).contains(&revision),
            "run pinned unknown revision {revision}"
        );
        let run = ledger.workflow_run(&run_id).unwrap();
        assert_eq!(run.revision, revision, "pin is immutable");
    }
}

#[test]
fn blind_run_cap_is_pinned_per_revision_without_daily_revaluation() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    write_plane(root);
    let first_text = DOC.replace(
        "max_runs_per_day = 20",
        "max_runs_per_day = 20\nmax_cost_per_run_usd = 3.0",
    );
    let first_doc = write_doc(root, "blind-rev1.toml", &first_text);
    bb_ok(root, &["workflow", "create", first_doc.to_str().unwrap()]);
    bb_ok(root, &["workflow", "activate", "pr-review"]);
    let first = bb_json(root, &["workflow", "accept", "pr-review", "--json"]);
    assert_eq!(first["run"]["estimated_cost_usd"], 3.0);
    let second_text = first_text
        .replace(
            "goal = \"Review every reviewable pull-request head and post one formal review.\"",
            "goal = \"Review each pull request with the revised blind reservation.\"",
        )
        .replace("max_cost_per_run_usd = 3.0", "max_cost_per_run_usd = 7.0");
    let second_doc = write_doc(root, "blind-rev2.toml", &second_text);
    bb_ok(
        root,
        &[
            "workflow",
            "revise",
            "pr-review",
            second_doc.to_str().unwrap(),
        ],
    );
    bb_ok(
        root,
        &["workflow", "activate", "pr-review", "--revision", "2"],
    );
    let second = bb_json(root, &["workflow", "accept", "pr-review", "--json"]);
    assert_eq!(second["run"]["estimated_cost_usd"], 7.0);
    let runs = bb_json(root, &["workflow", "runs", "pr-review", "--json"]);
    assert_eq!(runs[0]["estimated_cost_usd"], 3.0, "{runs}");
    assert_eq!(runs[1]["estimated_cost_usd"], 7.0, "{runs}");
}

#[test]
fn recheck_stopped_run_without_attempt_releases_capacity() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    fs::write(
        root.join("plane.toml"),
        "dev = true\n[budget]\nmax_cost_per_day_usd = 1.0\n",
    )
    .unwrap();
    let text = DOC.replace(
        "max_runs_per_day = 20",
        "max_runs_per_day = 20\nmax_cost_per_run_usd = 1.0\nmax_cost_per_day_usd = 10.0",
    );
    let doc = write_doc(root, "release.toml", &text);
    bb_ok(root, &["workflow", "create", doc.to_str().unwrap()]);
    bb_ok(root, &["workflow", "activate", "pr-review"]);
    let first = bb_json(root, &["workflow", "accept", "pr-review", "--json"]);
    let first_id = first["run"]["id"].as_str().unwrap();
    fs::write(
        root.join("plane.toml"),
        "dev = true\n[budget]\nmax_cost_per_day_usd = 0.5\n",
    )
    .unwrap();
    let stopped = bb_json(root, &["workflow", "execute", first_id, "--json"]);
    assert_eq!(stopped["status"]["state"], "stopped", "{stopped}");
    assert!(stopped["steps"].as_array().unwrap().is_empty(), "{stopped}");
    let spend = bb_json(root, &["workflow", "spend", "pr-review", "--json"]);
    assert_eq!(spend["reserved_usd"], 0.0, "{spend}");
    assert_eq!(spend["estimated_usd"], 0.0, "{spend}");
    fs::write(
        root.join("plane.toml"),
        "dev = true\n[budget]\nmax_cost_per_day_usd = 1.0\n",
    )
    .unwrap();
    let second = bb_json(root, &["workflow", "accept", "pr-review", "--json"]);
    assert_eq!(second["disposition"], "accepted", "{second}");
}

#[test]
fn legacy_workflow_runs_backfill_pinned_estimate_and_status() {
    let dir = tempfile::tempdir().unwrap();
    let db = dir.path().join("legacy.db");
    let document: WorkflowDoc = toml::from_str(&DOC.replace(
        "max_runs_per_day = 20",
        "max_runs_per_day = 20\nmax_cost_per_run_usd = 4.0",
    ))
    .unwrap();
    let canonical = serde_json::to_string(&document).unwrap();
    {
        let conn = rusqlite::Connection::open(&db).unwrap();
        conn.execute_batch(
            "CREATE TABLE workflows (id TEXT PRIMARY KEY, name TEXT NOT NULL, state TEXT NOT NULL, active_revision INTEGER, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
             CREATE TABLE workflow_revisions (workflow_id TEXT NOT NULL, revision INTEGER NOT NULL, document TEXT NOT NULL, source TEXT NOT NULL, note TEXT, created_at TEXT NOT NULL, PRIMARY KEY(workflow_id, revision));
             CREATE TABLE workflow_runs (id TEXT PRIMARY KEY, workflow_id TEXT NOT NULL, revision INTEGER NOT NULL, trigger_kind TEXT NOT NULL, payload TEXT, dedupe_key TEXT, created_at TEXT NOT NULL);",
        ).unwrap();
        conn.execute("INSERT INTO workflows VALUES ('wf-legacy','pr-review','active',1,'2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')", []).unwrap();
        conn.execute("INSERT INTO workflow_revisions VALUES ('wf-legacy',1,?1,'import',NULL,'2026-01-01T00:00:00Z')", [&canonical]).unwrap();
        conn.execute("INSERT INTO workflow_runs VALUES ('wfr-legacy','wf-legacy',1,'test',NULL,NULL,'2026-01-01T00:00:00Z')", []).unwrap();
    }
    let ledger = Ledger::open(&db).unwrap();
    let run = ledger.workflow_run("wfr-legacy").unwrap();
    assert_eq!(run.estimated_cost_usd, 4.0);
    assert_eq!(
        ledger
            .workflow_run_status("wfr-legacy")
            .unwrap()
            .unwrap()
            .state,
        "queued"
    );
}

#[test]
fn failed_legacy_backfill_rolls_back_schema_changes() {
    let dir = tempfile::tempdir().unwrap();
    let db = dir.path().join("invalid-legacy.db");
    {
        let conn = rusqlite::Connection::open(&db).unwrap();
        conn.execute_batch(
            "CREATE TABLE workflows (id TEXT PRIMARY KEY, name TEXT NOT NULL, state TEXT NOT NULL, active_revision INTEGER, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
             CREATE TABLE workflow_revisions (workflow_id TEXT NOT NULL, revision INTEGER NOT NULL, document TEXT NOT NULL, source TEXT NOT NULL, note TEXT, created_at TEXT NOT NULL, PRIMARY KEY(workflow_id, revision));
             CREATE TABLE workflow_runs (id TEXT PRIMARY KEY, workflow_id TEXT NOT NULL, revision INTEGER NOT NULL, trigger_kind TEXT NOT NULL, payload TEXT, dedupe_key TEXT, created_at TEXT NOT NULL);
             INSERT INTO workflows VALUES ('wf-invalid','invalid','active',1,'2026-01-01T00:00:00Z','2026-01-01T00:00:00Z');
             INSERT INTO workflow_revisions VALUES ('wf-invalid',1,'not-json','import',NULL,'2026-01-01T00:00:00Z');
             INSERT INTO workflow_runs VALUES ('wfr-invalid','wf-invalid',1,'test',NULL,NULL,'2026-01-01T00:00:00Z');",
        )
        .unwrap();
    }

    assert!(Ledger::open(&db).is_err());
    let conn = rusqlite::Connection::open(&db).unwrap();
    let columns = conn
        .prepare("PRAGMA table_info(workflow_runs)")
        .unwrap()
        .query_map([], |row| row.get::<_, String>(1))
        .unwrap()
        .collect::<rusqlite::Result<Vec<_>>>()
        .unwrap();
    assert!(!columns.iter().any(|name| name == "estimated_cost_usd"));
    let status_table: i64 = conn
        .query_row(
            "SELECT COUNT(*) FROM sqlite_master
             WHERE type = 'table' AND name = 'workflow_run_status'",
            [],
            |row| row.get(0),
        )
        .unwrap();
    assert_eq!(status_table, 0);
    let version: i64 = conn
        .pragma_query_value(None, "user_version", |row| row.get(0))
        .unwrap();
    assert_eq!(version, 0);
}
