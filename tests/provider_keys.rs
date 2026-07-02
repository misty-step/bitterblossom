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
    serde_json::json!({
        "hash": KEY_HASH,
        "name": "bb:test:a:openrouter-a:2026-07-02T00:00:00Z",
        "label": "bb scoped key",
        "limit": 12.5,
        "limit_remaining": 12.5,
        "limit_reset": null,
        "usage": 0.0,
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
