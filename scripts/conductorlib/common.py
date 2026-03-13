from __future__ import annotations

import pathlib
import shlex
import sqlite3
import subprocess
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Any


ROOT = pathlib.Path(__file__).resolve().parents[2]
DEFAULT_DB = ROOT / ".bb" / "conductor.db"
DEFAULT_EVENT_LOG = ROOT / ".bb" / "events.jsonl"
DEFAULT_BUILDER_TEMPLATE = ROOT / "scripts" / "prompts" / "conductor-builder-template.md"
DEFAULT_REVIEWER_TEMPLATE = ROOT / "scripts" / "prompts" / "conductor-reviewer-template.md"
DEFAULT_LABEL = "autopilot"
DEFAULT_LEASE_BUFFER_SECONDS = 300
ROUTER_TIMEOUT_SECONDS = 120
WORKSPACE_PREPARE_LOCK_WAIT_SECONDS = 240
WORKSPACE_CLEANUP_LOCK_WAIT_SECONDS = 120
WORKSPACE_PREPARE_ATTEMPTS = 3
WORKSPACE_PREPARE_RETRY_DELAY_SECONDS = 2
SUCCESSFUL_CHECK_CONCLUSIONS = {"SUCCESS", "NEUTRAL", "SKIPPED"}
FAILED_CHECK_CONCLUSIONS = {"FAILURE", "ERROR", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED", "STALE", "STARTUP_FAILURE"}
FAILED_STATUS_CONTEXTS = {"FAILURE", "ERROR"}
QA_DEDUPE_PAGE_SIZE = 100
TRUSTED_REVIEW_AUTHOR_ASSOCIATIONS = {"OWNER", "MEMBER", "COLLABORATOR"}
TRUSTED_REVIEW_BOT_LOGINS = {
    "chatgpt-codex-connector",
    "coderabbitai",
    "coderabbitai[bot]",
    "gemini-code-assist",
    "github-actions",
    "github-actions[bot]",
    "greptile-apps",
    "greptile-apps[bot]",
}
FINDING_CLASSIFICATIONS = {"bug", "risk", "style", "question", "unspecified"}
FINDING_SEVERITIES = {"critical", "high", "medium", "low", "unknown"}
FINDING_DECISIONS = {"fix_now", "defer", "reject", "noise", "pending"}
FINDING_STATUSES = {"open", "addressed", "deferred", "rejected", "duplicate", "pending"}
INACTIVE_FINDING_STATUSES = {"addressed", "deferred", "rejected", "duplicate"}
TRUSTED_THREAD_METADATA_FIELDS = frozenset({"classification", "severity", "decision"})
WORKER_SLOT_ACTIVE = "active"
WORKER_SLOT_DRAINED = "drained"
WORKER_SLOT_STATES = {WORKER_SLOT_ACTIVE, WORKER_SLOT_DRAINED}
REPOSITORY_STATE_ACTIVE = "active"
REPOSITORY_STATE_PAUSED = "paused"
REPOSITORY_STATE_DRAINING = "draining"
REPOSITORY_STATES = {REPOSITORY_STATE_ACTIVE, REPOSITORY_STATE_PAUSED, REPOSITORY_STATE_DRAINING}
DEFAULT_REPOSITORY_DESIRED_CONCURRENCY = 1
WORKER_DRAIN_FAILURE_THRESHOLD = 2
MAX_WORKER_SLOT_COUNT = 1000
TERMINAL_RUN_STATUSES = {"merged", "failed", "blocked", "closed"}
BUILDER_WORKSPACE_CLEANUP_KIND = "builder_workspace_cleanup"
UNSET = object()


@dataclass(slots=True)
class Issue:
    number: int
    title: str
    body: str
    url: str
    labels: list[str]
    updated_at: str = ""


@dataclass(slots=True)
class ReadinessResult:
    ready: bool
    reasons: list[str]


@dataclass(slots=True)
class RouteDecision:
    issue: Issue
    profile: str
    rationale: str
    readiness_failures: dict[int, list[str]]


@dataclass(slots=True)
class QAFinding:
    title: str
    summary: str
    severity: str
    target_url: str
    environment: str
    repro_steps: list[str]
    evidence: list[dict[str, str]]
    dedupe_key: str
    priority_label: str
    labels: list[str]


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
    author_association: str = ""


@dataclass(slots=True)
class ReviewWave:
    id: int
    run_id: str
    kind: str
    ordinal: int
    pr_number: int | None
    status: str
    reviewer_count: int
    started_at: str
    completed_at: str | None


@dataclass(slots=True)
class ReviewWaveReview:
    wave_id: int
    reviewer: str
    verdict: str
    summary: str
    source_kind: str
    payload: dict[str, Any]
    created_at: str


@dataclass(slots=True)
class ReviewFinding:
    id: int | None
    run_id: str
    wave_id: int
    reviewer: str
    source_kind: str
    source_id: str
    fingerprint: str
    classification: str
    severity: str
    decision: str
    status: str
    path: str
    line: int | None
    message: str
    raw: dict[str, Any]
    created_at: str | None = None
    updated_at: str | None = None


@dataclass(slots=True)
class DispatchTask:
    sprite: str
    prompt: str
    artifact_path: str
    workspace: str | None = None


@dataclass(slots=True)
class DispatchSession:
    task: DispatchTask
    argv: list[str]
    proc: Any
    log_path: pathlib.Path
    last_error: str = ""


@dataclass(slots=True)
class LeaseAcquireResult:
    acquired: bool
    reclaimed_run_id: str | None = None
    reason: str | None = None


@dataclass(slots=True)
class WorkerSlot:
    id: int
    repo: str
    worker: str
    slot_index: int
    state: str
    consecutive_failures: int
    current_run_id: str | None
    last_probe_at: str | None
    last_error: str | None
    updated_at: str

    @classmethod
    def from_row(cls, row: sqlite3.Row) -> "WorkerSlot":
        return cls(
            id=int(row["id"]),
            repo=str(row["repo"]),
            worker=str(row["worker"]),
            slot_index=int(row["slot_index"]),
            state=str(row["state"]),
            consecutive_failures=int(row["consecutive_failures"]),
            current_run_id=str(row["current_run_id"]) if row["current_run_id"] is not None else None,
            last_probe_at=str(row["last_probe_at"]) if row["last_probe_at"] is not None else None,
            last_error=str(row["last_error"]) if row["last_error"] is not None else None,
            updated_at=str(row["updated_at"]),
        )


@dataclass(slots=True)
class RepositoryRecord:
    repo: str
    state: str
    desired_concurrency: int
    updated_at: str

    @classmethod
    def from_row(cls, row: sqlite3.Row) -> "RepositoryRecord":
        return cls(
            repo=str(row["repo"]),
            state=str(row["state"]),
            desired_concurrency=int(row["desired_concurrency"]),
            updated_at=str(row["updated_at"]),
        )


@dataclass(slots=True)
class RepositorySchedulingView:
    repo: str
    state: str
    desired_concurrency: int
    active_runs: int
    available_capacity: int
    scheduling_allowed: bool
    scheduling_reason: str | None
    updated_at: str | None = None


@dataclass(slots=True)
class GovernanceRun:
    issue: Issue
    run_id: str
    worker: str
    worker_slot: WorkerSlot
    branch: str
    pr_number: int
    pr_url: str
    builder_workspace: str


class CmdError(RuntimeError):
    pass


class WorkspacePreparationError(RuntimeError):
    pass


class LeaseLostError(RuntimeError):
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


def utc_now() -> datetime:
    return datetime.now(timezone.utc).replace(microsecond=0)


def now_utc() -> str:
    return utc_now().isoformat().replace("+00:00", "Z")


def ensure_parent(path: pathlib.Path) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
