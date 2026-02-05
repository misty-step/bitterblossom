#!/usr/bin/env python3
"""
Destructive command guard for Claude Code.

Blocks dangerous git and filesystem commands that can lose uncommitted work.
PreToolUse hook - runs before Bash commands execute.

Exit 0 + JSON with permissionDecision: "deny" = block the command
Exit 0 + no output = allow the command
"""
import json
import re
import subprocess
import sys

# Regex patterns for commands that need smarter matching
RM_COMMAND_PATTERN = re.compile(
    r'(^|[;&|`]|\$\()\s*rm\s',
    re.MULTILINE
)

# Simple substring patterns
DESTRUCTIVE_SUBSTRINGS = [
    # Git commands
    ("git checkout -- ", "Discards uncommitted changes permanently. Use 'git stash' first."),
    ("git reset --hard", "Destroys all uncommitted work. Use 'git stash' first."),
    ("git clean -f", "Deletes untracked files permanently. Use 'git clean -n' to preview first."),
    ("git push --force", "Overwrites remote history. Use '--force-with-lease' instead."),
    ("git push -f ", "Overwrites remote history. Use '--force-with-lease' instead."),
    ("git branch -D ", "Force-deletes branch without merge check. Use '-d' for safety."),
    ("git stash drop", "Permanently deletes stashed changes."),
    ("git stash clear", "Permanently deletes ALL stashed changes."),
    # GitHub CLI commands
    ("gh pr merge", "Merges PR and may delete branch. Run manually to review."),
    ("gh repo delete", "Permanently deletes repository. Extremely destructive."),
    ("gh release delete", "Permanently deletes a release."),
    ("gh issue delete", "Permanently deletes an issue."),
    ("gh repo archive", "Archives repository, making it read-only."),
]

DESTRUCTIVE_PATTERNS = [
    (re.compile(r'(^|[;&|`]|\$\()\s*git\s+restore\s+(?!--staged|-S)'),
     "git restore can discard uncommitted changes. Use 'git restore --staged' for safe unstaging."),
]

DANGEROUS_FLAGS = [
    ("--no-verify", "Skips git hooks. Hooks enforce quality gates."),
    ("--no-gpg-sign", "Skips commit signing. May violate repo policy."),
]

SAFE = [
    "git checkout -b",
    "git checkout --orphan",
    "git restore --staged",
    "git restore -S",
    "git clean -n",
    "git clean --dry-run",
    "--force-with-lease",
    "--force-if-includes",
]


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


def check_merge_protection(cmd: str) -> tuple[bool, str]:
    merge_match = re.match(r"^git\s+merge\s+(\S+)", cmd)
    if not merge_match:
        return False, ""

    target_branch = merge_match.group(1)
    current_branch = get_current_branch()

    if is_protected_branch(current_branch):
        return True, f"Merging into {current_branch} is blocked. Create a PR instead."

    if target_branch in ("main", "master"):
        return False, ""

    return True, (
        f"Merging {target_branch} is blocked. "
        "Only 'git merge main' or 'git merge master' allowed on feature branches."
    )


def check_rebase_protection(cmd: str) -> tuple[bool, str]:
    if re.match(r"^git\s+rebase\b", cmd):
        return True, "git rebase is blocked (rewrites history). Use 'git merge main' instead."
    return False, ""


def check_push_protection(cmd: str) -> tuple[bool, str]:
    push_match = re.match(r"^git\s+push\b(.*)$", cmd)
    if not push_match:
        return False, ""

    push_args = push_match.group(1).strip()

    explicit = re.search(r"\b(\w+)\s+(main|master)\s*$", push_args)
    if explicit:
        return True, (
            f"Direct push to {explicit.group(2)} blocked.\n\n"
            "Use PR workflow:\n"
            "  git push origin <feature-branch>\n"
            "  # Create PR on GitHub"
        )

    refspec = re.search(r":\s*(main|master)\b", push_args)
    if refspec:
        return True, f"Refspec targeting {refspec.group(1)} blocked. Use PR workflow."

    current = get_current_branch()
    if is_protected_branch(current):
        has_branch = re.search(r"\b\w+\s+[\w\-/]+\s*$", push_args)
        if not has_branch:
            return True, (
                f"On {current}. Direct push blocked.\n\n"
                "Switch to feature branch first:\n"
                "  git checkout -b <feature>\n"
                "  git push -u origin <feature>"
            )

    return False, ""


def strip_quoted_content(cmd: str) -> str:
    result = []
    i = 0
    in_single = False
    in_double = False

    while i < len(cmd):
        char = cmd[i]

        if char == '\\' and i + 1 < len(cmd):
            if not in_single and not in_double:
                result.append(char)
                result.append(cmd[i + 1])
            i += 2
            continue

        if char == '"' and not in_single:
            in_double = not in_double
            result.append(char)
        elif char == "'" and not in_double:
            in_single = not in_single
            result.append(char)
        elif not in_single and not in_double:
            result.append(char)

        i += 1

    return ''.join(result)


def check_command(cmd: str) -> tuple[bool, str]:
    if not cmd:
        return False, ""

    for safe in SAFE:
        if safe in cmd:
            return False, ""

    blocked, reason = check_merge_protection(cmd)
    if blocked:
        return True, reason

    blocked, reason = check_rebase_protection(cmd)
    if blocked:
        return True, reason

    blocked, reason = check_push_protection(cmd)
    if blocked:
        return True, reason

    cmd_stripped = strip_quoted_content(cmd)

    # NOTE: Adapted for Linux — no /usr/bin/trash on Linux
    if RM_COMMAND_PATTERN.search(cmd_stripped):
        return True, "rm is blocked. Use a safe alternative (trash-cli, gio trash) or run manually."

    for pattern, reason in DESTRUCTIVE_SUBSTRINGS:
        if pattern in cmd_stripped:
            return True, reason

    for pattern, reason in DESTRUCTIVE_PATTERNS:
        if pattern.search(cmd_stripped):
            return True, reason

    for flag, reason in DANGEROUS_FLAGS:
        if flag in cmd:
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
                f"Run this yourself if truly needed."
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
