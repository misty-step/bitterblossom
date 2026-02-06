#!/usr/bin/env python3
"""
Destructive Git guard for sprites.

Blocks dangerous git operations that can lose history or bypass quality gates.
Sprites run on disposable machines so filesystem operations (rm, etc.) are fine.
Git operations are dangerous because they affect shared remote state.

PreToolUse hook — runs before Bash commands execute.
"""
import json
import re
import subprocess
import sys


def get_current_branch() -> str | None:
    try:
        result = subprocess.run(
            ["git", "branch", "--show-current"],
            capture_output=True, text=True, timeout=5,
        )
        if result.returncode == 0:
            return result.stdout.strip()
    except (subprocess.TimeoutExpired, FileNotFoundError):
        pass
    return None


def is_protected_branch(branch: str | None) -> bool:
    if not branch:
        return False
    return branch in ("main", "master")


# Commands that destroy git history or bypass quality gates
DESTRUCTIVE_GIT = [
    ("git reset --hard", "Destroys all uncommitted work. Use 'git stash' first."),
    ("git stash drop", "Permanently deletes stashed changes."),
    ("git stash clear", "Permanently deletes ALL stashed changes."),
    ("gh pr merge", "Merges PR. Run manually to review."),
    ("gh repo delete", "Permanently deletes repository."),
]

DANGEROUS_FLAGS = [
    ("--no-verify", "Skips git hooks. Hooks enforce quality gates — don't bypass them."),
]

SAFE_FORCE_FLAGS = ("--force-with-lease", "--force-if-includes")


def check_push_protection(cmd: str) -> tuple[bool, str]:
    """Block direct pushes to main/master."""
    push_match = re.match(r"^git\s+push\b(.*)$", cmd)
    if not push_match:
        return False, ""

    push_args = push_match.group(1).strip()

    # Explicit push to main/master (with optional +refspec and refs/heads/ prefix)
    explicit = re.search(r"\b(\S+)\s+\+?(refs/heads/)?(main|master)(?:\s|$)", push_args)
    if explicit:
        return True, (
            f"Direct push to {explicit.group(3)} blocked. Use PR workflow:\n"
            "  git push origin <feature-branch>\n"
            "  gh pr create"
        )

    # Refspec targeting main/master (with optional +refspec and refs/heads/ prefix)
    refspec = re.search(r"\+?:\s*(refs/heads/)?(main|master)(?:\s|$)", push_args)
    if refspec:
        branch = refspec.group(2)
        return True, f"Refspec targeting {branch} blocked. Use PR workflow."

    # On protected branch with bare push
    current = get_current_branch()
    if is_protected_branch(current):
        has_branch = re.search(r"\b\w+\s+[\w\-/]+\s*$", push_args)
        if not has_branch:
            return True, (
                f"On {current}. Direct push blocked.\n"
                "Switch to feature branch:\n"
                f"  git checkout -b <feature>\n"
                "  git push -u origin <feature>"
            )

    return False, ""


def check_rebase_protection(cmd: str) -> tuple[bool, str]:
    """Block rebase and other history-rewriting commands."""
    if re.match(r"^git\s+rebase\b", cmd):
        return True, "git rebase rewrites history. Use 'git merge main' instead."
    if re.match(r"^git\s+filter-branch\b", cmd):
        return True, "git filter-branch rewrites history. Not allowed on sprites."
    return False, ""


def check_force_push_protection(cmd: str) -> tuple[bool, str]:
    """Block force pushes unless explicit safety flags are used."""
    if not re.match(r"^git\s+push\b", cmd):
        return False, ""

    has_force = re.search(r"(^|\s)(-f|--force)(\s|$)", cmd)
    if not has_force:
        return False, ""

    if any(flag in cmd for flag in SAFE_FORCE_FLAGS):
        return False, ""

    return True, "Overwrites remote history. Use '--force-with-lease' instead."


def check_clean_protection(cmd: str) -> tuple[bool, str]:
    """Block destructive git clean invocations."""
    if not re.match(r"^git\s+clean\b", cmd):
        return False, ""

    if " -n" in cmd or "--dry-run" in cmd:
        return False, ""

    return True, "git clean deletes untracked files permanently. Use 'git clean -n' to preview."


def _extract_subshells(cmd: str) -> list[str]:
    """Extract subshell contents at all nesting levels, innermost first."""
    results = []
    remaining = cmd
    while True:
        inner = re.findall(r"\$\(([^()]*)\)", remaining)
        backtick = re.findall(r"`([^`]*)`", remaining)
        if not inner and not backtick:
            break
        results.extend(inner)
        results.extend(backtick)
        remaining = re.sub(r"\$\([^()]*\)", " ", remaining)
        remaining = re.sub(r"`[^`]*`", " ", remaining)
    return results


def _strip_shell_grouping(cmd: str) -> str:
    """Strip bare subshell parens and brace groups from a command."""
    cmd = cmd.strip()
    if cmd.startswith("(") and cmd.endswith(")"):
        cmd = cmd[1:-1].strip()
    if cmd.startswith("{"):
        cmd = cmd.lstrip("{").rstrip("}").rstrip(";").strip()
    return cmd


def split_shell_commands(cmd: str) -> list[str]:
    """Split a compound shell command into individual commands.

    Handles &&, ||, ;, |, $() / backtick subshells, bare ()-subshells,
    and { } brace groups. Nested subshells are extracted iteratively.
    Conservative — false positives are safer than false negatives.
    """
    subshells = _extract_subshells(cmd)

    # Replace subshell markers with separators for outer command parsing
    remaining = cmd
    while re.search(r"\$\([^()]*\)", remaining) or re.search(r"`[^`]*`", remaining):
        remaining = re.sub(r"\$\([^()]*\)", " ", remaining)
        remaining = re.sub(r"`[^`]*`", " ", remaining)

    parts = re.split(r"\s*(?:&&|\|\||[;|])\s*", remaining)
    all_parts = [_strip_shell_grouping(p) for p in parts if p.strip()]

    # Also check commands inside subshells
    for sub in subshells:
        sub_parts = re.split(r"\s*(?:&&|\|\||[;|])\s*", sub)
        all_parts.extend(
            _strip_shell_grouping(p) for p in sub_parts if p.strip()
        )

    return [p for p in all_parts if p]


def check_single_command(cmd: str) -> tuple[bool, str]:
    """Check a single (non-compound) command for dangerous operations."""
    if not cmd:
        return False, ""

    # Check push protection
    blocked, reason = check_push_protection(cmd)
    if blocked:
        return True, reason

    # Check rebase protection
    blocked, reason = check_rebase_protection(cmd)
    if blocked:
        return True, reason

    # Check force-push protection
    blocked, reason = check_force_push_protection(cmd)
    if blocked:
        return True, reason

    # Check git clean protection
    blocked, reason = check_clean_protection(cmd)
    if blocked:
        return True, reason

    # Check destructive git commands (word-boundary match)
    for pattern, reason in DESTRUCTIVE_GIT:
        if pattern in cmd:
            return True, reason

    # Check dangerous flags
    for flag, reason in DANGEROUS_FLAGS:
        if re.search(r"(^|\s)" + re.escape(flag) + r"(\s|$)", cmd):
            return True, reason

    return False, ""


def check_command(cmd: str) -> tuple[bool, str]:
    if not cmd:
        return False, ""

    # Split compound commands and check each part
    for part in split_shell_commands(cmd):
        blocked, reason = check_single_command(part)
        if blocked:
            return True, reason

    return False, ""


def deny(cmd: str, reason: str) -> None:
    output = {
        "hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "permissionDecision": "deny",
            "permissionDecisionReason": (
                f"BLOCKED: {reason}\n\n"
                f"Command: {cmd}\n\n"
                f"If truly needed, ask your coordinator (OpenClaw)."
            )
        }
    }
    print(json.dumps(output))
    sys.exit(0)


def main():
    try:
        data = json.load(sys.stdin)
    except json.JSONDecodeError:
        sys.exit(0)

    if data.get("tool_name") != "Bash":
        sys.exit(0)

    tool_input = data.get("tool_input") or {}
    cmd = tool_input.get("command", "")

    if not isinstance(cmd, str) or not cmd:
        sys.exit(0)

    should_block, reason = check_command(cmd)
    if should_block:
        deny(cmd, reason)

    sys.exit(0)


if __name__ == "__main__":
    main()
