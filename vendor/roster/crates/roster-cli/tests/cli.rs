use assert_cmd::Command;
use predicates::prelude::*;
use std::{
    collections::BTreeMap,
    fs,
    io::{Read, Write},
    net::TcpListener,
    path::PathBuf,
    thread::{self, JoinHandle},
};

fn workspace_root() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .parent()
        .and_then(|path| path.parent())
        .expect("workspace root")
        .to_path_buf()
}

fn roster_cmd() -> Command {
    let mut command = Command::cargo_bin("roster").expect("roster binary");
    command.current_dir(workspace_root());
    command
}

struct PowderStub {
    base_url: String,
    handle: JoinHandle<String>,
}

impl PowderStub {
    fn once(status: &str, body: &str) -> Self {
        let listener = TcpListener::bind("127.0.0.1:0").expect("bind powder stub");
        let base_url = format!("http://{}", listener.local_addr().expect("stub addr"));
        let status = status.to_string();
        let body = body.to_string();
        let handle = thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept powder request");
            let mut request = Vec::new();
            let mut buffer = [0; 1024];
            loop {
                let read = stream.read(&mut buffer).expect("read powder request");
                if read == 0 {
                    break;
                }
                request.extend_from_slice(&buffer[..read]);
                if request.windows(4).any(|window| window == b"\r\n\r\n") {
                    break;
                }
            }

            let response = format!(
                "HTTP/1.1 {status}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
                body.len()
            );
            stream
                .write_all(response.as_bytes())
                .expect("write powder response");
            String::from_utf8(request).expect("utf8 powder request")
        });

        Self { base_url, handle }
    }

    fn request(self) -> String {
        self.handle.join().expect("powder stub thread")
    }
}

#[test]
fn list_prints_seed_agents() {
    roster_cmd()
        .arg("list")
        .assert()
        .success()
        .stdout(predicate::str::contains("cerberus\tgpt-5.5\txhigh"))
        .stdout(predicate::str::contains(
            "orchestrator\tclaude-fable-5\thigh",
        ))
        .stdout(predicate::str::contains(
            "sweep\topenrouter/deepseek/deepseek-v4-flash\tmedium",
        ));
}

#[test]
fn show_prints_agent_detail() {
    roster_cmd()
        .args(["show", "orchestrator"])
        .assert()
        .success()
        .stdout(predicate::str::contains("# orchestrator"))
        .stdout(predicate::str::contains(
            "Preferred model: claude-fable-5 (reasoning: high)",
        ))
        .stdout(predicate::str::contains("MCPs: powder"))
        .stdout(predicate::str::contains(
            "Contextual MCPs: qmd, todoist, bitterblossom, glass",
        ))
        .stdout(predicate::str::contains("Evidence Expectations"));
}

#[test]
fn materialize_codex_prints_brief_header() {
    roster_cmd()
        .args(["materialize", "cerberus", "--harness", "codex"])
        .assert()
        .success()
        .stdout(predicate::str::contains("# Roster Brief: cerberus"))
        .stdout(predicate::str::contains("Read:"))
        .stdout(predicate::str::contains("Code-review master"));
}

#[test]
fn materialize_bb_prints_agent_binding() {
    roster_cmd()
        .args(["materialize", "cerberus", "--harness", "bb"])
        .assert()
        .success()
        .stdout(predicate::str::contains(
            "# Generated from roster agent cerberus",
        ))
        .stdout(predicate::str::contains("harness = \"pi\""))
        .stdout(predicate::str::contains(
            "model = \"moonshotai/kimi-k2.7-code\"",
        ))
        .stdout(predicate::str::contains("role = \"cerberus\""))
        .stdout(predicate::str::contains("output_bytes_cap = 120000"))
        .stdout(predicate::str::contains("side_effect_policy = \"kill\""));
}

#[test]
fn materialize_claude_prints_native_subagent_frontmatter() {
    // Expected models come from primitives/models.yaml's `models` table:
    // orchestrator's preferred concrete id is claude-fable-5 (claude:
    // inherit), cerberus's is gpt-5.5 (claude: sonnet) -- resolved through
    // the table, not hardcoded per agent.
    for (agent, expected_tools, expected_model) in [
        (
            "orchestrator",
            "Read, Write, Edit, Grep, Glob, Bash, WebSearch",
            "inherit",
        ),
        ("cerberus", "Read, Grep, Glob, Bash", "sonnet"),
    ] {
        let output = roster_cmd()
            .args(["materialize", agent, "--harness", "claude"])
            .assert()
            .success()
            .get_output()
            .stdout
            .clone();
        let output = String::from_utf8(output).expect("utf8 stdout");
        let frontmatter = frontmatter_fields(&output);

        assert_eq!(
            frontmatter.keys().map(String::as_str).collect::<Vec<_>>(),
            ["description", "model", "name", "tools"]
        );
        assert_eq!(frontmatter.get("name").map(String::as_str), Some(agent));
        assert!(
            frontmatter
                .get("description")
                .is_some_and(|description| !description.is_empty())
        );
        assert_eq!(
            frontmatter.get("model").map(String::as_str),
            Some(expected_model)
        );
        assert_eq!(
            frontmatter.get("tools").map(String::as_str),
            Some(expected_tools)
        );
        assert!(
            !["fable-class", "codex-class", "openrouter-class"]
                .contains(&frontmatter["model"].as_str())
        );
    }
}

fn frontmatter_fields(output: &str) -> BTreeMap<String, String> {
    let frontmatter = output
        .strip_prefix("---\n")
        .and_then(|rest| rest.split_once("\n---\n"))
        .map(|(frontmatter, _)| frontmatter)
        .expect("claude subagent frontmatter");

    let mut fields = BTreeMap::new();
    for line in frontmatter.lines() {
        let (key, value) = line.split_once(':').expect("frontmatter key-value");
        assert!(
            fields
                .insert(key.to_string(), value.trim().to_string())
                .is_none(),
            "duplicate frontmatter key {key}"
        );
    }
    fields
}

#[test]
fn brief_without_card_renders_agent_context_and_overrides() {
    roster_cmd()
        .args([
            "brief",
            "sweep",
            "--add-skill",
            "extra-skill",
            "--add-mcp",
            "extra-mcp",
        ])
        .env_remove("POWDER_API_BASE_URL")
        .env_remove("POWDER_API_KEY")
        .assert()
        .success()
        .stdout(predicate::str::contains("# Roster Brief: sweep"))
        .stdout(predicate::str::contains("Read: "))
        .stdout(predicate::str::contains("- override: extra-skill"))
        .stdout(predicate::str::contains("- override: extra-mcp"))
        .stdout(predicate::str::contains("## Evidence Contract"))
        .stdout(predicate::str::contains("## Powder Card").not());
}

#[test]
fn brief_with_card_fetches_powder_context() {
    let stub = PowderStub::once(
        "200 OK",
        r#"{"card":{"title":"Test card","body":"Card body from Powder","acceptance":["first criterion","second criterion"]}}"#,
    );

    roster_cmd()
        .args(["brief", "orchestrator", "--card", "roster-123"])
        .env("POWDER_API_BASE_URL", &stub.base_url)
        .env("POWDER_API_KEY", "powder-test-key")
        .assert()
        .success()
        .stdout(predicate::str::contains("## Powder Card"))
        .stdout(predicate::str::contains("- ID: roster-123"))
        .stdout(predicate::str::contains("- Title: Test card"))
        .stdout(predicate::str::contains("- first criterion"))
        .stdout(predicate::str::contains("Card body from Powder"));

    let request = stub.request();
    assert!(request.starts_with("GET /api/v1/cards/roster-123 HTTP/1.1"));
    assert!(request.contains("Authorization: Bearer powder-test-key"));
}

#[test]
fn brief_card_404_reports_fetch_error() {
    let stub = PowderStub::once("404 Not Found", r#"{"error":"missing"}"#);

    roster_cmd()
        .args(["brief", "orchestrator", "--card", "missing-card"])
        .env("POWDER_API_BASE_URL", &stub.base_url)
        .env("POWDER_API_KEY", "powder-test-key")
        .assert()
        .failure()
        .stderr(predicate::str::contains("failed to fetch Powder card"))
        .stderr(predicate::str::contains("404"));

    let request = stub.request();
    assert!(request.starts_with("GET /api/v1/cards/missing-card HTTP/1.1"));
}

#[test]
fn brief_card_malformed_json_reports_decode_error() {
    let stub = PowderStub::once("200 OK", "not json");

    roster_cmd()
        .args(["brief", "orchestrator", "--card", "bad-json"])
        .env("POWDER_API_BASE_URL", &stub.base_url)
        .env("POWDER_API_KEY", "powder-test-key")
        .assert()
        .failure()
        .stderr(predicate::str::contains("failed to fetch Powder card"))
        .stderr(predicate::str::contains(
            "decode Powder response for bad-json",
        ));

    let request = stub.request();
    assert!(request.starts_with("GET /api/v1/cards/bad-json HTTP/1.1"));
}

#[test]
fn brief_card_requires_powder_environment() {
    roster_cmd()
        .args(["brief", "orchestrator", "--card", "roster-123"])
        .env_remove("POWDER_API_BASE_URL")
        .env_remove("POWDER_API_KEY")
        .assert()
        .failure()
        .stderr(predicate::str::contains("failed to fetch Powder card"))
        .stderr(predicate::str::contains(
            "POWDER_API_BASE_URL is required for --card",
        ));
}

#[test]
fn unknown_agent_is_reported() {
    roster_cmd()
        .args(["show", "missing-agent"])
        .assert()
        .failure()
        .stderr(predicate::str::contains("unknown agent \"missing-agent\""));
}

#[test]
fn sync_installs_orchestrator_and_curated_primitives_without_touching_harness_kit() {
    let home = tempfile::tempdir().expect("temp home");
    let codex_global = home.path().join(".codex/AGENTS.md");
    let claude_global = home.path().join(".claude/CLAUDE.md");
    let pi_settings = home.path().join(".pi/settings.json");
    write_file(&codex_global, "harness-kit codex global");
    write_file(&claude_global, "harness-kit claude global");
    write_file(&pi_settings, "{\"harness\":\"kit\"}");

    roster_cmd()
        .args(["sync", "--home"])
        .arg(home.path())
        .assert()
        .success()
        .stdout(predicate::str::contains(
            "Installed roster orchestrator sync",
        ))
        .stdout(predicate::str::contains(
            ".roster/orchestrator/manifest.json",
        ))
        .stdout(predicate::str::contains("roster sync --disable"));

    assert_eq!(
        fs::read_to_string(&codex_global).expect("codex global"),
        "harness-kit codex global"
    );
    assert_eq!(
        fs::read_to_string(&claude_global).expect("claude global"),
        "harness-kit claude global"
    );
    assert_eq!(
        fs::read_to_string(&pi_settings).expect("pi settings"),
        "{\"harness\":\"kit\"}"
    );

    let roster_brief = read(home.path().join(".roster/orchestrator/brief.md"));
    assert!(roster_brief.contains("# Roster Brief: orchestrator"));
    assert!(roster_brief.contains("Read: "));
    assert!(roster_brief.contains("## Skills To Read"));

    let claude_agent = read(home.path().join(".claude/agents/orchestrator.md"));
    assert!(claude_agent.contains("<!-- roster-sync:orchestrator:v1 -->"));
    // orchestrator's preferred concrete id is claude-fable-5; models.yaml
    // resolves that to `inherit` for the claude harness, not `sonnet`.
    assert!(claude_agent.contains("model: inherit"));
    assert!(claude_agent.contains("tools: Read, Write, Edit, Grep, Glob, Bash, WebSearch"));

    let codex_agent = read(home.path().join(".codex/agents/orchestrator.md"));
    assert!(codex_agent.contains("<!-- roster-sync:orchestrator:v1 -->"));
    assert!(codex_agent.contains("# Roster Brief: orchestrator"));

    let pi_agent = read(home.path().join(".pi/agents/orchestrator.md"));
    assert!(pi_agent.contains("<!-- roster-sync:orchestrator:v1 -->"));
    assert!(pi_agent.contains("# Roster Brief: orchestrator"));

    let skills_index = read(
        home.path()
            .join(".roster/orchestrator/primitives/skills-index.json"),
    );
    assert!(skills_index.contains("\"schema_version\": \"roster.sync.skills.v1\""));
    assert!(skills_index.contains("\"name\": \"orient\""));
    assert!(skills_index.contains("\"name\": \"powder\""));
    assert!(!skills_index.contains("\"name\": \"deliver\""));
    assert!(
        !home
            .path()
            .join(".roster/orchestrator/skills/orient/SKILL.md")
            .exists()
    );

    let manifest = read(home.path().join(".roster/orchestrator/manifest.json"));
    assert!(manifest.contains("\"schema_version\": \"roster.sync.v1\""));
    assert!(manifest.contains("\".codex/agents/orchestrator.md\""));
    assert!(manifest.contains("\".claude/agents/orchestrator.md\""));
    assert!(manifest.contains("\".pi/agents/orchestrator.md\""));

    let rollback = read(home.path().join(".roster/orchestrator/ROLLBACK.md"));
    assert!(rollback.contains("roster sync --disable"));
    assert!(rollback.contains("It leaves anything roster sync"));
    assert!(rollback.contains("declined to touch"));
}

#[test]
fn home_doctrine_composes_shared_doctrine_identity_skills_and_mcps() {
    let home = tempfile::tempdir().expect("temp home");

    roster_cmd()
        .args(["sync", "--home"])
        .arg(home.path())
        .assert()
        .success();

    // Doctrine links point at the composed home doctrine, not the bare
    // shared AGENTS.md — every default agent session should boot as the
    // declared orchestrator, not a copy of the undifferentiated doctrine.
    let home_doctrine_path = home.path().join(".roster/orchestrator/home-doctrine.md");
    assert_eq!(
        fs::read_link(home.path().join(".claude/CLAUDE.md")).unwrap(),
        home_doctrine_path
    );
    assert_eq!(
        fs::read_link(home.path().join(".codex/AGENTS.md")).unwrap(),
        home_doctrine_path
    );

    let doctrine = read(home.path().join(".roster/orchestrator/home-doctrine.md"));
    let shared_doctrine = read(workspace_root().join("primitives/shared/AGENTS.md"));

    // (a) the full shared operating doctrine, verbatim.
    assert!(doctrine.contains(shared_doctrine.trim()));
    // (b) a clearly-marked identity section carrying the orchestrator's
    // instructions.md verbatim.
    assert!(doctrine.contains("# Session Identity: orchestrator (roster)"));
    let orchestrator_instructions =
        read(workspace_root().join("agents/orchestrator/instructions.md"));
    assert!(doctrine.contains(orchestrator_instructions.trim()));
    // (c) the Skills To Read block, same rendering as the claude materializer.
    assert!(doctrine.contains("## Skills To Read"));
    assert!(doctrine.contains("- orient:"));
    // (d) MCP bindings as prose.
    assert!(doctrine.contains("## MCP Servers"));
    assert!(doctrine.contains("### Required"));
    assert!(doctrine.contains("- powder"));
    assert!(doctrine.contains("### Contextual"));
    assert!(doctrine.contains("- qmd"));
    // (e) a footer pointing at the rest of the roster and lane dispatch.
    assert!(doctrine.contains("roster list"));
    assert!(doctrine.contains("roster show"));
    assert!(doctrine.contains("roster materialize"));
    assert!(doctrine.contains("roster brief"));

    // The identity section must come after the shared doctrine, and skills
    // after identity, and MCPs after skills — composition order (a)-(e).
    let doctrine_end = doctrine
        .find(shared_doctrine.trim().lines().last().unwrap())
        .unwrap();
    let identity_pos = doctrine.find("# Session Identity").unwrap();
    let skills_pos = doctrine.find("## Skills To Read").unwrap();
    let mcp_pos = doctrine.find("## MCP Servers").unwrap();
    assert!(doctrine_end < identity_pos);
    assert!(identity_pos < skills_pos);
    assert!(skills_pos < mcp_pos);
}

#[test]
fn sync_disable_removes_only_roster_managed_files() {
    let home = tempfile::tempdir().expect("temp home");
    let codex_global = home.path().join(".codex/AGENTS.md");
    let unrelated_agent = home.path().join(".codex/agents/custom.md");
    write_file(&codex_global, "harness-kit codex global");
    write_file(&unrelated_agent, "operator-owned");

    roster_cmd()
        .args(["sync", "--home"])
        .arg(home.path())
        .assert()
        .success();

    roster_cmd()
        .args(["sync", "--home"])
        .arg(home.path())
        .arg("--disable")
        .assert()
        .success()
        .stdout(predicate::str::contains(
            "Disabled roster orchestrator sync",
        ))
        .stdout(predicate::str::contains(".codex/agents/orchestrator.md"));

    assert!(!home.path().join(".roster/orchestrator").exists());
    assert!(!home.path().join(".codex/agents/orchestrator.md").exists());
    assert!(!home.path().join(".claude/agents/orchestrator.md").exists());
    assert!(!home.path().join(".pi/agents/orchestrator.md").exists());
    assert_eq!(
        fs::read_to_string(&codex_global).expect("codex global"),
        "harness-kit codex global"
    );
    assert_eq!(
        fs::read_to_string(&unrelated_agent).expect("unrelated agent"),
        "operator-owned"
    );
}

#[test]
fn sync_disable_without_manifest_is_a_noop() {
    let home = tempfile::tempdir().expect("temp home");

    roster_cmd()
        .args(["sync", "--home"])
        .arg(home.path())
        .arg("--disable")
        .assert()
        .success()
        .stdout(predicate::str::contains(
            "No roster orchestrator sync manifest",
        ));
}

#[test]
fn sync_full_catalog_links_first_party_and_external_skills() {
    let home = tempfile::tempdir().expect("temp home");

    roster_cmd()
        .args(["sync", "--home"])
        .arg(home.path())
        .assert()
        .success();

    let claude_skill = home.path().join(".claude/skills/artifact");
    let codex_skill = home.path().join(".codex/skills/artifact");
    assert!(fs::symlink_metadata(&claude_skill).unwrap().is_symlink());
    assert_eq!(
        fs::read_link(&claude_skill).unwrap(),
        workspace_root().join("primitives/skills/artifact")
    );
    assert!(fs::symlink_metadata(&codex_skill).unwrap().is_symlink());

    // .external/* entries link by their own directory name, matching the
    // harness-kit farm convention.
    let external_skill = home.path().join(".claude/skills/leon-brutalist-skill");
    assert!(fs::symlink_metadata(&external_skill).unwrap().is_symlink());
    assert_eq!(
        fs::read_link(&external_skill).unwrap(),
        workspace_root().join("primitives/skills/.external/leon-brutalist-skill")
    );

    // pi is absent from this sandbox home, so its skills dir is never created.
    assert!(!home.path().join(".pi/skills").exists());
}

#[test]
fn sync_curated_catalog_only_links_orchestrators_skills() {
    let home = tempfile::tempdir().expect("temp home");

    roster_cmd()
        .args(["sync", "--home"])
        .arg(home.path())
        .args(["--catalog", "curated"])
        .assert()
        .success();

    assert!(
        home.path()
            .join(".claude/skills/orient")
            .symlink_metadata()
            .is_ok()
    );
    // "deliver" is not in the orchestrator's role.yaml skills list.
    assert!(!home.path().join(".claude/skills/deliver").exists());
}

#[test]
fn sync_replaces_harness_kit_symlink_but_refuses_real_unmanaged_file() {
    let home = tempfile::tempdir().expect("temp home");
    let claude_claude_md = home.path().join(".claude/CLAUDE.md");
    let codex_agents_md = home.path().join(".codex/AGENTS.md");

    // Simulate a pre-existing harness-kit-owned symlink (the real cutover
    // target) plus a genuine unmanaged real file (operator-authored, never
    // touched by any sync).
    fs::create_dir_all(claude_claude_md.parent().unwrap()).unwrap();
    let fake_harness_kit = home.path().join("fake-harness-kit/shared/AGENTS.md");
    fs::create_dir_all(fake_harness_kit.parent().unwrap()).unwrap();
    fs::write(&fake_harness_kit, "harness-kit doctrine").unwrap();
    std::os::unix::fs::symlink(&fake_harness_kit, &claude_claude_md).unwrap();
    write_file(&codex_agents_md, "operator-owned codex AGENTS.md");

    roster_cmd()
        .args(["sync", "--home"])
        .arg(home.path())
        .assert()
        .success()
        .stdout(predicate::str::contains(".codex/AGENTS.md"));

    assert!(
        fs::symlink_metadata(&claude_claude_md)
            .unwrap()
            .is_symlink()
    );
    assert_eq!(
        fs::read_link(&claude_claude_md).unwrap(),
        home.path().join(".roster/orchestrator/home-doctrine.md")
    );
    assert_eq!(
        fs::read_to_string(&codex_agents_md).unwrap(),
        "operator-owned codex AGENTS.md"
    );
}

#[test]
fn sync_is_idempotent_on_second_run() {
    let home = tempfile::tempdir().expect("temp home");

    let first_output = roster_cmd()
        .args(["sync", "--home"])
        .arg(home.path())
        .assert()
        .success()
        .get_output()
        .stdout
        .clone();
    let first_target = fs::read_link(home.path().join(".claude/skills/orient")).unwrap();
    let first_doctrine = read(home.path().join(".roster/orchestrator/home-doctrine.md"));
    let first_doctrine_link = fs::read_link(home.path().join(".claude/CLAUDE.md")).unwrap();

    let second_output = roster_cmd()
        .args(["sync", "--home"])
        .arg(home.path())
        .assert()
        .success()
        .get_output()
        .stdout
        .clone();
    let second_target = fs::read_link(home.path().join(".claude/skills/orient")).unwrap();
    let second_doctrine = read(home.path().join(".roster/orchestrator/home-doctrine.md"));
    let second_doctrine_link = fs::read_link(home.path().join(".claude/CLAUDE.md")).unwrap();

    assert_eq!(first_target, second_target);
    assert_eq!(first_doctrine, second_doctrine);
    assert_eq!(first_doctrine_link, second_doctrine_link);
    // The reported entry count must not grow between runs -- a growing
    // count on an unchanged catalog means some run is mistaking its own
    // prior side effect for new state to link (see the pi-skills
    // regression test above).
    assert_eq!(first_output, second_output);
}

#[test]
fn sync_disable_removes_skill_farm_and_doctrine_symlinks() {
    let home = tempfile::tempdir().expect("temp home");
    // Present pi and opencode so this run also plants their doctrine links,
    // proving disable tears down every presence-gated link, not just the
    // always-on claude/codex pair.
    write_file(&home.path().join(".pi/settings.json"), "{}");
    write_file(&home.path().join(".config/opencode/opencode.json"), "{}");

    roster_cmd()
        .args(["sync", "--home"])
        .arg(home.path())
        .assert()
        .success();
    assert!(home.path().join(".claude/skills/orient").exists());
    assert!(home.path().join(".claude/CLAUDE.md").exists());
    assert!(
        home.path()
            .join(".roster/orchestrator/home-doctrine.md")
            .exists()
    );
    assert!(home.path().join(".pi/agent/AGENTS.md").exists());
    assert!(home.path().join(".config/opencode/AGENTS.md").exists());

    roster_cmd()
        .args(["sync", "--home"])
        .arg(home.path())
        .arg("--disable")
        .assert()
        .success();

    assert!(!home.path().join(".claude/skills/orient").exists());
    assert!(!home.path().join(".claude/CLAUDE.md").exists());
    assert!(
        !home
            .path()
            .join(".roster/orchestrator/home-doctrine.md")
            .exists()
    );
    assert!(!home.path().join(".pi/agent/AGENTS.md").exists());
    assert!(!home.path().join(".config/opencode/AGENTS.md").exists());
    // opencode's own config file is not roster-managed; disable must not
    // touch it.
    assert!(home.path().join(".config/opencode/opencode.json").exists());
}

#[test]
fn sync_all_agents_materializes_every_agent() {
    let home = tempfile::tempdir().expect("temp home");

    roster_cmd()
        .args(["sync", "--home"])
        .arg(home.path())
        .arg("--all-agents")
        .assert()
        .success();

    let cerberus_claude = read(home.path().join(".claude/agents/cerberus.md"));
    assert!(cerberus_claude.contains("<!-- roster-sync:orchestrator:v1 -->"));
    let cerberus_codex = read(home.path().join(".codex/agents/cerberus.md"));
    assert!(cerberus_codex.contains("# Roster Brief: cerberus"));
}

#[test]
fn sync_links_pi_skills_only_when_pi_is_present() {
    let home = tempfile::tempdir().expect("temp home");
    // `.pi/settings.json` is pi's own native config file — a marker
    // roster sync never writes itself, unlike `.pi/agents/orchestrator.md`
    // which it always materializes regardless of pi presence.
    write_file(&home.path().join(".pi/settings.json"), "{}");

    roster_cmd()
        .args(["sync", "--home"])
        .arg(home.path())
        .assert()
        .success();

    assert!(
        home.path()
            .join(".pi/skills/orient")
            .symlink_metadata()
            .is_ok()
    );
    assert_eq!(
        fs::read_link(home.path().join(".pi/agent/AGENTS.md")).unwrap(),
        home.path().join(".roster/orchestrator/home-doctrine.md")
    );
}

#[test]
fn sync_does_not_link_pi_doctrine_when_pi_is_absent() {
    let home = tempfile::tempdir().expect("temp home");

    roster_cmd()
        .args(["sync", "--home"])
        .arg(home.path())
        .assert()
        .success();

    assert!(!home.path().join(".pi/agent/AGENTS.md").exists());
}

#[test]
fn sync_links_opencode_doctrine_only_when_opencode_is_present() {
    let home = tempfile::tempdir().expect("temp home");
    // `~/.config/opencode/opencode.json` is opencode's own native config
    // file, never written by roster sync — its presence means opencode
    // genuinely runs on this machine.
    write_file(&home.path().join(".config/opencode/opencode.json"), "{}");

    roster_cmd()
        .args(["sync", "--home"])
        .arg(home.path())
        .assert()
        .success();

    assert_eq!(
        fs::read_link(home.path().join(".config/opencode/AGENTS.md")).unwrap(),
        home.path().join(".roster/orchestrator/home-doctrine.md")
    );
    // The real opencode config file is untouched.
    assert_eq!(
        fs::read_to_string(home.path().join(".config/opencode/opencode.json")).unwrap(),
        "{}"
    );
}

#[test]
fn sync_does_not_link_opencode_doctrine_when_opencode_is_absent() {
    let home = tempfile::tempdir().expect("temp home");

    roster_cmd()
        .args(["sync", "--home"])
        .arg(home.path())
        .assert()
        .success();

    assert!(!home.path().join(".config/opencode/AGENTS.md").exists());
}

#[test]
fn sync_does_not_link_pi_skills_from_its_own_orchestrator_agent_side_effect() {
    // Regression test: `.pi/agents/orchestrator.md` is written unconditionally
    // on every sync (matching claude/codex), which creates `.pi/` itself.
    // A second run must not mistake that self-inflicted directory for a
    // genuine pi installation and start linking `.pi/skills/*`.
    let home = tempfile::tempdir().expect("temp home");

    roster_cmd()
        .args(["sync", "--home"])
        .arg(home.path())
        .assert()
        .success();
    assert!(home.path().join(".pi/agents/orchestrator.md").exists());
    assert!(!home.path().join(".pi/skills").exists());

    roster_cmd()
        .args(["sync", "--home"])
        .arg(home.path())
        .assert()
        .success();
    assert!(!home.path().join(".pi/skills").exists());
}

fn write_file(path: &std::path::Path, contents: &str) {
    fs::create_dir_all(path.parent().expect("parent")).expect("create parent");
    fs::write(path, contents).expect("write file");
}

fn read(path: impl AsRef<std::path::Path>) -> String {
    fs::read_to_string(path).expect("read file")
}

#[test]
fn synced_agent_files_keep_frontmatter_at_byte_zero() {
    // Claude Code silently ignores an agent file whose frontmatter is not the
    // first bytes — the sync marker must land AFTER the frontmatter block.
    let home = tempfile::tempdir().expect("home");
    let root = workspace_root();
    Command::cargo_bin("roster")
        .expect("roster binary")
        .current_dir(&root)
        .args([
            "sync",
            "--home",
            home.path().to_str().expect("utf8"),
            "--all-agents",
        ])
        .assert()
        .success();
    let agent = std::fs::read_to_string(home.path().join(".claude/agents/ai-scout.md"))
        .expect("ai-scout installed");
    assert!(
        agent.starts_with("---\n"),
        "agent file must start with frontmatter, got: {}",
        &agent[..40.min(agent.len())]
    );
    assert!(agent.contains("roster-sync:orchestrator:v1"));
}
