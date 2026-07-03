use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::process::{Command, Output, Stdio};
use std::time::{Duration, Instant};

use bitterblossom::{dispatch, ledger::Ledger, spec::Plane};

fn write_executable(path: &std::path::Path, content: &str) {
    fs::write(path, content).unwrap();
    fs::set_permissions(path, fs::Permissions::from_mode(0o755)).unwrap();
}

fn write_plane(root: &std::path::Path) {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
    let stub = root.join("slow.sh");
    write_executable(&stub, "#!/bin/sh\ncat > /dev/null\nsleep 1\necho done\n");
    fs::write(
        root.join("agents/stub.toml"),
        format!(
            "version = 1\nharness = \"command\"\nmodel = \"\"\nbin = \"{}\"\n",
            stub.display()
        ),
    )
    .unwrap();
    fs::write(root.join("tasks/demo/card.md"), "card\n").unwrap();
    fs::write(
        root.join("tasks/demo/task.toml"),
        "agent = \"stub\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
}

fn write_dispatch_plane(root: &std::path::Path) {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/dispatch")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
    let stub = root.join("dispatch-agent.sh");
    write_executable(
        &stub,
        r#"#!/bin/sh
printf 'dispatch-agent-start\n'
cat EVENT.json
printf '\ndispatch-agent-done\n'
printf '{"ok":true}\n' > REPORT.json
"#,
    );
    fs::write(
        root.join("agents/stub.toml"),
        format!(
            "version = 1\nharness = \"command\"\nmodel = \"\"\nbin = \"{}\"\n",
            stub.display()
        ),
    )
    .unwrap();
    fs::write(root.join("tasks/dispatch/card.md"), "dispatch card\n").unwrap();
    fs::write(
        root.join("tasks/dispatch/task.toml"),
        "agent = \"stub\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
}

fn bb(root: &str, args: &[&str]) -> Output {
    let mut cmd = Command::new(env!("CARGO_BIN_EXE_bb"));
    cmd.arg("--config")
        .arg(root)
        .args(args)
        .env("BB_RUN_HEARTBEAT_MS", "100")
        .env("BB_LOGS_POLL_MS", "50")
        .output()
        .unwrap()
}

fn bb_with_timeout(root: &str, args: &[&str], timeout: Duration) -> Output {
    let mut child = Command::new(env!("CARGO_BIN_EXE_bb"))
        .arg("--config")
        .arg(root)
        .args(args)
        .env("BB_RUN_HEARTBEAT_MS", "100")
        .env("BB_LOGS_POLL_MS", "50")
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .unwrap();
    let deadline = Instant::now() + timeout;
    loop {
        if child.try_wait().unwrap().is_some() {
            return child.wait_with_output().unwrap();
        }
        if Instant::now() >= deadline {
            let _ = child.kill();
            let output = child.wait_with_output().unwrap();
            panic!(
                "bb command timed out: {:?}\nstdout:\n{}\nstderr:\n{}",
                args,
                String::from_utf8_lossy(&output.stdout),
                String::from_utf8_lossy(&output.stderr)
            );
        }
        std::thread::sleep(Duration::from_millis(25));
    }
}

fn spawn_dispatch_worker(root: &std::path::Path, run_id: &str) -> std::thread::JoinHandle<()> {
    let root = root.to_path_buf();
    let run_id = run_id.to_string();
    std::thread::spawn(move || {
        let plane = Plane::load(&root).unwrap();
        let mut ledger = Ledger::open(&plane.db_path()).unwrap();
        dispatch::dispatch_run(&plane, &mut ledger, &run_id).unwrap();
    })
}

#[test]
fn run_human_mode_prints_early_receipt_and_heartbeat_without_json_noise() {
    let dir = tempfile::tempdir().unwrap();
    write_plane(dir.path());
    let root = dir.path().to_str().unwrap();

    let human = bb(root, &["run", "demo"]);
    assert!(
        human.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&human.stdout),
        String::from_utf8_lossy(&human.stderr)
    );
    let human_stdout = String::from_utf8_lossy(&human.stdout);
    let human_stderr = String::from_utf8_lossy(&human.stderr);
    assert!(human_stdout.contains("run "));
    assert!(human_stdout.contains(" success "));
    assert!(human_stderr.contains("accepted"));
    assert!(human_stderr.contains("heartbeat"));
    assert!(human_stderr.contains("state=running"));

    let json = bb(root, &["run", "demo", "--json"]);
    assert!(
        json.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&json.stdout),
        String::from_utf8_lossy(&json.stderr)
    );
    let doc: serde_json::Value = serde_json::from_slice(&json.stdout).unwrap();
    assert_eq!(doc["run"]["state"], "success");
    let json_stderr = String::from_utf8_lossy(&json.stderr);
    assert!(!json_stderr.contains("heartbeat"));
    assert!(!json_stderr.contains("accepted"));
}

#[test]
fn dispatch_enqueues_brief_payload_and_logs_follow_terminal_transcript() {
    let dir = tempfile::tempdir().unwrap();
    write_dispatch_plane(dir.path());
    let root = dir.path().to_str().unwrap();
    let brief = dir.path().join("brief.md");
    fs::write(&brief, "Say hello from the brief.").unwrap();

    let dispatch = bb(
        root,
        &[
            "dispatch",
            "--repo",
            dir.path().to_str().unwrap(),
            "--brief",
            brief.to_str().unwrap(),
            "--model",
            "openrouter/test-model",
            "--label",
            "codex-bb-dispatch",
        ],
    );
    assert!(
        dispatch.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&dispatch.stdout),
        String::from_utf8_lossy(&dispatch.stderr)
    );
    let run_id = String::from_utf8(dispatch.stdout)
        .unwrap()
        .trim()
        .to_string();
    assert!(!run_id.is_empty());

    let worker = spawn_dispatch_worker(dir.path(), &run_id);
    let logs = bb_with_timeout(root, &["logs", "-f", &run_id], Duration::from_secs(5));
    worker.join().unwrap();
    assert!(
        logs.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&logs.stdout),
        String::from_utf8_lossy(&logs.stderr)
    );
    let transcript = String::from_utf8_lossy(&logs.stdout);
    assert!(transcript.contains("state:running"), "{transcript}");
    assert!(transcript.contains("phase:executing"), "{transcript}");
    assert!(transcript.contains("stdout.txt"), "{transcript}");
    assert!(transcript.contains("dispatch-agent-start"), "{transcript}");
    assert!(
        transcript.contains("Say hello from the brief."),
        "{transcript}"
    );
    assert!(transcript.contains("openrouter/test-model"), "{transcript}");
    assert!(transcript.contains("codex-bb-dispatch"), "{transcript}");
    assert!(
        transcript.contains("terminal state=success"),
        "{transcript}"
    );

    let show = bb(root, &["runs", "show", &run_id, "--json"]);
    assert!(
        show.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&show.stdout),
        String::from_utf8_lossy(&show.stderr)
    );
    let doc: serde_json::Value = serde_json::from_slice(&show.stdout).unwrap();
    assert_eq!(doc["attempts"][0]["model"], "openrouter/test-model");
}
