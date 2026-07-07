//! Six Claude Code hook handlers ported faithfully from harness-kit's
//! `harness-kit-hooks` crate (`crates/harness-kit-hooks/src/claude_hooks.rs`
//! in the harness-kit repo). No behavior changes versus that source — same
//! stdin/stdout hook protocol, same patterns, same guard logic. Only the
//! five roster needs are carried over (`permission-auto-approve`,
//! `time-context`, `destructive-command-guard`, `github-cli-guard`,
//! `skill-invocation-tracker`, `secrets-read-guard`); harness-kit's other hooks stay there
//! pending a later phase.

use std::env;
use std::fs::{self, OpenOptions};
use std::io::{Read, Write};
use std::path::{Path, PathBuf};
use std::process::Command;

use anyhow::{Context, Result};
use chrono::{Local, SecondsFormat, Utc};
use regex::Regex;
use serde_json::{Map, Value, json};

const SAFE_BASH_COMMANDS: &[&str] = &[
    r"^ls\b",
    r"^cat\b",
    r"^head\b",
    r"^tail\b",
    r"^less\b",
    r"^more\b",
    r"^wc\b",
    r"^file\b",
    r"^stat\b",
    r"^du\b",
    r"^df\b",
    r"^tree\b",
    r"^find\b.*-print",
    r"^find\b.*-name",
    r"^find\b.*-type",
    r"^git\s+(status|log|diff|show|branch|remote|tag|stash\s+list)",
    r"^git\s+ls-",
    r"^git\s+rev-parse",
    r"^git\s+describe",
    r"^git\s+config\s+--get",
    r"^git\s+config\s+-l",
    r"^git\s+config\s+--list",
    r"^git\s+shortlog",
    r"^git\s+blame",
    r"^git\s+annotate",
    r"^git\s+worktree\s+list",
    r"^rg\b",
    r"^ag\b",
    r"^fd\b",
    r"^fzf\b",
    r"^jq\b",
    r"^yq\b",
    r"^bat\b",
    r"^eza?\b",
    r"^ast-grep\b",
    r"^tokei\b",
    r"^cloc\b",
    r"^scc\b",
    r"^npm\s+(list|ls|view|info|outdated|audit)",
    r"^pnpm\s+(list|ls|view|info|outdated|audit)",
    r"^yarn\s+(list|info|outdated|audit)",
    r"^pip\s+(list|show|freeze)",
    r"^cargo\s+(tree|metadata|pkgid)",
    r"^go\s+(list|mod\s+graph)",
    r"^uname\b",
    r"^whoami\b",
    r"^hostname\b",
    r"^pwd\b",
    r"^env\b",
    r"^printenv\b",
    r"^echo\s+\$",
    r"^which\b",
    r"^whereis\b",
    r"^type\b",
    r"^command\s+-v",
    r"^ps\b",
    r"^top\s+-l\s+1",
    r"^uptime\b",
    r"^date\b",
    r"^cal\b",
    r"^gh\s+(repo|issue|pr|release|workflow|run)\s+(view|list|status|diff)",
    r"^gh\s+api\s+.*-X\s+GET",
    r"^gh\s+api\s+[^-]*$",
    r"^gh\s+auth\s+status",
    r"^vercel\s+(list|ls|inspect|logs|env\s+ls)",
    r"^vercel\s+--help",
    r"^npx\s+convex\s+(env\s+list|dashboard|logs)",
];

const NEVER_APPROVE: &[&str] = &[
    r"rm\s",
    r"rmdir\s",
    r"unlink\s",
    r">\s",
    r">>\s",
    r"\|\s*tee\b",
    r"curl.*-[dXP]",
    r"wget\s",
    r"sudo\b",
    r"su\b",
    r"chmod\b",
    r"chown\b",
    r"chgrp\b",
    r"kill\b",
    r"pkill\b",
    r"killall\b",
];

const DESTRUCTIVE_SUBSTRINGS: &[(&str, &str)] = &[
    (
        "git reset --hard",
        "Destroys all uncommitted work. Use 'git stash' first.",
    ),
    (
        "git push --force",
        "Overwrites remote history. Use '--force-with-lease' instead.",
    ),
    (
        "git push -f ",
        "Overwrites remote history. Use '--force-with-lease' instead.",
    ),
    ("git stash drop", "Permanently deletes stashed changes."),
    (
        "git stash clear",
        "Permanently deletes ALL stashed changes.",
    ),
    (
        "gh repo delete",
        "Permanently deletes repository. Extremely destructive.",
    ),
    ("gh issue delete", "Permanently deletes an issue."),
    (
        "gh repo archive",
        "Archives repository, making it read-only.",
    ),
];

const DANGEROUS_FLAGS: &[(&str, &str)] = &[
    (
        "--no-verify",
        "Skips git hooks. Hooks enforce quality gates.",
    ),
    (
        "--no-gpg-sign",
        "Skips commit signing. May violate repo policy.",
    ),
];

const DESTRUCTIVE_SAFE: &[&str] = &[
    "git checkout -b",
    "git checkout --orphan",
    "--force-with-lease",
    "--force-if-includes",
    "git merge --abort",
    "git reset --hard origin/",
];

pub fn run_permission_auto_approve_from_stdin() -> Result<()> {
    let mut input = String::new();
    std::io::stdin()
        .read_to_string(&mut input)
        .context("failed to read stdin")?;
    if let Some(output) = permission_auto_approve(&input) {
        println!("{}", serde_json::to_string(&output)?);
    }
    Ok(())
}

pub fn run_time_context() -> Result<()> {
    println!("{}", serde_json::to_string(&time_context_message())?);
    Ok(())
}

pub fn run_destructive_command_guard_from_stdin() -> Result<()> {
    let mut input = String::new();
    std::io::stdin()
        .read_to_string(&mut input)
        .context("failed to read stdin")?;
    if let Some(output) = destructive_command_guard(&input, Path::new(".")) {
        println!("{}", serde_json::to_string(&output)?);
    }
    Ok(())
}

pub fn run_skill_invocation_tracker_from_stdin() -> Result<()> {
    let mut input = String::new();
    std::io::stdin()
        .read_to_string(&mut input)
        .context("failed to read stdin")?;
    run_skill_invocation_tracker(&input, &default_skill_tracker_log_path());
    Ok(())
}

pub fn permission_auto_approve(input: &str) -> Option<Value> {
    let data: Value = serde_json::from_str(input).ok()?;
    let tool_name = data
        .get("tool_name")
        .and_then(Value::as_str)
        .unwrap_or_default();
    let tool_input = data
        .get("tool_input")
        .and_then(Value::as_object)
        .cloned()
        .unwrap_or_default();
    if is_safe_tool(tool_name, &tool_input) {
        Some(json!({
            "hookSpecificOutput": {
                "hookEventName": "PreToolUse",
                "permissionDecision": "allow",
                "permissionDecisionReason": format!("Auto-approved: {tool_name} is read-only"),
            }
        }))
    } else {
        None
    }
}

pub fn time_context_message() -> Value {
    let now = Local::now();
    let friendly_time = now.format("%A, %B %d, %Y at %-I:%M %p %Z").to_string();
    json!({
        "result": "continue",
        "message": format!("Current time: {friendly_time} ({})", now.to_rfc3339()),
    })
}

pub fn destructive_command_guard(input: &str, cwd: &Path) -> Option<Value> {
    let data: Value = serde_json::from_str(input).ok()?;
    if data.get("tool_name").and_then(Value::as_str) != Some("Bash") {
        return None;
    }
    let command = data
        .get("tool_input")
        .and_then(Value::as_object)
        .and_then(|input| input.get("command"))
        .and_then(Value::as_str)?;
    if command.is_empty() {
        return None;
    }
    let reason = destructive_command_reason(command, cwd)?;
    Some(json!({
        "hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "permissionDecision": "deny",
            "permissionDecisionReason": format!("BLOCKED: {reason}\n\nCommand: {command}\n\nRun this yourself if truly needed."),
        }
    }))
}

pub fn run_skill_invocation_tracker(input: &str, log_path: &Path) {
    let Ok(data) = serde_json::from_str::<Value>(input) else {
        return;
    };
    let Some(entry) = build_skill_invocation_entry(&data) else {
        return;
    };
    let _ = append_jsonl(log_path, &entry);
}

fn is_safe_tool(tool_name: &str, tool_input: &Map<String, Value>) -> bool {
    match tool_name {
        "Read" | "Glob" | "Grep" | "LS" | "WebFetch" | "WebSearch" => true,
        "Bash" => tool_input
            .get("command")
            .and_then(Value::as_str)
            .is_some_and(is_safe_bash),
        "Task" => tool_input
            .get("subagent_type")
            .and_then(Value::as_str)
            .is_some_and(|subagent| matches!(subagent, "Explore" | "Plan")),
        _ => false,
    }
}

fn is_safe_bash(command: &str) -> bool {
    for pattern in NEVER_APPROVE {
        if Regex::new(&format!("(?i){pattern}"))
            .expect("never approve regex compiles")
            .is_match(command)
        {
            return false;
        }
    }
    let trimmed = command.trim();
    SAFE_BASH_COMMANDS.iter().any(|pattern| {
        Regex::new(&format!("(?i){pattern}"))
            .expect("safe bash regex compiles")
            .is_match(trimmed)
    })
}

pub fn build_skill_invocation_entry(data: &Value) -> Option<Value> {
    if data.get("tool_name").and_then(Value::as_str) != Some("Skill") {
        return None;
    }
    let tool_input = data.get("tool_input").and_then(Value::as_object)?;
    let skill = tool_input.get("skill").and_then(Value::as_str)?;
    if skill.is_empty() {
        return None;
    }

    let cwd = data.get("cwd").and_then(Value::as_str).unwrap_or_default();
    let project = Path::new(cwd)
        .file_name()
        .map(|name| name.to_string_lossy().to_string())
        .unwrap_or_default();

    let mut entry = Map::new();
    entry.insert(
        "schema_version".to_string(),
        data.get("schema_version")
            .cloned()
            .unwrap_or_else(|| json!(2)),
    );
    entry.insert(
        "event_type".to_string(),
        data.get("event_type")
            .cloned()
            .unwrap_or_else(|| json!("skill_invocation")),
    );
    entry.insert(
        "ts".to_string(),
        json!(Utc::now().to_rfc3339_opts(SecondsFormat::Micros, true)),
    );
    entry.insert(
        "harness".to_string(),
        data.get("harness")
            .cloned()
            .unwrap_or_else(|| json!("claude")),
    );
    entry.insert(
        "source_protocol".to_string(),
        data.get("source_protocol")
            .cloned()
            .unwrap_or_else(|| json!("post_tool_use")),
    );
    entry.insert("skill".to_string(), json!(skill));
    entry.insert(
        "args".to_string(),
        tool_input.get("args").cloned().unwrap_or_else(|| json!("")),
    );
    entry.insert(
        "session_id".to_string(),
        data.get("session_id").cloned().unwrap_or_else(|| json!("")),
    );
    entry.insert("cwd".to_string(), json!(cwd));
    entry.insert("project".to_string(), json!(project));
    entry.insert(
        "invocation_kind".to_string(),
        json!(crate::invocation_kind::classify(data)),
    );

    for field in ["model_id", "outcome", "duration_ms", "usage"] {
        if let Some(value) = data.get(field) {
            entry.insert(field.to_string(), value.clone());
        }
    }

    Some(Value::Object(entry))
}

fn default_skill_tracker_log_path() -> PathBuf {
    if let Some(path) = env::var_os("SKILL_TRACKER_LOG_PATH") {
        return PathBuf::from(path);
    }
    let home = env::var_os("HOME")
        .map(PathBuf::from)
        .unwrap_or_else(|| PathBuf::from("."));
    home.join(".claude/skill-invocations.jsonl")
}

fn append_jsonl(path: &Path, value: &Value) -> Result<()> {
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent)
            .with_context(|| format!("failed to create {}", parent.display()))?;
    }
    let mut file = OpenOptions::new()
        .create(true)
        .append(true)
        .open(path)
        .with_context(|| format!("failed to open {}", path.display()))?;
    writeln!(file, "{}", serde_json::to_string(value)?)
        .with_context(|| format!("failed to append {}", path.display()))?;
    Ok(())
}

fn destructive_command_reason(command: &str, cwd: &Path) -> Option<String> {
    for (flag, reason) in DANGEROUS_FLAGS {
        if command.contains(flag) {
            return Some((*reason).to_string());
        }
    }
    if DESTRUCTIVE_SAFE.iter().any(|safe| command.contains(safe)) {
        return None;
    }
    if Regex::new(r"^git\s+merge\s+\S+").unwrap().is_match(command)
        && current_branch(cwd).is_some_and(|branch| is_protected_branch(&branch))
    {
        let branch = current_branch(cwd).unwrap_or_default();
        return Some(format!(
            "Merging into {branch} is blocked. Create a PR instead."
        ));
    }
    if let Some(captures) = Regex::new(r"git\s+branch\s+-D\s+(.*)")
        .unwrap()
        .captures(command)
    {
        for branch in captures[1].split_whitespace() {
            if is_protected_branch(branch) {
                return Some(format!(
                    "Force-deleting {branch} is blocked. Protected branch."
                ));
            }
        }
    }

    let stripped = strip_quoted_content(command);
    if Regex::new(r"(?m)(^|[;&|`]|\$\()\s*rm\s")
        .unwrap()
        .is_match(&stripped)
    {
        return Some(
            "Use /usr/bin/trash instead. Moves to Trash (recoverable). Example: /usr/bin/trash file.txt"
                .to_string(),
        );
    }
    for (pattern, reason) in DESTRUCTIVE_SUBSTRINGS {
        if stripped.contains(pattern) {
            return Some((*reason).to_string());
        }
    }
    None
}

fn current_branch(cwd: &Path) -> Option<String> {
    let output = Command::new("git")
        .args(["branch", "--show-current"])
        .current_dir(cwd)
        .output()
        .ok()?;
    if !output.status.success() {
        return None;
    }
    Some(String::from_utf8_lossy(&output.stdout).trim().to_string())
}

fn is_protected_branch(branch: &str) -> bool {
    matches!(branch, "main" | "master")
}

fn strip_quoted_content(command: &str) -> String {
    let mut result = String::new();
    let mut chars = command.chars().peekable();
    let mut in_single = false;
    let mut in_double = false;
    while let Some(ch) = chars.next() {
        if ch == '\\' && chars.peek().is_some() {
            if !in_single && !in_double {
                result.push(ch);
                if let Some(next) = chars.next() {
                    result.push(next);
                }
            } else {
                let _ = chars.next();
            }
            continue;
        }
        if ch == '"' && !in_single {
            in_double = !in_double;
            result.push(ch);
        } else if ch == '\'' && !in_double {
            in_single = !in_single;
            result.push(ch);
        } else if !in_single && !in_double {
            result.push(ch);
        }
    }
    result
}

/// Designated secret files: reachable relative to `$HOME`, checked against
/// both the literal `~/...` form (a model may write the tilde verbatim) and
/// the `$HOME`-expanded absolute form. Extend this list as new flat
/// secret-bearing files are designated (harness-kit-913).
const SECRET_FILE_HOME_SUFFIXES: &[&str] = &[".secrets"];

/// Bash verbs whose first argument is commonly a file to dump to stdout.
/// Each is checked for a designated secret file appearing anywhere in the
/// argument list — not just as the first arg — so `grep KEY ~/.secrets`
/// (pattern before path) is caught, not just `cat ~/.secrets`.
const SECRET_READ_VERBS: &[&str] = &[
    "cat", "grep", "egrep", "fgrep", "head", "tail", "less", "more", "strings", "od", "awk",
    "hexdump", "xxd",
];

pub fn run_secrets_read_guard_from_stdin() -> Result<()> {
    let mut input = String::new();
    std::io::stdin()
        .read_to_string(&mut input)
        .context("failed to read stdin")?;
    if let Some(output) = secrets_read_guard(&input, &home_dir()) {
        println!("{}", serde_json::to_string(&output)?);
    }
    Ok(())
}

pub fn secrets_read_guard(input: &str, home: &Path) -> Option<Value> {
    let data: Value = serde_json::from_str(input).ok()?;
    if data.get("tool_name").and_then(Value::as_str) != Some("Bash") {
        return None;
    }
    let command = data
        .get("tool_input")
        .and_then(Value::as_object)
        .and_then(|input| input.get("command"))
        .and_then(Value::as_str)?;
    if command.is_empty() {
        return None;
    }
    let reason = secrets_read_reason(command, home)?;
    Some(json!({
        "hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "permissionDecision": "deny",
            "permissionDecisionReason": format!(
                "BLOCKED: {reason}\n\nCommand: {command}\n\nUse `source ~/{}` (or `.`) instead — the sanctioned access pattern. Never cat/grep/head/tail a secret file; its value can land in this transcript, which is QMD-indexed and permanently searchable.",
                SECRET_FILE_HOME_SUFFIXES[0]
            ),
        }
    }))
}

fn secrets_read_reason(command: &str, home: &Path) -> Option<String> {
    let stripped = strip_quoted_content(command);
    // `source`/`.` are the sanctioned pattern — never block those, even if a
    // secret path also appears as a direct-read verb's argument elsewhere in
    // a compound command (e.g. `source ~/.secrets && cat notes.txt` is fine;
    // classification below is per verb-invocation, not per whole command).
    for secret_path in secret_file_paths(home) {
        for verb in SECRET_READ_VERBS {
            // Match the verb as a standalone command word (start of string,
            // or after a shell separator/pipe/subshell marker) followed
            // eventually by the secret path as a standalone argument word —
            // catches `grep KEY ~/.secrets` (path not first) and compound
            // commands (`cat ~/.secrets; true`), not just a bare prefix.
            let verb_pattern = format!(r"(?:^|[;&|`]|\$\()\s*{}\b", regex::escape(verb));
            let Some(verb_match) = Regex::new(&verb_pattern).unwrap().find(&stripped) else {
                continue;
            };
            let after_verb = &stripped[verb_match.end()..];
            let path_pattern = format!(
                "(?:^|[[:space:]'\"]){}(?:[[:space:]'\"]|$)",
                regex::escape(&secret_path)
            );
            // Only look within the same simple command (up to the next
            // separator), so `cat foo.txt; source ~/.secrets` isn't flagged.
            let segment_end = after_verb
                .find([';', '&', '|', '\n'])
                .unwrap_or(after_verb.len());
            let segment = &after_verb[..segment_end];
            if Regex::new(&path_pattern).unwrap().is_match(segment) {
                return Some(format!(
                    "Direct read of a designated secret file via `{verb}`. Its value can be printed into this transcript."
                ));
            }
        }
    }
    None
}

/// Both forms a model may write for a designated secret file: the literal
/// `~/name` shorthand (never shell-expanded before the policy sees it) and
/// the `$HOME`-expanded absolute path.
fn secret_file_paths(home: &Path) -> Vec<String> {
    SECRET_FILE_HOME_SUFFIXES
        .iter()
        .flat_map(|suffix| {
            vec![
                format!("~/{suffix}"),
                home.join(suffix).to_string_lossy().to_string(),
            ]
        })
        .collect()
}

fn home_dir() -> PathBuf {
    env::var_os("HOME")
        .map(PathBuf::from)
        .unwrap_or_else(|| PathBuf::from("."))
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    fn read_rows(path: &Path) -> Vec<Value> {
        fs::read_to_string(path)
            .unwrap()
            .lines()
            .map(|line| serde_json::from_str(line).unwrap())
            .collect()
    }

    #[test]
    fn skill_invocation_appends_jsonl_entry() {
        let temp = TempDir::new().unwrap();
        let log = temp.path().join("skill-invocations.jsonl");
        run_skill_invocation_tracker(
            r#"{"tool_name":"Skill","tool_input":{"skill":"commit","args":"-m fix"},"session_id":"abc","cwd":"/tmp/myproject"}"#,
            &log,
        );
        let rows = read_rows(&log);
        assert_eq!(rows.len(), 1);
        let entry = &rows[0];
        assert_eq!(entry["skill"], "commit");
        assert_eq!(entry["args"], "-m fix");
        assert_eq!(entry["session_id"], "abc");
        assert_eq!(entry["cwd"], "/tmp/myproject");
        assert_eq!(entry["project"], "myproject");
        assert_eq!(entry["harness"], "claude");
        assert_eq!(entry["schema_version"], 2);
        assert_eq!(entry["event_type"], "skill_invocation");
        assert_eq!(entry["source_protocol"], "post_tool_use");
        assert!(entry.get("ts").is_some());
    }

    #[test]
    fn optional_usage_fields_pass_through_when_available() {
        let temp = TempDir::new().unwrap();
        let log = temp.path().join("skill-invocations.jsonl");
        run_skill_invocation_tracker(
            r#"{"tool_name":"Skill","tool_input":{"skill":"qa","args":""},"session_id":"abc","cwd":"/tmp/myproject","model_id":"claude-opus-4-8","outcome":"succeeded","duration_ms":1200,"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15,"cost_usd":0.001,"cost_source":"provider_reported"}}"#,
            &log,
        );
        let entry = read_rows(&log).remove(0);
        assert_eq!(entry["model_id"], "claude-opus-4-8");
        assert_eq!(entry["outcome"], "succeeded");
        assert_eq!(entry["duration_ms"], 1200);
        assert_eq!(entry["usage"]["total_tokens"], 15);
    }

    #[test]
    fn non_skill_invalid_and_empty_skill_inputs_are_ignored() {
        let temp = TempDir::new().unwrap();
        let log = temp.path().join("skill-invocations.jsonl");
        run_skill_invocation_tracker(
            r#"{"tool_name":"Bash","tool_input":{"command":"ls"}}"#,
            &log,
        );
        run_skill_invocation_tracker("", &log);
        run_skill_invocation_tracker("not json at all", &log);
        run_skill_invocation_tracker(
            r#"{"tool_name":"Skill","tool_input":{"skill":"","args":""},"session_id":"abc","cwd":"/tmp/myproject"}"#,
            &log,
        );
        assert!(!log.exists());
    }

    #[test]
    fn multiple_invocations_append() {
        let temp = TempDir::new().unwrap();
        let log = temp.path().join("skill-invocations.jsonl");
        for skill in ["commit", "review", "investigate"] {
            run_skill_invocation_tracker(
                &format!(
                    r#"{{"tool_name":"Skill","tool_input":{{"skill":"{skill}","args":""}},"session_id":"sess1","cwd":"/tmp/proj"}}"#
                ),
                &log,
            );
        }
        let skills: Vec<String> = read_rows(&log)
            .into_iter()
            .map(|row| row["skill"].as_str().unwrap().to_string())
            .collect();
        assert_eq!(skills, ["commit", "review", "investigate"]);
    }

    #[test]
    fn invocation_kind_falls_back_to_unknown_without_a_readable_transcript() {
        let entry = build_skill_invocation_entry(&serde_json::json!({
            "tool_name": "Skill",
            "tool_input": {"skill": "commit", "args": ""},
        }))
        .unwrap();
        assert_eq!(entry["invocation_kind"], "unknown");
    }

    #[test]
    fn invocation_kind_flows_through_build_skill_invocation_entry() {
        let temp = TempDir::new().unwrap();
        let transcript_path = temp.path().join("transcript.jsonl");
        let rows = [
            json!({"type": "user", "message": {"content": "make this look brutalist"}}),
            json!({"type": "assistant", "message": {"content": [
                {"type": "tool_use", "name": "Skill", "input": {"skill": "design", "args": ""}}
            ]}}),
            // The routed call itself, mirroring what the real transcript
            // looks like by the time PostToolUse fires.
            json!({"type": "assistant", "message": {"content": [
                {"type": "tool_use", "name": "Skill", "input": {"skill": "leon-brutalist-skill", "args": ""}}
            ]}}),
        ]
        .into_iter()
        .map(|row| serde_json::to_string(&row).unwrap())
        .collect::<Vec<_>>()
        .join("\n");
        fs::write(&transcript_path, rows).unwrap();

        let entry = build_skill_invocation_entry(&json!({
            "tool_name": "Skill",
            "tool_input": {"skill": "leon-brutalist-skill", "args": ""},
            "transcript_path": transcript_path.display().to_string(),
        }))
        .unwrap();
        assert_eq!(entry["invocation_kind"], "routed");
    }

    #[test]
    fn permission_auto_approve_allows_read_only_tools() {
        let output = permission_auto_approve(
            r#"{"tool_name":"Read","tool_input":{"file_path":"README.md"}}"#,
        )
        .unwrap();
        assert_eq!(output["hookSpecificOutput"]["permissionDecision"], "allow");
        assert_eq!(output["hookSpecificOutput"]["hookEventName"], "PreToolUse");
    }

    #[test]
    fn permission_auto_approve_allows_safe_bash_and_blocks_mutating_bash() {
        let safe = permission_auto_approve(
            r#"{"tool_name":"Bash","tool_input":{"command":"git status --short"}}"#,
        )
        .unwrap();
        assert_eq!(safe["hookSpecificOutput"]["permissionDecision"], "allow");
        assert!(
            permission_auto_approve(
                r#"{"tool_name":"Bash","tool_input":{"command":"cat README.md > out.txt"}}"#
            )
            .is_none()
        );
        assert!(
            permission_auto_approve(
                r#"{"tool_name":"Bash","tool_input":{"command":"rm README.md"}}"#
            )
            .is_none()
        );
    }

    #[test]
    fn permission_auto_approve_handles_invalid_json_silently() {
        assert!(permission_auto_approve("not json").is_none());
    }

    #[test]
    fn permission_auto_approve_allows_explore_and_plan_tasks_only() {
        assert!(
            permission_auto_approve(
                r#"{"tool_name":"Task","tool_input":{"subagent_type":"Explore"}}"#
            )
            .is_some()
        );
        assert!(
            permission_auto_approve(
                r#"{"tool_name":"Task","tool_input":{"subagent_type":"Build"}}"#
            )
            .is_none()
        );
    }

    #[test]
    fn time_context_message_has_continue_result_and_current_time() {
        let output = time_context_message();
        assert_eq!(output["result"], "continue");
        assert!(
            output["message"]
                .as_str()
                .unwrap()
                .contains("Current time:")
        );
    }

    #[test]
    fn destructive_guard_blocks_reset_rm_and_dangerous_flags() {
        let temp = TempDir::new().unwrap();
        for command in ["git reset --hard", "rm README.md", "git commit --no-verify"] {
            let output = destructive_command_guard(
                &format!(r#"{{"tool_name":"Bash","tool_input":{{"command":"{command}"}}}}"#),
                temp.path(),
            )
            .unwrap();
            assert_eq!(output["hookSpecificOutput"]["permissionDecision"], "deny");
        }
    }

    #[test]
    fn destructive_guard_ignores_rm_inside_quotes_and_allows_safe_force_with_lease() {
        let temp = TempDir::new().unwrap();
        assert!(
            destructive_command_guard(
                r#"{"tool_name":"Bash","tool_input":{"command":"git commit -m \"rm all files\"}}"#,
                temp.path(),
            )
            .is_none()
        );
        assert!(
            destructive_command_guard(
                r#"{"tool_name":"Bash","tool_input":{"command":"git push --force-with-lease"}}"#,
                temp.path(),
            )
            .is_none()
        );
    }

    #[test]
    fn destructive_guard_blocks_protected_branch_delete() {
        let temp = TempDir::new().unwrap();
        let output = destructive_command_guard(
            r#"{"tool_name":"Bash","tool_input":{"command":"git branch -D feature main"}}"#,
            temp.path(),
        )
        .unwrap();
        assert!(
            output["hookSpecificOutput"]["permissionDecisionReason"]
                .as_str()
                .unwrap()
                .contains("Force-deleting main")
        );
    }

    fn secrets_guard_command(home: &Path, command: &str) -> Option<Value> {
        secrets_read_guard(
            &json!({"tool_name": "Bash", "tool_input": {"command": command}}).to_string(),
            home,
        )
    }

    #[test]
    fn secrets_guard_blocks_cat_and_grep_against_expanded_and_tilde_paths() {
        let temp = TempDir::new().unwrap();
        let home = temp.path();
        for command in [
            "cat ~/.secrets",
            &format!("cat {}", home.join(".secrets").display()),
            "grep POWDER_API_KEY ~/.secrets",
        ] {
            let output = secrets_guard_command(home, command)
                .unwrap_or_else(|| panic!("expected block for: {command}"));
            assert_eq!(output["hookSpecificOutput"]["permissionDecision"], "deny");
        }
    }

    #[test]
    fn secrets_guard_blocks_read_verb_inside_compound_command() {
        let temp = TempDir::new().unwrap();
        let home = temp.path();
        let output =
            secrets_guard_command(home, "echo debug; grep CANARY_API_KEY ~/.secrets | head -1")
                .unwrap();
        assert_eq!(output["hookSpecificOutput"]["permissionDecision"], "deny");
    }

    #[test]
    fn secrets_guard_allows_source_and_dot_of_the_same_file() {
        let temp = TempDir::new().unwrap();
        let home = temp.path();
        for command in [
            "source ~/.secrets && exec powder-mcp",
            &format!(". {} && run-thing", home.join(".secrets").display()),
        ] {
            assert!(
                secrets_guard_command(home, command).is_none(),
                "expected allow for: {command}"
            );
        }
    }

    #[test]
    fn secrets_guard_ignores_unrelated_files_and_non_bash_tools() {
        let temp = TempDir::new().unwrap();
        let home = temp.path();
        assert!(secrets_guard_command(home, "cat README.md").is_none());
        assert!(secrets_guard_command(home, "grep TODO src/main.rs").is_none());
        assert!(
            secrets_read_guard(
                r#"{"tool_name":"Read","tool_input":{"file_path":"~/.secrets"}}"#,
                home,
            )
            .is_none()
        );
    }

    #[test]
    fn secrets_guard_blocks_the_exact_shape_that_leaked_powder_api_key_2026_07_06() {
        // Regression pin: `grep POWDER_API_KEY ~/.secrets --` bypassed
        // Codex's argv-prefix execpolicy rule and printed a live key into a
        // transcript (harness-kit-913 live testing, 2026-07-06). The hook
        // searches the full command string, so this exact shape must block.
        let temp = TempDir::new().unwrap();
        let output = secrets_guard_command(temp.path(), "grep POWDER_API_KEY ~/.secrets --")
            .expect("must block the exact command shape that caused a live leak");
        assert_eq!(output["hookSpecificOutput"]["permissionDecision"], "deny");
    }
}
