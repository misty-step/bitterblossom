use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::Path;
use std::process::Command;
use std::thread;

use bitterblossom::dispatch;
use bitterblossom::ledger::{IngressRequest, Ledger};
use bitterblossom::spec::Plane;
use tiny_http::{Header, Method, Response, Server, StatusCode};

const CHILD_KEY: &str = "sk-or-v1-test-child-key-material";
const KEY_HASH: &str = "f01d52606dc8f0a8303a7b5cc3fa07109c2e346cec7c0a16b40de462992ce943";

fn write_executable(path: &Path, content: &str) {
    fs::write(path, content).unwrap();
    fs::set_permissions(path, fs::Permissions::from_mode(0o755)).unwrap();
}

fn setup_plane(root: &Path) {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
    let stub = root.join("stub.sh");
    write_executable(
        &stub,
        r#"#!/bin/sh
cat > /dev/null
[ "$OPENROUTER_API_KEY" = "sk-or-v1-test-child-key-material" ] || {
  echo "wrong child key: ${OPENROUTER_API_KEY:+present}" >&2
  exit 9
}
printf '{"status":"ok","artifact_paths":["REPORT.json"]}\n' > REPORT.json
printf '{"schema_version":"bb.command_result.v1","result":"child key injected","cost_usd":0.0}\n'
"#,
    );
    fs::write(
        root.join("agents/a.toml"),
        format!(
            r#"version = 1
harness = "command"
model = ""
bin = "{}"
secrets = ["OPENROUTER_API_KEY"]
[policy]
authority = "edit"
provider_key_name = "openrouter-a"
provider_spend_cap_usd = 12.5
model_allowlist = []
trigger_bindings = ["manual"]
wall_clock_minutes = 1
side_effect_policy = "log"
"#,
            stub.display()
        ),
    )
    .unwrap();
    fs::write(
        root.join("tasks/demo/card.md"),
        "prove child key injection\n",
    )
    .unwrap();
    fs::write(
        root.join("tasks/demo/task.toml"),
        "agent = \"a\"\nsubstrate = \"local\"\nrequired_artifacts = [\"REPORT.json\"]\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
}

fn start_openrouter_fake() -> (String, thread::JoinHandle<()>) {
    let server = Server::http("127.0.0.1:0").unwrap();
    let addr = format!("http://{}/api/v1", server.server_addr());
    let handle = thread::spawn(move || {
        for expected in ["create", "list", "delete"] {
            let mut req = server.recv().unwrap();
            let auth = req
                .headers()
                .iter()
                .find(|h| h.field.equiv("Authorization"))
                .map(|h| h.value.as_str())
                .unwrap_or("");
            assert_eq!(auth, "Bearer fake-management-key");
            match expected {
                "create" => {
                    assert_eq!(req.method(), &Method::Post);
                    assert_eq!(req.url(), "/api/v1/keys");
                    let mut body = String::new();
                    req.as_reader().read_to_string(&mut body).unwrap();
                    let body: serde_json::Value = serde_json::from_str(&body).unwrap();
                    assert!(body["name"].as_str().unwrap().starts_with("bb:"));
                    assert_eq!(body["limit"], 12.5);
                    assert_eq!(body["include_byok_in_limit"], false);
                    req.respond(json_response(
                        StatusCode(201),
                        serde_json::json!({
                            "data": key_data(false),
                            "key": CHILD_KEY
                        }),
                    ))
                    .unwrap();
                }
                "list" => {
                    assert_eq!(req.method(), &Method::Get);
                    assert_eq!(req.url(), "/api/v1/keys");
                    req.respond(json_response(
                        StatusCode(200),
                        serde_json::json!({"data": [key_data(false)]}),
                    ))
                    .unwrap();
                }
                "delete" => {
                    assert_eq!(req.method(), &Method::Delete);
                    assert_eq!(req.url(), format!("/api/v1/keys/{KEY_HASH}"));
                    req.respond(json_response(
                        StatusCode(200),
                        serde_json::json!({"success": true}),
                    ))
                    .unwrap();
                }
                _ => unreachable!(),
            }
        }
    });
    (addr, handle)
}

fn key_data(disabled: bool) -> serde_json::Value {
    key_data_with_limit(disabled, 12.5, 12.5, 0.0)
}

fn key_data_with_limit(
    disabled: bool,
    limit: f64,
    limit_remaining: f64,
    usage: f64,
) -> serde_json::Value {
    serde_json::json!({
        "hash": KEY_HASH,
        "name": "bb:test:a:openrouter-a:2026-07-02T00:00:00Z",
        "label": "bb scoped key",
        "limit": limit,
        "limit_remaining": limit_remaining,
        "limit_reset": null,
        "usage": usage,
        "disabled": disabled,
        "created_at": "2026-07-02T00:00:00Z",
        "updated_at": null,
        "byok_usage": 0.0,
        "byok_usage_daily": 0.0,
        "byok_usage_monthly": 0.0,
        "byok_usage_weekly": 0.0,
        "creator_user_id": null,
        "include_byok_in_limit": false,
        "usage_daily": 0.0,
        "usage_monthly": 0.0,
        "usage_weekly": 0.0,
        "workspace_id": "00000000-0000-0000-0000-000000000000"
    })
}

fn start_openrouter_list_fake(rows: Vec<serde_json::Value>) -> (String, thread::JoinHandle<()>) {
    let server = Server::http("127.0.0.1:0").unwrap();
    let addr = format!("http://{}/api/v1", server.server_addr());
    let handle = thread::spawn(move || {
        let req = server.recv().unwrap();
        let auth = req
            .headers()
            .iter()
            .find(|h| h.field.equiv("Authorization"))
            .map(|h| h.value.as_str())
            .unwrap_or("");
        assert_eq!(auth, "Bearer fake-management-key");
        assert_eq!(req.method(), &Method::Get);
        assert_eq!(req.url(), "/api/v1/keys?include_disabled=true");
        req.respond(json_response(
            StatusCode(200),
            serde_json::json!({"data": rows}),
        ))
        .unwrap();
    });
    (addr, handle)
}

fn write_stored_child_key(root: &Path, spend_cap_usd: f64) {
    let dir = root.join(".bb/provider-keys/openrouter");
    fs::create_dir_all(&dir).unwrap();
    fs::write(
        dir.join("a.json"),
        serde_json::to_vec_pretty(&serde_json::json!({
            "schema_version": 1,
            "provider": "openrouter",
            "agent": "a",
            "provider_key_name": "openrouter-a",
            "name": "bb:test:a:openrouter-a:2026-07-02T00:00:00Z",
            "hash": KEY_HASH,
            "label": "bb scoped key",
            "spend_cap_usd": spend_cap_usd,
            "limit_remaining_usd": spend_cap_usd,
            "limit_reset": null,
            "usage_usd": 0.0,
            "disabled": false,
            "created_at": "2026-07-02T00:00:00Z",
            "updated_at": null,
            "minted_at": "2026-07-02T00:00:01Z",
            "revoked_at": null,
            "api_key": CHILD_KEY
        }))
        .unwrap(),
    )
    .unwrap();
}

fn json_response(
    status: StatusCode,
    body: serde_json::Value,
) -> Response<std::io::Cursor<Vec<u8>>> {
    let bytes = serde_json::to_vec(&body).unwrap();
    Response::from_data(bytes)
        .with_status_code(status)
        .with_header(Header::from_bytes(&b"Content-Type"[..], &b"application/json"[..]).unwrap())
}

fn bb(root: &Path, args: &[&str], base_url: &str) -> std::process::Output {
    Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(["--config", root.to_str().unwrap()])
        .args(args)
        .env("OPENROUTER_MANAGEMENT_KEY", "fake-management-key")
        .env("BB_OPENROUTER_KEYS_BASE_URL", base_url)
        .env_remove("OPENROUTER_API_KEY")
        .output()
        .unwrap()
}

#[test]
fn openrouter_child_key_mint_lists_injects_and_revokes_without_printing_secret() {
    let dir = tempfile::tempdir().unwrap();
    setup_plane(dir.path());
    let (base_url, server) = start_openrouter_fake();

    let minted = bb(dir.path(), &["keys", "mint", "a", "--json"], &base_url);
    assert!(
        minted.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&minted.stdout),
        String::from_utf8_lossy(&minted.stderr)
    );
    let minted_stdout = String::from_utf8_lossy(&minted.stdout);
    assert!(!minted_stdout.contains(CHILD_KEY));
    let doc: serde_json::Value = serde_json::from_slice(&minted.stdout).unwrap();
    assert_eq!(doc["keys"][0]["agent"], "a");
    assert_eq!(doc["keys"][0]["spend_cap_usd"], 12.5);
    assert_eq!(doc["keys"][0]["hash"], KEY_HASH);
    assert_eq!(doc["keys"][0]["secret_available"], true);

    let key_path = dir.path().join(".bb/provider-keys/openrouter/a.json");
    assert_eq!(
        fs::metadata(&key_path).unwrap().permissions().mode() & 0o777,
        0o600
    );

    let remote = bb(
        dir.path(),
        &["keys", "list", "--remote", "--json"],
        &base_url,
    );
    assert!(remote.status.success());
    let remote_doc: serde_json::Value = serde_json::from_slice(&remote.stdout).unwrap();
    assert_eq!(remote_doc[0]["hash"], KEY_HASH);
    assert_eq!(remote_doc[0]["limit"], 12.5);
    assert!(!String::from_utf8_lossy(&remote.stdout).contains(CHILD_KEY));

    let plane = Plane::load(dir.path()).unwrap();
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();
    let run_id = ledger
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
    std::env::remove_var("OPENROUTER_API_KEY");
    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();
    assert_eq!(run.state, "success", "reason={:?}", run.state_reason);

    let revoked = bb(dir.path(), &["keys", "revoke", "a", "--json"], &base_url);
    assert!(
        revoked.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&revoked.stdout),
        String::from_utf8_lossy(&revoked.stderr)
    );
    let revoked_doc: serde_json::Value = serde_json::from_slice(&revoked.stdout).unwrap();
    assert_eq!(revoked_doc["revoked"], true);
    assert_eq!(revoked_doc["secret_available"], false);
    assert!(!String::from_utf8_lossy(&revoked.stdout).contains(CHILD_KEY));

    server.join().unwrap();
}

#[test]
fn key_sync_detects_remote_cap_drift_and_updates_local_metadata_without_printing_secret() {
    let dir = tempfile::tempdir().unwrap();
    setup_plane(dir.path());
    write_stored_child_key(dir.path(), 12.5);
    let (base_url, server) =
        start_openrouter_list_fake(vec![key_data_with_limit(false, 10.0, 9.25, 0.75)]);

    let synced = bb(
        dir.path(),
        &["keys", "sync", "a", "--check", "--json"],
        &base_url,
    );
    assert!(
        !synced.status.success(),
        "check should fail on drift\nstdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&synced.stdout),
        String::from_utf8_lossy(&synced.stderr)
    );
    let stdout = String::from_utf8_lossy(&synced.stdout);
    assert!(!stdout.contains(CHILD_KEY));
    let doc: serde_json::Value = serde_json::from_slice(&synced.stdout).unwrap();
    assert_eq!(doc["operation"], "sync");
    assert_eq!(doc["ok"], false);
    assert_eq!(doc["keys"][0]["agent"], "a");
    assert_eq!(doc["keys"][0]["status"], "drift");
    assert_eq!(doc["keys"][0]["configured_spend_cap_usd"], 12.5);
    assert_eq!(doc["keys"][0]["remote_limit_usd"], 10.0);
    assert_eq!(doc["keys"][0]["limit_remaining_usd"], 9.25);
    assert_eq!(doc["keys"][0]["usage_usd"], 0.75);
    let drift = doc["keys"][0]["drift"].as_array().unwrap();
    assert!(drift.iter().any(|item| item
        .as_str()
        .unwrap()
        .contains("remote provider limit $10.0000")));
    assert!(String::from_utf8_lossy(&synced.stderr).contains("provider key drift detected"));

    let listed = bb(dir.path(), &["keys", "list", "--json"], &base_url);
    assert!(listed.status.success());
    let local: serde_json::Value = serde_json::from_slice(&listed.stdout).unwrap();
    assert_eq!(local[0]["remote_limit_usd"], 10.0);
    assert_eq!(local[0]["limit_remaining_usd"], 9.25);
    assert_eq!(local[0]["usage_usd"], 0.75);
    assert!(local[0]["last_synced_at"].is_string());
    assert!(!String::from_utf8_lossy(&listed.stdout).contains(CHILD_KEY));

    let check = bb(dir.path(), &["check", "--json"], &base_url);
    assert!(check.status.success());
    let check_doc: serde_json::Value = serde_json::from_slice(&check.stdout).unwrap();
    assert_eq!(check_doc["provider_keys"][0]["status"], "drift");
    assert_eq!(
        check_doc["task_details"][0]["provider_key"]["remote_limit_usd"],
        10.0
    );

    let status = bb(dir.path(), &["status", "--json"], &base_url);
    assert!(status.status.success());
    let status_doc: serde_json::Value = serde_json::from_slice(&status.stdout).unwrap();
    assert_eq!(status_doc["tasks"][0]["provider_key"]["status"], "drift");
    assert_eq!(
        status_doc["tasks"][0]["provider_key"]["limit_remaining_usd"],
        9.25
    );

    server.join().unwrap();
}
