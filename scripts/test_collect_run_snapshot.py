from __future__ import annotations

import importlib.util
import json
from pathlib import Path

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


def test_graphql_review_threads_rejects_graphql_errors(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(
        collect_run_snapshot,
        "run_json",
        lambda _argv: {"errors": [{"message": "rate limited"}], "data": None},
    )

    with pytest.raises(collect_run_snapshot.SnapshotError, match="GraphQL error fetching review threads: rate limited"):
        collect_run_snapshot.graphql_review_threads("misty-step/bitterblossom", 491)


def test_graphql_review_threads_rejects_missing_data(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(collect_run_snapshot, "run_json", lambda _argv: {"data": None})

    with pytest.raises(collect_run_snapshot.SnapshotError, match="GraphQL returned no data for PR 491"):
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
    assert not any(part == "cursor=" for part in calls[0])
    assert any(part == "cursor=cursor-1" for part in calls[1])
