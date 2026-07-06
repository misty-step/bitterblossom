//! bitterblossom-930: the `bb ask` CLI side of the HITL ask/answer primitive.
//! A dispatched attempt shells out to `bb ask` via its own Bash tool -- any
//! harness, no per-harness MCP wiring needed. Raise + poll happen over plain
//! HTTP against the plane's own `/api/asks` surface (curl, matching this
//! repo's existing convention of shelling to curl for outbound calls --
//! canary.rs, notify.rs -- rather than adding an HTTP client dependency).
//! Secrets (the ask token) never touch argv: curl reads its config from
//! stdin, matching `ingress::sign_hmac`'s sibling delivery code.

use std::io::Write;
use std::path::Path;
use std::process::{Command, Stdio};
use std::time::Duration;

use anyhow::{bail, Context, Result};

/// Exit code contract (documented in the ask/answer commission cards):
/// 0 = answered, answer on stdout. This code = parked; the caller must write
/// its own episodic handoff packet (`ASK_PACKET_FILENAME`, authored by the
/// agent, never by bb) and stop, not retry.
pub const PARKED_EXIT_CODE: i32 = 75;

const POLL_INTERVAL: Duration = Duration::from_secs(3);

pub enum RaiseOutcome {
    Answered(String),
    Parked,
}

struct RunIdentity {
    run_id: String,
    task: String,
    ask_token: String,
}

fn read_run_identity(run_json_path: &Path) -> Result<RunIdentity> {
    let raw = std::fs::read_to_string(run_json_path)
        .with_context(|| format!("read {}", run_json_path.display()))?;
    let v: serde_json::Value = serde_json::from_str(&raw).context("RUN.json not valid JSON")?;
    let field = |name: &str| -> Result<String> {
        v.get(name)
            .and_then(serde_json::Value::as_str)
            .map(str::to_string)
            .with_context(|| format!("RUN.json missing '{name}'"))
    };
    Ok(RunIdentity {
        run_id: field("run_id")?,
        task: field("task")?,
        ask_token: field("ask_token")?,
    })
}

#[allow(clippy::too_many_arguments)]
pub fn raise(
    base_url: &str,
    run_json_path: &Path,
    kind: &str,
    question: &str,
    context: Option<&str>,
    blocking: bool,
    window_seconds: i64,
) -> Result<RaiseOutcome> {
    let identity = read_run_identity(run_json_path)?;
    let body = serde_json::json!({
        "run_id": identity.run_id,
        "task": identity.task,
        "kind": kind,
        "question": question,
        "context": context,
        "blocking": blocking,
        "window_seconds": window_seconds,
    })
    .to_string();
    let raised: serde_json::Value = curl_json(
        base_url,
        "/api/asks",
        "POST",
        Some(&identity.ask_token),
        Some(&body),
    )?;
    let id = raised
        .get("id")
        .and_then(serde_json::Value::as_str)
        .context("raise response missing 'id'")?
        .to_string();

    loop {
        let polled: serde_json::Value = curl_json(
            base_url,
            &format!("/api/asks/{id}"),
            "GET",
            Some(&identity.ask_token),
            None,
        )?;
        match polled.get("state").and_then(serde_json::Value::as_str) {
            Some("answered") => {
                let answer = polled
                    .get("answer")
                    .and_then(serde_json::Value::as_str)
                    .context("answered ask missing 'answer'")?
                    .to_string();
                return Ok(RaiseOutcome::Answered(answer));
            }
            Some("parked") => return Ok(RaiseOutcome::Parked),
            Some("open") => std::thread::sleep(POLL_INTERVAL),
            other => bail!("ask {id} in unexpected state {other:?}"),
        }
    }
}

pub fn answer(
    base_url: &str,
    api_token: &str,
    ask_id: &str,
    answer: &str,
    answered_by: &str,
) -> Result<String> {
    let body = serde_json::json!({ "answer": answer, "answered_by": answered_by }).to_string();
    let response: serde_json::Value = curl_json(
        base_url,
        &format!("/api/asks/{ask_id}/answer"),
        "POST",
        Some(api_token),
        Some(&body),
    )?;
    Ok(serde_json::to_string_pretty(&response)?)
}

fn curl_json(
    base_url: &str,
    path: &str,
    method: &str,
    bearer: Option<&str>,
    body: Option<&str>,
) -> Result<serde_json::Value> {
    let url = format!("{}{path}", base_url.trim_end_matches('/'));
    let mut config = format!(
        "fail\nsilent\nshow-error\nmax-time = 30\nrequest = \"{}\"\nurl = \"{}\"\n",
        curl_escape(method),
        curl_escape(&url),
    );
    if let Some(token) = bearer {
        config.push_str(&format!(
            "header = \"Authorization: Bearer {}\"\n",
            curl_escape(token)
        ));
    }
    if let Some(body) = body {
        config.push_str("header = \"Content-Type: application/json\"\n");
        config.push_str(&format!("data = \"{}\"\n", curl_escape(body)));
    }
    let mut child = Command::new("curl")
        .args(["--config", "-"])
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .context("spawn curl")?;
    child
        .stdin
        .take()
        .context("curl stdin")?
        .write_all(config.as_bytes())?;
    let out = child.wait_with_output().context("wait for curl")?;
    if !out.status.success() {
        bail!(
            "curl {method} {path} failed: {}",
            String::from_utf8_lossy(&out.stderr).trim()
        );
    }
    serde_json::from_slice(&out.stdout).with_context(|| {
        format!(
            "curl {method} {path} returned non-JSON: {}",
            String::from_utf8_lossy(&out.stdout)
        )
    })
}

fn curl_escape(value: &str) -> String {
    value.replace('\\', "\\\\").replace('"', "\\\"")
}
