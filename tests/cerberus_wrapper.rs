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

fn write_event_and_run(dir: &std::path::Path) {
    fs::write(
        dir.join("EVENT.json"),
        r#"{"repo":"misty-step/example","pr":42,"measurement":true}"#,
    )
    .unwrap();
    fs::write(dir.join("RUN.json"), r#"{"run_id":"r1","task":"review"}"#).unwrap();
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
summary_target=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --out-dir) out_dir="$2"; shift 2;;
    --summary-target) summary_target="$2"; shift 2;;
    --dry-run) mode="dry-run"; shift;;
    --post) mode="post"; shift;;
    *) shift;;
  esac
done
test "$mode" = "dry-run"
test "$summary_target" = "status"
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
    write_event_and_run(dir.path());

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

#[test]
fn cerberus_wrapper_prefers_source_checkout_over_stale_target_binary() {
    let dir = tempfile::tempdir().unwrap();
    write_event_and_run(dir.path());

    let stale_binary = dir.path().join("cerberus/target/debug/cerberus");
    fs::create_dir_all(stale_binary.parent().unwrap()).unwrap();
    write_executable(
        &stale_binary,
        r#"#!/bin/sh
touch stale-target-used
exit 42
"#,
    );
    fs::write(dir.path().join("cerberus/Cargo.toml"), "[package]\n").unwrap();

    let fake_bin = dir.path().join("bin");
    fs::create_dir_all(&fake_bin).unwrap();
    write_executable(
        &fake_bin.join("cargo"),
        r#"#!/bin/sh
set -eu
touch cargo-used
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
printf '{"run":{}}\n' > "$out_dir/artifact.json"
printf 'review body\n' > "$out_dir/review.md"
printf '{"receipt":true}\n' > "$out_dir/receipt-bundle.json"
"#,
    );

    let output = Command::new(repo_root().join("scripts/cerberus-review-wrapper.sh"))
        .current_dir(dir.path())
        .env(
            "PATH",
            format!("{}:/usr/bin:/bin:/usr/sbin:/sbin", fake_bin.display()),
        )
        .output()
        .unwrap();
    assert!(
        output.status.success(),
        "stdout={}\nstderr={}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );
    assert!(dir.path().join("cargo-used").exists());
    assert!(!dir.path().join("stale-target-used").exists());
    assert!(dir.path().join("REPORT.json").exists());
}

#[test]
fn cerberus_wrapper_uses_omp_when_opencode_is_unavailable() {
    let dir = tempfile::tempdir().unwrap();
    write_event_and_run(dir.path());
    fs::create_dir_all(dir.path().join("cerberus")).unwrap();
    fs::write(dir.path().join("cerberus/Cargo.toml"), "[package]\n").unwrap();

    let fake_bin = dir.path().join("bin");
    fs::create_dir_all(&fake_bin).unwrap();
    write_executable(&fake_bin.join("bun"), "#!/bin/sh\nexit 0\n");
    write_executable(&fake_bin.join("omp"), "#!/bin/sh\nexit 0\n");
    write_executable(
        &fake_bin.join("cargo"),
        r#"#!/bin/sh
set -eu
out_dir=""
mode=""
harness=""
omp_binary=""
allow_env=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --out-dir) out_dir="$2"; shift 2;;
    --dry-run) mode="dry-run"; shift;;
    --post) mode="post"; shift;;
    --harness) harness="$2"; shift 2;;
    --omp-binary) omp_binary="$2"; shift 2;;
    --allow-env) allow_env="$2"; shift 2;;
    *) shift;;
  esac
done
test "$mode" = "dry-run"
[ "$harness" = "omp" ] || { echo "harness=$harness" >&2; exit 1; }
case "$omp_binary" in
  /*/bin/omp) ;;
  *) echo "omp_binary=$omp_binary" >&2; exit 1;;
esac
[ -x "$omp_binary" ] || { echo "omp_binary is not executable: $omp_binary" >&2; exit 1; }
[ -x "$HOME/.local/bin/bun" ] || { echo "bun shim missing" >&2; exit 1; }
[ "$allow_env" = "OPENROUTER_API_KEY" ] || { echo "allow_env=$allow_env" >&2; exit 1; }
mkdir -p "$out_dir"
printf '{"run":{}}\n' > "$out_dir/artifact.json"
printf 'review body\n' > "$out_dir/review.md"
printf '{"receipt":true}\n' > "$out_dir/receipt-bundle.json"
"#,
    );

    let output = Command::new(repo_root().join("scripts/cerberus-review-wrapper.sh"))
        .current_dir(dir.path())
        .env(
            "PATH",
            format!("{}:/usr/bin:/bin:/usr/sbin:/sbin", fake_bin.display()),
        )
        .env("HOME", dir.path().join("home"))
        .env("OPENROUTER_API_KEY", "test-key")
        .output()
        .unwrap();
    assert!(
        output.status.success(),
        "stdout={}\nstderr={}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );
}
