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
        .stdout(predicate::str::contains("cerberus\tcodex-class\txhigh"))
        .stdout(predicate::str::contains("orchestrator\tfable-class\tlow"))
        .stdout(predicate::str::contains("sweep\topenrouter-class\tmedium"));
}

#[test]
fn show_prints_agent_detail() {
    roster_cmd()
        .args(["show", "orchestrator"])
        .assert()
        .success()
        .stdout(predicate::str::contains("# orchestrator"))
        .stdout(predicate::str::contains("Preferred model: fable-class"))
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
    // Expected models come from primitives/tiers.yaml's `tiers` table:
    // orchestrator's tier is fable-class (claude: inherit), cerberus's is
    // codex-class (claude: sonnet) -- resolved through the table, not
    // hardcoded per agent.
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
    // orchestrator's tier is fable-class; tiers.yaml resolves that to
    // `inherit` for the claude harness, not the old hardcoded `sonnet`.
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
    assert!(rollback.contains("leaves harness-kit bootstrap files untouched"));
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

fn write_file(path: &std::path::Path, contents: &str) {
    fs::create_dir_all(path.parent().expect("parent")).expect("create parent");
    fs::write(path, contents).expect("write file");
}

fn read(path: impl AsRef<std::path::Path>) -> String {
    fs::read_to_string(path).expect("read file")
}
