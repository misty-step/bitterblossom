//! `roster-hooks claude-hook <name>` — the Claude Code hook protocol entry
//! point. Each hook reads its event JSON from stdin and prints its decision
//! (if any) to stdout, matching Claude Code's PreToolUse/PostToolUse/
//! SessionStart hook contract. Ported from harness-kit's
//! `harness-kit-checks claude-hook <name>` dispatch — same five names, same
//! behavior.

mod claude_hooks;
mod invocation_kind;

use std::process::ExitCode;

fn main() -> ExitCode {
    let mut args = std::env::args().skip(1);
    let Some(command) = args.next() else {
        eprintln!("usage: roster-hooks claude-hook <name>");
        return ExitCode::FAILURE;
    };

    match command.as_str() {
        "claude-hook" => run_claude_hook(args.next()),
        other => {
            eprintln!("unknown command {other:?}; expected `claude-hook`");
            ExitCode::FAILURE
        }
    }
}

fn run_claude_hook(name: Option<String>) -> ExitCode {
    let Some(name) = name else {
        eprintln!("usage: roster-hooks claude-hook <name>");
        return ExitCode::FAILURE;
    };

    let result = match name.as_str() {
        "permission-auto-approve" => claude_hooks::run_permission_auto_approve_from_stdin(),
        "time-context" => claude_hooks::run_time_context(),
        "destructive-command-guard" => claude_hooks::run_destructive_command_guard_from_stdin(),
        "skill-invocation-tracker" => claude_hooks::run_skill_invocation_tracker_from_stdin(),
        "secrets-read-guard" => claude_hooks::run_secrets_read_guard_from_stdin(),
        other => {
            eprintln!(
                "unknown claude-hook {other:?}; expected one of: \
                 permission-auto-approve, time-context, destructive-command-guard, \
                 skill-invocation-tracker, secrets-read-guard"
            );
            return ExitCode::FAILURE;
        }
    };

    match result {
        Ok(()) => ExitCode::SUCCESS,
        Err(error) => {
            eprintln!("roster-hooks claude-hook {name}: {error:#}");
            ExitCode::FAILURE
        }
    }
}
