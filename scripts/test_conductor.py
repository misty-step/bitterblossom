from __future__ import annotations

import argparse
import pathlib
import subprocess
import sys

import pytest


sys.path.insert(0, str(pathlib.Path(__file__).parent))
import conductor  # noqa: E402


def test_issue_priority_prefers_explicit_priority_labels() -> None:
    assert conductor.issue_priority(["bug", "P2"]) == (2, "P2")
    assert conductor.issue_priority(["enhancement", "P0"]) == (0, "P0")
    assert conductor.issue_priority(["autopilot"]) == (9, "")


def test_branch_name_is_stable_and_bounded() -> None:
    got = conductor.branch_name(42, "Fix status output for gh auth failures!!!", "run-42-1777")
    assert got.startswith("factory/42-fix-status-output-for-gh-auth-fa-1777")


def test_db_init_and_lease_cycle(tmp_path: pathlib.Path) -> None:
    db_path = tmp_path / "conductor.db"
    conn = conductor.open_db(db_path)

    assert conductor.acquire_lease(conn, "misty-step/bitterblossom", 12, "run-12-1") is True
    assert conductor.acquire_lease(conn, "misty-step/bitterblossom", 12, "run-12-2") is False

    conductor.release_lease(conn, "misty-step/bitterblossom", 12)
    assert conductor.acquire_lease(conn, "misty-step/bitterblossom", 12, "run-12-3") is True


def test_acquire_lease_reclaims_expired_active_lease(tmp_path: pathlib.Path) -> None:
    conn = conductor.open_db(tmp_path / "conductor.db")
    assert conductor.acquire_lease(conn, "misty-step/bitterblossom", 12, "run-12-1") is True
    conn.execute(
        """
        update leases
        set released_at = null, lease_expires_at = '2000-01-01T00:00:00Z'
        where repo = 'misty-step/bitterblossom' and issue_number = 12
        """
    )
    conn.commit()

    assert conductor.acquire_lease(conn, "misty-step/bitterblossom", 12, "run-12-2") is True


def test_pick_issue_skips_leased_and_prefers_higher_priority(tmp_path: pathlib.Path) -> None:
    conn = conductor.open_db(tmp_path / "conductor.db")
    assert conductor.acquire_lease(conn, "misty-step/bitterblossom", 2, "run-2-1") is True

    issues = [
        conductor.Issue(number=2, title="leased p0", body="", url="u2", labels=["autopilot", "P0"], updated_at="2026-03-06T00:00:00Z"),
        conductor.Issue(number=3, title="free p1", body="", url="u3", labels=["autopilot", "P1"], updated_at="2026-03-06T00:00:00Z"),
        conductor.Issue(number=4, title="free p2", body="", url="u4", labels=["autopilot", "P2"], updated_at="2026-03-05T00:00:00Z"),
    ]

    picked = conductor.pick_issue(conn, issues, "misty-step/bitterblossom")
    assert picked is not None
    assert picked.number == 3


def test_pick_issue_reaps_expired_leases(tmp_path: pathlib.Path) -> None:
    conn = conductor.open_db(tmp_path / "conductor.db")
    assert conductor.acquire_lease(conn, "misty-step/bitterblossom", 2, "run-2-1") is True
    conn.execute(
        """
        update leases
        set released_at = null, lease_expires_at = '2000-01-01T00:00:00Z'
        where repo = 'misty-step/bitterblossom' and issue_number = 2
        """
    )
    conn.commit()

    issues = [
        conductor.Issue(number=2, title="expired lease", body="", url="u2", labels=["autopilot", "P1"], updated_at="2026-03-06T00:00:00Z"),
    ]

    picked = conductor.pick_issue(conn, issues, "misty-step/bitterblossom")
    assert picked is not None
    assert picked.number == 2


def test_summarize_reviews_includes_findings() -> None:
    reviews = [
        conductor.ReviewResult(
            reviewer="fern",
            verdict="fix",
            summary="missing test",
            findings=[{"severity": "important", "path": "cmd/bb/status.go", "line": 10, "message": "add coverage"}],
        )
    ]
    summary = conductor.summarize_reviews(reviews)
    assert "fern: verdict=fix summary=missing test" in summary
    assert "important cmd/bb/status.go:10 add coverage" in summary


def test_wait_for_json_artifact_retries_until_available(monkeypatch: pytest.MonkeyPatch) -> None:
    calls = {"count": 0}

    def fake_fetch(_runner: object, _sprite: str, _path: str) -> dict[str, object]:
        calls["count"] += 1
        if calls["count"] < 3:
            raise conductor.CmdError("not ready")
        return {"status": "ready_for_review"}

    monkeypatch.setattr(conductor, "fetch_json_artifact", fake_fetch)
    monkeypatch.setattr(conductor.time, "sleep", lambda _seconds: None)

    payload = conductor.wait_for_json_artifact(object(), "fern", "/tmp/artifact.json", timeout_seconds=1, poll_seconds=0)

    assert payload == {"status": "ready_for_review"}
    assert calls["count"] == 3


def test_wait_for_json_artifact_times_out(monkeypatch: pytest.MonkeyPatch) -> None:
    ticks = iter([0.0, 0.0, 0.5, 1.1])

    monkeypatch.setattr(conductor.time, "time", lambda: next(ticks))
    monkeypatch.setattr(conductor.time, "sleep", lambda _seconds: None)
    monkeypatch.setattr(conductor, "fetch_json_artifact", lambda *_args, **_kwargs: (_ for _ in ()).throw(conductor.CmdError("missing")))

    with pytest.raises(conductor.CmdError, match="artifact not available"):
        conductor.wait_for_json_artifact(object(), "fern", "/tmp/artifact.json", timeout_seconds=1, poll_seconds=0)


class _RunnerSpy:
    def __init__(self, responses: list[str] | None = None) -> None:
        self.responses = responses or []
        self.calls: list[list[str]] = []

    def run(self, argv: list[str], *, timeout: int | None = None, check: bool = True) -> str:
        _ = (timeout, check)
        self.calls.append(argv)
        if self.responses:
            return self.responses.pop(0)
        return ""


class _MergeRunner:
    def __init__(self) -> None:
        self.calls: list[list[str]] = []

    def run(self, argv: list[str], *, timeout: int | None = None, check: bool = True) -> str:
        _ = (timeout, check)
        self.calls.append(argv)
        if argv[:3] == ["gh", "pr", "merge"] and "--auto" not in argv:
            raise conductor.CmdError("base branch policy prohibits the merge. add the `--auto` flag.")
        if argv[:3] == ["gh", "pr", "view"]:
            return '{"state":"MERGED","mergedAt":"2026-03-06T00:00:00Z"}'
        return ""


class _ProcStub:
    def __init__(self, poll_values: list[int | None] | None = None) -> None:
        self.poll_values = poll_values or [None]
        self.wait_calls: list[int] = []
        self.terminated = False
        self.killed = False
        self.returncode: int | None = None

    def poll(self) -> int | None:
        value = self.poll_values.pop(0) if self.poll_values else self.returncode
        if value is not None:
            self.returncode = value
        return value

    def wait(self, timeout: int | None = None) -> int:
        if timeout is not None:
            self.wait_calls.append(timeout)
        if self.returncode is None:
            self.returncode = 0
        return self.returncode

    def terminate(self) -> None:
        self.terminated = True
        self.returncode = 0

    def kill(self) -> None:
        self.killed = True
        self.returncode = 0


def test_dispatch_does_not_depend_on_wait_flag() -> None:
    runner = _RunnerSpy()

    conductor.dispatch(
        runner,
        "fern",
        "ship it",
        "misty-step/bitterblossom",
        pathlib.Path("scripts/prompts/conductor-builder-template.md"),
        10,
    )

    assert runner.calls
    assert "--wait" not in runner.calls[0]


def test_ensure_pr_ready_only_marks_drafts_ready() -> None:
    runner = _RunnerSpy(['{"isDraft": true}', ""])

    conductor.ensure_pr_ready(runner, "misty-step/bitterblossom", 42)

    assert runner.calls == [
        ["gh", "pr", "view", "42", "--repo", "misty-step/bitterblossom", "--json", "isDraft"],
        ["gh", "pr", "ready", "42", "--repo", "misty-step/bitterblossom"],
    ]


def test_dispatch_until_artifact_reaps_sprite_when_artifact_arrives_first(monkeypatch: pytest.MonkeyPatch) -> None:
    proc = _ProcStub([None, 0])

    monkeypatch.setattr(conductor.subprocess, "Popen", lambda *args, **kwargs: proc)
    monkeypatch.setattr(conductor, "fetch_json_artifact", lambda *_args, **_kwargs: {"status": "ready"})
    cleanup_calls: list[str] = []
    monkeypatch.setattr(conductor, "cleanup_sprite_processes", lambda _runner, sprite: cleanup_calls.append(sprite))

    payload = conductor.dispatch_until_artifact(
        _RunnerSpy(),
        "fern",
        "ship it",
        "misty-step/bitterblossom",
        pathlib.Path("scripts/prompts/conductor-builder-template.md"),
        10,
        "/tmp/artifact.json",
    )

    assert payload == {"status": "ready"}
    assert cleanup_calls == ["fern"]
    assert proc.wait_calls


def test_merge_pr_falls_back_to_auto_when_required() -> None:
    runner = _MergeRunner()

    conductor.merge_pr(runner, "misty-step/bitterblossom", 452)

    assert runner.calls == [
        ["gh", "pr", "merge", "452", "--repo", "misty-step/bitterblossom", "--squash", "--delete-branch"],
        ["gh", "pr", "merge", "452", "--repo", "misty-step/bitterblossom", "--squash", "--delete-branch", "--auto"],
        ["gh", "pr", "view", "452", "--repo", "misty-step/bitterblossom", "--json", "state,mergedAt"],
    ]


def test_merge_pr_supports_admin_mode(monkeypatch: pytest.MonkeyPatch) -> None:
    runner = _RunnerSpy()
    monkeypatch.setenv("BB_PR_MERGE_MODE", "admin")

    conductor.merge_pr(runner, "misty-step/bitterblossom", 452)

    assert runner.calls == [
        ["gh", "pr", "merge", "452", "--repo", "misty-step/bitterblossom", "--squash", "--delete-branch", "--admin"]
    ]


def test_parse_builder_result_rejects_branch_mismatch() -> None:
    with pytest.raises(conductor.CmdError, match="branch mismatch"):
        conductor.parse_builder_result(
            {
                "status": "ready_for_review",
                "branch": "wrong",
                "pr_number": 12,
                "pr_url": "https://github.com/misty-step/bitterblossom/pull/12",
                "summary": "done",
                "tests": [],
            },
            "expected",
        )


def test_wait_for_pr_checks_timeout_returns_failure(monkeypatch: pytest.MonkeyPatch) -> None:
    def fake_run(*_args: object, **_kwargs: object) -> object:
        raise subprocess.TimeoutExpired(cmd=["gh"], timeout=60)

    monkeypatch.setattr(conductor.subprocess, "run", fake_run)

    ok, output = conductor.wait_for_pr_checks(_RunnerSpy(), "misty-step/bitterblossom", 42, 5)

    assert ok is False
    assert "timed out waiting for PR #42 checks" in output


def test_ensure_required_checks_present_accepts_matching_contexts() -> None:
    runner = _RunnerSpy(
        [
            '{"baseRefName":"master","statusCheckRollup":[{"__typename":"CheckRun","name":"merge-gate"}]}',
            '{"required_status_checks":{"contexts":["merge-gate"]}}',
        ]
    )

    conductor.ensure_required_checks_present(runner, "misty-step/bitterblossom", 42)

    assert runner.calls == [
        ["gh", "pr", "view", "42", "--repo", "misty-step/bitterblossom", "--json", "baseRefName,statusCheckRollup"],
        ["gh", "api", "repos/misty-step/bitterblossom/branches/master/protection"],
    ]


def test_ensure_required_checks_present_rejects_missing_context() -> None:
    runner = _RunnerSpy(
        [
            '{"baseRefName":"master","statusCheckRollup":[{"__typename":"CheckRun","name":"Go Checks"}]}',
            '{"required_status_checks":{"contexts":["merge-gate","Go Checks"]}}',
        ]
    )

    with pytest.raises(conductor.CmdError, match="required status checks missing.*merge-gate"):
        conductor.ensure_required_checks_present(runner, "misty-step/bitterblossom", 42)


def test_run_once_releases_lease_on_failure_after_comment_error(monkeypatch: pytest.MonkeyPatch, tmp_path: pathlib.Path) -> None:
    issue = conductor.Issue(number=447, title="test", body="body", url="https://example.com/447", labels=["autopilot"])

    monkeypatch.setattr(conductor, "get_issue", lambda *_args, **_kwargs: issue)
    monkeypatch.setattr(conductor, "comment_issue", lambda *_args, **_kwargs: (_ for _ in ()).throw(conductor.CmdError("comment down")))
    monkeypatch.setattr(conductor, "select_worker", lambda *_args, **_kwargs: (_ for _ in ()).throw(conductor.CmdError("worker down")))

    args = argparse.Namespace(
        repo="misty-step/bitterblossom",
        issue=447,
        label="autopilot",
        limit=20,
        db=str(tmp_path / "conductor.db"),
        event_log=str(tmp_path / "events.jsonl"),
        builder_profile="default",
        worker=["noble-blue-serpent"],
        builder_template=str(pathlib.Path("scripts/prompts/conductor-builder-template.md")),
        reviewer=[],
        reviewer_template=str(pathlib.Path("scripts/prompts/conductor-reviewer-template.md")),
        builder_timeout=10,
        review_timeout=10,
        ci_timeout=10,
        review_quorum=2,
        max_revision_rounds=1,
        max_ci_rounds=1,
    )

    rc = conductor.run_once(args)

    assert rc == 1
    conn = conductor.open_db(pathlib.Path(args.db))
    lease = conn.execute(
        "select released_at from leases where repo = ? and issue_number = ?",
        (args.repo, issue.number),
    ).fetchone()
    assert lease is not None
    assert lease["released_at"] is not None


def test_run_once_keeps_merged_truth_when_issue_comment_fails(monkeypatch: pytest.MonkeyPatch, tmp_path: pathlib.Path) -> None:
    issue = conductor.Issue(number=447, title="test", body="body", url="https://example.com/447", labels=["autopilot"])
    builder = conductor.BuilderResult(
        status="ready_for_review",
        branch="factory/447-test-123",
        pr_number=448,
        pr_url="https://github.com/misty-step/bitterblossom/pull/448",
        summary="done",
        tests=[],
    )
    reviews = [
        conductor.ReviewResult(reviewer="fern", verdict="pass", summary="ok", findings=[]),
        conductor.ReviewResult(reviewer="sage", verdict="pass", summary="ok", findings=[]),
        conductor.ReviewResult(reviewer="thorn", verdict="pass", summary="ok", findings=[]),
    ]

    monkeypatch.setattr(conductor, "get_issue", lambda *_args, **_kwargs: issue)
    monkeypatch.setattr(conductor, "select_worker", lambda *_args, **_kwargs: "noble-blue-serpent")
    monkeypatch.setattr(conductor, "run_builder", lambda *_args, **_kwargs: (builder, {"status": "ready_for_review"}))
    monkeypatch.setattr(conductor, "run_review_round", lambda *_args, **_kwargs: reviews)
    monkeypatch.setattr(conductor, "ensure_pr_ready", lambda *_args, **_kwargs: None)
    monkeypatch.setattr(conductor, "wait_for_pr_checks", lambda *_args, **_kwargs: (True, "green"))
    monkeypatch.setattr(conductor, "merge_pr", lambda *_args, **_kwargs: None)
    monkeypatch.setattr(conductor, "comment_pr", lambda *_args, **_kwargs: None)
    monkeypatch.setattr(conductor, "comment_issue", lambda *_args, **_kwargs: (_ for _ in ()).throw(conductor.CmdError("comment down")))

    args = argparse.Namespace(
        repo="misty-step/bitterblossom",
        issue=447,
        label="autopilot",
        limit=20,
        db=str(tmp_path / "conductor.db"),
        event_log=str(tmp_path / "events.jsonl"),
        builder_profile="default",
        worker=["noble-blue-serpent"],
        builder_template=str(pathlib.Path("scripts/prompts/conductor-builder-template.md")),
        reviewer=["fern", "sage", "thorn"],
        reviewer_template=str(pathlib.Path("scripts/prompts/conductor-reviewer-template.md")),
        builder_timeout=10,
        review_timeout=10,
        ci_timeout=10,
        review_quorum=2,
        max_revision_rounds=1,
        max_ci_rounds=1,
    )

    rc = conductor.run_once(args)

    assert rc == 0
    conn = conductor.open_db(pathlib.Path(args.db))
    run = conn.execute("select status, phase from runs limit 1").fetchone()
    assert run is not None
    assert run["status"] == "merged"
    assert run["phase"] == "merged"


def test_reconcile_run_marks_merged(monkeypatch: pytest.MonkeyPatch, tmp_path: pathlib.Path, capsys: pytest.CaptureFixture[str]) -> None:
    conn = conductor.open_db(tmp_path / "conductor.db")
    issue = conductor.Issue(number=450, title="test", body="body", url="https://example.com/450", labels=["autopilot"])
    conductor.create_run(conn, "run-450-1", "misty-step/bitterblossom", issue, "default")
    conductor.update_run(conn, "run-450-1", phase="failed", status="failed", pr_number=452, pr_url="https://example.com/pr/452")

    monkeypatch.setattr(
        conductor,
        "gh_json",
        lambda *_args, **_kwargs: {
            "number": 452,
            "url": "https://github.com/misty-step/bitterblossom/pull/452",
            "state": "MERGED",
            "mergedAt": "2026-03-06T16:33:51Z",
        },
    )

    args = argparse.Namespace(
        db=str(tmp_path / "conductor.db"),
        event_log=str(tmp_path / "events.jsonl"),
        run_id="run-450-1",
    )

    rc = conductor.reconcile_run(args)

    assert rc == 0
    out = capsys.readouterr().out
    assert '"run_id": "run-450-1"' in out

    run = conn.execute("select phase, status, pr_url from runs where run_id = 'run-450-1'").fetchone()
    assert run is not None
    assert run["phase"] == "merged"
    assert run["status"] == "merged"
    assert run["pr_url"] == "https://github.com/misty-step/bitterblossom/pull/452"


def test_show_events_prints_recent_events(tmp_path: pathlib.Path, capsys: pytest.CaptureFixture[str]) -> None:
    conn = conductor.open_db(tmp_path / "conductor.db")
    conductor.record_event(conn, tmp_path / "events.jsonl", "run-1", "lease_acquired", {"issue": 1})
    conductor.record_event(conn, tmp_path / "events.jsonl", "run-1", "builder_selected", {"sprite": "fern"})

    args = argparse.Namespace(db=str(tmp_path / "conductor.db"), run_id="run-1", limit=2)
    rc = conductor.show_events(args)

    assert rc == 0
    lines = [line for line in capsys.readouterr().out.splitlines() if line]
    assert len(lines) == 2
    assert '"event_type": "builder_selected"' in lines[0]
