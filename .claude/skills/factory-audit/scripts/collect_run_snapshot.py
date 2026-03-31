#!/usr/bin/env python3
"""Collect a Bitterblossom fleet snapshot for factory audits.

Gathers fleet health, store events, and GitHub PR state into one JSON document.
Replaces the old run-centric snapshot (run IDs, leases, incidents are deleted).
"""

import argparse
import json
import subprocess
import sys
from pathlib import Path


class SnapshotError(RuntimeError):
    pass


def find_repo_root(start: Path) -> Path:
    root = next((p for p in start.resolve().parents if (p / ".git").exists()), None)
    if root is None:
        raise SnapshotError(f"could not locate repository root above {start}")
    return root


ROOT = find_repo_root(Path(__file__))
CONDUCTOR_DIR = ROOT / "conductor"
TIMEOUT_SECONDS = 120


def run(argv: list[str], cwd: Path | None = None) -> str:
    try:
        proc = subprocess.run(
            argv,
            cwd=cwd or ROOT,
            text=True,
            capture_output=True,
            check=False,
            timeout=TIMEOUT_SECONDS,
        )
    except subprocess.TimeoutExpired as exc:
        raise SnapshotError(f"command timed out: {' '.join(argv)}") from exc
    if proc.returncode != 0:
        raise SnapshotError(
            (proc.stderr or proc.stdout).strip() or f"command failed: {' '.join(argv)}"
        )
    return proc.stdout


def run_json(argv: list[str], cwd: Path | None = None) -> object:
    output = run(argv, cwd=cwd)
    try:
        return json.loads(output)
    except json.JSONDecodeError as exc:
        raise SnapshotError(
            f"command produced non-JSON output: {' '.join(argv)}\n{output[:200]}"
        ) from exc


def collect_fleet_health(fleet_path: str) -> dict | None:
    """Collect fleet health via mix conductor fleet --json."""
    try:
        return run_json(
            ["mix", "conductor", "fleet", "--fleet", fleet_path, "--json"],
            cwd=CONDUCTOR_DIR,
        )
    except SnapshotError:
        return None


def collect_store_events(limit: int) -> list[dict]:
    """Collect recent store events via mix conductor events."""
    try:
        output = run(
            ["mix", "conductor", "events", "--limit", str(limit)],
            cwd=CONDUCTOR_DIR,
        )
        items = []
        for line in output.splitlines():
            line = line.strip()
            if not line:
                continue
            try:
                items.append(json.loads(line))
            except json.JSONDecodeError:
                continue
        return items
    except SnapshotError:
        return []


def collect_open_prs(repo: str) -> list[dict]:
    """Collect open PRs from GitHub."""
    try:
        return run_json([
            "gh", "pr", "list",
            "--repo", repo,
            "--state", "open",
            "--limit", "20",
            "--json", "number,title,url,state,isDraft,headRefName,createdAt,updatedAt,"
                      "mergeStateStatus,mergeable,reviewDecision,statusCheckRollup",
        ])
    except SnapshotError:
        return []


def collect_backlog(backlog_dir: Path) -> list[dict]:
    """Collect backlog items from backlog.d/."""
    items = []
    if not backlog_dir.is_dir():
        return items
    for f in sorted(backlog_dir.glob("*.md")):
        content = f.read_text(encoding="utf-8")
        # Extract frontmatter-style fields from the first few lines
        lines = content.splitlines()
        item = {"file": f.name, "title": "", "priority": "", "status": ""}
        for line in lines[:10]:
            stripped = line.strip()
            if stripped.startswith("# "):
                item["title"] = stripped[2:]
            elif stripped.lower().startswith("priority:"):
                item["priority"] = stripped.split(":", 1)[1].strip()
            elif stripped.lower().startswith("status:"):
                item["status"] = stripped.split(":", 1)[1].strip()
        items.append(item)
    return items


def main() -> int:
    parser = argparse.ArgumentParser(description="Collect a Bitterblossom fleet snapshot")
    parser.add_argument("--repo", default="misty-step/bitterblossom")
    parser.add_argument("--fleet", default="../fleet.toml",
                        help="Path to fleet.toml relative to conductor/")
    parser.add_argument("--limit", type=int, default=100,
                        help="Max store events to collect")
    parser.add_argument("--out", help="Output file path (stdout if omitted)")
    args = parser.parse_args()

    snapshot = {
        "fleet_health": collect_fleet_health(args.fleet),
        "store_events": collect_store_events(args.limit),
        "open_prs": collect_open_prs(args.repo),
        "backlog": collect_backlog(ROOT / "backlog.d"),
    }

    payload = json.dumps(snapshot, indent=2)
    if args.out:
        Path(args.out).write_text(payload + "\n", encoding="utf-8")
    else:
        sys.stdout.write(payload + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
