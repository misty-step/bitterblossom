from __future__ import annotations

import os

from conductorlib.common import (
    CmdError,
    ROOT,
    WORKSPACE_CLEANUP_LOCK_WAIT_SECONDS,
    WORKSPACE_PREPARE_LOCK_WAIT_SECONDS,
    Runner,
)


def repo_dir(repo: str) -> str:
    return f"/home/sprite/workspace/{repo.split('/')[-1]}"


def mirror_lock_path(repo: str) -> str:
    return f"{repo_dir(repo)}/.bb/conductor/mirror.lock"


def run_root(repo: str, run_id: str) -> str:
    return f"{repo_dir(repo)}/.bb/conductor/{run_id}"


def run_workspace(repo: str, run_id: str, lane: str) -> str:
    return f"{run_root(repo, run_id)}/{lane}-worktree"


def artifact_rel(run_id: str, name: str) -> str:
    return f".bb/conductor/{run_id}/{name}"


def artifact_abs(repo: str, rel_path: str) -> str:
    return f"{repo_dir(repo)}/{rel_path}"


def resolve_org() -> str:
    return os.environ.get("SPRITES_ORG") or os.environ.get("FLY_ORG") or "personal"


def sprite_bash(runner: Runner, sprite: str, script: str, *, timeout: int = 120) -> str:
    return runner.run(
        ["sprite", "-o", resolve_org(), "-s", sprite, "exec", "bash", "-lc", script],
        timeout=timeout,
    )


def parse_workspace_prepare_output(output: str, workspace: str, sprite: str) -> str:
    lines = [line.strip() for line in output.splitlines() if line.strip()]
    if lines and lines[-1] == workspace:
        return workspace
    raise CmdError(f"unexpected workspace prepare output for {sprite}: {output!r}")


def workspace_lock_python(
    *,
    mirror: str,
    workspace: str,
    lockfile: str,
    wait_seconds: int,
    timeout_message: str,
    lane: str,
) -> str:
    return "\n".join(
        [
            "set -euo pipefail",
            "python3 - <<'PY'",
            "import fcntl",
            "import pathlib",
            "import shutil",
            "import subprocess",
            "import sys",
            "import time",
            f"mirror = {mirror!r}",
            f"workspace = {workspace!r}",
            f"lockfile = {lockfile!r}",
            f"wait_seconds = {wait_seconds}",
            f"timeout_message = {timeout_message!r}",
            f"lane = {lane!r}",
            'pathlib.Path(lockfile).parent.mkdir(parents=True, exist_ok=True)',
            'pathlib.Path(workspace).parent.mkdir(parents=True, exist_ok=True)',
            "with open(lockfile, 'w', encoding='utf-8') as lock_handle:",
            "    deadline = time.monotonic() + wait_seconds",
            "    while True:",
            "        try:",
            "            fcntl.flock(lock_handle.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)",
            "            break",
            "        except BlockingIOError:",
            "            if time.monotonic() >= deadline:",
            "                print(timeout_message, file=sys.stderr)",
            "                raise SystemExit(1)",
            "            time.sleep(0.01)",
            "    if lane == 'prepare':",
            "        subprocess.run(['git', '-C', mirror, 'fetch', '--all', '--prune'], check=True)",
            "        master = subprocess.run(['git', '-C', mirror, 'show-ref', '--verify', '--quiet', 'refs/remotes/origin/master'])",
            "        if master.returncode == 0:",
            "            base_ref = 'origin/master'",
            "        else:",
            "            main = subprocess.run(['git', '-C', mirror, 'show-ref', '--verify', '--quiet', 'refs/remotes/origin/main'])",
            "            if main.returncode == 0:",
            "                base_ref = 'origin/main'",
            "            else:",
            "                symbolic = subprocess.run(",
            "                    ['git', '-C', mirror, 'symbolic-ref', '--quiet', '--short', 'refs/remotes/origin/HEAD'],",
            "                    check=False,",
            "                    capture_output=True,",
            "                    text=True,",
            "                )",
            "                if symbolic.returncode == 0 and symbolic.stdout.strip():",
            "                    base_ref = symbolic.stdout.strip()",
            "                else:",
            "                    head = subprocess.run(",
            "                        ['git', '-C', mirror, 'rev-parse', 'HEAD'],",
            "                        check=True,",
            "                        capture_output=True,",
            "                        text=True,",
            "                    )",
            "                    base_ref = head.stdout.strip()",
            "        if pathlib.Path(workspace).exists():",
            "            shutil.rmtree(workspace)",
            "        subprocess.run(['git', '-C', mirror, 'worktree', 'prune'], check=True)",
            "        subprocess.run(['git', '-C', mirror, 'worktree', 'add', '--detach', workspace, base_ref], check=True)",
            "        print(workspace)",
            "    else:",
            "        remove = subprocess.run(",
            "            ['git', '-C', mirror, 'worktree', 'remove', '--force', workspace],",
            "            check=False,",
            "            capture_output=True,",
            "            text=True,",
            "        )",
            "        if remove.returncode != 0:",
            "            shutil.rmtree(workspace, ignore_errors=True)",
            "        subprocess.run(['git', '-C', mirror, 'worktree', 'prune'], check=True)",
            "PY",
        ]
    )


def prepare_run_workspace(runner: Runner, sprite: str, repo: str, run_id: str, lane: str) -> str:
    mirror = repo_dir(repo)
    workspace = run_workspace(repo, run_id, lane)
    lockfile = mirror_lock_path(repo)
    script = workspace_lock_python(
        mirror=mirror,
        workspace=workspace,
        lockfile=lockfile,
        wait_seconds=WORKSPACE_PREPARE_LOCK_WAIT_SECONDS,
        timeout_message="mirror lock acquisition timed out",
        lane="prepare",
    )
    output = sprite_bash(runner, sprite, script, timeout=300)
    return parse_workspace_prepare_output(output, workspace, sprite)


def cleanup_run_workspace(runner: Runner, sprite: str, repo: str, run_id: str, lane: str) -> None:
    mirror = repo_dir(repo)
    workspace = run_workspace(repo, run_id, lane)
    lockfile = mirror_lock_path(repo)
    script = workspace_lock_python(
        mirror=mirror,
        workspace=workspace,
        lockfile=lockfile,
        wait_seconds=WORKSPACE_CLEANUP_LOCK_WAIT_SECONDS,
        timeout_message="mirror lock acquisition timed out during cleanup",
        lane="cleanup",
    )
    sprite_bash(runner, sprite, script, timeout=180)
