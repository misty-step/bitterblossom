//! Ingress contract: HMAC validation, trigger-defined dedupe, cron fire
//! idempotency. No socket needed — the HTTP layer is plumbing.

use std::fs;
use std::path::Path;

use bitterblossom::ingress::{
    cron_catchup, cron_catchup_guarded, derive_dedupe_key, due_fires, handle_webhook,
    ingest_cron_fire, parse_schedule, sign_hmac,
};
use bitterblossom::ledger::Ledger;
use bitterblossom::spec::Plane;
use chrono::{TimeZone, Utc};
use rusqlite::params;

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

fn make_canary_triage_plane(root: &Path) -> Plane {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/canary-triage")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
    fs::write(
        root.join("agents/a.toml"),
        "harness = \"pi\"\nmodel = \"m\"\nauth = \"api\"\n",
    )
    .unwrap();
    fs::write(root.join("tasks/canary-triage/card.md"), "card\n").unwrap();
    fs::write(
        root.join("tasks/canary-triage/task.toml"),
        "agent = \"a\"\nsubstrate = \"local\"\n\n\
         [[trigger]]\nkind = \"webhook\"\nroute = \"canary-triage\"\nsecret_env = \"BB_TEST_CANARY_SECRET\"\n\
         dedupe_key = \"header:X-Delivery-Id\"\n\n\
         [[trigger.filter]]\npointer = \"/event\"\nany_of = [\"incident.opened\", \"incident.updated\"]\n\
         [[trigger.filter]]\npointer = \"/incident/service\"\nany_of = [\"canary\"]\n",
    )
    .unwrap();
    Plane::load(root).unwrap()
}

fn canary_headers(
    secret: &str,
    timestamp: &str,
    delivery: &str,
    body: &str,
) -> Vec<(String, String)> {
    let signature = sign_hmac(secret, format!("{timestamp}.{delivery}.{body}").as_bytes());
    vec![
        ("X-Canary-Signature".to_string(), signature),
        ("X-Timestamp".to_string(), timestamp.to_string()),
        ("X-Delivery-Id".to_string(), delivery.to_string()),
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
fn webhook_body_cap_rejects_before_ledger_growth() {
    let dir = tempfile::tempdir().unwrap();
    let mut plane = make_plane(dir.path());
    plane.spec.ingress.max_body_bytes = 8;
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    std::env::set_var(SECRET_ENV, "hunter2");

    let body = r#"{"too":"large"}"#;
    let sig = sign_hmac("hunter2", body.as_bytes());
    let resp = handle_webhook(&plane, &mut ledger, "demo", &headers(&sig, "d-1"), body).unwrap();
    assert_eq!(resp.status, 413, "{}", resp.body);
    assert!(ledger.list_runs(Some("demo"), None).unwrap().is_empty());
    assert_eq!(ledger.ingress_event_count("demo").unwrap(), 0);

    let counts = ledger.guard_event_counts().unwrap();
    let oversized = counts
        .iter()
        .find(|c| c.kind == "ingress_oversized")
        .unwrap();
    assert_eq!(oversized.total, 1);
    let events = ledger.list_guard_events(10).unwrap();
    assert_eq!(events[0].task.as_deref(), Some("demo"));
    assert!(events[0].detail.as_deref().unwrap().contains("max=8"));
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
fn webhook_attention_debt_brake_refuses_without_ingesting() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    ledger
        .record_dead_letter("old-run", "demo", Some("{}"), "operator debt")
        .unwrap();
    std::env::set_var(SECRET_ENV, "hunter2");

    let body = r#"{"action":"opened","number":7}"#;
    let sig = sign_hmac("hunter2", body.as_bytes());
    let resp = handle_webhook(&plane, &mut ledger, "demo", &headers(&sig, "d-1"), body).unwrap();
    assert_eq!(resp.status, 429, "{}", resp.body);
    assert!(resp.body.contains("attention debt brake"), "{}", resp.body);
    assert!(ledger.list_runs(Some("demo"), None).unwrap().is_empty());
    assert_eq!(ledger.ingress_event_count("demo").unwrap(), 0);

    let events = ledger.list_guard_events(10).unwrap();
    assert_eq!(events[0].kind, "attention_debt_brake");
    assert_eq!(events[0].task.as_deref(), Some("demo"));
    assert!(events[0].detail.as_deref().unwrap().contains("open_dlq=1"));
}

#[test]
fn webhook_accepts_canary_timestamped_signature_and_delivery_id() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_canary_triage_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    std::env::set_var("BB_TEST_CANARY_SECRET", "s3cret");

    let body = include_str!("fixtures/contracts/canary.incident_event.v1.valid.json");
    let timestamp = "2026-07-01T17:00:00Z";
    let delivery = "DLV-canary-live";
    let headers = canary_headers("s3cret", timestamp, delivery, body);
    let resp = handle_webhook(&plane, &mut ledger, "canary-triage", &headers, body).unwrap();
    assert_eq!(resp.status, 202, "{}", resp.body);
    assert!(!resp.body.contains("filtered"), "{}", resp.body);

    let runs = ledger.list_runs(Some("canary-triage"), None).unwrap();
    assert_eq!(runs.len(), 1);
    assert_eq!(
        runs[0].idempotency_key.as_deref(),
        Some("wh:canary-triage:DLV-canary-live")
    );
    assert_eq!(
        ledger.run_payload(&runs[0].id).unwrap().as_deref(),
        Some(body)
    );
    let duplicate = handle_webhook(&plane, &mut ledger, "canary-triage", &headers, body).unwrap();
    assert_eq!(duplicate.status, 202);
    assert!(duplicate.body.contains("\"duplicate\":true"));
    assert_eq!(
        ledger.list_runs(Some("canary-triage"), None).unwrap().len(),
        1
    );
}

#[test]
fn webhook_filters_subject_only_canary_payload_without_a_run() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_canary_triage_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    std::env::set_var("BB_TEST_CANARY_SECRET", "s3cret");

    let body = r#"{"schema_version":"canary.incident_event.v1","event":"incident.opened","subject":{"type":"incident","id":"INC-live","service":"canary"}}"#;
    let headers = canary_headers("s3cret", "2026-07-01T17:00:00Z", "DLV-subject-only", body);
    let resp = handle_webhook(&plane, &mut ledger, "canary-triage", &headers, body).unwrap();

    assert_eq!(resp.status, 200, "{}", resp.body);
    assert!(resp.body.contains("filtered"), "{}", resp.body);
    assert!(resp.body.contains("/incident/service"), "{}", resp.body);
    assert!(ledger
        .list_runs(Some("canary-triage"), None)
        .unwrap()
        .is_empty());
    assert_eq!(ledger.ingress_event_count("canary-triage").unwrap(), 0);
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
fn cron_catchup_collapses_old_fires_and_records_count() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let schedule = parse_schedule("* * * * *").unwrap();
    let after = Utc.with_ymd_and_hms(2026, 6, 10, 12, 0, 0).unwrap();
    let until = Utc.with_ymd_and_hms(2026, 6, 10, 12, 6, 0).unwrap();
    let fires = due_fires(&schedule, after, until);
    assert!(fires.len() > 2, "{fires:?}");

    let outcome = cron_catchup(&mut ledger, "demo", &schedule, after, until, 2).unwrap();
    assert_eq!(outcome.ingested, 2);
    assert_eq!(outcome.duplicates, 0);
    assert_eq!(outcome.skipped, fires.len() - 2);
    assert_eq!(ledger.ingress_event_count("demo").unwrap(), 2);

    let collapsed = ledger
        .guard_event_counts()
        .unwrap()
        .into_iter()
        .find(|c| c.kind == "cron_collapse")
        .unwrap();
    assert_eq!(collapsed.total, outcome.skipped as i64);
}

#[test]
fn guarded_cron_attention_debt_brake_refuses_due_fires() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    ledger
        .record_dead_letter("old-run", "demo", Some("{}"), "operator debt")
        .unwrap();
    let schedule = parse_schedule("* * * * *").unwrap();
    let after = Utc.with_ymd_and_hms(2026, 6, 10, 12, 0, 0).unwrap();
    let until = Utc.with_ymd_and_hms(2026, 6, 10, 12, 6, 0).unwrap();

    let outcome =
        cron_catchup_guarded(&plane, &mut ledger, "demo", &schedule, after, until, 2).unwrap();

    assert_eq!(outcome.ingested, 0);
    assert_eq!(outcome.duplicates, 0);
    assert_eq!(outcome.skipped, 6);
    assert_eq!(ledger.ingress_event_count("demo").unwrap(), 0);
    let events = ledger.list_guard_events(10).unwrap();
    assert_eq!(events[0].kind, "attention_debt_brake");
    assert_eq!(events[0].task.as_deref(), Some("demo"));
    assert!(events[0].detail.as_deref().unwrap().contains("source=cron"));
}

#[test]
fn cron_catchup_records_collapse_only_after_ingest_success() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    rusqlite::Connection::open(plane.db_path())
        .unwrap()
        .execute_batch(
            "CREATE TRIGGER fail_second_ingress
             BEFORE INSERT ON ingress_events
             WHEN (SELECT COUNT(*) FROM ingress_events) >= 1
             BEGIN
               SELECT RAISE(FAIL, 'simulated ingress failure');
             END;",
        )
        .unwrap();

    let schedule = parse_schedule("* * * * *").unwrap();
    let after = Utc.with_ymd_and_hms(2026, 6, 10, 12, 0, 0).unwrap();
    let until = Utc.with_ymd_and_hms(2026, 6, 10, 12, 6, 0).unwrap();

    let err = cron_catchup(&mut ledger, "demo", &schedule, after, until, 2).unwrap_err();
    assert!(format!("{err:#}").contains("simulated ingress failure"));
    assert_eq!(ledger.ingress_event_count("demo").unwrap(), 1);
    assert!(ledger
        .guard_event_counts()
        .unwrap()
        .into_iter()
        .all(|c| c.kind != "cron_collapse"));
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
         [[trigger.filter]]\npointer = \"/sender/login\"\nnot_any_of = [\"dependabot[bot]\", \"renovate[bot]\"]\n\
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

    let in_scope = r#"{"action":"opened","sender":{"login":"human"},"repository":{"full_name":"good/repo"},"pull_request":{"draft":false,"additions":12,"head":{"sha":"a1"}}}"#;
    assert_eq!(deliver(&mut ledger, in_scope).status, 202);

    // Wrong repo, bot sender, draft PR, ignored action, oversized diff, missing field:
    // all acknowledged with 200 and no run row.
    let cases = [
        r#"{"action":"opened","sender":{"login":"human"},"repository":{"full_name":"evil/repo"},"pull_request":{"draft":false,"additions":1}}"#,
        r#"{"action":"opened","sender":{"login":"dependabot[bot]"},"repository":{"full_name":"good/repo"},"pull_request":{"draft":false,"additions":1}}"#,
        r#"{"action":"opened","sender":{"login":"human"},"repository":{"full_name":"good/repo"},"pull_request":{"draft":true,"additions":1}}"#,
        r#"{"action":"labeled","sender":{"login":"human"},"repository":{"full_name":"good/repo"},"pull_request":{"draft":false,"additions":1}}"#,
        r#"{"action":"opened","sender":{"login":"human"},"repository":{"full_name":"good/repo"},"pull_request":{"draft":false,"additions":99999}}"#,
        r#"{"action":"opened","sender":{"login":"human"},"repository":{"full_name":"good/repo"}}"#,
    ];
    for body in cases {
        let resp = deliver(&mut ledger, body);
        assert_eq!(resp.status, 200, "{body} -> {}", resp.body);
        assert!(resp.body.contains("filtered"), "{}", resp.body);
    }
    assert_eq!(ledger.list_runs(None, None).unwrap().len(), 1);
}

fn make_pr_storm_plane(root: &Path) -> Plane {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::write(
        root.join("plane.toml"),
        "dev = true\n[gate]\nrequired = [\"correctness\", \"security\"]\n",
    )
    .unwrap();
    fs::write(
        root.join("agents/a.toml"),
        "harness = \"pi\"\nmodel = \"m\"\nauth = \"api\"\n",
    )
    .unwrap();
    for task in ["review", "correctness", "security"] {
        fs::create_dir_all(root.join("tasks").join(task)).unwrap();
        fs::write(root.join(format!("tasks/{task}/card.md")), "card\n").unwrap();
    }
    fs::write(
        root.join("tasks/review/task.toml"),
        "agent = \"a\"\nsubstrate = \"local\"\n\n\
         [[trigger]]\nkind = \"webhook\"\nroute = \"review\"\nsecret_env = \"BB_TEST_PR_STORM_SECRET\"\n\
         dedupe_key = \"json:/pull_request/html_url|json:/pull_request/head/sha\"\n\
         [trigger.action]\nkind = \"submission_storm\"\n\
         change = \"json:/pull_request/html_url\"\n\
         rev = \"json:/pull_request/head/sha\"\n\
         repo = \"json:/repository/full_name\"\n\
         version = \"json:/pull_request/updated_at\"\n\
         [[trigger.filter]]\npointer = \"/repository/full_name\"\nany_of = [\"good/repo\"]\n\
         [[trigger.filter]]\npointer = \"/action\"\nany_of = [\"opened\", \"ready_for_review\", \"synchronize\"]\n\
         [[trigger.filter]]\npointer = \"/pull_request/draft\"\nequals = false\n",
    )
    .unwrap();
    for task in ["correctness", "security"] {
        fs::write(
            root.join(format!("tasks/{task}/task.toml")),
            format!(
                "agent = \"a\"\nsubstrate = \"local\"\nverdict = \"{task}\"\n[[trigger]]\nkind = \"manual\"\n"
            ),
        )
        .unwrap();
    }
    Plane::load(root).unwrap()
}

fn pr_body(rev: &str, additions: i64) -> String {
    pr_body_for(42, rev, additions)
}

fn pr_body_for(number: i64, rev: &str, additions: i64) -> String {
    pr_body_for_version(number, rev, additions, "2026-06-17T04:00:00Z")
}

fn pr_body_for_version(number: i64, rev: &str, additions: i64, updated_at: &str) -> String {
    format!(
        r#"{{
          "action":"synchronize",
          "number":{number},
          "repository":{{"full_name":"good/repo"}},
          "pull_request":{{
            "draft":false,
            "title":"Large but reviewable",
            "html_url":"https://github.com/good/repo/pull/{number}",
            "updated_at":"{updated_at}",
            "additions":{additions},
            "head":{{"sha":"{rev}"}}
          }}
        }}"#
    )
}

#[test]
fn webhook_submission_storm_distinguishes_distinct_prs_with_same_head_sha() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_pr_storm_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    std::env::set_var("BB_TEST_PR_STORM_SECRET", "s3cret");

    for number in [42, 43] {
        let body = pr_body_for(number, "shared-sha", 12);
        let sig = sign_hmac("s3cret", body.as_bytes());
        let resp = handle_webhook(
            &plane,
            &mut ledger,
            "review",
            &headers(&sig, &format!("delivery-{number}")),
            &body,
        )
        .unwrap();
        assert_eq!(resp.status, 202, "{}", resp.body);
    }

    assert!(ledger
        .latest_submission("https://github.com/good/repo/pull/42")
        .unwrap()
        .is_some());
    assert!(ledger
        .latest_submission("https://github.com/good/repo/pull/43")
        .unwrap()
        .is_some());
    assert_eq!(ledger.list_runs(Some("review"), None).unwrap().len(), 2);
    assert_eq!(
        ledger.list_runs(Some("correctness"), None).unwrap().len(),
        2
    );
    assert_eq!(ledger.list_runs(Some("security"), None).unwrap().len(), 2);
}

#[test]
fn webhook_submission_storm_accepts_oversized_pr_and_enqueues_gate_members() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_pr_storm_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    std::env::set_var("BB_TEST_PR_STORM_SECRET", "s3cret");

    let body = pr_body("sha-large", 99_999);
    let sig = sign_hmac("s3cret", body.as_bytes());
    let resp = handle_webhook(
        &plane,
        &mut ledger,
        "review",
        &headers(&sig, "delivery-1"),
        &body,
    )
    .unwrap();
    assert_eq!(resp.status, 202, "{}", resp.body);

    let sub = ledger
        .latest_submission("https://github.com/good/repo/pull/42")
        .unwrap()
        .expect("submission created");
    assert_eq!(sub.rev, "sha-large");
    assert_eq!(sub.context, None);

    let runs = ledger.list_runs(None, None).unwrap();
    assert_eq!(runs.len(), 3, "{runs:#?}");
    let control = runs.iter().find(|r| r.task == "review").unwrap();
    for kind in ["correctness", "security"] {
        let run = runs.iter().find(|r| r.task == kind).unwrap();
        assert_eq!(
            run.idempotency_key.as_deref(),
            Some(format!("storm:{}:{kind}", sub.id).as_str())
        );
        assert_eq!(run.parent_run_id.as_deref(), Some(control.id.as_str()));
        let payload = ledger.run_payload(&run.id).unwrap().unwrap();
        let event: serde_json::Value = serde_json::from_str(&payload).unwrap();
        assert_eq!(event["submission"], sub.id);
        assert_eq!(event["repo"], "good/repo");
        assert_eq!(event["rev"], "sha-large");
        assert_eq!(event["change"], "https://github.com/good/repo/pull/42");
        assert!(event.get("context").is_none());
    }

    let second = handle_webhook(
        &plane,
        &mut ledger,
        "review",
        &headers(&sig, "delivery-2"),
        &body,
    )
    .unwrap();
    assert_eq!(second.status, 202);
    assert!(
        second.body.contains("\"duplicate\":true"),
        "{}",
        second.body
    );
    assert_eq!(ledger.list_submissions(10).unwrap().len(), 1);
    assert_eq!(ledger.list_runs(None, None).unwrap().len(), 3);
}

#[test]
fn webhook_submission_storm_redelivery_repairs_missing_member_run() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_pr_storm_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    std::env::set_var("BB_TEST_PR_STORM_SECRET", "s3cret");

    let body = pr_body("sha-repair", 12);
    let sig = sign_hmac("s3cret", body.as_bytes());
    handle_webhook(
        &plane,
        &mut ledger,
        "review",
        &headers(&sig, "delivery-1"),
        &body,
    )
    .unwrap();
    let sub = ledger
        .latest_submission("https://github.com/good/repo/pull/42")
        .unwrap()
        .unwrap();
    let missing_key = format!("storm:{}:security", sub.id);
    rusqlite::Connection::open(plane.db_path())
        .unwrap()
        .execute(
            "DELETE FROM runs WHERE idempotency_key = ?1",
            params![missing_key],
        )
        .unwrap();
    assert_eq!(ledger.list_runs(Some("security"), None).unwrap().len(), 0);

    let resp = handle_webhook(
        &plane,
        &mut ledger,
        "review",
        &headers(&sig, "delivery-2"),
        &body,
    )
    .unwrap();
    assert_eq!(resp.status, 202);
    assert!(resp.body.contains("\"duplicate\":true"));
    assert_eq!(
        ledger.list_runs(Some("correctness"), None).unwrap().len(),
        1
    );
    assert_eq!(ledger.list_runs(Some("security"), None).unwrap().len(), 1);
}

#[test]
fn webhook_submission_storm_supersedes_open_submission_on_new_pr_head() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_pr_storm_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    std::env::set_var("BB_TEST_PR_STORM_SECRET", "s3cret");

    for (rev, delivery, updated_at) in [
        ("sha-old", "delivery-1", "2026-06-17T04:00:00Z"),
        ("sha-new", "delivery-2", "2026-06-17T04:01:00Z"),
    ] {
        let body = pr_body_for_version(42, rev, 12, updated_at);
        let sig = sign_hmac("s3cret", body.as_bytes());
        let resp = handle_webhook(
            &plane,
            &mut ledger,
            "review",
            &headers(&sig, delivery),
            &body,
        )
        .unwrap();
        assert_eq!(resp.status, 202, "{}", resp.body);
    }

    let submissions = ledger.list_submissions(10).unwrap();
    assert_eq!(submissions.len(), 2, "{submissions:#?}");
    assert_eq!(submissions[0].submission.rev, "sha-new");
    assert_eq!(submissions[0].submission.state, "open");
    assert_eq!(submissions[1].submission.rev, "sha-old");
    assert_eq!(submissions[1].submission.state, "abandoned");

    for kind in ["correctness", "security"] {
        let expected = format!("storm:{}:{kind}", submissions[0].submission.id);
        assert!(ledger
            .list_runs(Some(kind), None)
            .unwrap()
            .iter()
            .any(|r| r.idempotency_key.as_deref() == Some(expected.as_str())));
    }

    let old_body = pr_body_for_version(42, "sha-old", 12, "2026-06-17T04:00:00Z");
    let old_sig = sign_hmac("s3cret", old_body.as_bytes());
    let resp = handle_webhook(
        &plane,
        &mut ledger,
        "review",
        &headers(&old_sig, "delivery-3"),
        &old_body,
    )
    .unwrap();
    assert_eq!(resp.status, 202);
    assert_eq!(
        ledger
            .latest_submission("https://github.com/good/repo/pull/42")
            .unwrap()
            .unwrap()
            .rev,
        "sha-new"
    );
}

#[test]
fn webhook_submission_storm_rejects_late_first_delivery_for_stale_head() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_pr_storm_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    std::env::set_var("BB_TEST_PR_STORM_SECRET", "s3cret");

    for (rev, delivery, updated_at) in [
        ("sha-new", "delivery-new", "2026-06-17T04:01:00Z"),
        ("sha-old", "delivery-old-late", "2026-06-17T04:00:00Z"),
    ] {
        let body = pr_body_for_version(42, rev, 12, updated_at);
        let sig = sign_hmac("s3cret", body.as_bytes());
        let resp = handle_webhook(
            &plane,
            &mut ledger,
            "review",
            &headers(&sig, delivery),
            &body,
        )
        .unwrap();
        assert_eq!(resp.status, 202, "{}", resp.body);
    }

    let submissions = ledger.list_submissions(10).unwrap();
    assert_eq!(submissions.len(), 1, "{submissions:#?}");
    assert_eq!(submissions[0].submission.rev, "sha-new");
    assert_eq!(ledger.list_runs(Some("review"), None).unwrap().len(), 2);
    assert_eq!(
        ledger.list_runs(Some("correctness"), None).unwrap().len(),
        1
    );
    assert_eq!(ledger.list_runs(Some("security"), None).unwrap().len(), 1);
}

#[test]
fn webhook_submission_storm_is_idempotent_after_settle() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_pr_storm_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    std::env::set_var("BB_TEST_PR_STORM_SECRET", "s3cret");

    let body = pr_body("sha-settled", 12);
    let sig = sign_hmac("s3cret", body.as_bytes());

    handle_webhook(
        &plane,
        &mut ledger,
        "review",
        &headers(&sig, "delivery-1"),
        &body,
    )
    .unwrap();
    let sub = ledger
        .latest_submission("https://github.com/good/repo/pull/42")
        .unwrap()
        .expect("submission created");
    assert_eq!(
        ledger.list_runs(Some("correctness"), None).unwrap().len(),
        1
    );

    // The gate settles the submission; `clear` is terminal.
    assert!(ledger.settle_submission(&sub.id, "clear", "{}").unwrap());

    // A routine GitHub redelivery of the same head must be an idempotent no-op:
    // no new submission, no re-fired (paid) storm members, no duplicate PR comments.
    let again = handle_webhook(
        &plane,
        &mut ledger,
        "review",
        &headers(&sig, "delivery-2"),
        &body,
    )
    .unwrap();
    assert_eq!(again.status, 202, "{}", again.body);
    assert_eq!(
        ledger.list_submissions(10).unwrap().len(),
        1,
        "redelivery after settle must not open a second submission"
    );
    assert_eq!(
        ledger.list_runs(Some("correctness"), None).unwrap().len(),
        1,
        "redelivery after settle must not re-fire storm members"
    );
    assert_eq!(ledger.list_runs(Some("security"), None).unwrap().len(), 1);
}

#[test]
fn webhook_submission_storm_rejects_stale_older_head_after_settle() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_pr_storm_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    std::env::set_var("BB_TEST_PR_STORM_SECRET", "s3cret");

    let newest_body = pr_body_for_version(42, "sha-new", 12, "2026-06-17T04:01:00Z");
    let newest_sig = sign_hmac("s3cret", newest_body.as_bytes());
    handle_webhook(
        &plane,
        &mut ledger,
        "review",
        &headers(&newest_sig, "delivery-new"),
        &newest_body,
    )
    .unwrap();
    let sub = ledger
        .latest_submission("https://github.com/good/repo/pull/42")
        .unwrap()
        .expect("submission created");
    assert_eq!(sub.head_version.as_deref(), Some("2026-06-17T04:01:00Z"));
    assert!(ledger.settle_submission(&sub.id, "clear", "{}").unwrap());
    let settled = ledger.submission(&sub.id).unwrap();
    assert_eq!(
        settled.head_version.as_deref(),
        Some("2026-06-17T04:01:00Z")
    );
    assert_eq!(settled.report_json.as_deref(), Some("{}"));

    let stale_body = pr_body_for_version(42, "sha-old", 12, "2026-06-17T04:00:00Z");
    let stale_sig = sign_hmac("s3cret", stale_body.as_bytes());
    let resp = handle_webhook(
        &plane,
        &mut ledger,
        "review",
        &headers(&stale_sig, "delivery-old-after-settle"),
        &stale_body,
    )
    .unwrap();
    assert_eq!(resp.status, 202, "{}", resp.body);

    let submissions = ledger.list_submissions(10).unwrap();
    assert_eq!(
        submissions.len(),
        1,
        "stale older head after settle must not open a new submission"
    );
    assert_eq!(submissions[0].submission.rev, "sha-new");
    assert_eq!(
        ledger.list_runs(Some("correctness"), None).unwrap().len(),
        1,
        "stale older head after settle must not re-fire storm members"
    );
    assert_eq!(ledger.list_runs(Some("security"), None).unwrap().len(), 1);
}

#[test]
fn webhook_submission_storm_opens_newer_head_after_settle() {
    let dir = tempfile::tempdir().unwrap();
    let plane = make_pr_storm_plane(dir.path());
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    std::env::set_var("BB_TEST_PR_STORM_SECRET", "s3cret");

    let old_body = pr_body_for_version(42, "sha-old", 12, "2026-06-17T04:00:00Z");
    let old_sig = sign_hmac("s3cret", old_body.as_bytes());
    handle_webhook(
        &plane,
        &mut ledger,
        "review",
        &headers(&old_sig, "delivery-old"),
        &old_body,
    )
    .unwrap();
    let sub = ledger
        .latest_submission("https://github.com/good/repo/pull/42")
        .unwrap()
        .expect("submission created");
    assert_eq!(sub.head_version.as_deref(), Some("2026-06-17T04:00:00Z"));
    assert!(ledger.settle_submission(&sub.id, "clear", "{}").unwrap());

    let newest_body = pr_body_for_version(42, "sha-new", 12, "2026-06-17T04:01:00Z");
    let newest_sig = sign_hmac("s3cret", newest_body.as_bytes());
    let resp = handle_webhook(
        &plane,
        &mut ledger,
        "review",
        &headers(&newest_sig, "delivery-new-after-settle"),
        &newest_body,
    )
    .unwrap();
    assert_eq!(resp.status, 202, "{}", resp.body);

    let submissions = ledger.list_submissions(10).unwrap();
    assert_eq!(submissions.len(), 2, "{submissions:#?}");
    assert_eq!(submissions[0].submission.rev, "sha-new");
    assert_eq!(submissions[0].submission.state, "open");
    assert_eq!(
        submissions[0].submission.head_version.as_deref(),
        Some("2026-06-17T04:01:00Z")
    );
    assert_eq!(submissions[1].submission.rev, "sha-old");
    assert_eq!(submissions[1].submission.state, "clear");
    assert_eq!(
        ledger.list_runs(Some("correctness"), None).unwrap().len(),
        2,
        "newer head after settle should fire a new storm"
    );
    assert_eq!(ledger.list_runs(Some("security"), None).unwrap().len(), 2);
}
