#!/usr/bin/env python3
"""Deterministic command-harness workloads for repository hygiene reflexes.

This is workload logic, intentionally kept out of the Rust event-plane spine.
It reads EVENT.json/RUN.json in the bb workspace, writes REPORT.json, and emits
a final bb.command_result.v1 line for the command harness parser.
"""

from __future__ import annotations

import datetime as dt
import fnmatch
import json
import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Any


REPORT = "REPORT.json"


def main() -> int:
    if len(sys.argv) != 2 or sys.argv[1] not in {"branch-prune", "dependabot-triage"}:
        print("usage: hygiene-reflex.py branch-prune|dependabot-triage", file=sys.stderr)
        return 64

    event = read_json(Path(os.environ.get("BB_EVENT_FILE", "EVENT.json")), default={})
    run = read_json(Path(os.environ.get("BB_RUN_FILE", "RUN.json")), default={})

    try:
        if sys.argv[1] == "branch-prune":
            report = branch_prune_report(event, run)
            result = (
                f"branch-prune {report['mode']}: {report['summary']['repo_count']} repos, "
                f"{report['summary']['total_would_delete']} branches would delete"
            )
        else:
            report = dependabot_triage_report(event, run)
            result = (
                f"dependabot-triage {report['mode']}: {report['summary']['repo_count']} repos, "
                f"{report['summary']['merge_candidates']} merge candidates"
            )
    except Exception as exc:  # The run still owes the operator a report artifact.
        report = blocked_report(sys.argv[1], event, run, str(exc))
        result = f"{sys.argv[1]} blocked: {exc}"

    write_report(report)
    print(json.dumps({"schema_version": "bb.command_result.v1", "result": result}, sort_keys=True))
    return 0


def read_json(path: Path, default: Any) -> Any:
    if not path.exists():
        return default
    with path.open("r", encoding="utf-8") as handle:
        return json.load(handle)


def write_report(report: dict[str, Any]) -> None:
    with open(REPORT, "w", encoding="utf-8") as handle:
        json.dump(report, handle, indent=2, sort_keys=True)
        handle.write("\n")


def blocked_report(kind: str, event: dict[str, Any], run: dict[str, Any], reason: str) -> dict[str, Any]:
    schema = (
        "bb.branch_prune_report.v1"
        if kind == "branch-prune"
        else "bb.dependabot_triage_report.v1"
    )
    return {
        "schema_version": schema,
        "mode": str(event.get("mode") or "report"),
        "status": "blocked",
        "run": run_context(run),
        "authority": {
            "current": "report-only",
            "delete_enabled": False,
            "merge_enabled": False,
            "no_side_effects": True,
        },
        "summary": {
            "repo_count": 0,
            "total_would_delete": 0,
            "open_dependabot_prs": 0,
            "merge_candidates": 0,
        },
        "repos": [],
        "artifact_paths": [REPORT],
        "residual_risk": [reason],
    }


def run_context(run: dict[str, Any]) -> dict[str, Any]:
    trigger = run.get("trigger") if isinstance(run.get("trigger"), dict) else {}
    return {
        "bb_run_id": run.get("run_id") or run.get("id") or "",
        "task": run.get("task") or "",
        "trigger_kind": trigger.get("kind") or run.get("trigger_kind") or "",
        "idempotency_key": trigger.get("idempotency_key") or run.get("idempotency_key") or "",
    }


def repo_entries(event: dict[str, Any]) -> list[dict[str, Any]]:
    repos = event.get("repos") or event.get("repo_configs") or []
    if not isinstance(repos, list):
        raise ValueError("EVENT.json repos must be an array")
    return [repo for repo in repos if isinstance(repo, dict)]


def branch_prune_report(event: dict[str, Any], run: dict[str, Any]) -> dict[str, Any]:
    repos = repo_entries(event)
    mode = str(event.get("mode") or "report")
    if mode not in {"report", "delete"}:
        raise ValueError("branch-prune mode must be report or delete")
    if not repos:
        raise ValueError("branch-prune requires at least one configured repo")

    delete_env = os.environ.get("BRANCH_PRUNE_ENABLE_DELETE") == "1"
    reports = [inspect_branch_repo(repo, mode, delete_env) for repo in repos]
    total_would_delete = sum(item["would_delete_count"] for item in reports)
    total_deleted = sum(item["deleted_count"] for item in reports)
    return {
        "schema_version": "bb.branch_prune_report.v1",
        "mode": mode,
        "status": "ok",
        "run": run_context(run),
        "authority": {
            "current": "delete" if mode == "delete" and delete_env else "report-only",
            "delete_enabled": mode == "delete" and delete_env,
            "requires_repo_delete_enabled": True,
            "requires_env": "BRANCH_PRUNE_ENABLE_DELETE",
            "no_side_effects": not (mode == "delete" and delete_env),
            "forbidden_actions": ["force_push", "delete_default_branch", "delete_unmerged_branch", "delete_open_pr_branch"],
        },
        "summary": {
            "repo_count": len(reports),
            "total_remote_branches": sum(item["total_remote_branches"] for item in reports),
            "total_merged_remote_branches": sum(item["merged_remote_branches"] for item in reports),
            "total_would_delete": total_would_delete,
            "total_deleted": total_deleted,
        },
        "repos": reports,
        "artifact_paths": [REPORT],
        "residual_risk": [
            "GitHub branch protection is not consulted in report mode",
            "Deletion mode remains disabled unless mode=delete, repo delete_enabled=true, and BRANCH_PRUNE_ENABLE_DELETE=1",
        ],
    }


def inspect_branch_repo(repo: dict[str, Any], mode: str, delete_env: bool) -> dict[str, Any]:
    name = str(repo.get("repo") or repo.get("name") or "")
    remote = str(repo.get("remote") or "")
    if not name and not remote:
        raise ValueError("branch-prune repo entry needs repo or remote")
    if not remote:
        remote = f"https://github.com/{name}.git"
    default_branch = str(repo.get("default_branch") or repo.get("default") or "")
    if not default_branch:
        default_branch = gh_default_branch(name)
    if not default_branch:
        raise ValueError(f"{name or remote}: default branch is unknown")

    open_pr_branches = sorted(set(configured_open_pr_branches(repo) or gh_open_pr_branches(name)))
    never_patterns = sorted(set([default_branch, "HEAD", *list_of_strings(repo.get("never"))]))
    delete_requested = mode == "delete"
    repo_delete_enabled = bool(repo.get("delete_enabled"))
    delete_active = delete_requested and delete_env and repo_delete_enabled

    with tempfile.TemporaryDirectory(prefix="bb-branch-prune.") as tmp:
        checkout = Path(tmp) / "repo"
        clone_repo(name=name, remote=remote, checkout=checkout)
        git(["fetch", "--quiet", "origin", "+refs/heads/*:refs/remotes/origin/*", "--prune"], checkout)
        branches = remote_branches(checkout)
        merged = merged_branches(checkout, default_branch)

        would_delete: list[str] = []
        kept: list[dict[str, Any]] = []
        for branch in branches:
            reasons: list[str] = []
            if branch == default_branch:
                reasons.append("default_branch")
            if matches_any(branch, never_patterns):
                reasons.append("explicit_never")
            if branch not in merged:
                reasons.append("unmerged")
            if branch in open_pr_branches:
                reasons.append("open_pr")
            if reasons:
                kept.append({"branch": branch, "reasons": sorted(set(reasons))})
            else:
                would_delete.append(branch)

        deleted: list[str] = []
        delete_errors: list[dict[str, str]] = []
        if delete_active:
            for branch in would_delete:
                proc = git_result(["push", "origin", "--delete", branch], checkout)
                if proc.returncode == 0:
                    deleted.append(branch)
                else:
                    delete_errors.append({"branch": branch, "error": proc.stderr.strip()})

    return {
        "repo": name or remote,
        "remote": remote,
        "default_branch": default_branch,
        "mode": mode,
        "delete_requested": delete_requested,
        "delete_enabled": delete_active,
        "total_remote_branches": len(branches),
        "merged_remote_branches": len(merged),
        "open_pr_branches": open_pr_branches,
        "never": never_patterns,
        "would_delete_count": len(would_delete),
        "would_delete": would_delete,
        "deleted_count": len(deleted),
        "deleted": deleted,
        "delete_errors": delete_errors,
        "kept": kept,
    }


def configured_open_pr_branches(repo: dict[str, Any]) -> list[str] | None:
    if "open_pr_branches" not in repo:
        return None
    return list_of_strings(repo.get("open_pr_branches"))


def list_of_strings(value: Any) -> list[str]:
    if not isinstance(value, list):
        return []
    return [str(item) for item in value if str(item)]


def matches_any(branch: str, patterns: list[str]) -> bool:
    return any(branch == pattern or fnmatch.fnmatch(branch, pattern) for pattern in patterns)


def clone_repo(name: str, remote: str, checkout: Path) -> None:
    if name and not remote.startswith("/") and not remote.startswith("file:"):
        proc = subprocess.run(
            ["gh", "repo", "clone", name, str(checkout), "--", "--no-tags", "--filter=blob:none"],
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
        if proc.returncode == 0:
            return
    proc = subprocess.run(
        ["git", "clone", "--quiet", "--no-tags", "--filter=blob:none", remote, str(checkout)],
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if proc.returncode != 0:
        proc = subprocess.run(
            ["git", "clone", "--quiet", "--no-tags", remote, str(checkout)],
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
    if proc.returncode != 0:
        raise RuntimeError(f"clone failed for {name or remote}: {proc.stderr.strip()}")


def remote_branches(checkout: Path) -> list[str]:
    out = git(["for-each-ref", "refs/remotes/origin", "--format=%(refname:short)"], checkout)
    branches = []
    for line in out.splitlines():
        branch = line.strip()
        if not branch.startswith("origin/"):
            continue
        branch = branch[len("origin/") :]
        if branch == "HEAD":
            continue
        branches.append(branch)
    return sorted(set(branches))


def merged_branches(checkout: Path, default_branch: str) -> set[str]:
    out = git(["branch", "-r", "--merged", f"origin/{default_branch}", "--format=%(refname:short)"], checkout)
    merged = set()
    for line in out.splitlines():
        branch = line.strip()
        if not branch.startswith("origin/"):
            continue
        branch = branch[len("origin/") :]
        if branch != "HEAD":
            merged.add(branch)
    return merged


def dependabot_triage_report(event: dict[str, Any], run: dict[str, Any]) -> dict[str, Any]:
    repos = repo_entries(event)
    mode = str(event.get("mode") or "report")
    if mode not in {"report", "merge_on_green"}:
        raise ValueError("dependabot-triage mode must be report or merge_on_green")
    if not repos:
        raise ValueError("dependabot-triage requires at least one configured repo")

    merge_env = os.environ.get("DEPENDABOT_TRIAGE_ENABLE_MERGE") == "1"
    reports = [inspect_dependabot_repo(repo, mode, merge_env) for repo in repos]
    return {
        "schema_version": "bb.dependabot_triage_report.v1",
        "mode": mode,
        "status": "ok",
        "run": run_context(run),
        "authority": {
            "current": "merge-on-green" if mode == "merge_on_green" and merge_env else "report-only",
            "merge_enabled": mode == "merge_on_green" and merge_env,
            "requires_repo_merge_on_green_enabled": True,
            "requires_env": "DEPENDABOT_TRIAGE_ENABLE_MERGE",
            "no_side_effects": not (mode == "merge_on_green" and merge_env),
            "forbidden_actions": ["merge_major", "merge_runtime_dependency", "merge_red_or_pending_ci"],
        },
        "summary": {
            "repo_count": len(reports),
            "open_dependabot_prs": sum(item["open_dependabot_pr_count"] for item in reports),
            "merge_candidates": sum(item["merge_candidates_count"] for item in reports),
            "merged": sum(item["merged_count"] for item in reports),
        },
        "repos": reports,
        "artifact_paths": [REPORT],
        "residual_risk": [
            "Dev dependency detection is conservative; unknown package scopes require a human",
            "Merge mode remains disabled unless mode=merge_on_green, repo merge_on_green_enabled=true, and DEPENDABOT_TRIAGE_ENABLE_MERGE=1",
        ],
    }


def inspect_dependabot_repo(repo: dict[str, Any], mode: str, merge_env: bool) -> dict[str, Any]:
    name = str(repo.get("repo") or repo.get("name") or "")
    if not name:
        raise ValueError("dependabot repo entry needs repo")
    raw_prs = repo.get("dependabot_prs")
    if raw_prs is None:
        raw_prs = gh_dependabot_prs(name)
    if not isinstance(raw_prs, list):
        raise ValueError(f"{name}: dependabot_prs must be an array")
    merge_active = mode == "merge_on_green" and merge_env and bool(repo.get("merge_on_green_enabled"))

    prs = []
    merged = []
    merge_errors = []
    for raw in raw_prs:
        if not isinstance(raw, dict):
            continue
        item = classify_dependabot_pr(raw)
        if merge_active and item["merge_eligible"]:
            proc = subprocess.run(
                ["gh", "pr", "merge", str(item["number"]), "--repo", name, "--squash", "--delete-branch"],
                text=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
            )
            if proc.returncode == 0:
                item["would_merge"] = True
                item["merged"] = True
                merged.append(item["number"])
            else:
                item["merge_error"] = proc.stderr.strip()
                merge_errors.append({"number": item["number"], "error": proc.stderr.strip()})
        prs.append(item)

    return {
        "repo": name,
        "mode": mode,
        "merge_enabled": merge_active,
        "open_dependabot_pr_count": len(prs),
        "merge_candidates_count": sum(1 for pr in prs if pr["merge_eligible"]),
        "merged_count": len(merged),
        "merged": merged,
        "merge_errors": merge_errors,
        "prs": prs,
    }


def classify_dependabot_pr(pr: dict[str, Any]) -> dict[str, Any]:
    title = str(pr.get("title") or "")
    files = normalize_files(pr.get("files"))
    version_class = classify_version(title)
    dependency_scope = classify_scope(title, files)
    ci_state = classify_ci(pr.get("statusCheckRollup"))
    created_at = str(pr.get("createdAt") or pr.get("created_at") or "")
    age_days = age_in_days(created_at)
    is_draft = bool(pr.get("isDraft") or pr.get("draft"))

    reasons: list[str] = []
    if is_draft:
        reasons.append("draft")
    if version_class == "major":
        reasons.append("major_update")
    elif version_class not in {"patch", "minor"}:
        reasons.append("unknown_update_size")
    if dependency_scope not in {"ci", "dev"}:
        reasons.append("runtime_or_unknown_dependency")
    if ci_state != "green":
        reasons.append(f"ci_{ci_state}")

    merge_eligible = not reasons
    if merge_eligible:
        decision = "merge_candidate"
    elif "major_update" in reasons:
        decision = "needs_human_major"
    elif "runtime_or_unknown_dependency" in reasons:
        decision = "needs_human_runtime_dependency"
    elif any(reason.startswith("ci_") for reason in reasons):
        decision = "wait_for_green"
    else:
        decision = "needs_human"

    return {
        "number": pr.get("number"),
        "title": title,
        "url": pr.get("url") or "",
        "head_ref": pr.get("headRefName") or pr.get("head_ref") or "",
        "created_at": created_at,
        "age_days": age_days,
        "version_class": version_class,
        "dependency_scope": dependency_scope,
        "ci_state": ci_state,
        "merge_eligible": merge_eligible,
        "would_merge": False,
        "merged": False,
        "decision": decision,
        "reasons": reasons,
        "files": files,
    }


def normalize_files(files: Any) -> list[str]:
    if not isinstance(files, list):
        return []
    paths = []
    for item in files:
        if isinstance(item, dict) and item.get("path"):
            paths.append(str(item["path"]))
        elif isinstance(item, str):
            paths.append(item)
    return paths


def classify_version(title: str) -> str:
    match = re.search(r"\bfrom\s+v?([0-9][0-9A-Za-z.+-]*)\s+to\s+v?([0-9][0-9A-Za-z.+-]*)", title)
    if not match:
        lowered = title.lower()
        if "major" in lowered:
            return "major"
        if "minor-and-patch" in lowered or "minor" in lowered:
            return "minor"
        if "patch" in lowered:
            return "patch"
        return "unknown"
    old = semantic_parts(match.group(1))
    new = semantic_parts(match.group(2))
    if not old or not new:
        return "unknown"
    if new[0] > old[0]:
        return "major"
    if len(new) > 1 and len(old) > 1 and new[1] > old[1]:
        return "minor"
    if len(new) > 2 and len(old) > 2 and new[2] > old[2]:
        return "patch"
    return "patch"


def semantic_parts(version: str) -> list[int]:
    base = re.split(r"[-+]", version, maxsplit=1)[0]
    parts = []
    for piece in base.split(".")[:3]:
        if not piece.isdigit():
            return []
        parts.append(int(piece))
    while len(parts) < 3:
        parts.append(0)
    return parts


def classify_scope(title: str, files: list[str]) -> str:
    lowered = title.lower()
    if files and all(is_ci_file(path) for path in files):
        return "ci"
    if (
        "github_actions" in lowered
        or "github actions" in lowered
        or "actions/" in lowered
        or "chore(ci)" in lowered
        or "gha-" in lowered
        or " gha" in lowered
    ):
        return "ci"
    if "deps-dev" in lowered or "devdependencies" in lowered or "dev dependency" in lowered:
        return "dev"
    if files and all(is_dev_file(path) for path in files):
        return "dev"
    return "runtime_or_unknown"


def is_ci_file(path: str) -> bool:
    return path.startswith(".github/") or path.startswith(".circleci/") or path.startswith(".buildkite/")


def is_dev_file(path: str) -> bool:
    return path in {
        ".pre-commit-config.yaml",
        ".tool-versions",
        "rust-toolchain.toml",
        "flake.lock",
    } or path.startswith("dev/")


def classify_ci(status_rollup: Any) -> str:
    if not isinstance(status_rollup, list) or not status_rollup:
        return "unknown"
    states = []
    for item in status_rollup:
        if not isinstance(item, dict):
            continue
        raw = item.get("conclusion") or item.get("state") or item.get("status")
        if raw is not None:
            states.append(str(raw).lower())
    if not states:
        return "unknown"
    red = {"failure", "failed", "error", "cancelled", "timed_out", "action_required"}
    pending = {"pending", "queued", "in_progress", "waiting", "requested", "expected"}
    green = {"success", "successful", "completed", "neutral", "skipped"}
    if any(state in red for state in states):
        return "red"
    if any(state in pending for state in states):
        return "pending"
    if all(state in green for state in states):
        return "green"
    return "unknown"


def age_in_days(created_at: str) -> int | None:
    if not created_at:
        return None
    try:
        created = dt.datetime.fromisoformat(created_at.replace("Z", "+00:00"))
    except ValueError:
        return None
    now = dt.datetime.now(dt.timezone.utc)
    return max(0, (now - created).days)


def gh_default_branch(repo: str) -> str:
    data = gh_json(["repo", "view", repo, "--json", "defaultBranchRef"])
    return str(((data.get("defaultBranchRef") or {}).get("name")) or "")


def gh_open_pr_branches(repo: str) -> list[str]:
    data = gh_json(["pr", "list", "--repo", repo, "--state", "open", "--limit", "200", "--json", "headRefName"])
    return sorted({str(item.get("headRefName")) for item in data if isinstance(item, dict) and item.get("headRefName")})


def gh_dependabot_prs(repo: str) -> list[dict[str, Any]]:
    fields = "number,title,url,createdAt,headRefName,baseRefName,author,mergeStateStatus,statusCheckRollup,files,isDraft"
    cmd = [
        "pr",
        "list",
        "--repo",
        repo,
        "--state",
        "open",
        "--limit",
        "100",
        "--search",
        "author:app/dependabot",
        "--json",
        fields,
    ]
    try:
        data = gh_json(cmd)
    except RuntimeError:
        data = gh_json([
            "pr",
            "list",
            "--repo",
            repo,
            "--state",
            "open",
            "--limit",
            "100",
            "--json",
            fields,
        ])
    return [item for item in data if is_dependabot_pr(item)]


def is_dependabot_pr(item: Any) -> bool:
    if not isinstance(item, dict):
        return False
    author = item.get("author") if isinstance(item.get("author"), dict) else {}
    login = str(author.get("login") or "").lower()
    head = str(item.get("headRefName") or "").lower()
    return "dependabot" in login or head.startswith("dependabot/")


def gh_json(args: list[str]) -> Any:
    proc = subprocess.run(["gh", *args], text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if proc.returncode != 0:
        raise RuntimeError(f"gh {' '.join(args[:3])} failed: {proc.stderr.strip()}")
    return json.loads(proc.stdout or "null")


def git(args: list[str], cwd: Path) -> str:
    proc = git_result(args, cwd)
    if proc.returncode != 0:
        raise RuntimeError(f"git {' '.join(args)} failed in {cwd}: {proc.stderr.strip()}")
    return proc.stdout


def git_result(args: list[str], cwd: Path) -> subprocess.CompletedProcess[str]:
    return subprocess.run(["git", *args], cwd=cwd, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)


if __name__ == "__main__":
    raise SystemExit(main())
