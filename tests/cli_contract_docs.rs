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
        "docs/spine.md",
        "skills/bitterblossom/SKILL.md",
        "skills/bitterblossom/references/operator-recipes.md",
        ".agents/skills/bb-dogfood/SKILL.md",
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
    assert!(ops.contains("scripts/production-ops-drill.sh --remote"));
    assert!(ops.contains("scripts/production-ops-drill.sh --local"));
    assert!(ops.contains("doctl apps get \"$BB_DO_APP_ID\""));
    assert!(ops.contains("doctl apps get-deployment \"$BB_DO_APP_ID\""));
    assert!(ops.contains("doctl apps list-deployments \"$BB_DO_APP_ID\""));
    assert!(ops.contains("doctl apps logs \"$BB_DO_APP_ID\" plane --type run"));
    assert!(ops.contains("BB_LITESTREAM_REQUIRED=1"));
    assert!(ops.contains("BB_LITESTREAM_REPLICA_URL_ENV=LITESTREAM_REPLICA_URL"));
    assert!(ops.contains("BB_PLANE_CONFIG_URL"));
    assert!(ops.contains("litestream restore -config /tmp/bb-litestream.yml"));
    assert!(ops.contains("ledger.schema_version"));
    assert!(ops.contains("newer than the rollback target supports"));
    assert!(ops.contains("Do not edit `PRAGMA user_version`"));
    assert!(ops.contains("backup.status == \"fresh\""));
    assert!(ops.contains("last_success_path = \".bb/backup-last-success\""));
    assert!(ops.contains("git revert <bad-commit>"));
    assert!(ops.contains("doctl apps console \"$BB_DO_APP_ID\" plane"));
    assert!(ops.contains("bb dlq replay <id> --json"));
    assert!(ops.contains("bb dlq ack <id> --reason <text>"));
    assert!(!ops.contains("there is no first-class acknowledge"));
    assert!(!ops.contains("?token=$BB_API_TOKEN"));

    let script = read("scripts/production-ops-drill.sh");
    assert!(script.contains("backup_restore_check"));
    assert!(script.contains("restore_read_surface_check"));
    assert!(script.contains("gate --change ops-drill --json"));
    assert!(script.contains("backup status was not fresh"));
    assert!(script.contains("expect_bearer_code remote-tasks"));
    assert!(script.contains("--do-app-id"));
    assert!(script.contains("doctl apps get \"$BB_DO_APP_ID\""));
    assert!(script.contains("doctl apps get-deployment \"$BB_DO_APP_ID\""));
    assert!(script.contains("phase was not ACTIVE"));
    assert!(script.contains("curl --config -"));
    assert!(!script.contains("-H \"Authorization: Bearer $BB_API_TOKEN\""));
    assert!(!script.contains("?token="));

    let dockerfile = read("Dockerfile");
    assert!(dockerfile.contains("ARG LITESTREAM_VERSION=0.5.13"));
    assert!(dockerfile.contains("ca-certificates git curl openssh-client passwd socat util-linux"));
    assert!(dockerfile.contains("useradd --system --create-home"));
    assert!(dockerfile.contains("chown bb:bb \"$BB_PLANE_DIR\""));
    assert!(dockerfile.contains("litestream-${LITESTREAM_VERSION}-linux-${litestream_arch}.tar.gz"));
    assert!(dockerfile.contains("FROM tailscale/tailscale:stable@sha256:"));
    assert!(dockerfile.contains("COPY --from=tailscale /usr/local/bin/tailscaled"));
    assert!(dockerfile.contains("COPY --from=tailscale /usr/local/bin/tailscale"));
    assert!(dockerfile.contains("ENTRYPOINT [\"/usr/local/bin/bb-mint-tailnet-entrypoint\"]"));
    assert!(!dockerfile.contains("LITESTREAM_REPLICA_URL="));

    let mint_container_smoke = read("scripts/mint-tailnet-container-smoke.sh");
    assert!(mint_container_smoke.contains("NoNewPrivs:"));
    assert!(mint_container_smoke.contains("Cap(Inh|Prm|Eff|Bnd|Amb):"));
    assert!(mint_container_smoke.contains("Mint Powder capability probe failed; stopping bb"));
    assert!(mint_container_smoke.contains("/run/bb-mint/tailscaled.sock"));
    assert!(mint_container_smoke.contains("__mint.powder.bitterblossom__"));

    let ci = read(".github/workflows/ci.yml");
    assert!(ci.contains("scripts/mint-tailnet-container-smoke.sh"));

    assert!(!Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("fly.toml")
        .exists());

    let entrypoint = read("scripts/bb-litestream-entrypoint.sh");
    assert!(entrypoint.contains("litestream replicate -config \"$config_path\""));
    assert!(entrypoint.contains(
        "litestream restore -if-replica-exists -o \"$db_path\" -config \"$config_path\" \"$db_path\""
    ));
    assert!(entrypoint
        .contains("litestream sync -socket \"$socket_path\" -wait -timeout \"$sync_timeout\""));
    assert!(entrypoint.contains("url: ${%s}"));
    assert!(entrypoint.contains("date -u '+%Y-%m-%dT%H:%M:%SZ' >\"$heartbeat_path\""));
    assert!(entrypoint.contains("BB_TAILNET_SSH_PRIVATE_KEY"));
    assert!(entrypoint.contains("BB_TAILNET_SSH_KNOWN_HOSTS"));
    assert!(entrypoint.contains("BB_TAILNET_SSH_DIR:-/root/.ssh"));
    assert!(entrypoint.contains("chmod 0700 \"$ssh_dir\""));
    assert!(entrypoint.contains("chmod 0600 \"$ssh_dir/id_ed25519\""));
    assert!(entrypoint.contains("chmod 0600 \"$ssh_dir/known_hosts\""));

    let verify = read("scripts/verify.sh");
    assert!(verify.contains("scripts/production-ops-drill.sh --local"));
}

#[test]
fn active_operations_cannot_recreate_the_retired_fly_app() {
    for rel in [
        "docs/operations/README.md",
        "docs/spine.md",
        "scripts/production-ops-drill.sh",
        ".agents/skills/bb-dogfood/SKILL.md",
        ".agents/skills/bb-dogfood/references/session-notes-template.md",
    ] {
        let content = read(rel);
        for forbidden in [
            "BB_FLY_APP",
            "flyctl deploy",
            "flyctl status",
            "flyctl volumes",
            "flyctl ssh",
            "flyctl secrets",
            "flyctl releases",
        ] {
            assert!(
                !content.contains(forbidden),
                "{rel} still contains retired hosted-app operation `{forbidden}`"
            );
        }
    }
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
