use std::fs;
use std::path::Path;
use std::process::Command;

fn help(args: &[&str]) -> String {
    let output = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args(args)
        .output()
        .unwrap();
    assert!(
        output.status.success(),
        "stdout:\n{}\nstderr:\n{}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );
    format!(
        "{}\n{}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    )
}

fn read(rel: &str) -> String {
    let root = Path::new(env!("CARGO_MANIFEST_DIR"));
    fs::read_to_string(root.join(rel)).unwrap()
}

fn repo_files_under(root: &Path, rel: &str, out: &mut Vec<String>) {
    let path = root.join(rel);
    if path.is_file() {
        out.push(rel.to_string());
        return;
    }
    for entry in fs::read_dir(&path).unwrap() {
        let path = entry.unwrap().path();
        let name = path.file_name().unwrap().to_string_lossy();
        let child = format!("{rel}/{name}");
        if path.is_dir() {
            repo_files_under(root, &child, out);
        } else if matches!(
            path.extension().and_then(|e| e.to_str()),
            Some("md" | "txt" | "sh" | "rs" | "yml" | "yaml")
        ) {
            out.push(child);
        }
    }
}

fn has_stale_go_test_command(text: &str) -> bool {
    text.match_indices("go test").any(|(idx, _)| {
        let before = text[..idx].chars().next_back();
        let after = text[idx + "go test".len()..].chars().next();
        let before_boundary = before.is_none_or(|c| !c.is_ascii_alphanumeric() && c != '_');
        let after_boundary = after.is_none_or(|c| c.is_whitespace() || c == '.' || c == '/');
        before_boundary && after_boundary
    })
}

fn historical_doc_context(rel: &str) -> bool {
    rel.starts_with("docs/archive/")
        || rel.starts_with("docs/plans/")
        || rel.starts_with("docs/audits/")
        || rel.starts_with("docs/audit-reports/")
        || rel.starts_with("docs/shakedowns/")
        || rel.starts_with("docs/walkthroughs/")
        || rel == "docs/stale-doc-inventory.md"
        || matches!(
            rel,
            "docs/adr/001-claude-code-canonical-harness.md"
                | "docs/adr/002-architecture-minimalism.md"
                | "docs/adr/003-conductor-control-plane.md"
                | "docs/adr/004-bounded-review-governance.md"
                | "docs/adr/004-elixir-conductor-architecture.md"
        )
}

#[test]
fn live_help_exposes_current_agent_cli_contract() {
    let run = help(&["run", "--help"]);
    assert!(run.contains("Usage: bb run [OPTIONS] <TASK>"));
    assert!(run.contains("--payload <PAYLOAD>"));
    assert!(run.contains("--payload-file <PAYLOAD_FILE>"));
    assert!(run.contains("--json"));
    assert!(!run.contains("--var"));

    let export = help(&["runs", "export", "--help"]);
    assert!(export.contains("Usage: bb runs export [OPTIONS]"));
    assert!(!export.contains("--since"));

    let unpark = help(&["task", "unpark", "--help"]);
    assert!(unpark.contains("Usage: bb task unpark [OPTIONS] <TASK>"));
    assert!(unpark.contains("--since <SINCE>"));
    assert!(unpark.contains("--run-id <RUN_IDS>"));
    assert!(unpark.contains("--yes"));

    let artifacts = help(&["artifacts", "--help"]);
    assert!(artifacts.contains("bundle"));
    let bundle = help(&["artifacts", "bundle", "--help"]);
    assert!(bundle.contains("Usage: bb artifacts bundle [OPTIONS] --out <OUT> <RUN_ID>"));
    assert!(bundle.contains("--out <OUT>"));

    let gate = help(&["gate", "--help"]);
    assert!(gate.contains("--submission <SUBMISSION>"));
    assert!(gate.contains("--change <CHANGE>"));
    assert!(gate.contains("--json"));

    let keys = help(&["keys", "--help"]);
    assert!(keys.contains("sync"));
    let key_sync = help(&["keys", "sync", "--help"]);
    assert!(key_sync.contains("--check"));
    assert!(key_sync.contains("--json"));
}

#[test]
fn current_docs_and_skills_match_live_cli_contract() {
    let current_contracts = [
        "README.md",
        "VISION.md",
        "docs/spine.md",
        "skills/bitterblossom/SKILL.md",
        "skills/bitterblossom/references/operator-recipes.md",
        ".agents/skills/bb-dogfood/SKILL.md",
        ".agents/skills/bb-dogfood/references/session-notes-template.md",
    ];
    for rel in current_contracts {
        let text = read(rel);
        assert!(!text.contains("--var"), "{rel} documents stale --var");
    }

    let spine = read("docs/spine.md");
    assert!(spine.contains(
        "bb run <task> [--idempotency-key K] [--payload JSON | --payload-file PATH] [--json]"
    ));
    assert!(spine.contains("bb task list [--json]"));
    assert!(spine.contains("bb runs export"));
    assert!(spine.contains("bb gate --change K | --submission ID [--json]"));
    assert!(spine.contains("bb keys sync <agent> | --all [--check] [--json]"));

    let skill = read("skills/bitterblossom/SKILL.md");
    assert!(skill.contains("bb --config <plane> run <task> --payload '<json>' --json"));
    assert!(skill.contains("bb --config <plane> runs export"));
    assert!(skill.contains("bb --config <plane> gate --submission <submission> --json"));
    assert!(skill.contains("bb --config <plane> keys sync --all --check --json"));

    let recipes = read("skills/bitterblossom/references/operator-recipes.md");
    assert!(recipes.contains("bb --config <plane> runs export"));
    assert!(recipes.contains("bb --config <plane> dlq replay <id> --json"));
    assert!(recipes.contains("curl --config -"));
    assert!(recipes.contains("bb --config <plane> keys sync --all --check --json"));
    assert!(!recipes.contains("curl -H \"Authorization: Bearer $BB_API_TOKEN\""));

    let dogfood = read(".agents/skills/bb-dogfood/SKILL.md");
    assert!(dogfood.contains("./target/debug/bb --config \"$BB_RUNTIME_PLANE\" status --json"));
    assert!(dogfood.contains(
        "./target/debug/bb --config \"$BB_RUNTIME_PLANE\" gate --submission <submission> --json"
    ));
}

#[test]
fn stale_operational_commands_stay_out_of_current_guidance() {
    let root = Path::new(env!("CARGO_MANIFEST_DIR"));
    let mut files = Vec::new();
    for rel in [
        "README.md",
        "CLAUDE.md",
        "AGENTS.md",
        "docs",
        "skills",
        ".agents/skills",
        "scripts",
    ] {
        repo_files_under(root, rel, &mut files);
    }

    let mut offenders = Vec::new();
    for rel in files {
        if historical_doc_context(&rel) {
            continue;
        }
        let text = read(&rel);
        let stale =
            text.contains("cmd/bb") || text.contains("--var") || has_stale_go_test_command(&text);
        if stale {
            offenders.push(rel);
        }
    }

    assert!(
        offenders.is_empty(),
        "stale operational commands in current guidance: {offenders:?}"
    );
}

#[test]
fn historical_adrs_are_explicitly_superseded() {
    for rel in [
        "docs/adr/001-claude-code-canonical-harness.md",
        "docs/adr/002-architecture-minimalism.md",
        "docs/adr/003-conductor-control-plane.md",
        "docs/adr/004-bounded-review-governance.md",
        "docs/adr/004-elixir-conductor-architecture.md",
    ] {
        let text = read(rel);
        assert!(
            text.contains("Superseded for current Bitterblossom operation by"),
            "{rel} must warn readers that it is historical"
        );
        assert!(text.contains("005-rust-event-plane.md"));
        assert!(text.contains("../spine.md"));
    }
}

#[test]
fn walkthrough_terminal_transcripts_are_archived() {
    let root = Path::new(env!("CARGO_MANIFEST_DIR"));
    let live = root.join("docs/walkthroughs");
    for entry in fs::read_dir(&live).unwrap() {
        let path = entry.unwrap().path();
        let name = path.file_name().unwrap().to_string_lossy();
        assert!(
            !name.ends_with("-terminal.txt"),
            "{} should live under docs/archive/walkthrough-terminal-transcripts/",
            path.display()
        );
    }

    for rel in [
        "docs/archive/walkthrough-terminal-transcripts/codex-simplify-bb-sprite-transport-terminal.txt",
        "docs/archive/walkthrough-terminal-transcripts/codex-simplify-bb-workspace-contract-terminal.txt",
        "docs/archive/walkthrough-terminal-transcripts/codex-simplify-governance-session-terminal.txt",
        "docs/archive/walkthrough-terminal-transcripts/issue-505-qa-intake-terminal.txt",
        "docs/archive/walkthrough-terminal-transcripts/issue-529-trusted-thread-metadata-terminal.txt",
    ] {
        assert!(root.join(rel).exists(), "{rel} missing from transcript archive");
    }
}

#[test]
fn operations_runbook_and_drill_are_wired_into_the_gate() {
    let ops = read("docs/operations/README.md");
    for needle in [
        "local-primary",
        "com.misty-step.bb-serve",
        "127.0.0.1:7093",
        "dev = false",
        "allow_local_substrate = true",
        "plane/.bb/plane.db",
        "LITESTREAM_REPLICA_URL",
        "/api/status",
        "mode=ro&immutable=0",
        "PRAGMA integrity_check",
        "SIGTERM",
        "launchctl kickstart",
        "--dev-temp",
        "backup",
        "restore",
        "rollback",
        "READINESS BLOCKED",
        "status = \"open\"",
        "BB_ENV_FILE",
        "0600",
        "hosted-app-platform-reference.md",
    ] {
        assert!(
            ops.contains(needle),
            "operations runbook missing {needle:?}"
        );
    }
    assert!(!ops.contains("127.0.0.1:7077"));
    assert!(!ops.contains("doctl"));
    assert!(!ops.contains("ondigitalocean"));

    let script = read("scripts/production-ops-drill.sh");
    assert!(script.contains("primary_api_auth"));
    assert!(script.contains("BB_ENV_FILE"));
    for needle in [
        "--primary",
        "--dev-temp",
        "127.0.0.1:7093",
        "allow_local_substrate = true",
        "/api/status",
        "/api/dlq",
        "journal_mode",
        "integrity_check",
        "SIGTERM",
        "restore_read_surface_check",
        "READINESS BLOCKED",
        "BB_ENV_FILE",
        "primary-status-no-auth",
        "primary-status-auth",
    ] {
        assert!(script.contains(needle), "drill missing {needle:?}");
    }
    assert!(!script.contains("--remote"));
    assert!(!script.contains("7077"));
    assert!(!script.contains("doctl"));
    assert!(!script.contains("ondigitalocean"));
    let primary = script
        .split("run_primary() {")
        .nth(1)
        .and_then(|tail| tail.split("case \"$MODE\" in").next())
        .expect("primary function");
    for forbidden in [
        "doctor",
        " recover ",
        " status --json",
        " runs list",
        " dlq list",
        " serve",
    ] {
        assert!(
            !primary.contains(forbidden),
            "primary path invokes forbidden CLI: {forbidden}"
        );
    }

    let verify = read("scripts/verify.sh");
    assert!(verify.contains("scripts/production-ops-drill.sh --dev-temp"));
    assert!(!verify.contains("scripts/production-ops-drill.sh --local"));

    let reference = read("docs/archive/operations/hosted-app-platform-reference.md");
    assert!(reference.contains("Historical reference only"));
    assert!(reference.contains("DigitalOcean"));
}

#[test]
fn current_operations_docs_use_local_primary_and_keep_dev_temp_separate() {
    for rel in [
        "README.md",
        "docs/spine.md",
        "docs/credential-refusal-doctrine.md",
        "skills/bitterblossom/SKILL.md",
        "skills/bitterblossom/references/operator-recipes.md",
    ] {
        let text = read(rel);
        assert!(
            text.contains("127.0.0.1:7093"),
            "{rel} must name the live bind"
        );
        assert!(
            text.contains("local-primary"),
            "{rel} must name local-primary"
        );
        assert!(
            !text.contains("127.0.0.1:7077"),
            "{rel} carries stale 7077 guidance"
        );
        assert!(
            !text.contains("ondigitalocean"),
            "{rel} carries stale hosted URL"
        );
    }
}

#[test]
fn product_default_bind_is_ephemeral_for_unconfigured_planes() {
    let spec = read("src/spec.rs");
    assert!(spec.contains("\"127.0.0.1:0\".to_string()"));
    assert!(!spec.contains("\"127.0.0.1:7093\".to_string()"));
}

#[test]
fn local_primary_assets_pin_launchd_and_fixture_boundaries() {
    let vision = read("VISION.md");
    for needle in [
        "com.misty-step.bb-serve",
        "dev = false",
        "allow_local_substrate = true",
        "127.0.0.1:7093",
        "127.0.0.1:7091",
        "127.0.0.1:7077",
        "interactive lead sessions are not part of the current product boundary",
    ] {
        assert!(vision.contains(needle), "VISION missing {needle:?}");
    }
    let installer = read("scripts/install-bb-local-primary.sh");
    assert!(installer.contains("--retire-legacy-dashboard"));
    assert!(installer.contains("mv -f"));
    assert!(installer.contains("bb.previous"));
    assert!(installer.contains("plutil -lint"));
    assert!(installer.contains("wait_for_bootout"));
    for needle in [
        "target/release/bb",
        "BB_INSTALL_DIR",
        "mktemp",
        "mv -f",
        "--retire-legacy-dashboard",
        "com.misty-step.bb-serve",
        "com.misty-step.bb-plane-litestream",
        "127.0.0.1:7093",
        "dev = false",
        "allow_local_substrate = true",
    ] {
        assert!(installer.contains(needle), "installer missing {needle:?}");
    }
    let env_helper = read("scripts/bb-operator-env.sh");
    assert!(env_helper.contains("BB_ENV_FILE"));
    assert!(env_helper.contains(".env.bb.local-primary"));
    assert!(env_helper.contains("0600"));
    let entrypoint = read("scripts/bb-serve-local-entrypoint.sh");
    assert!(entrypoint.contains("BB_LOCAL_PRIMARY_BIN"));
    assert!(entrypoint.contains(".local/libexec/bitterblossom/bb"));
    assert!(!entrypoint.contains("target/release/bb"));
    let plist = read("deploy/launchd/com.misty-step.bb-serve.plist.template");
    assert!(plist.contains("com.misty-step.bb-serve"));
    assert!(plist.contains("bb-serve-local-entrypoint.sh"));
    assert!(plist.contains("__BB_INSTALL_BIN__"));
    assert!(!plist.contains("target/release/bb"));
    let sidecar = read("deploy/launchd/com.misty-step.bb-plane-litestream.plist.template");
    assert!(sidecar.contains("com.misty-step.bb-plane-litestream"));
    assert!(sidecar.contains("bb-litestream-local-entrypoint.sh"));
    let serve_path = read("deploy/launchd/com.misty-step.bb-serve.plist.template");
    assert!(serve_path.contains("/usr/bin:/bin:/usr/sbin"));
    assert!(sidecar.contains("/usr/bin:/bin:/usr/sbin"));
    assert_eq!(
        serve_path
            .matches("/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin")
            .count(),
        1
    );
    assert_eq!(
        sidecar
            .matches("/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin")
            .count(),
        1
    );
    let dashboard = read("docs/archive/operations/bb-dashboard.md");
    assert!(dashboard.contains("Historical reference only"));
    assert!(dashboard.contains("7091"));
    assert!(dashboard.contains("dev") || dashboard.contains("demo"));
}

#[test]
fn refactor_stays_a_read_only_review_lens_not_a_dispatch_workload() {
    let root = Path::new(env!("CARGO_MANIFEST_DIR"));
    assert!(
        !root.join("plane/tasks/refactor").exists(),
        "071 decided not to add a mutating refactor workload"
    );

    let decision = read("docs/refactor-lens.md");
    assert!(decision.contains("Refactor remains a read-only review lens"));
    assert!(decision.contains("canonical implementation is the existing `simplification`"));
    assert!(decision.contains("no auto-merge"));
    assert!(decision.contains("re-enters the submission storm"));
    assert!(decision.contains("do not add `plane/tasks/refactor`"));

    let spine = read("docs/spine.md");
    assert!(spine.contains("`simplification` is the read-only refactor lens"));
    assert!(spine.contains("standalone mutating refactor workload in v1"));

    let fixture = read("tests/fixtures/model-eval-plane/tasks/simplification/card.md");
    assert!(fixture.contains("canonical simplification gate member"));
}
