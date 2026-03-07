from __future__ import annotations

import argparse
import importlib.util
import json
import subprocess
from pathlib import Path
from types import SimpleNamespace

import pytest


MODULE_PATH = (
    Path(__file__).resolve().parents[1]
    / ".claude"
    / "skills"
    / "factory-audit"
    / "scripts"
    / "collect_run_snapshot.py"
)


def load_module():
    spec = importlib.util.spec_from_file_location("collect_run_snapshot", MODULE_PATH)
    assert spec is not None
    assert spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


collect_run_snapshot = load_module()


def test_find_repo_root_raises_snapshot_error_for_missing_git(tmp_path: Path) -> None:
    outside = tmp_path / "outside" / "nested" / "tool.py"
    outside.parent.mkdir(parents=True)
    outside.write_text("#!/usr/bin/env python3\n", encoding="utf-8")

    with pytest.raises(collect_run_snapshot.SnapshotError, match="could not locate repository root"):
        collect_run_snapshot.find_repo_root(outside)


def test_run_json_wraps_decode_error(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(collect_run_snapshot, "run", lambda _argv: "warning banner")

    with pytest.raises(collect_run_snapshot.SnapshotError, match="non-JSON output"):
        collect_run_snapshot.run_json(["gh", "api"])


def test_run_jsonl_wraps_decode_error(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(collect_run_snapshot, "run", lambda _argv: json.dumps({"ok": True}) + "\nnot-json\n")

    with pytest.raises(collect_run_snapshot.SnapshotError, match="non-JSONL line"):
        collect_run_snapshot.run_jsonl(["python", "scripts/conductor.py"])


def test_run_jsonl_rejects_non_object_rows(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(collect_run_snapshot, "run", lambda _argv: "[1,2,3]\n")

    with pytest.raises(collect_run_snapshot.SnapshotError, match="non-object JSONL line"):
        collect_run_snapshot.run_jsonl(["python", "scripts/conductor.py"])


def test_run_raises_on_timeout(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(
        collect_run_snapshot.subprocess,
        "run",
        lambda *_args, **_kwargs: (_ for _ in ()).throw(
            subprocess.TimeoutExpired(cmd=["simulate"], timeout=120),
        ),
    )

    with pytest.raises(collect_run_snapshot.SnapshotError, match="command timed out"):
        collect_run_snapshot.run(["simulate"])


def test_run_raises_on_failure(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(
        collect_run_snapshot.subprocess,
        "run",
        lambda *_args, **_kwargs: SimpleNamespace(
            returncode=1,
            stdout="",
            stderr="fatal: command failed",
        ),
    )

    with pytest.raises(collect_run_snapshot.SnapshotError, match="fatal: command failed"):
        collect_run_snapshot.run(["simulate"])


def test_graphql_review_threads_rejects_graphql_errors(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(
        collect_run_snapshot,
        "run_json",
        lambda _argv: {"errors": [{"message": "rate limited"}], "data": None},
    )

    with pytest.raises(collect_run_snapshot.SnapshotError, match="GraphQL error fetching review threads: rate limited"):
        collect_run_snapshot.graphql_review_threads("misty-step/bitterblossom", 491)


def test_graphql_review_threads_rejects_invalid_repo_format() -> None:
    with pytest.raises(
        collect_run_snapshot.SnapshotError,
        match="--repo must be in 'owner/name' format",
    ):
        collect_run_snapshot.graphql_review_threads("misty-step", 491)


def test_graphql_review_threads_rejects_missing_data(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(collect_run_snapshot, "run_json", lambda _argv: {"data": None})

    with pytest.raises(collect_run_snapshot.SnapshotError, match="GraphQL returned no data for PR 491"):
        collect_run_snapshot.graphql_review_threads("misty-step/bitterblossom", 491)


@pytest.mark.parametrize(
    ("review_threads", "message"),
    [
        ({}, "GraphQL returned no review thread nodes for PR 491"),
        ({"nodes": []}, "GraphQL returned no pageInfo in reviewThreads for PR 491"),
        (
            {"nodes": [], "pageInfo": {"endCursor": None}},
            "GraphQL returned invalid hasNextPage in reviewThreads for PR 491",
        ),
        (
            {"nodes": [], "pageInfo": {"hasNextPage": True, "endCursor": None}},
            "GraphQL returned invalid endCursor in reviewThreads for PR 491",
        ),
    ],
)
def test_graphql_review_threads_rejects_missing_terminal_review_thread_fields(
    monkeypatch: pytest.MonkeyPatch, review_threads: dict[str, object], message: str
) -> None:
    monkeypatch.setattr(
        collect_run_snapshot,
        "run_json",
        lambda _argv: {
            "data": {
                "repository": {
                    "pullRequest": {
                        "reviewThreads": review_threads,
                    }
                }
            }
        },
    )

    with pytest.raises(collect_run_snapshot.SnapshotError, match=message):
        collect_run_snapshot.graphql_review_threads("misty-step/bitterblossom", 491)


def test_graphql_review_threads_paginates(monkeypatch: pytest.MonkeyPatch) -> None:
    payloads = iter(
        [
            {
                "data": {
                    "repository": {
                        "pullRequest": {
                            "reviewThreads": {
                                "nodes": [{"id": "thread-1"}],
                                "pageInfo": {"hasNextPage": True, "endCursor": "cursor-1"},
                            }
                        }
                    }
                }
            },
            {
                "data": {
                    "repository": {
                        "pullRequest": {
                            "reviewThreads": {
                                "nodes": [{"id": "thread-2"}],
                                "pageInfo": {"hasNextPage": False, "endCursor": None},
                            }
                        }
                    }
                }
            },
        ]
    )
    calls: list[list[str]] = []

    def fake_run_json(argv: list[str]) -> object:
        calls.append(argv)
        return next(payloads)

    monkeypatch.setattr(collect_run_snapshot, "run_json", fake_run_json)

    result = collect_run_snapshot.graphql_review_threads("misty-step/bitterblossom", 491)

    assert result == {"reviewThreads": {"nodes": [{"id": "thread-1"}, {"id": "thread-2"}]}}
    assert not any(part.startswith("cursor=") for part in calls[0])
    assert calls[1][-2:] == ["-F", "cursor=cursor-1"]


def test_main_builds_snapshot_with_pr(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    output = tmp_path / "snapshot.json"

    monkeypatch.setattr(
        argparse.ArgumentParser,
        "parse_args",
        lambda self: argparse.Namespace(
            run_id="run-1",
            repo="misty-step/bitterblossom",
            limit=25,
            out=str(output),
        ),
    )
    monkeypatch.setattr(
        collect_run_snapshot,
        "run_jsonl",
        lambda argv: (
            [{"run_id": "run-1", "issue_number": 10, "pr_number": 77}]
            if "show-runs" in argv
            else [{"type": "event", "name": "built"}]
        ),
    )
    monkeypatch.setattr(
        collect_run_snapshot,
        "run_json",
        lambda argv: (
            {"number": 77, "title": "PR", "url": "https://example.com/pr/77"}
            if argv[:3] == ["gh", "pr", "view"]
            else {"number": 10, "title": "Issue", "url": "https://example.com/issues/10"}
        ),
    )
    monkeypatch.setattr(
        collect_run_snapshot,
        "graphql_review_threads",
        lambda _repo, pr_number: {"reviewThreads": {"nodes": [{"id": f"thread-for-{pr_number}"}]}},
    )

    rc = collect_run_snapshot.main()

    assert rc == 0
    payload = json.loads(output.read_text(encoding="utf-8"))
    assert payload["run"]["pr_number"] == 77
    assert payload["pr"]["number"] == 77
    assert payload["review_threads"] == {"reviewThreads": {"nodes": [{"id": "thread-for-77"}]}}
    assert payload["issue"]["number"] == 10


def test_main_builds_snapshot_without_pr(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    output = tmp_path / "snapshot.json"

    monkeypatch.setattr(
        argparse.ArgumentParser,
        "parse_args",
        lambda self: argparse.Namespace(
            run_id="run-1",
            repo="misty-step/bitterblossom",
            limit=25,
            out=str(output),
        ),
    )
    monkeypatch.setattr(
        collect_run_snapshot,
        "run_jsonl",
        lambda argv: (
            [{"run_id": "run-1", "issue_number": 10}]
            if "show-runs" in argv
            else [{"type": "event", "name": "built"}]
        ),
    )
    monkeypatch.setattr(
        collect_run_snapshot,
        "run_json",
        lambda argv: {"number": 10, "title": "Issue", "url": "https://example.com/issues/10"},
    )

    rc = collect_run_snapshot.main()

    assert rc == 0
    payload = json.loads(output.read_text(encoding="utf-8"))
    assert payload["run"]["run_id"] == "run-1"
    assert payload["events"][0]["name"] == "built"
    assert payload["pr"] is None
    assert payload["review_threads"] is None


def test_main_raises_when_run_id_not_found(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(
        collect_run_snapshot,
        "run_jsonl",
        lambda _argv: [{"run_id": "run-2", "issue_number": 10}],
    )
    monkeypatch.setattr(
        argparse.ArgumentParser,
        "parse_args",
        lambda self: argparse.Namespace(
            run_id="run-1",
            repo="misty-step/bitterblossom",
            limit=25,
            out=None,
        ),
    )

    with pytest.raises(collect_run_snapshot.SnapshotError, match="run not found in show-runs output: run-1"):
        collect_run_snapshot.main()


def test_main_raises_when_issue_number_missing(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(
        collect_run_snapshot,
        "run_jsonl",
        lambda argv: (
            [{"run_id": "run-1", "pr_number": 77}]
            if "show-runs" in argv
            else [{"type": "event", "name": "built"}]
        ),
    )
    monkeypatch.setattr(
        argparse.ArgumentParser,
        "parse_args",
        lambda self: argparse.Namespace(
            run_id="run-1",
            repo="misty-step/bitterblossom",
            limit=25,
            out=None,
        ),
    )
    monkeypatch.setattr(
        collect_run_snapshot,
        "run_json",
        lambda argv: {"number": 77, "title": "PR", "url": "https://example.com/pr/77"},
    )
    monkeypatch.setattr(
        collect_run_snapshot,
        "graphql_review_threads",
        lambda _repo, pr_number: {"reviewThreads": {"nodes": [{"id": "thread-for-missing-issue-number"}]}},
    )

    with pytest.raises(collect_run_snapshot.SnapshotError, match="run snapshot missing issue_number: run-1"):
        collect_run_snapshot.main()
