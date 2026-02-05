#!/usr/bin/env python3
"""
Fast feedback after every file edit.

PostToolUse hook that runs type checking immediately after Edit/Write/MultiEdit.
Detects project type and runs appropriate fast check (~2-5s).
Exit 0 always (inform, don't block) - Claude sees errors and self-corrects.
"""
import subprocess
import sys
import os
import json


def get_cwd():
    try:
        hook_input = json.loads(sys.stdin.read())
        return hook_input.get("cwd", os.getcwd())
    except Exception:
        return os.getcwd()


def detect_project(cwd):
    if os.path.exists(os.path.join(cwd, "tsconfig.json")):
        return "typescript"
    if os.path.exists(os.path.join(cwd, "pyproject.toml")) or \
       os.path.exists(os.path.join(cwd, "setup.py")):
        return "python"
    if os.path.exists(os.path.join(cwd, "Cargo.toml")):
        return "rust"
    if os.path.exists(os.path.join(cwd, "go.mod")):
        return "go"
    return None


def run_check(project_type, cwd):
    commands = {
        "typescript": ["npx", "tsc", "--noEmit", "--pretty"],
        "python": ["ruff", "check", "."],
        "rust": ["cargo", "check", "--message-format=short"],
        "go": ["go", "vet", "./..."],
    }

    timeouts = {
        "typescript": 30,
        "python": 15,
        "rust": 60,
        "go": 30,
    }

    cmd = commands.get(project_type)
    timeout = timeouts.get(project_type, 30)

    if not cmd:
        return None

    try:
        result = subprocess.run(
            cmd, capture_output=True, text=True, timeout=timeout, cwd=cwd
        )
        return result
    except FileNotFoundError:
        return None
    except subprocess.TimeoutExpired:
        return None


def main():
    cwd = get_cwd()
    project_type = detect_project(cwd)

    if not project_type:
        sys.exit(0)

    result = run_check(project_type, cwd)

    if result and result.returncode != 0:
        output = result.stdout + result.stderr
        if output.strip():
            print(f"[fast-feedback] {project_type} issues:\n{output.strip()}")

    sys.exit(0)


if __name__ == "__main__":
    main()
