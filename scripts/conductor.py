#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import os
import pathlib
import shlex
import sqlite3
import subprocess
import sys
import tempfile
import time
from dataclasses import asdict, dataclass
from datetime import datetime, timedelta, timezone
from typing import Any, Callable


ROOT = pathlib.Path(__file__).resolve().parents[1]
DEFAULT_DB = ROOT / ".bb" / "conductor.db"
DEFAULT_EVENT_LOG = ROOT / ".bb" / "events.jsonl"
DEFAULT_BUILDER_TEMPLATE = ROOT / "scripts" / "prompts" / "conductor-builder-template.md"
DEFAULT_REVIEWER_TEMPLATE = ROOT / "scripts" / "prompts" / "conductor-reviewer-template.md"
DEFAULT_LABEL = "autopilot"
DEFAULT_LEASE_BUFFER_SECONDS = 300
SUCCESSFUL_CHECK_CONCLUSIONS = {"SUCCESS", "NEUTRAL", "SKIPPED"}
FAILED_CHECK_CONCLUSIONS = {"FAILURE", "ERROR", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED", "STALE", "STARTUP_FAILURE"}
FAILED_STATUS_CONTEXTS = {"FAILURE", "ERROR"}


@dataclass(slots=True)
class Issue:
    number: int
    title: str
    body: str
    url: str
    labels: list[str]
    updated_at: str = ""


@dataclass(slots=True)
class BuilderResult:
    status: str
    branch: str
    pr_number: int
    pr_url: str
    summary: str
    tests: list[dict[str, Any]]


@dataclass(slots=True)
class ReviewResult:
    reviewer: str
    verdict: str
    summary: str
    findings: list[dict[str, Any]]


@dataclass(slots=True)
class ReviewThread:
    id: str
    path: str
    line: int | None
    author_login: str
    body: str
    url: str


@dataclass(slots=True)
class DispatchTask:
    sprite: str
    prompt: str
    artifact_path: str


@dataclass(slots=True)
class DispatchSession:
    task: DispatchTask
    argv: list[str]
    proc: Any
    log_path: pathlib.Path
    last_error: str = ""


class CmdError(RuntimeError):
    pass


class Runner:
    def __init__(self, cwd: pathlib.Path) -> None:
        self.cwd = cwd

    def run(self, argv: list[str], *, timeout: int | None = None, check: bool = True) -> str:
        proc = subprocess.run(
            argv,
            cwd=self.cwd,
            text=True,
            capture_output=True,
            timeout=timeout,
            check=False,
        )
        if check and proc.returncode != 0:
            raise CmdError(
                f"command failed ({proc.returncode}): {' '.join(shlex.quote(a) for a in argv)}\n"
                f"stdout:\n{proc.stdout}\n"
                f"stderr:\n{proc.stderr}"
        )
        return proc.stdout


def stringify_exc(exc: BaseException) -> str:
    return str(exc) or exc.__class__.__name__


def now_utc() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def ensure_parent(path: pathlib.Path) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)


def init_db(conn: sqlite3.Connection) -> None:
    conn.executescript(
        """
        create table if not exists runs (
            run_id text primary key,
            repo text not null,
            issue_number integer not null,
            issue_title text not null,
            phase text not null,
            status text not null,
            builder_sprite text,
            builder_profile text,
            branch text,
            pr_number integer,
            pr_url text,
            heartbeat_at text,
            created_at text not null,
            updated_at text not null
        );

        create table if not exists leases (
            repo text not null,
            issue_number integer not null,
            run_id text not null,
            leased_at text not null,
            heartbeat_at text,
            lease_expires_at text,
            released_at text,
            primary key (repo, issue_number)
        );

        create table if not exists reviews (
            run_id text not null,
            reviewer_sprite text not null,
            verdict text not null,
            summary text not null,
            findings_json text not null,
            created_at text not null,
            primary key (run_id, reviewer_sprite)
        );

        create table if not exists events (
            id integer primary key autoincrement,
            run_id text not null,
            event_type text not null,
            payload_json text not null,
            created_at text not null
        );
        """
    )
    ensure_column(conn, "runs", "heartbeat_at", "text")
    ensure_column(conn, "leases", "heartbeat_at", "text")
    ensure_column(conn, "leases", "lease_expires_at", "text")
    conn.commit()


def ensure_column(conn: sqlite3.Connection, table: str, column: str, decl: str) -> None:
    cols = conn.execute(f"pragma table_info({table})").fetchall()
    names = {row[1] for row in cols}
    if column in names:
        return
    conn.execute(f"alter table {table} add column {column} {decl}")


def open_db(path: pathlib.Path) -> sqlite3.Connection:
    ensure_parent(path)
    conn = sqlite3.connect(path)
    conn.row_factory = sqlite3.Row
    init_db(conn)
    return conn


def record_event(conn: sqlite3.Connection, event_log: pathlib.Path, run_id: str, event_type: str, payload: dict[str, Any]) -> None:
    ts = now_utc()
    conn.execute(
        "insert into events (run_id, event_type, payload_json, created_at) values (?, ?, ?, ?)",
        (run_id, event_type, json.dumps(payload, separators=(",", ":")), ts),
    )
    conn.commit()
    ensure_parent(event_log)
    with event_log.open("a", encoding="utf-8") as fh:
        fh.write(json.dumps({"run_id": run_id, "event": event_type, "ts": ts, "payload": payload}, separators=(",", ":")) + "\n")


def create_run(conn: sqlite3.Connection, run_id: str, repo: str, issue: Issue, builder_profile: str) -> None:
    ts = now_utc()
    conn.execute(
        """
        insert into runs (
            run_id, repo, issue_number, issue_title, phase, status,
            builder_profile, heartbeat_at, created_at, updated_at
        ) values (?, ?, ?, ?, 'leased', 'active', ?, ?, ?, ?)
        """,
        (run_id, repo, issue.number, issue.title, builder_profile, ts, ts, ts),
    )
    conn.commit()


def update_run(conn: sqlite3.Connection, run_id: str, **fields: Any) -> None:
    if not fields:
        return
    fields["updated_at"] = now_utc()
    cols = ", ".join(f"{key} = ?" for key in fields)
    values = list(fields.values()) + [run_id]
    conn.execute(f"update runs set {cols} where run_id = ?", values)
    conn.commit()


def acquire_lease(conn: sqlite3.Connection, repo: str, issue_number: int, run_id: str) -> bool:
    row = conn.execute(
        "select run_id, released_at, lease_expires_at from leases where repo = ? and issue_number = ?",
        (repo, issue_number),
    ).fetchone()
    ts = now_utc()
    expires_at = ts_plus(lease_ttl_seconds())
    if row is None:
        conn.execute(
            """
            insert into leases (repo, issue_number, run_id, leased_at, heartbeat_at, lease_expires_at)
            values (?, ?, ?, ?, ?, ?)
            """,
            (repo, issue_number, run_id, ts, ts, expires_at),
        )
        conn.commit()
        return True

    if row["released_at"] is None and not lease_expired(row["lease_expires_at"]):
        return False

    conn.execute(
        """
        update leases
        set run_id = ?, leased_at = ?, heartbeat_at = ?, lease_expires_at = ?, released_at = null
        where repo = ? and issue_number = ?
        """,
        (run_id, ts, ts, expires_at, repo, issue_number),
    )
    conn.commit()
    return True


def release_lease(conn: sqlite3.Connection, repo: str, issue_number: int) -> None:
    conn.execute(
        "update leases set released_at = ? where repo = ? and issue_number = ? and released_at is null",
        (now_utc(), repo, issue_number),
    )
    conn.commit()


def ts_plus(seconds: int) -> str:
    value = datetime.now(timezone.utc).replace(microsecond=0) + timedelta(seconds=seconds)
    return value.isoformat().replace("+00:00", "Z")


def lease_ttl_seconds() -> int:
    raw = os.environ.get("BB_LEASE_TTL_SECONDS", "").strip()
    if not raw:
        return 1800
    try:
        return max(60, int(raw))
    except ValueError:
        return 1800


def lease_expired(lease_expires_at: str | None) -> bool:
    if not lease_expires_at:
        return False
    try:
        expires = datetime.fromisoformat(lease_expires_at.replace("Z", "+00:00"))
    except ValueError:
        return False
    return datetime.now(timezone.utc) >= expires


def reap_expired_leases(conn: sqlite3.Connection) -> int:
    rows = conn.execute(
        "select repo, issue_number, lease_expires_at from leases where released_at is null"
    ).fetchall()
    expired = [(row["repo"], row["issue_number"]) for row in rows if lease_expired(row["lease_expires_at"])]
    for repo, issue_number in expired:
        conn.execute(
            "update leases set released_at = ? where repo = ? and issue_number = ? and released_at is null",
            (now_utc(), repo, issue_number),
        )
    conn.commit()
    return len(expired)


def heartbeat_run(conn: sqlite3.Connection, run_id: str) -> None:
    ts = now_utc()
    conn.execute(
        "update runs set heartbeat_at = ?, updated_at = ? where run_id = ?",
        (ts, ts, run_id),
    )
    conn.commit()


def renew_lease(conn: sqlite3.Connection, repo: str, issue_number: int, run_id: str, ttl_seconds: int) -> None:
    ts = now_utc()
    conn.execute(
        """
        update leases
        set heartbeat_at = ?, lease_expires_at = ?
        where repo = ? and issue_number = ? and run_id = ? and released_at is null
        """,
        (ts, ts_plus(ttl_seconds), repo, issue_number, run_id),
    )
    conn.commit()


def touch_run(conn: sqlite3.Connection, repo: str, issue_number: int, run_id: str, ttl_seconds: int) -> None:
    heartbeat_run(conn, run_id)
    renew_lease(conn, repo, issue_number, run_id, ttl_seconds)


def issue_priority(labels: list[str]) -> tuple[int, str]:
    order = {"P0": 0, "P1": 1, "P2": 2, "P3": 3}
    best = 9
    matched = ""
    for label in labels:
        upper = label.upper()
        if upper in order and order[upper] < best:
            best = order[upper]
            matched = upper
    return best, matched


def run_id_for(issue_number: int) -> str:
    return f"run-{issue_number}-{int(time.time())}"


def branch_name(issue_number: int, title: str, run_id: str) -> str:
    slug = "".join(ch.lower() if ch.isalnum() else "-" for ch in title).strip("-")
    slug = "-".join(part for part in slug.split("-") if part)[:32] or "issue"
    suffix = run_id.rsplit("-", 1)[-1]
    return f"factory/{issue_number}-{slug}-{suffix}"


def repo_dir(repo: str) -> str:
    return f"/home/sprite/workspace/{repo.split('/')[-1]}"


def artifact_rel(run_id: str, name: str) -> str:
    return f".bb/conductor/{run_id}/{name}"


def artifact_abs(repo: str, rel_path: str) -> str:
    return f"{repo_dir(repo)}/{rel_path}"


def resolve_org() -> str:
    return os.environ.get("SPRITES_ORG") or os.environ.get("FLY_ORG") or "personal"


def require_runtime_env() -> None:
    missing: list[str] = []
    if not os.environ.get("GITHUB_TOKEN"):
        missing.append("GITHUB_TOKEN")
    if not (os.environ.get("SPRITE_TOKEN") or os.environ.get("FLY_API_TOKEN")):
        missing.append("SPRITE_TOKEN|FLY_API_TOKEN")
    if missing:
        raise CmdError(f"missing required environment: {', '.join(missing)}")


def gh_json(runner: Runner, args: list[str]) -> Any:
    out = runner.run(["gh", *args], timeout=60)
    return json.loads(out)


def split_repo(repo: str) -> tuple[str, str]:
    owner, _, name = repo.partition("/")
    if not owner or not name:
        raise CmdError(f"invalid repo slug: {repo!r}")
    return owner, name


def gh_graphql(runner: Runner, query: str, variables: dict[str, str | int]) -> Any:
    argv = ["gh", "api", "graphql", "-f", f"query={query}"]
    for key, value in variables.items():
        argv.extend(["-F", f"{key}={value}"])
    out = runner.run(argv, timeout=60)
    return json.loads(out)


def list_candidate_issues(runner: Runner, repo: str, label: str, limit: int) -> list[Issue]:
    issues = gh_json(
        runner,
        [
            "issue",
            "list",
            "--repo",
            repo,
            "--state",
            "open",
            "--label",
            label,
            "--limit",
            str(limit),
            "--json",
            "number,title,body,url,labels,updatedAt",
        ],
    )
    return [
        Issue(
            number=item["number"],
            title=item["title"],
            body=item.get("body") or "",
            url=item["url"],
            labels=[label_obj["name"] for label_obj in item.get("labels", [])],
            updated_at=item.get("updatedAt") or "",
        )
        for item in issues
    ]


def get_issue(runner: Runner, repo: str, issue_number: int) -> Issue:
    item = gh_json(
        runner,
        [
            "issue",
            "view",
            str(issue_number),
            "--repo",
            repo,
            "--json",
            "number,title,body,url,labels,updatedAt",
        ],
    )
    return Issue(
        number=item["number"],
        title=item["title"],
        body=item.get("body") or "",
        url=item["url"],
        labels=[label_obj["name"] for label_obj in item.get("labels", [])],
        updated_at=item.get("updatedAt") or "",
    )


def pick_issue(conn: sqlite3.Connection, issues: list[Issue], repo: str) -> Issue | None:
    reap_expired_leases(conn)
    eligible: list[Issue] = []
    for issue in issues:
        leased = conn.execute(
            "select 1 from leases where repo = ? and issue_number = ? and released_at is null",
            (repo, issue.number),
        ).fetchone()
        if leased is None:
            eligible.append(issue)

    if not eligible:
        return None

    def key(issue: Issue) -> tuple[int, str, int]:
        priority, matched = issue_priority(issue.labels)
        return (priority, issue.updated_at or "", issue.number)

    return sorted(eligible, key=key)[0]


def select_worker(runner: Runner, repo: str, workers: list[str], prompt_template: pathlib.Path) -> str:
    bb_bin = str(ROOT / "bin" / "bb")
    last_error = ""
    for worker in workers:
        try:
            proc = subprocess.run(
                [
                    bb_bin,
                    "dispatch",
                    worker,
                    "conductor availability probe",
                    "--repo",
                    repo,
                    "--dry-run",
                    "--prompt-template",
                    str(prompt_template),
                ],
                cwd=ROOT,
                text=True,
                capture_output=True,
                timeout=120,
                check=False,
            )
        except subprocess.TimeoutExpired:
            last_error = f"worker probe timed out for {worker}"
            continue
        if proc.returncode == 0:
            return worker
        last_error = proc.stderr or proc.stdout
    raise CmdError(f"no available worker in {workers}: {last_error}")


def build_builder_task(
    issue: Issue,
    run_id: str,
    branch: str,
    artifact_path: str,
    feedback: str | None = None,
    *,
    pr_number: int | None = None,
    pr_url: str | None = None,
) -> str:
    lines = [
        f"Run ID: {run_id}",
        f"Issue: #{issue.number} - {issue.title}",
        f"Issue URL: {issue.url}",
        f"Branch: {branch}",
        f"Builder artifact path: {artifact_path}",
        "",
        "Implementation target:",
        issue.body or "(no body provided)",
        "",
        "PR requirements:",
        f"- Include `Closes #{issue.number}` in the PR body.",
        "- Open a draft PR as soon as the branch is pushed.",
        "- Stop after the PR is ready for reviewer council.",
    ]
    if pr_number and pr_url:
        lines.extend(
            [
                "",
                "Existing PR context:",
                f"- PR: #{pr_number}",
                f"- PR URL: {pr_url}",
            ]
        )
    if feedback:
        lines.extend(["", "Revision feedback to address:", feedback])
    return "\n".join(lines)


def build_review_task(issue: Issue, run_id: str, pr_number: int, pr_url: str, artifact_path: str) -> str:
    return "\n".join(
        [
            f"Run ID: {run_id}",
            f"Issue: #{issue.number} - {issue.title}",
            f"Issue URL: {issue.url}",
            f"PR: #{pr_number}",
            f"PR URL: {pr_url}",
            f"Review artifact path: {artifact_path}",
            "",
            "Review target:",
            issue.body or "(no body provided)",
            "",
            "Required output:",
            "- Review the PR diff against the issue and repo guidance.",
            "- Write the review artifact JSON before TASK_COMPLETE.",
        ]
    )


def fetch_json_artifact(runner: Runner, sprite: str, path: str) -> dict[str, Any]:
    org = resolve_org()
    out = runner.run(
        [
            "sprite",
            "-o",
            org,
            "-s",
            sprite,
            "exec",
            "bash",
            "-lc",
            f"cat {shlex.quote(path)}",
        ],
        timeout=60,
    )
    return json.loads(out)


def wait_for_json_artifact(
    runner: Runner,
    sprite: str,
    path: str,
    *,
    timeout_seconds: int,
    poll_seconds: int = 10,
) -> dict[str, Any]:
    deadline = time.time() + timeout_seconds
    last_error = ""
    while time.time() < deadline:
        try:
            return fetch_json_artifact(runner, sprite, path)
        except (CmdError, json.JSONDecodeError) as exc:
            last_error = str(exc)
            time.sleep(poll_seconds)
    raise CmdError(
        f"artifact not available after {timeout_seconds}s: {path} on {sprite}\n"
        f"last error:\n{last_error}"
    )


def parse_builder_result(payload: dict[str, Any], expected_branch: str) -> BuilderResult:
    try:
        status = str(payload["status"])
        branch = str(payload["branch"])
        pr_number = int(payload["pr_number"])
        pr_url = str(payload["pr_url"])
        summary = str(payload["summary"])
        tests = list(payload.get("tests", []))
    except (KeyError, TypeError, ValueError) as exc:
        raise CmdError(f"invalid builder artifact: {payload}") from exc

    if status != "ready_for_review":
        raise CmdError(f"builder artifact has unexpected status {status!r}")
    if branch != expected_branch:
        raise CmdError(f"builder artifact branch mismatch: expected {expected_branch!r}, got {branch!r}")
    if pr_number <= 0:
        raise CmdError(f"builder artifact has invalid pr_number {pr_number!r}")
    if f"/pull/{pr_number}" not in pr_url:
        raise CmdError(f"builder artifact PR URL does not match PR number: {pr_url!r}")

    return BuilderResult(
        status=status,
        branch=branch,
        pr_number=pr_number,
        pr_url=pr_url,
        summary=summary,
        tests=tests,
    )


def parse_review_result(reviewer: str, payload: dict[str, Any]) -> ReviewResult:
    try:
        verdict = str(payload["verdict"])
        summary = str(payload["summary"])
        findings = list(payload.get("findings", []))
    except (KeyError, TypeError, ValueError) as exc:
        raise CmdError(f"invalid review artifact from {reviewer}: {payload}") from exc

    if verdict not in {"pass", "fix", "block"}:
        raise CmdError(f"invalid review verdict from {reviewer}: {verdict!r}")

    return ReviewResult(
        reviewer=reviewer,
        verdict=verdict,
        summary=summary,
        findings=findings,
    )


def verify_builder_pr(runner: Runner, repo: str, pr_number: int, expected_branch: str) -> tuple[int, str]:
    pr = gh_json(runner, ["pr", "view", str(pr_number), "--repo", repo, "--json", "number,url,headRefName,state"])
    if int(pr["number"]) != pr_number:
        raise CmdError(f"builder artifact PR number mismatch: expected {pr_number}, got {pr['number']}")
    if pr["headRefName"] != expected_branch:
        raise CmdError(
            f"builder artifact PR head mismatch: expected {expected_branch!r}, got {pr['headRefName']!r}"
        )
    if pr["state"] != "OPEN":
        raise CmdError(f"builder artifact PR is not open: #{pr_number} state={pr['state']}")
    return int(pr["number"]), str(pr["url"])


def comment_issue(runner: Runner, repo: str, issue_number: int, body: str) -> None:
    runner.run(["gh", "issue", "comment", str(issue_number), "--repo", repo, "--body", body], timeout=60)


def comment_pr(runner: Runner, repo: str, pr_number: int, body: str) -> None:
    runner.run(["gh", "pr", "comment", str(pr_number), "--repo", repo, "--body", body], timeout=60)


def rollup_item_name(item: dict[str, Any]) -> str:
    typename = str(item.get("__typename", ""))
    if typename == "CheckRun":
        return str(item.get("name", ""))
    if typename == "StatusContext":
        return str(item.get("context", ""))
    return ""


def rollup_item_state(item: dict[str, Any]) -> tuple[str, bool, bool]:
    typename = str(item.get("__typename", ""))
    if typename == "CheckRun":
        status = str(item.get("status", "")).upper()
        if status != "COMPLETED":
            return status or "PENDING", False, False
        conclusion = str(item.get("conclusion", "")).upper()
        if conclusion in FAILED_CHECK_CONCLUSIONS:
            return conclusion or "FAILURE", True, True
        return conclusion or "SUCCESS", True, False

    if typename == "StatusContext":
        state = str(item.get("state", "")).upper()
        if state in FAILED_STATUS_CONTEXTS:
            return state, True, True
        if state == "SUCCESS":
            return state, True, False
        return state or "PENDING", False, False

    return "", False, False


def summarize_status_check_rollup(payload: dict[str, Any]) -> str:
    lines: list[str] = []
    for item in payload.get("statusCheckRollup", []):
        name = rollup_item_name(item)
        if not name:
            continue
        state, _terminal, _failed = rollup_item_state(item)
        lines.append(f"{name}: {state}")
    return "\n".join(lines) or "(no checks reported)"


def checks_complete(payload: dict[str, Any], required: set[str]) -> tuple[bool, bool]:
    required_remaining = set(required)
    all_present_terminal = True
    saw_any = False

    for item in payload.get("statusCheckRollup", []):
        name = rollup_item_name(item)
        if not name:
            continue
        saw_any = True
        state, terminal, failed = rollup_item_state(item)
        if failed:
            return False, True
        if required:
            if name in required_remaining and terminal and state in SUCCESSFUL_CHECK_CONCLUSIONS | {"SUCCESS"}:
                required_remaining.discard(name)
            elif name in required and not terminal:
                all_present_terminal = False
            elif name in required and terminal and state not in SUCCESSFUL_CHECK_CONCLUSIONS | {"SUCCESS"}:
                return False, True
        elif not terminal:
            all_present_terminal = False

    if required:
        return not required_remaining and all_present_terminal, False
    return saw_any and all_present_terminal, False


def wait_for_pr_checks(runner: Runner, repo: str, pr_number: int, timeout_minutes: int) -> tuple[bool, str]:
    deadline = time.time() + max(60, timeout_minutes * 60)
    payload: dict[str, Any] = {}
    required: set[str] | None = None

    while time.time() < deadline:
        payload = gh_json(runner, ["pr", "view", str(pr_number), "--repo", repo, "--json", "baseRefName,statusCheckRollup"])
        if required is None:
            required = set(required_status_checks(runner, repo, str(payload.get("baseRefName", ""))))

        summary = summarize_status_check_rollup(payload)
        complete, failed = checks_complete(payload, required or set())
        if complete:
            return True, summary
        if failed:
            return False, summary
        time.sleep(10)

    return False, f"timed out waiting for PR #{pr_number} checks after {timeout_minutes}m\n{summarize_status_check_rollup(payload)}"


def status_check_snapshot(payload: dict[str, Any]) -> tuple[tuple[str, str, str, str], ...]:
    snapshot: list[tuple[str, str, str, str]] = []
    for item in payload.get("statusCheckRollup", []):
        typename = str(item.get("__typename", ""))
        if typename == "CheckRun":
            name = str(item.get("name", ""))
            status = str(item.get("status", ""))
            started = str(item.get("startedAt", ""))
            completed = str(item.get("completedAt", ""))
        elif typename == "StatusContext":
            name = str(item.get("context", ""))
            status = str(item.get("state", ""))
            started = str(item.get("startedAt", ""))
            completed = ""
        else:
            continue
        snapshot.append((name, status, started, completed))
    return tuple(sorted(snapshot))


def wait_for_check_refresh(
    runner: Runner,
    repo: str,
    pr_number: int,
    before: tuple[tuple[str, str, str, str], ...],
    *,
    timeout_seconds: int = 60,
) -> None:
    deadline = time.time() + timeout_seconds
    while time.time() < deadline:
        pr = gh_json(runner, ["pr", "view", str(pr_number), "--repo", repo, "--json", "statusCheckRollup"])
        after = status_check_snapshot(pr)
        if after != before:
            return
        time.sleep(5)
    raise CmdError(f"timed out waiting for PR #{pr_number} checks to refresh after marking ready")


def required_status_checks(runner: Runner, repo: str, base_branch: str) -> list[str]:
    try:
        payload = json.loads(runner.run(["gh", "api", f"repos/{repo}/branches/{base_branch}/protection"], timeout=60))
    except CmdError as exc:
        text = stringify_exc(exc)
        if "Branch not found" in text or "404" in text:
            return []
        raise

    checks = payload.get("required_status_checks") or {}
    contexts = checks.get("contexts") or []
    return [str(context) for context in contexts if context]


def list_unresolved_review_threads(runner: Runner, repo: str, pr_number: int) -> list[ReviewThread]:
    owner, name = split_repo(repo)
    query = """
query($owner:String!, $repo:String!, $number:Int!) {
  repository(owner:$owner, name:$repo) {
    pullRequest(number:$number) {
      reviewThreads(first:100) {
        nodes {
          id
          isResolved
          path
          line
          comments(first:20) {
            nodes {
              author { login }
              body
              url
            }
          }
        }
      }
    }
  }
}
""".strip()
    payload = gh_graphql(
        runner,
        query,
        {"owner": owner, "repo": name, "number": pr_number},
    )
    nodes = (
        payload.get("data", {})
        .get("repository", {})
        .get("pullRequest", {})
        .get("reviewThreads", {})
        .get("nodes", [])
    )
    threads: list[ReviewThread] = []
    for node in nodes:
        if node.get("isResolved"):
            continue
        comments = node.get("comments", {}).get("nodes", [])
        comment = comments[-1] if comments else {}
        line = node.get("line")
        try:
            thread_line = int(line) if line is not None else None
        except (TypeError, ValueError):
            thread_line = None
        threads.append(
            ReviewThread(
                id=str(node.get("id", "")),
                path=str(node.get("path") or ""),
                line=thread_line,
                author_login=str(comment.get("author", {}).get("login") or "unknown"),
                body=str(comment.get("body") or ""),
                url=str(comment.get("url") or ""),
            )
        )
    return [thread for thread in threads if thread.id]


def summarize_review_threads(threads: list[ReviewThread]) -> str:
    lines = [
        "Unresolved PR review threads are blocking merge.",
        "Address the feedback on the existing PR, push any needed updates, then resolve each addressed thread before handing back to the conductor.",
    ]
    for thread in threads:
        location = thread.path or "(unknown path)"
        if thread.line is not None:
            location = f"{location}:{thread.line}"
        body = " ".join(thread.body.split())
        if len(body) > 280:
            body = body[:277].rstrip() + "..."
        lines.append(f"- {location} by @{thread.author_login}: {body} ({thread.url})")
    return "\n".join(lines)


def resolve_review_threads(runner: Runner, repo: str, pr_number: int, thread_ids: list[str]) -> None:
    _ = (repo, pr_number)
    query = """
mutation($threadId:ID!) {
  resolveReviewThread(input:{threadId:$threadId}) {
    thread {
      isResolved
    }
  }
}
""".strip()
    for thread_id in thread_ids:
        gh_graphql(runner, query, {"threadId": thread_id})


def present_pr_status_checks(runner: Runner, repo: str, pr_number: int) -> tuple[str, set[str]]:
    pr = gh_json(runner, ["pr", "view", str(pr_number), "--repo", repo, "--json", "baseRefName,statusCheckRollup"])
    names: set[str] = set()
    for item in pr.get("statusCheckRollup", []):
        typename = item.get("__typename")
        if typename == "CheckRun":
            name = item.get("name")
        elif typename == "StatusContext":
            name = item.get("context")
        else:
            name = None
        if name:
            names.add(str(name))
    return str(pr["baseRefName"]), names


def ensure_required_checks_present(runner: Runner, repo: str, pr_number: int) -> None:
    base_branch, present = present_pr_status_checks(runner, repo, pr_number)
    required = required_status_checks(runner, repo, base_branch)
    missing = [context for context in required if context not in present]
    if missing:
        joined = ", ".join(sorted(missing))
        raise CmdError(
            f"required status checks missing for PR #{pr_number} on {base_branch}: {joined}"
        )


def wait_for_pr_merged(runner: Runner, repo: str, pr_number: int, *, timeout_seconds: int = 600) -> None:
    deadline = time.time() + timeout_seconds
    while time.time() < deadline:
        pr = gh_json(runner, ["pr", "view", str(pr_number), "--repo", repo, "--json", "state,mergedAt"])
        if pr.get("mergedAt"):
            return
        if pr["state"] == "CLOSED":
            raise CmdError(f"PR #{pr_number} closed without merging")
        time.sleep(5)
    raise CmdError(f"timed out waiting for PR #{pr_number} to merge")


def merge_mode() -> str:
    return os.environ.get("BB_PR_MERGE_MODE", "auto").strip().lower() or "auto"


def merge_pr(runner: Runner, repo: str, pr_number: int) -> None:
    argv = ["gh", "pr", "merge", str(pr_number), "--repo", repo, "--squash", "--delete-branch"]
    mode = merge_mode()

    if mode == "admin":
        runner.run([*argv, "--admin"], timeout=120)
        return
    if mode == "normal":
        runner.run(argv, timeout=120)
        return

    try:
        runner.run(argv, timeout=120)
        return
    except CmdError as exc:
        if "--auto" not in str(exc):
            raise
    runner.run([*argv, "--auto"], timeout=120)
    wait_for_pr_merged(runner, repo, pr_number)


def ensure_pr_ready(runner: Runner, repo: str, pr_number: int) -> bool:
    pr = gh_json(runner, ["pr", "view", str(pr_number), "--repo", repo, "--json", "isDraft,statusCheckRollup"])
    if not pr["isDraft"]:
        return False
    before = status_check_snapshot(pr)
    runner.run(["gh", "pr", "ready", str(pr_number), "--repo", repo], timeout=60)
    wait_for_check_refresh(runner, repo, pr_number, before)
    return True


def best_effort_issue_comment(
    runner: Runner,
    conn: sqlite3.Connection,
    event_log: pathlib.Path,
    run_id: str,
    repo: str,
    issue_number: int,
    body: str,
    *,
    event_type: str,
) -> None:
    try:
        comment_issue(runner, repo, issue_number, body)
    except CmdError as exc:
        record_event(conn, event_log, run_id, event_type, {"error": stringify_exc(exc), "body": body})


def best_effort_pr_comment(
    runner: Runner,
    conn: sqlite3.Connection,
    event_log: pathlib.Path,
    run_id: str,
    repo: str,
    pr_number: int,
    body: str,
    *,
    event_type: str,
) -> None:
    try:
        comment_pr(runner, repo, pr_number, body)
    except CmdError as exc:
        record_event(conn, event_log, run_id, event_type, {"error": stringify_exc(exc), "body": body})


def dispatch(runner: Runner, sprite: str, prompt: str, repo: str, prompt_template: pathlib.Path, timeout_minutes: int) -> None:
    bb_bin = str(ROOT / "bin" / "bb")
    runner.run(
        [
            bb_bin,
            "dispatch",
            sprite,
            prompt,
            "--repo",
            repo,
            "--prompt-template",
            str(prompt_template),
            "--timeout",
            f"{timeout_minutes}m",
        ],
        timeout=max(300, timeout_minutes * 60 + 120),
    )


def dispatch_command(sprite: str, prompt: str, repo: str, prompt_template: pathlib.Path, timeout_minutes: int) -> list[str]:
    bb_bin = str(ROOT / "bin" / "bb")
    return [
        bb_bin,
        "dispatch",
        sprite,
        prompt,
        "--repo",
        repo,
        "--prompt-template",
        str(prompt_template),
        "--timeout",
        f"{timeout_minutes}m",
    ]


def cleanup_sprite_processes(runner: Runner, sprite: str) -> None:
    bb_bin = str(ROOT / "bin" / "bb")
    try:
        runner.run([bb_bin, "kill", sprite], timeout=120)
    except CmdError:
        return


def read_log_tail(path: pathlib.Path, *, max_chars: int = 4000) -> str:
    if not path.exists():
        return ""
    text = path.read_text(encoding="utf-8", errors="replace")
    if len(text) <= max_chars:
        return text
    return text[-max_chars:]


def start_dispatch_session(
    sprite: str,
    prompt: str,
    repo: str,
    prompt_template: pathlib.Path,
    timeout_minutes: int,
) -> DispatchSession:
    argv = dispatch_command(sprite, prompt, repo, prompt_template, timeout_minutes)
    with tempfile.NamedTemporaryFile(prefix="bb-dispatch-", suffix=".log", delete=False) as fh:
        log_path = pathlib.Path(fh.name)
        proc = subprocess.Popen(
            argv,
            cwd=ROOT,
            text=True,
            stdout=fh,
            stderr=fh,
        )
    return DispatchSession(
        task=DispatchTask(sprite=sprite, prompt=prompt, artifact_path=""),
        argv=argv,
        proc=proc,
        log_path=log_path,
    )


def stop_dispatch_session(runner: Runner, session: DispatchSession, *, reap_sprite: bool) -> None:
    try:
        if reap_sprite:
            cleanup_sprite_processes(runner, session.task.sprite)
        if session.proc.poll() is None:
            session.proc.terminate()
            try:
                session.proc.wait(timeout=15)
            except subprocess.TimeoutExpired:
                session.proc.kill()
                session.proc.wait(timeout=15)
    finally:
        try:
            session.log_path.unlink()
        except FileNotFoundError:
            pass


def session_exit_error(session: DispatchSession, artifact_error: str) -> CmdError:
    return CmdError(
        f"dispatch exited before artifact was ready ({session.proc.returncode}): "
        f"{' '.join(shlex.quote(a) for a in session.argv)}\n"
        f"artifact error:\n{artifact_error}\n"
        f"dispatch log:\n{read_log_tail(session.log_path)}"
    )


def artifact_timeout_error(session: DispatchSession, timeout_seconds: int) -> CmdError:
    return CmdError(
        f"artifact not available after {timeout_seconds}s: {session.task.artifact_path} on {session.task.sprite}\n"
        f"last error:\n{session.last_error}\n"
        f"dispatch log:\n{read_log_tail(session.log_path)}"
    )


def dispatch_tasks_until_artifacts(
    runner: Runner,
    tasks: list[DispatchTask],
    repo: str,
    prompt_template: pathlib.Path,
    timeout_minutes: int,
    *,
    poll_seconds: int = 5,
    on_artifact: Callable[[str, dict[str, Any]], None] | None = None,
    on_tick: Callable[[], None] | None = None,
) -> dict[str, dict[str, Any]]:
    artifact_timeout = max(90, timeout_minutes * 60 + 60)
    deadline = time.time() + artifact_timeout
    sessions: dict[str, DispatchSession] = {}
    payloads: dict[str, dict[str, Any]] = {}

    for task in tasks:
        session = start_dispatch_session(task.sprite, task.prompt, repo, prompt_template, timeout_minutes)
        session.task.artifact_path = task.artifact_path
        sessions[task.sprite] = session

    try:
        while sessions and time.time() < deadline:
            if on_tick is not None:
                on_tick()

            for sprite in list(sessions):
                session = sessions[sprite]
                try:
                    payload = fetch_json_artifact(runner, sprite, session.task.artifact_path)
                except (CmdError, json.JSONDecodeError) as exc:
                    session.last_error = stringify_exc(exc)
                else:
                    payloads[sprite] = payload
                    stop_dispatch_session(runner, session, reap_sprite=True)
                    if on_artifact is not None:
                        on_artifact(sprite, payload)
                    del sessions[sprite]
                    continue

                if session.proc.poll() is None:
                    continue

                if session.proc.returncode == 0:
                    try:
                        payload = wait_for_json_artifact(
                            runner,
                            sprite,
                            session.task.artifact_path,
                            timeout_seconds=5,
                            poll_seconds=1,
                        )
                    except CmdError as exc:
                        session.last_error = stringify_exc(exc)
                    else:
                        payloads[sprite] = payload
                        stop_dispatch_session(runner, session, reap_sprite=False)
                        if on_artifact is not None:
                            on_artifact(sprite, payload)
                        del sessions[sprite]
                        continue

                raise session_exit_error(session, session.last_error)

            if sessions:
                time.sleep(poll_seconds)

        if sessions:
            pending = list(sessions.values())
            raise artifact_timeout_error(pending[0], artifact_timeout)

        return payloads
    finally:
        for sprite in list(sessions):
            stop_dispatch_session(runner, sessions[sprite], reap_sprite=True)
            del sessions[sprite]


def dispatch_until_artifact(
    runner: Runner,
    sprite: str,
    prompt: str,
    repo: str,
    prompt_template: pathlib.Path,
    timeout_minutes: int,
    artifact_path: str,
) -> dict[str, Any]:
    payloads = dispatch_tasks_until_artifacts(
        runner,
        [DispatchTask(sprite=sprite, prompt=prompt, artifact_path=artifact_path)],
        repo,
        prompt_template,
        timeout_minutes,
    )
    return payloads[sprite]


def run_builder(
    runner: Runner,
    repo: str,
    worker: str,
    issue: Issue,
    run_id: str,
    branch: str,
    prompt_template: pathlib.Path,
    timeout_minutes: int,
    feedback: str | None = None,
    pr_number: int | None = None,
    pr_url: str | None = None,
) -> tuple[BuilderResult, dict[str, Any]]:
    cleanup_sprite_processes(runner, worker)
    builder_rel = artifact_rel(run_id, "builder-result.json")
    builder_prompt = build_builder_task(
        issue,
        run_id,
        branch,
        builder_rel,
        feedback=feedback,
        pr_number=pr_number,
        pr_url=pr_url,
    )
    payload = dispatch_until_artifact(
        runner,
        worker,
        builder_prompt,
        repo,
        prompt_template,
        timeout_minutes,
        artifact_abs(repo, builder_rel),
    )
    builder = parse_builder_result(payload, branch)
    builder.pr_number, builder.pr_url = verify_builder_pr(runner, repo, builder.pr_number, branch)
    return builder, payload


def run_review_round(
    runner: Runner,
    conn: sqlite3.Connection,
    event_log: pathlib.Path,
    repo: str,
    issue: Issue,
    run_id: str,
    pr_number: int,
    pr_url: str,
    reviewers: list[str],
    prompt_template: pathlib.Path,
    timeout_minutes: int,
    *,
    on_tick: Callable[[], None] | None = None,
) -> list[ReviewResult]:
    reviews: dict[str, ReviewResult] = {}
    tasks: list[DispatchTask] = []
    for reviewer in reviewers:
        cleanup_sprite_processes(runner, reviewer)
        review_rel = artifact_rel(run_id, f"review-{reviewer}.json")
        review_prompt = build_review_task(issue, run_id, pr_number, pr_url, review_rel)
        tasks.append(
            DispatchTask(
                sprite=reviewer,
                prompt=review_prompt,
                artifact_path=artifact_abs(repo, review_rel),
            )
        )

    def handle_artifact(reviewer: str, payload: dict[str, Any]) -> None:
        review = parse_review_result(reviewer, payload)
        persist_review(conn, run_id, review)
        reviews[reviewer] = review
        record_event(conn, event_log, run_id, "review_complete", {"reviewer": review.reviewer, "verdict": review.verdict})

    dispatch_tasks_until_artifacts(
        runner,
        tasks,
        repo,
        prompt_template,
        timeout_minutes,
        on_artifact=handle_artifact,
        on_tick=on_tick,
    )
    return [reviews[reviewer] for reviewer in reviewers]


def summarize_reviews(reviews: list[ReviewResult]) -> str:
    chunks: list[str] = []
    for review in reviews:
        chunks.append(f"{review.reviewer}: verdict={review.verdict} summary={review.summary}")
        for finding in review.findings:
            severity = finding.get("severity", "unknown")
            path = finding.get("path", "")
            line = finding.get("line", "")
            message = finding.get("message", "")
            location = path
            if line:
                location = f"{location}:{line}" if location else str(line)
            chunks.append(f"- {severity} {location} {message}".strip())
    return "\n".join(chunks)


def persist_review(conn: sqlite3.Connection, run_id: str, review: ReviewResult) -> None:
    conn.execute(
        """
        insert or replace into reviews (
            run_id, reviewer_sprite, verdict, summary, findings_json, created_at
        ) values (?, ?, ?, ?, ?, ?)
        """,
        (run_id, review.reviewer, review.verdict, review.summary, json.dumps(review.findings), now_utc()),
    )
    conn.commit()


def format_council_comment(reviews: list[ReviewResult]) -> str:
    lines = ["Bitterblossom reviewer council results:"]
    for review in reviews:
        lines.append(f"- `{review.reviewer}`: `{review.verdict}` — {review.summary}")
        for finding in review.findings[:5]:
            severity = finding.get("severity", "unknown")
            message = finding.get("message", "")
            path = finding.get("path", "")
            line = finding.get("line")
            location = path
            if line:
                location = f"{location}:{line}" if location else str(line)
            detail = f"{severity} {location} {message}".strip()
            lines.append(f"  - {detail}")
    return "\n".join(lines)


def run_once(args: argparse.Namespace) -> int:
    runner = Runner(ROOT)
    conn = open_db(pathlib.Path(args.db))
    event_log = pathlib.Path(args.event_log)

    if args.issue:
        issue = get_issue(runner, args.repo, args.issue)
    else:
        issues = list_candidate_issues(runner, args.repo, args.label, args.limit)
        issue = pick_issue(conn, issues, args.repo)
        if issue is None:
            print("no eligible issues")
            return 0

    run_id = run_id_for(issue.number)
    if not acquire_lease(conn, args.repo, issue.number, run_id):
        print(f"issue #{issue.number} already leased")
        return 0

    create_run(conn, run_id, args.repo, issue, args.builder_profile)
    merged = False
    max_pr_feedback_rounds = getattr(args, "max_pr_feedback_rounds", 1)

    try:
        record_event(conn, event_log, run_id, "lease_acquired", {"issue": issue.number})
        best_effort_issue_comment(
            runner,
            conn,
            event_log,
            run_id,
            args.repo,
            issue.number,
            f"Bitterblossom lease acquired for `{run_id}`.",
            event_type="issue_comment_failed",
        )
        worker = select_worker(runner, args.repo, args.worker, pathlib.Path(args.builder_template))
        update_run(conn, run_id, phase="building", builder_sprite=worker)
        touch_run(conn, args.repo, issue.number, run_id, args.builder_timeout * 60 + DEFAULT_LEASE_BUFFER_SECONDS)
        record_event(conn, event_log, run_id, "builder_selected", {"sprite": worker})

        branch = branch_name(issue.number, issue.title, run_id)
        builder, builder_payload = run_builder(
            runner,
            args.repo,
            worker,
            issue,
            run_id,
            branch,
            pathlib.Path(args.builder_template),
            args.builder_timeout,
        )
        update_run(
            conn,
            run_id,
            phase="reviewing",
            branch=builder.branch,
            pr_number=builder.pr_number,
            pr_url=builder.pr_url,
        )
        record_event(conn, event_log, run_id, "builder_complete", builder_payload)

        review_rounds = 0
        ci_rounds = 0
        pr_feedback_rounds = 0
        last_pr_feedback_thread_ids: tuple[str, ...] = ()
        while True:
            touch_run(
                conn,
                args.repo,
                issue.number,
                run_id,
                args.review_timeout * 60 * max(1, len(args.reviewer)) + DEFAULT_LEASE_BUFFER_SECONDS,
            )
            reviews = run_review_round(
                runner,
                conn,
                event_log,
                args.repo,
                issue,
                run_id,
                builder.pr_number,
                builder.pr_url,
                args.reviewer,
                pathlib.Path(args.reviewer_template),
                args.review_timeout,
                on_tick=lambda: touch_run(
                    conn,
                    args.repo,
                    issue.number,
                    run_id,
                    args.review_timeout * 60 + DEFAULT_LEASE_BUFFER_SECONDS,
                ),
            )

            best_effort_pr_comment(
                runner,
                conn,
                event_log,
                run_id,
                args.repo,
                builder.pr_number,
                format_council_comment(reviews),
                event_type="pr_comment_failed",
            )

            passes = sum(1 for review in reviews if review.verdict == "pass")
            blocks = [review for review in reviews if review.verdict == "block"]
            fixes = [review for review in reviews if review.verdict == "fix"]

            if blocks or passes < args.review_quorum:
                if review_rounds >= args.max_revision_rounds:
                    update_run(conn, run_id, phase="blocked", status="blocked")
                    record_event(conn, event_log, run_id, "council_blocked", {"reviews": [asdict(review) for review in reviews]})
                    best_effort_issue_comment(
                        runner,
                        conn,
                        event_log,
                        run_id,
                        args.repo,
                        issue.number,
                        f"Bitterblossom blocked `{run_id}` after review.",
                        event_type="issue_comment_failed",
                    )
                    return 2

                feedback = summarize_reviews(blocks + fixes)
                update_run(conn, run_id, phase="revising")
                record_event(conn, event_log, run_id, "revision_requested", {"feedback": feedback, "reason": "review"})
                builder, builder_payload = run_builder(
                    runner,
                    args.repo,
                    worker,
                    issue,
                    run_id,
                    branch,
                    pathlib.Path(args.builder_template),
                    args.builder_timeout,
                    feedback=feedback,
                    pr_number=builder.pr_number,
                    pr_url=builder.pr_url,
                )
                update_run(conn, run_id, phase="reviewing", branch=builder.branch, pr_number=builder.pr_number, pr_url=builder.pr_url)
                record_event(conn, event_log, run_id, "builder_revised", builder_payload)
                review_rounds += 1
                continue

            update_run(conn, run_id, phase="ci_wait")
            touch_run(conn, args.repo, issue.number, run_id, args.ci_timeout * 60 + DEFAULT_LEASE_BUFFER_SECONDS)
            ensure_pr_ready(runner, args.repo, builder.pr_number)
            ok, checks_output = wait_for_pr_checks(runner, args.repo, builder.pr_number, args.ci_timeout)
            record_event(conn, event_log, run_id, "ci_wait_complete", {"passed": ok, "output": checks_output})
            if ok:
                ensure_required_checks_present(runner, args.repo, builder.pr_number)
                unresolved_threads = list_unresolved_review_threads(runner, args.repo, builder.pr_number)
                if unresolved_threads:
                    thread_ids = tuple(sorted(thread.id for thread in unresolved_threads))
                    if pr_feedback_rounds > 0 and thread_ids == last_pr_feedback_thread_ids:
                        resolve_review_threads(runner, args.repo, builder.pr_number, list(thread_ids))
                        record_event(
                            conn,
                            event_log,
                            run_id,
                            "review_threads_resolved",
                            {"pr_number": builder.pr_number, "thread_ids": list(thread_ids), "reason": "unchanged_after_revision"},
                        )
                        unresolved_threads = list_unresolved_review_threads(runner, args.repo, builder.pr_number)
                    if unresolved_threads:
                        if pr_feedback_rounds >= max_pr_feedback_rounds:
                            update_run(conn, run_id, phase="blocked", status="blocked")
                            record_event(
                                conn,
                                event_log,
                                run_id,
                                "pr_feedback_blocked",
                                {
                                    "pr_number": builder.pr_number,
                                    "threads": [asdict(thread) for thread in unresolved_threads],
                                },
                            )
                            best_effort_issue_comment(
                                runner,
                                conn,
                                event_log,
                                run_id,
                                args.repo,
                                issue.number,
                                f"Bitterblossom blocked `{run_id}` because PR review threads still require resolution.",
                                event_type="issue_comment_failed",
                            )
                            return 2

                        feedback = summarize_review_threads(unresolved_threads)
                        update_run(conn, run_id, phase="revising")
                        record_event(
                            conn,
                            event_log,
                            run_id,
                            "revision_requested",
                            {
                                "feedback": feedback,
                                "reason": "pr_feedback",
                                "pr_number": builder.pr_number,
                                "threads": [asdict(thread) for thread in unresolved_threads],
                            },
                        )
                        last_pr_feedback_thread_ids = thread_ids
                        builder, builder_payload = run_builder(
                            runner,
                            args.repo,
                            worker,
                            issue,
                            run_id,
                            branch,
                            pathlib.Path(args.builder_template),
                            args.builder_timeout,
                            feedback=feedback,
                            pr_number=builder.pr_number,
                            pr_url=builder.pr_url,
                        )
                        update_run(
                            conn,
                            run_id,
                            phase="reviewing",
                            branch=builder.branch,
                            pr_number=builder.pr_number,
                            pr_url=builder.pr_url,
                        )
                        record_event(conn, event_log, run_id, "builder_revised", builder_payload)
                        pr_feedback_rounds += 1
                        continue
                break

            if ci_rounds >= args.max_ci_rounds:
                update_run(conn, run_id, phase="failed", status="failed")
                best_effort_issue_comment(
                    runner,
                    conn,
                    event_log,
                    run_id,
                    args.repo,
                    issue.number,
                    f"Bitterblossom failed `{run_id}` because PR checks did not pass.",
                    event_type="issue_comment_failed",
                )
                return 1

            feedback = f"CI checks failed for PR #{builder.pr_number}:\n{checks_output}"
            update_run(conn, run_id, phase="revising")
            record_event(conn, event_log, run_id, "revision_requested", {"feedback": feedback, "reason": "ci"})
            builder, builder_payload = run_builder(
                runner,
                args.repo,
                worker,
                issue,
                run_id,
                branch,
                pathlib.Path(args.builder_template),
                args.builder_timeout,
                feedback=feedback,
                pr_number=builder.pr_number,
                pr_url=builder.pr_url,
            )
            update_run(conn, run_id, phase="reviewing", branch=builder.branch, pr_number=builder.pr_number, pr_url=builder.pr_url)
            record_event(conn, event_log, run_id, "builder_revised", builder_payload)
            ci_rounds += 1
            continue

        update_run(conn, run_id, phase="merge_ready")
        touch_run(conn, args.repo, issue.number, run_id, 600)
        merge_pr(runner, args.repo, builder.pr_number)
        update_run(conn, run_id, phase="merged", status="merged")
        merged = True
        record_event(conn, event_log, run_id, "merged", {"pr_number": builder.pr_number, "pr_url": builder.pr_url})
        best_effort_issue_comment(
            runner,
            conn,
            event_log,
            run_id,
            args.repo,
            issue.number,
            f"Bitterblossom merged `{run_id}` via PR #{builder.pr_number}.",
            event_type="issue_comment_failed",
        )
        return 0
    except CmdError as exc:
        if merged:
            record_event(conn, event_log, run_id, "post_merge_warning", {"error": stringify_exc(exc)})
            return 0
        update_run(conn, run_id, phase="failed", status="failed")
        record_event(conn, event_log, run_id, "command_failed", {"error": str(exc)})
        best_effort_issue_comment(
            runner,
            conn,
            event_log,
            run_id,
            args.repo,
            issue.number,
            f"Bitterblossom failed `{run_id}`.\n\n```\n{str(exc)[:1500]}\n```",
            event_type="issue_comment_failed",
        )
        return 1
    except Exception as exc:  # noqa: BLE001
        if merged:
            record_event(conn, event_log, run_id, "post_merge_warning", {"error": stringify_exc(exc)})
            return 0
        update_run(conn, run_id, phase="failed", status="failed")
        message = f"unexpected conductor error: {stringify_exc(exc)}"
        record_event(conn, event_log, run_id, "unexpected_error", {"error": message})
        best_effort_issue_comment(
            runner,
            conn,
            event_log,
            run_id,
            args.repo,
            issue.number,
            f"Bitterblossom failed `{run_id}`.\n\n```\n{message[:1500]}\n```",
            event_type="issue_comment_failed",
        )
        return 1
    finally:
        release_lease(conn, args.repo, issue.number)


def loop(args: argparse.Namespace) -> int:
    while True:
        rc = run_once(args)
        if rc not in (0,):
            return rc
        if args.issue:
            return 0
        time.sleep(args.poll_seconds)


def show_runs(args: argparse.Namespace) -> int:
    conn = open_db(pathlib.Path(args.db))
    rows = conn.execute(
        """
        select run_id, issue_number, issue_title, phase, status, builder_sprite, pr_number, updated_at
        from runs
        order by created_at desc
        limit ?
        """,
        (args.limit,),
    ).fetchall()
    for row in rows:
        print(
            json.dumps(
                {
                    "run_id": row["run_id"],
                    "issue_number": row["issue_number"],
                    "issue_title": row["issue_title"],
                    "phase": row["phase"],
                    "status": row["status"],
                    "builder_sprite": row["builder_sprite"],
                    "pr_number": row["pr_number"],
                    "updated_at": row["updated_at"],
                }
            )
        )
    return 0


def show_events(args: argparse.Namespace) -> int:
    conn = open_db(pathlib.Path(args.db))
    rows = conn.execute(
        """
        select run_id, event_type, payload_json, created_at
        from events
        where run_id = ?
        order by id desc
        limit ?
        """,
        (args.run_id, args.limit),
    ).fetchall()
    for row in rows:
        print(
            json.dumps(
                {
                    "run_id": row["run_id"],
                    "event_type": row["event_type"],
                    "payload": json.loads(row["payload_json"]),
                    "created_at": row["created_at"],
                }
            )
        )
    return 0


def reconcile_run(args: argparse.Namespace) -> int:
    runner = Runner(ROOT)
    conn = open_db(pathlib.Path(args.db))
    event_log = pathlib.Path(args.event_log)
    row = conn.execute(
        """
        select run_id, repo, issue_number, phase, status, pr_number, pr_url
        from runs
        where run_id = ?
        """,
        (args.run_id,),
    ).fetchone()
    if row is None:
        raise CmdError(f"unknown run_id: {args.run_id}")
    if row["pr_number"] is None:
        raise CmdError(f"run {args.run_id} has no PR to reconcile")

    pr = gh_json(
        runner,
        [
            "pr",
            "view",
            str(row["pr_number"]),
            "--repo",
            row["repo"],
            "--json",
            "number,url,state,mergedAt",
        ],
    )
    payload = {
        "pr_number": pr["number"],
        "pr_url": pr["url"],
        "state": pr["state"],
        "merged_at": pr.get("mergedAt"),
    }
    if pr.get("mergedAt"):
        update_run(conn, args.run_id, phase="merged", status="merged", pr_url=pr["url"])
        record_event(conn, event_log, args.run_id, "reconciled_merged", payload)
    elif pr["state"] == "OPEN":
        update_run(conn, args.run_id, pr_url=pr["url"])
        record_event(conn, event_log, args.run_id, "reconciled_open", payload)
    else:
        update_run(conn, args.run_id, phase="closed", status="closed", pr_url=pr["url"])
        record_event(conn, event_log, args.run_id, "reconciled_closed", payload)

    print(json.dumps({"run_id": args.run_id, **payload}))
    return 0


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Bitterblossom conductor MVP")
    sub = parser.add_subparsers(dest="cmd", required=True)

    def add_common(p: argparse.ArgumentParser) -> None:
        p.add_argument("--repo", required=True)
        p.add_argument("--db", default=str(DEFAULT_DB))
        p.add_argument("--event-log", default=str(DEFAULT_EVENT_LOG))
        p.add_argument("--worker", action="append", required=True)
        p.add_argument("--reviewer", action="append", required=True)
        p.add_argument("--issue", type=int)
        p.add_argument("--label", default=DEFAULT_LABEL)
        p.add_argument("--limit", type=int, default=25)
        p.add_argument("--builder-template", default=str(DEFAULT_BUILDER_TEMPLATE))
        p.add_argument("--reviewer-template", default=str(DEFAULT_REVIEWER_TEMPLATE))
        p.add_argument("--builder-timeout", type=int, default=45)
        p.add_argument("--review-timeout", type=int, default=20)
        p.add_argument("--ci-timeout", type=int, default=30)
        p.add_argument("--max-ci-rounds", type=int, default=1)
        p.add_argument("--review-quorum", type=int, default=2)
        p.add_argument("--max-revision-rounds", type=int, default=1)
        p.add_argument("--max-pr-feedback-rounds", type=int, default=1)
        p.add_argument("--builder-profile", default="claude-sonnet")

    once_p = sub.add_parser("run-once", help="Run one conductor cycle")
    add_common(once_p)
    once_p.set_defaults(func=run_once)

    loop_p = sub.add_parser("loop", help="Run conductor continuously")
    add_common(loop_p)
    loop_p.add_argument("--poll-seconds", type=int, default=60)
    loop_p.set_defaults(func=loop)

    show_p = sub.add_parser("show-runs", help="Show recent runs")
    show_p.add_argument("--db", default=str(DEFAULT_DB))
    show_p.add_argument("--limit", type=int, default=20)
    show_p.set_defaults(func=show_runs)

    events_p = sub.add_parser("show-events", help="Show recent events for a run")
    events_p.add_argument("--db", default=str(DEFAULT_DB))
    events_p.add_argument("--run-id", required=True)
    events_p.add_argument("--limit", type=int, default=20)
    events_p.set_defaults(func=show_events)

    reconcile_p = sub.add_parser("reconcile-run", help="Reconcile a run against GitHub PR state")
    reconcile_p.add_argument("--db", default=str(DEFAULT_DB))
    reconcile_p.add_argument("--event-log", default=str(DEFAULT_EVENT_LOG))
    reconcile_p.add_argument("--run-id", required=True)
    reconcile_p.set_defaults(func=reconcile_run)

    return parser.parse_args(argv)


def main(argv: list[str]) -> int:
    args = parse_args(argv)
    if args.cmd in {"run-once", "loop", "reconcile-run"}:
        require_runtime_env()
    return int(args.func(args))


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
