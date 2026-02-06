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
import shlex
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
    return branch in PROTECTED_BRANCHES


PROTECTED_BRANCHES = ("main", "master")


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

GIT_GLOBAL_OPTS_WITH_VALUE = {
    "-C",
    "-c",
    "--config-env",
    "--exec-path",
    "--git-dir",
    "--namespace",
    "--super-prefix",
    "--work-tree",
}

GIT_GLOBAL_FLAGS = {
    "--bare",
    "--literal-pathspecs",
    "--glob-pathspecs",
    "--noglob-pathspecs",
    "--icase-pathspecs",
    "--no-pager",
    "--paginate",
    "--no-optional-locks",
}

PUSH_ALL_FLAGS = {"--all", "--mirror"}


def _shell_split(cmd: str) -> list[str]:
    """Split a shell command while honoring quotes."""
    try:
        return shlex.split(cmd)
    except ValueError:
        return cmd.split()


def _push_args(cmd: str) -> list[str]:
    """Return args after `git push` or empty when command is not push."""
    tokens = _shell_split(cmd)
    if not tokens or tokens[0] != "git":
        return []

    i = 1
    while i < len(tokens):
        token = tokens[i]

        if token == "--":
            return []

        if token == "push":
            return tokens[i + 1:]

        if token in GIT_GLOBAL_OPTS_WITH_VALUE:
            if i + 1 >= len(tokens):
                return []
            i += 2
            continue

        if any(token.startswith(f"{opt}=") for opt in GIT_GLOBAL_OPTS_WITH_VALUE):
            i += 1
            continue

        if token in GIT_GLOBAL_FLAGS:
            i += 1
            continue

        if token.startswith("-"):
            i += 1
            continue

        return []

    return []


def _normalize_branch_ref(ref: str) -> str:
    return ref.removeprefix("refs/heads/")


def _extract_push_targets(args: list[str]) -> list[str]:
    """Extract push targets/refspecs from git push args.

    Assumes first non-option token is remote unless it clearly looks
    like a ref/refspec.
    """
    non_option = [a for a in args if a and not a.startswith("-")]
    if not non_option:
        return []

    targets = non_option[1:]
    first = non_option[0]
    if (
        ":" in first
        or first.startswith("+")
        or first.startswith("refs/heads/")
        or _normalize_branch_ref(first) in PROTECTED_BRANCHES
    ):
        targets.insert(0, first)
    return targets


def check_push_protection(cmd: str) -> tuple[bool, str]:
    """Block direct pushes to main/master."""
    args = _push_args(cmd)
    if not args:
        return False, ""

    push_all = next((a for a in args if a in PUSH_ALL_FLAGS), None)
    if push_all:
        return True, (
            f"{push_all} can update protected branches. Use PR workflow:\n"
            "  git push origin <feature-branch>\n"
            "  gh pr create"
        )

    targets = _extract_push_targets(args)
    for target in targets:
        normalized = target.lstrip("+")
        if ":" in normalized:
            branch = _normalize_branch_ref(normalized.rsplit(":", 1)[1])
            if branch in PROTECTED_BRANCHES:
                return True, f"Refspec targeting {branch} blocked. Use PR workflow."
            continue

        branch = _normalize_branch_ref(normalized)
        if branch in PROTECTED_BRANCHES:
            return True, (
                f"Direct push to {branch} blocked. Use PR workflow:\n"
                "  git push origin <feature-branch>\n"
                "  gh pr create"
            )

    # On protected branch with bare push
    current = get_current_branch()
    if is_protected_branch(current):
        if not targets:
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
    args = _push_args(cmd)
    if not args:
        return False, ""

    targets = _extract_push_targets(args)
    if any(t.startswith("+") for t in targets):
        return True, "Force refspec (+) overwrites remote history. Use '--force-with-lease' instead."

    has_safe_force = any(
        a == "--force-if-includes" or a.startswith("--force-with-lease")
        for a in args
    )
    if has_safe_force:
        return False, ""

    has_force = any(
        a == "--force"
        or a == "-f"
        or (
            a.startswith("-")
            and not a.startswith("--")
            and "f" in a[1:]
        )
        for a in args
    )
    if has_force:
        return True, "Overwrites remote history. Use '--force-with-lease' instead."

    return False, ""


def check_clean_protection(cmd: str) -> tuple[bool, str]:
    """Block destructive git clean invocations."""
    tokens = _shell_split(cmd)
    if len(tokens) < 2 or tokens[0] != "git" or tokens[1] != "clean":
        return False, ""

    for arg in tokens[2:]:
        if arg == "--":
            break
        if arg == "--dry-run":
            return False, ""
        if arg.startswith("-") and not arg.startswith("--") and "n" in arg[1:]:
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
    while cmd and cmd[0] in "({":
        cmd = cmd[1:].lstrip()
    while cmd and cmd[-1] in ")};":
        cmd = cmd[:-1].rstrip()
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
