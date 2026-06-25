use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::process::Command;

use bitterblossom::harness::parse_output;

fn repo_root() -> std::path::PathBuf {
    std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR"))
}

fn write_executable(path: &std::path::Path, content: &str) {
    fs::write(path, content).unwrap();
    fs::set_permissions(path, fs::Permissions::from_mode(0o755)).unwrap();
}

#[test]
fn cerberus_wrapper_emits_report_and_structured_command_result() {
    let dir = tempfile::tempdir().unwrap();
    let stub = dir.path().join("cerberus-stub.sh");
    write_executable(
        &stub,
        r#"#!/bin/sh
set -eu
out_dir=""
mode=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --out-dir) out_dir="$2"; shift 2;;
    --dry-run) mode="dry-run"; shift;;
    --post) mode="post"; shift;;
    *) shift;;
  esac
done
test "$mode" = "dry-run"
mkdir -p "$out_dir"
cat > "$out_dir/artifact.json" <<'JSON'
{
  "run": {"cost_usd": "0.42"},
  "receipts": [
    {"usage": {"prompt_tokens": 1234, "completion_tokens": 567, "cost_usd": 0.42}}
  ]
}
JSON
printf 'review body\n' > "$out_dir/review.md"
printf '{"receipt":true}\n' > "$out_dir/receipt-bundle.json"
"#,
    );
    fs::write(
        dir.path().join("EVENT.json"),
        r#"{"repo":"misty-step/example","pr":42,"measurement":true}"#,
    )
    .unwrap();
    fs::write(
        dir.path().join("RUN.json"),
        r#"{"run_id":"r1","task":"review"}"#,
    )
    .unwrap();

    let output = Command::new(repo_root().join("scripts/cerberus-review-wrapper.sh"))
        .current_dir(dir.path())
        .env("CERBERUS_BIN", &stub)
        .output()
        .unwrap();
    assert!(
        output.status.success(),
        "stdout={}\nstderr={}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );

    let report: serde_json::Value =
        serde_json::from_str(&fs::read_to_string(dir.path().join("REPORT.json")).unwrap()).unwrap();
    assert_eq!(report["schema_version"], "bb.cerberus_review_report.v1");
    assert_eq!(report["repo"], "misty-step/example");
    assert_eq!(report["pr"], 42);
    assert_eq!(report["mode"], "dry-run");
    assert_eq!(report["usage"]["cost_usd"], 0.42);
    assert_eq!(report["artifact_paths"][0], "REPORT.json");

    let stdout = String::from_utf8(output.stdout).unwrap();
    let parsed = parse_output("command", &stdout).unwrap();
    assert_eq!(
        parsed.result,
        "cerberus review dry-run complete for misty-step/example#42"
    );
    assert_eq!(parsed.stats.tokens_in, Some(1234));
    assert_eq!(parsed.stats.tokens_out, Some(567));
    assert_eq!(parsed.stats.cost_usd, Some(0.42));
}
