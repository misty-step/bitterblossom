use std::io::Write;
use std::process::{Command, Stdio};
use std::sync::Once;
use std::time::Duration;

const CHECKIN_INTERVAL: Duration = Duration::from_secs(60);
const REQUEST_TIMEOUT_SECONDS: u64 = 10;
const MONITOR_NAME: &str = "bb-plane";
// Proof monitor created by the 2026-07-02 factory-fleet-enrollment sweep
// (docs/architecture/factory-fleet-enrollment-2026-07-02.md in the canary
// repo). That sweep sent one manual check-in via an admin key and left the
// recurring send as an app-lane residual, which is why it went overdue on
// 2026-07-04 with nobody noticing until canary-908's /readyz probe caught it
// (bitterblossom-112). Folding it into the same health loop as MONITOR_NAME
// means it can no longer go silent without bb-plane also going silent.
const FLEET_HEARTBEAT_MONITOR_NAME: &str = "bitterblossom-plane-fleet-heartbeat";

static DISABLED_WARNING: Once = Once::new();

fn config() -> Option<(String, String)> {
    let endpoint = std::env::var("CANARY_ENDPOINT").ok()?;
    let key = std::env::var("CANARY_INGEST_KEY").ok()?;
    (!endpoint.is_empty() && !key.is_empty()).then_some((endpoint, key))
}

fn config_or_warn() -> Option<(String, String)> {
    let config = config();
    if config.is_none() {
        DISABLED_WARNING.call_once(|| {
            eprintln!("canary: self-reporting disabled; set CANARY_ENDPOINT and CANARY_INGEST_KEY");
        });
    }
    config
}

pub fn check_in() {
    send_check_in(MONITOR_NAME, "BB plane heartbeat");
}

pub fn check_in_fleet_heartbeat() {
    send_check_in(FLEET_HEARTBEAT_MONITOR_NAME, "BB plane fleet heartbeat");
}

fn send_check_in(monitor: &str, summary: &str) {
    let Some((ep, key)) = config_or_warn() else {
        return;
    };
    let payload = serde_json::json!({
        "monitor": monitor,
        "status": "alive",
        "summary": summary,
        "ttl_ms": 120_000,
    });
    deliver(&ep, &key, "/api/v1/check-ins", &payload.to_string());
}

pub fn report_error(class: &str, message: &str) {
    let Some((ep, key)) = config_or_warn() else {
        return;
    };
    let payload = serde_json::json!({
        "service": "bitterblossom-plane",
        "error_class": class,
        "message": message,
        "severity": "error",
    });
    deliver(&ep, &key, "/api/v1/errors", &payload.to_string());
}

pub fn start_health_loop() {
    if config().is_none() {
        return;
    }
    std::thread::Builder::new()
        .name("bb-canary-health".into())
        .spawn(move || loop {
            std::thread::sleep(CHECKIN_INTERVAL);
            check_in();
            check_in_fleet_heartbeat();
        })
        .ok();
}

fn notify_bin() -> String {
    std::env::var("BB_NOTIFY_BIN").unwrap_or_else(|_| "curl".into())
}

fn deliver(endpoint: &str, key: &str, path: &str, body: &str) {
    let url = format!(
        "{}/{}",
        endpoint.trim_end_matches('/'),
        path.trim_start_matches('/')
    );
    let config = curl_config(&url, key, body);
    let spawned = Command::new(notify_bin())
        .args(["--config", "-"])
        .stdin(Stdio::piped())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn();
    match spawned {
        Ok(mut child) => {
            if let Some(mut stdin) = child.stdin.take() {
                let _ = stdin.write_all(config.as_bytes());
            }
            match child.wait() {
                Ok(status) if !status.success() => {
                    eprintln!("canary: delivery failed path={path} curl_exit={status}");
                }
                Err(e) => eprintln!("canary: delivery failed path={path} cannot_wait_curl={e}"),
                _ => {}
            }
        }
        Err(e) => eprintln!("canary: delivery failed path={path} cannot_spawn_curl={e}"),
    }
}

fn curl_config(url: &str, key: &str, body: &str) -> String {
    format!(
        "fail\nsilent\nshow-error\nmax-time = {REQUEST_TIMEOUT_SECONDS}\nrequest = \"POST\"\n\
         url = \"{}\"\nheader = \"Authorization: Bearer {}\"\n\
         header = \"Content-Type: application/json\"\ndata = \"{}\"\n",
        curl_config_escape(url),
        curl_config_escape(key),
        curl_config_escape(body),
    )
}

fn curl_config_escape(value: &str) -> String {
    value.replace('\\', "\\\\").replace('"', "\\\"")
}
