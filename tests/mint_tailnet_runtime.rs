use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::{Path, PathBuf};
use std::process::Command;

fn repo_root() -> PathBuf {
    Path::new(env!("CARGO_MANIFEST_DIR")).to_path_buf()
}

fn write_executable(path: &Path, body: &str) {
    fs::write(path, body).unwrap();
    let mut permissions = fs::metadata(path).unwrap().permissions();
    permissions.set_mode(0o755);
    fs::set_permissions(path, permissions).unwrap();
}

#[test]
fn mint_tailnet_wrapper_bootstraps_from_a_file_and_gives_bb_only_placeholders() {
    let dir = tempfile::tempdir().unwrap();
    let bin = dir.path().join("bin");
    fs::create_dir(&bin).unwrap();
    let log = dir.path().join("calls.log");
    let secret = "tskey-auth-sentinel-must-not-leak";

    write_executable(
        &bin.join("tailscaled"),
        &format!(
            "#!/bin/sh\ntest -z \"${{POWDER_API_KEY:-}}\"\ntest -z \"${{POWDER_API_BASE_URL:-}}\"\ntest -z \"${{BB_TEST_SHOULD_NOT_REACH_TUNNEL:-}}\"\nprintf 'tailscaled\\n' >>'{}'\nsleep 30\n",
            log.display()
        ),
    );
    write_executable(
        &bin.join("tailscale"),
        &format!(
            r#"#!/bin/sh
set -eu
case " $* " in *" --ephemeral "*) exit 64 ;; esac
case "$*" in
  *" up "*)
    key_file=$(printf '%s\n' "$*" | sed -n 's/.*--auth-key=file:\([^ ]*\).*/\1/p')
    test -n "$key_file"
    test "$(cat "$key_file")" = "{}"
    printf 'tailscale-up-file:%s\n' "$*" >>"{}"
    ;;
  *" nc "*) printf 'tailscale-nc\n' >>"{}" ;;
esac
"#,
            secret,
            log.display(),
            log.display()
        ),
    );
    write_executable(
        &bin.join("socat"),
        &format!(
            "#!/bin/sh\ntest -z \"${{POWDER_API_KEY:-}}\"\ntest -z \"${{POWDER_API_BASE_URL:-}}\"\ntest -z \"${{BB_TEST_SHOULD_NOT_REACH_TUNNEL:-}}\"\nprintf 'socat:%s\\n' \"$*\" >>'{}'\nsleep 30\n",
            log.display()
        ),
    );
    write_executable(
        &bin.join("curl"),
        &format!(
            "#!/bin/sh\nprintf 'curl:%s\\n' \"$*\" >>'{}'\ncase \"$*\" in *\"Authorization: Bearer __mint.powder.bitterblossom__\"*\"/proxy/https/sanctum.tail5f5eb4.ts.net:10001/api/v1/cards?limit=1\"*) printf '200'; exit 0 ;; *) exit 9 ;; esac\n",
            log.display()
        ),
    );
    write_executable(
        &bin.join("setpriv"),
        r#"#!/bin/sh
set -eu
printf 'setpriv:%s\n' "$*" >>"$BB_TEST_LOG"
while [ "$#" -gt 0 ]; do
  case "$1" in
    --reuid=*|--regid=*|--init-groups|--no-new-privs|--bounding-set=*|--inh-caps=*|--ambient-caps=*) shift ;;
    *) break ;;
  esac
done
exec "$@"
"#,
    );
    write_executable(&bin.join("setsid"), "#!/bin/sh\nexec \"$@\"\n");
    write_executable(
        &bin.join("bb"),
        r#"#!/bin/sh
set -eu
test -z "${BB_MINT_TAILNET_AUTHKEY:-}"
test "$BB_TEST_SHOULD_NOT_REACH_TUNNEL" = "application-only-sentinel"
test "$POWDER_API_BASE_URL" = "http://127.0.0.1:4949/proxy/https/sanctum.tail5f5eb4.ts.net:10001"
test "$POWDER_API_KEY" = "__mint.powder.bitterblossom__"
printf 'bb:%s\n' "$*" >>"$BB_TEST_LOG"
sleep 1
"#,
    );
    write_executable(
        &bin.join("bb-litestream-entrypoint"),
        &format!(
            "#!/bin/sh\nexec {} \"$@\"\n",
            repo_root()
                .join("scripts/bb-litestream-entrypoint.sh")
                .display()
        ),
    );

    let path = format!("{}:{}", bin.display(), std::env::var("PATH").unwrap());
    let output = Command::new(repo_root().join("scripts/bb-mint-tailnet-entrypoint.sh"))
        .env("PATH", path)
        .env("BB_TEST_LOG", &log)
        .env(
            "BB_TEST_SHOULD_NOT_REACH_TUNNEL",
            "application-only-sentinel",
        )
        .env("BB_MINT_TAILNET_AUTHKEY", secret)
        .env("POWDER_API_KEY", "direct-powder-sentinel-must-not-leak")
        .env("POWDER_API_BASE_URL", "https://direct-powder.invalid")
        .env("BB_MINT_RUNTIME_DIR", dir.path().join("bb-mint-runtime"))
        .env("BB_MINT_STARTUP_TIMEOUT_SECONDS", "2")
        .env("BB_MINT_PROBE_INTERVAL_SECONDS", "0")
        .args(["bb", "serve"])
        .output()
        .unwrap();

    assert!(
        output.status.success(),
        "stderr: {}",
        String::from_utf8_lossy(&output.stderr)
    );
    assert!(!String::from_utf8_lossy(&output.stdout).contains(secret));
    assert!(!String::from_utf8_lossy(&output.stderr).contains(secret));
    assert!(!dir.path().join("bb-mint-runtime/authkey").exists());
    let calls = fs::read_to_string(log).unwrap();
    assert!(calls.contains("tailscale-up-file:"));
    assert!(!calls.contains("--ephemeral"));
    assert!(calls.contains("--accept-routes=false"));
    assert!(calls.contains("socat:TCP-LISTEN:4949,bind=127.0.0.1,reuseaddr,fork,max-children=16"));
    assert!(calls.contains(
        "setpriv:--reuid=bb --regid=bb --init-groups --no-new-privs --bounding-set=-all --inh-caps=-all --ambient-caps=-all"
    ));
    assert!(calls.contains("/proxy/https/sanctum.tail5f5eb4.ts.net:10001/api/v1/cards?limit=1"));
    assert!(calls.contains("bb:serve"));
    assert!(!calls.contains(secret));
}

#[test]
fn wrapper_stops_bb_when_the_live_mint_health_path_fails() {
    let dir = tempfile::tempdir().unwrap();
    let bin = dir.path().join("bin");
    fs::create_dir(&bin).unwrap();
    let curl_count = dir.path().join("curl-count");

    write_executable(&bin.join("tailscaled"), "#!/bin/sh\nsleep 30\n");
    write_executable(&bin.join("tailscale"), "#!/bin/sh\nexit 0\n");
    write_executable(&bin.join("socat"), "#!/bin/sh\nsleep 30\n");
    write_executable(
        &bin.join("curl"),
        &format!(
            r#"#!/bin/sh
set -eu
count=$(cat "{}" 2>/dev/null || printf '0')
count=$((count + 1))
printf '%s' "$count" >"{}"
if [ "$count" -eq 1 ]; then
  printf '200'
else
  printf '302'
fi
"#,
            curl_count.display(),
            curl_count.display()
        ),
    );
    write_executable(
        &bin.join("setpriv"),
        "#!/bin/sh\nwhile [ \"$#\" -gt 0 ]; do case \"$1\" in --reuid=*|--regid=*|--init-groups|--no-new-privs|--bounding-set=*|--inh-caps=*|--ambient-caps=*) shift ;; *) break ;; esac; done\nexec \"$@\"\n",
    );
    write_executable(&bin.join("setsid"), "#!/bin/sh\nexec \"$@\"\n");
    write_executable(
        &bin.join("bb-litestream-entrypoint"),
        "#!/bin/sh\nsleep 1\n",
    );

    let path = format!("{}:{}", bin.display(), std::env::var("PATH").unwrap());
    let output = Command::new(repo_root().join("scripts/bb-mint-tailnet-entrypoint.sh"))
        .env("PATH", path)
        .env("BB_MINT_TAILNET_AUTHKEY", "sentinel")
        .env("BB_MINT_RUNTIME_DIR", dir.path().join("bb-mint-runtime"))
        .env("BB_MINT_STARTUP_TIMEOUT_SECONDS", "2")
        .env("BB_MINT_PROBE_INTERVAL_SECONDS", "0")
        .args(["bb", "serve"])
        .output()
        .unwrap();

    assert!(!output.status.success());
    assert!(String::from_utf8_lossy(&output.stderr)
        .contains("Mint Powder capability probe failed; stopping bb"));
}

#[test]
fn wrapper_rejects_a_shell_active_runtime_path_before_starting_processes() {
    let output = Command::new(repo_root().join("scripts/bb-mint-tailnet-entrypoint.sh"))
        .env("BB_MINT_TAILNET_AUTHKEY", "sentinel")
        .env("BB_MINT_RUNTIME_DIR", "/tmp/socket;touch-pwned")
        .args(["bb", "serve"])
        .output()
        .unwrap();
    assert!(!output.status.success());
    assert!(String::from_utf8_lossy(&output.stderr).contains("runtime path"));
}

#[test]
fn wrapper_without_a_tailnet_key_preserves_the_existing_entrypoint() {
    let dir = tempfile::tempdir().unwrap();
    let bin = dir.path().join("bin");
    fs::create_dir(&bin).unwrap();
    write_executable(
        &bin.join("bb-litestream-entrypoint"),
        "#!/bin/sh\nprintf 'delegated:%s\\n' \"$*\"\n",
    );
    let path = format!("{}:{}", bin.display(), std::env::var("PATH").unwrap());
    let output = Command::new(repo_root().join("scripts/bb-mint-tailnet-entrypoint.sh"))
        .env("PATH", path)
        .env_remove("BB_MINT_TAILNET_AUTHKEY")
        .args(["bb", "serve"])
        .output()
        .unwrap();
    assert!(output.status.success());
    assert_eq!(
        String::from_utf8_lossy(&output.stdout),
        "delegated:bb serve\n"
    );
}
