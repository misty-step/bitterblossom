//! Tailnet-substrate e2e against a stub `ssh` CLI (the real ssh target is a
//! network boundary). The stub strips ssh's own flags/host argument and
//! execs the remaining remote command locally with HOME redirected to a
//! fake sandbox dir, so `~/.bb-tailnet/...` paths in the substrate's own
//! scripts land inside the test's tempdir instead of the real machine.

use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::Path;
use std::process::Command;

use bitterblossom::dispatch;
use bitterblossom::ledger::{IngressRequest, Ledger};
use bitterblossom::spec::Plane;
use bitterblossom::substrate::tailnet::TailnetSubstrate;
use bitterblossom::substrate::{ProbeResult, Substrate};

const SSH_STUB: &str = r#"#!/bin/bash
log="$SSH_STUB_LOG"
home="$SSH_FAKE_HOME"
mkdir -p "$home"
home="$(cd "$home" && pwd -P)"
echo "$*" >> "$log"
while [ $# -gt 0 ]; do
  case "$1" in
    -o) shift 2;;
    --) shift; break;;
    *) shift;;
  esac
done
export HOME="$home"
# OpenSSH does not preserve a remote argv. It joins the remaining local argv
# into one command string and asks the remote login shell to parse it. Emulate
# that boundary here so scripts containing spaces fail unless BB shell-quotes
# every remote argument before invoking ssh.
remote="$*"
# macOS test hosts have no setsid; the session-leader semantics are exercised
# on a real tailnet host (Linux), not by this stub.
case "$remote" in
  "setsid -w sh"|"'setsid' '-w' 'sh'") remote="sh";;
esac
exec /bin/sh -c "$remote"
"#;

const COMMAND_STUB: &str = r#"#!/bin/sh
cat > /dev/null
[ -z "${GH_TOKEN:-}" ] || { echo "GH_TOKEN leaked into tailnet workload" >&2; exit 7; }
printf '{"schema_version":"bb.command_result.v1","result":"tailnet says hi","turns":1,"cost_usd":0.01}\n' > REPORT.json
cat REPORT.json
"#;

// Env vars are process-global; tests setting BB_SSH_BIN must serialize.
static ENV_LOCK: std::sync::Mutex<()> = std::sync::Mutex::new(());

fn write_executable(path: &Path, content: &str) {
    fs::write(path, content).unwrap();
    fs::set_permissions(path, fs::Permissions::from_mode(0o755)).unwrap();
}

#[test]
fn tailnet_task_runs_end_to_end() {
    let _guard = ENV_LOCK.lock().unwrap();
    let dir = tempfile::tempdir().unwrap();
    let root_buf = dir.path().canonicalize().unwrap();
    let root = root_buf.as_path();
    let fake_home = root.join("fake-remote-home");
    let source = root.join("source");
    let real_git = String::from_utf8(
        Command::new("sh")
            .args(["-c", "command -v git"])
            .output()
            .unwrap()
            .stdout,
    )
    .unwrap()
    .trim()
    .to_string();
    Command::new(&real_git)
        .args(["init", "-q", source.to_str().unwrap()])
        .status()
        .unwrap();
    Command::new(&real_git)
        .args([
            "-C",
            source.to_str().unwrap(),
            "config",
            "user.email",
            "fixture@example.invalid",
        ])
        .status()
        .unwrap();
    Command::new(&real_git)
        .args([
            "-C",
            source.to_str().unwrap(),
            "config",
            "user.name",
            "Fixture",
        ])
        .status()
        .unwrap();
    fs::write(source.join("README.md"), "fixture\n").unwrap();
    Command::new(&real_git)
        .args(["-C", source.to_str().unwrap(), "add", "README.md"])
        .status()
        .unwrap();
    Command::new(&real_git)
        .args([
            "-C",
            source.to_str().unwrap(),
            "commit",
            "-q",
            "-m",
            "fixture",
        ])
        .status()
        .unwrap();
    let fake_bin = root.join("bin");
    fs::create_dir(&fake_bin).unwrap();
    let transport_log = root.join("git-transport.log");
    write_executable(
        &fake_bin.join("git"),
        &format!("#!/bin/sh\nprintf '%s|%s\\n' \"${{GH_TOKEN:-}}\" \"$*\" >> {transport_log:?}\nexec {real_git:?} \"$@\"\n"),
    );

    let ssh_stub = root.join("ssh-stub.sh");
    write_executable(&ssh_stub, SSH_STUB);
    let command_stub = root.join("command-stub.sh");
    write_executable(&command_stub, COMMAND_STUB);

    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    fs::write(
        root.join("agents/remote.toml"),
        format!(
            "version = 1\nharness = \"command\"\nmodel = \"\"\nbin = \"{}\"\ncheckout_secrets = [\"GH_TOKEN\"]\n",
            command_stub.display()
        ),
    )
    .unwrap();
    fs::write(root.join("tasks/demo/card.md"), "# Tailnet demo card\n").unwrap();
    fs::write(
        root.join("tasks/demo/task.toml"),
        format!("agent = \"remote\"\nsubstrate = \"tailnet\"\n\n[workspace]\nhost = \"test-tailnet-host\"\nrepos = [{{ url = {:?}, ref = \"master\" }}]\n\n[[trigger]]\nkind = \"manual\"\n", source.to_string_lossy()),
    )
    .unwrap();
    let plane = Plane::load(root).unwrap();
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let log = root.join("ssh-stub.log");
    std::env::set_var("BB_SSH_BIN", &ssh_stub);
    std::env::set_var("SSH_STUB_LOG", &log);
    std::env::set_var("SSH_FAKE_HOME", &fake_home);
    let old_path = std::env::var("PATH").unwrap();
    std::env::set_var("PATH", format!("{}:{old_path}", fake_bin.display()));
    std::env::set_var("GH_TOKEN", "transport-token");
    std::env::set_var("BB_GIT_TRANSPORT_LOG", &transport_log);

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
    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();

    std::env::remove_var("BB_SSH_BIN");
    std::env::remove_var("SSH_STUB_LOG");
    std::env::remove_var("SSH_FAKE_HOME");
    std::env::remove_var("GH_TOKEN");
    std::env::remove_var("BB_GIT_TRANSPORT_LOG");
    std::env::set_var("PATH", old_path);

    assert_eq!(run.state, "success", "reason: {:?}", run.state_reason);
    let attempts = ledger.attempts(&run_id).unwrap();
    assert_eq!(attempts.len(), 1);
    assert_eq!(attempts[0].outcome.as_deref(), Some("success"));
    let artifact_dir = Path::new(attempts[0].artifact_dir.as_deref().unwrap());
    let report = fs::read_to_string(artifact_dir.join("REPORT.json")).unwrap();
    assert!(report.contains("bb.command_result.v1"));
    assert!(
        fs::read_to_string(&transport_log)
            .unwrap()
            .contains("transport-token|"),
        "clone transport did not receive GH_TOKEN"
    );
    let ssh_log = fs::read_to_string(&log).unwrap();
    assert!(
        !ssh_log.contains("transport-token"),
        "checkout token leaked into ssh argv: {ssh_log}"
    );

    // The card was materialized into the (fake) remote workspace over ssh.
    let prepared_workspace = fs::read_dir(fake_home.join(".bb-tailnet"))
        .unwrap()
        .filter_map(Result::ok)
        .map(|entry| entry.path())
        .find(|path| {
            path.file_name()
                .and_then(|name| name.to_str())
                .is_some_and(|name| name.starts_with("demo-bb-attempt-"))
        })
        .expect("attempt-scoped tailnet workspace");
    assert!(prepared_workspace.join("LANE_CARD.md").exists());
    assert!(prepared_workspace.join("RUN.json").exists());
}

#[test]
fn tailnet_acquire_fails_with_plain_language_when_host_unreachable() {
    let _guard = ENV_LOCK.lock().unwrap();
    let dir = tempfile::tempdir().unwrap();
    let ssh_stub = dir.path().join("ssh-fail.sh");
    write_executable(
        &ssh_stub,
        "#!/bin/sh\necho 'ssh: connect to host unreachable-host port 22: Operation timed out' >&2\nexit 255\n",
    );
    std::env::set_var("BB_SSH_BIN", &ssh_stub);

    let attempt_dir = dir.path().join("attempt");
    let result = TailnetSubstrate.acquire("unreachable-host", &attempt_dir);

    std::env::remove_var("BB_SSH_BIN");

    let Err(err) = result else {
        panic!("expected acquire to fail against an unreachable host");
    };
    let msg = format!("{err:#}");
    assert!(msg.contains("unreachable-host"), "{msg}");
    assert!(msg.contains("unreachable"), "{msg}");
}

/// bitterblossom-938: the edge-case policy for "no reachable host" is the
/// plane's existing dead-letter queue, not a new parking mechanism --
/// mechanical retries (MAX_RETRIES) then a dead letter with a
/// plain-language reason, exactly like any other pre-execute failure. The
/// plane holds no judgment about *which* substrate to fall back to (that
/// would be a decision, not a mechanism); `bb dlq replay` is the declared
/// "queue for later" path once the host is confirmed reachable again.
#[test]
fn tailnet_dispatch_dead_letters_with_plain_language_reason_when_host_unreachable() {
    let _guard = ENV_LOCK.lock().unwrap();
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path();

    let ssh_fail = root.join("ssh-fail.sh");
    write_executable(
        &ssh_fail,
        "#!/bin/sh\necho 'ssh: connect to host gone-host port 22: Operation timed out' >&2\nexit 255\n",
    );
    let command_stub = root.join("command-stub.sh");
    write_executable(&command_stub, COMMAND_STUB);

    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    fs::write(
        root.join("agents/remote.toml"),
        format!(
            "version = 1\nharness = \"command\"\nmodel = \"\"\nbin = \"{}\"\n",
            command_stub.display()
        ),
    )
    .unwrap();
    fs::write(root.join("tasks/demo/card.md"), "# card\n").unwrap();
    fs::write(
        root.join("tasks/demo/task.toml"),
        "agent = \"remote\"\nsubstrate = \"tailnet\"\n\n[workspace]\nhost = \"gone-host\"\n\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
    let plane = Plane::load(root).unwrap();
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    std::env::set_var("BB_SSH_BIN", &ssh_fail);

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
    let run = dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();

    std::env::remove_var("BB_SSH_BIN");

    assert_eq!(run.state, "failure");
    let reason = run.state_reason.as_deref().unwrap_or_default();
    assert!(reason.starts_with("dead_letter:"), "{reason}");
    assert!(reason.contains("gone-host"), "{reason}");
    assert!(reason.contains("unreachable"), "{reason}");
    let dlq = ledger.list_dead_letters().unwrap();
    assert_eq!(dlq.len(), 1);
    assert_eq!(dlq[0].task, "demo");
}

#[test]
fn tailnet_probe_rejects_nonpositive_pidfile() {
    let _guard = ENV_LOCK.lock().unwrap();
    let dir = tempfile::tempdir().unwrap();
    let fake_home = dir.path().join("fake-remote-home");
    fs::create_dir_all(fake_home.join(".bb-tailnet")).unwrap();
    fs::write(
        fake_home.join(".bb-tailnet/probe-marker.pid"),
        "0|never-started",
    )
    .unwrap();

    let ssh_stub = dir.path().join("ssh-stub.sh");
    write_executable(&ssh_stub, SSH_STUB);
    std::env::set_var("BB_SSH_BIN", &ssh_stub);
    std::env::set_var("SSH_FAKE_HOME", &fake_home);
    std::env::set_var("SSH_STUB_LOG", dir.path().join("probe.log"));

    let result = TailnetSubstrate.probe("test-tailnet-host", dir.path(), "probe-marker");

    std::env::remove_var("BB_SSH_BIN");
    std::env::remove_var("SSH_FAKE_HOME");
    std::env::remove_var("SSH_STUB_LOG");

    assert!(
        matches!(result, ProbeResult::Unknown(ref reason) if reason.contains("malformed pidfile")),
        "{result:?}"
    );
}
