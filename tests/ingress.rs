//! Ingress contract: HMAC validation, trigger-defined dedupe, cron fire
//! idempotency. No socket needed — the HTTP layer is plumbing.

use std::fs;
use std::path::Path;

use bitterblossom::ingress::{
    derive_dedupe_key, due_fires, handle_webhook, ingest_cron_fire, parse_schedule, sign_hmac,
};
use bitterblossom::ledger::Ledger;
use bitterblossom::spec::Plane;
use chrono::{TimeZone, Utc};

const SECRET_ENV: &str = "BB_TEST_HOOK_SECRET";

fn make_plane(root: &Path) -> Plane {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
    fs::write(
        root.join("agents/a.toml"),
        "harness = \"pi\"\nmodel = \"m\"\n",
    )
    .unwrap();
    fs::write(root.join("tasks/demo/card.md"), "card\n").unwrap();
    fs::write(
        root.join("tasks/demo/task.toml"),
        format!(
            "agent = \"a\"\nsubstrate = \"local\"\n\n\
             [[trigger]]\nkind = \"webhook\"\nroute = \"demo\"\nsecret_env = \"{SECRET_ENV}\"\n\
             dedupe_key = \"header:X-GitHub-Delivery\"\n\n\
             [[trigger]]\nkind = \"cron\"\nschedule = \"0 */6 * * *\"\n"
        ),
    )
    .unwrap();
    Plane::load(root).unwrap()
}

fn headers(sig: &str, delivery: &str) -> Vec<(String, String)> {
    vec![
        ("X-Hub-Signature-256".into(), sig.into()),
        ("X-GitHub-Delivery".into(), delivery.into()),
    ]
}

#[test]
fn webhook_valid_hmac_creates_durable_run_before_ack() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    std::env::set_var(SECRET_ENV, "hunter2");

    let body = r#"{"action":"opened","number":7}"#;
    let sig = sign_hmac("hunter2", body.as_bytes());
    let resp = handle_webhook(&plane, &mut ledger, "demo", &headers(&sig, "d-1"), body).unwrap();
    assert_eq!(resp.status, 202, "{}", resp.body);

    let runs = ledger.list_runs(Some("demo"), None).unwrap();
    assert_eq!(runs.len(), 1);
    assert_eq!(runs[0].trigger_kind, "webhook");
    // Payload preserved for replay.
    assert_eq!(
        ledger.run_payload(&runs[0].id).unwrap().as_deref(),
        Some(body)
    );
}

#[test]
fn webhook_accepts_canary_x_signature_header() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    std::env::set_var(SECRET_ENV, "hunter2");

    let body = r#"{"event":"incident.opened","incident":{"service":"canary"}}"#;
    let sig = sign_hmac("hunter2", body.as_bytes());
    let headers = vec![
        ("X-Signature".to_string(), sig),
        ("X-GitHub-Delivery".to_string(), "d-canary".to_string()),
    ];
    let resp = handle_webhook(&plane, &mut ledger, "demo", &headers, body).unwrap();

    assert_eq!(resp.status, 202, "{}", resp.body);
    assert_eq!(ledger.list_runs(Some("demo"), None).unwrap().len(), 1);
}

#[test]
fn webhook_invalid_signature_rejected_with_no_row() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    std::env::set_var(SECRET_ENV, "hunter2");

    let body = r#"{"action":"opened"}"#;
    let bad = sign_hmac("wrong-secret", body.as_bytes());
    let resp = handle_webhook(&plane, &mut ledger, "demo", &headers(&bad, "d-1"), body).unwrap();
    assert_eq!(resp.status, 401);
    assert!(ledger.list_runs(Some("demo"), None).unwrap().is_empty());
    assert_eq!(ledger.ingress_event_count("demo").unwrap(), 0);

    // Missing signature entirely: same refusal.
    let resp = handle_webhook(&plane, &mut ledger, "demo", &[], body).unwrap();
    assert_eq!(resp.status, 401);
}

#[test]
fn webhook_redelivery_same_dedupe_key_records_event_no_second_run() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    std::env::set_var(SECRET_ENV, "hunter2");

    let body = r#"{"action":"opened","number":7}"#;
    let sig = sign_hmac("hunter2", body.as_bytes());
    let first = handle_webhook(&plane, &mut ledger, "demo", &headers(&sig, "dup"), body).unwrap();
    let second = handle_webhook(&plane, &mut ledger, "demo", &headers(&sig, "dup"), body).unwrap();
    assert_eq!(first.status, 202);
    assert_eq!(second.status, 202);
    assert!(
        second.body.contains("\"duplicate\":true"),
        "{}",
        second.body
    );
    assert_eq!(ledger.list_runs(Some("demo"), None).unwrap().len(), 1);
    assert_eq!(ledger.ingress_event_count("demo").unwrap(), 2);
}

#[test]
fn webhook_unknown_route_404s_with_no_row() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let resp = handle_webhook(&plane, &mut ledger, "nope", &[], "{}").unwrap();
    assert_eq!(resp.status, 404);
}

#[test]
fn dedupe_key_derivations() {
    let headers = vec![("X-GitHub-Delivery".to_string(), "abc-123".to_string())];
    let body = r#"{"pull_request":{"head":{"sha":"deadbeef"}}}"#;
    assert_eq!(
        derive_dedupe_key("header:X-GitHub-Delivery", &headers, body).unwrap(),
        "abc-123"
    );
    assert_eq!(
        derive_dedupe_key("json:/pull_request/head/sha", &headers, body).unwrap(),
        "deadbeef"
    );
    assert!(derive_dedupe_key("header:Missing", &headers, body).is_err());
    assert!(derive_dedupe_key("bogus", &headers, body).is_err());
}

#[test]
fn cron_due_fires_and_scheduled_timestamp_dedupes() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let schedule = parse_schedule("*/15 * * * *").unwrap();
    let after = Utc.with_ymd_and_hms(2026, 6, 10, 12, 0, 0).unwrap();
    let until = Utc.with_ymd_and_hms(2026, 6, 10, 12, 31, 0).unwrap();
    let fires = due_fires(&schedule, after, until);
    assert_eq!(fires.len(), 2, "{fires:?}"); // 12:15, 12:30

    let a = ingest_cron_fire(&mut ledger, "demo", fires[0]).unwrap();
    let b = ingest_cron_fire(&mut ledger, "demo", fires[0]).unwrap();
    assert!(!a.duplicate);
    assert!(b.duplicate, "same scheduled timestamp must not double-fire");
    assert_eq!(a.run_id, b.run_id);
    assert_eq!(ledger.list_runs(Some("demo"), None).unwrap().len(), 1);
}

#[test]
fn five_field_cron_schedules_are_accepted_and_bad_ones_fail_at_load() {
    parse_schedule("0 */6 * * *").unwrap();
    parse_schedule("0 0 */6 * * *").unwrap();
    assert!(parse_schedule("not a schedule").is_err());

    // Plane::load rejects a bad schedule up front.
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/bad")).unwrap();
    fs::write(
        root.join("agents/a.toml"),
        "harness = \"pi\"\nmodel = \"m\"\n",
    )
    .unwrap();
    fs::write(root.join("tasks/bad/card.md"), "card\n").unwrap();
    fs::write(
        root.join("tasks/bad/task.toml"),
        "agent = \"a\"\n\n[[trigger]]\nkind = \"cron\"\nschedule = \"garbage\"\n",
    )
    .unwrap();
    assert!(Plane::load(root).is_err());
}

#[test]
fn webhook_filters_reject_out_of_scope_deliveries_without_a_run() {
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/rev")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
    fs::write(
        root.join("agents/a.toml"),
        "harness = \"pi\"\nmodel = \"m\"\n",
    )
    .unwrap();
    fs::write(root.join("tasks/rev/card.md"), "card\n").unwrap();
    fs::write(
        root.join("tasks/rev/task.toml"),
        "agent = \"a\"\nsubstrate = \"local\"\n\n\
         [[trigger]]\nkind = \"webhook\"\nroute = \"rev\"\nsecret_env = \"BB_TEST_FILTER_SECRET\"\n\
         [[trigger.filter]]\npointer = \"/repository/full_name\"\nany_of = [\"good/repo\"]\n\
         [[trigger.filter]]\npointer = \"/action\"\nany_of = [\"opened\", \"synchronize\"]\n\
         [[trigger.filter]]\npointer = \"/pull_request/draft\"\nequals = false\n\
         [[trigger.filter]]\npointer = \"/pull_request/additions\"\nmax = 4000\n",
    )
    .unwrap();
    let plane = Plane::load(root).unwrap();
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    std::env::set_var("BB_TEST_FILTER_SECRET", "s3cret");

    let deliver = |ledger: &mut Ledger, body: &str| {
        let sig = sign_hmac("s3cret", body.as_bytes());
        handle_webhook(&plane, ledger, "rev", &headers(&sig, "d1"), body).unwrap()
    };

    let in_scope = r#"{"action":"opened","repository":{"full_name":"good/repo"},"pull_request":{"draft":false,"additions":12,"head":{"sha":"a1"}}}"#;
    assert_eq!(deliver(&mut ledger, in_scope).status, 202);

    // Wrong repo, draft PR, ignored action, oversized diff, missing field:
    // all acknowledged with 200 and no run row.
    let cases = [
        r#"{"action":"opened","repository":{"full_name":"evil/repo"},"pull_request":{"draft":false,"additions":1}}"#,
        r#"{"action":"opened","repository":{"full_name":"good/repo"},"pull_request":{"draft":true,"additions":1}}"#,
        r#"{"action":"labeled","repository":{"full_name":"good/repo"},"pull_request":{"draft":false,"additions":1}}"#,
        r#"{"action":"opened","repository":{"full_name":"good/repo"},"pull_request":{"draft":false,"additions":99999}}"#,
        r#"{"action":"opened","repository":{"full_name":"good/repo"}}"#,
    ];
    for body in cases {
        let resp = deliver(&mut ledger, body);
        assert_eq!(resp.status, 200, "{body} -> {}", resp.body);
        assert!(resp.body.contains("filtered"), "{}", resp.body);
    }
    assert_eq!(ledger.list_runs(None, None).unwrap().len(), 1);
}
