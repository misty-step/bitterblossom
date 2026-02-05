#!/usr/bin/env python3
"""
GitHub CLI guard for Claude Code.

Transforms `gh issue view` commands to use --json with explicit fields,
avoiding the deprecated projectCards field that causes GraphQL errors.
"""
import json
import re
import sys

SAFE_FIELDS = [
    "title", "body", "comments", "author", "state", "labels",
    "assignees", "milestone", "number", "url", "createdAt", "updatedAt",
]

GH_ISSUE_VIEW = re.compile(
    r"^gh\s+issue\s+view\s+(\d+|[A-Za-z0-9_/-]+#\d+)(.*)$"
)


def parse_command(cmd: str) -> tuple[bool, str, str | None]:
    if not cmd:
        return False, cmd, None

    cmd = cmd.strip()
    match = GH_ISSUE_VIEW.match(cmd)
    if not match:
        return False, cmd, None

    issue_ref = match.group(1)
    flags = match.group(2).strip()

    if "--json" in flags:
        return False, cmd, None

    if "--web" in flags or "-w" in flags:
        return False, cmd, None

    fields = ",".join(SAFE_FIELDS)

    remaining_flags = flags
    if "--comments" in remaining_flags:
        remaining_flags = remaining_flags.replace("--comments", "").strip()

    if remaining_flags:
        new_cmd = f"gh issue view {issue_ref} {remaining_flags} --json {fields}"
    else:
        new_cmd = f"gh issue view {issue_ref} --json {fields}"

    message = (
        f"[github-cli-guard] Transformed to avoid deprecated projectCards field\n"
        f"  Original: {cmd}\n"
        f"  Using: {new_cmd}"
    )

    return True, new_cmd, message


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

    needs_transform, new_cmd, message = parse_command(cmd)

    if needs_transform:
        output = {
            "hookSpecificOutput": {
                "hookEventName": "PreToolUse",
                "modifiedToolInput": {
                    "command": new_cmd,
                    "description": tool_input.get("description", "View GitHub issue"),
                },
            }
        }
        print(message, file=sys.stderr)
        print(json.dumps(output))

    sys.exit(0)


if __name__ == "__main__":
    main()
