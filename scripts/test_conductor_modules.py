from __future__ import annotations

import pathlib
import sys

import pytest


sys.path.insert(0, str(pathlib.Path(__file__).parent))

from conductorlib import governance, tracker, workspace  # noqa: E402
from conductorlib.common import CmdError, Issue, ReviewThread  # noqa: E402


def test_workspace_paths_stay_run_scoped() -> None:
    run_id = "run-42-1777"
    assert workspace.run_root("misty-step/bitterblossom", run_id).endswith(f"/.bb/conductor/{run_id}")
    assert workspace.run_workspace("misty-step/bitterblossom", run_id, "builder").endswith("/builder-worktree")
    assert workspace.artifact_rel(run_id, "builder-result.json") == ".bb/conductor/run-42-1777/builder-result.json"


def test_parse_workspace_prepare_output_requires_workspace_echo() -> None:
    target = "/tmp/worktree"
    assert workspace.parse_workspace_prepare_output(f"noise\n{target}\n", target, "fern") == target
    with pytest.raises(CmdError, match="unexpected workspace prepare output"):
        workspace.parse_workspace_prepare_output("noise only", target, "fern")


def test_has_markdown_heading_requires_matching_fence_length() -> None:
    body = "````python\n## Product Spec\n```\n"

    assert tracker.has_markdown_heading(body, "## Product Spec") is False


def test_collect_routable_issues_respects_lease_and_readiness_boundaries() -> None:
    ready = Issue(
        number=1,
        title="ready",
        body="## Product Spec\n\n### Intent Contract",
        url="https://example.com/1",
        labels=["p1"],
    )
    missing_spec = Issue(
        number=2,
        title="missing",
        body="plain body",
        url="https://example.com/2",
        labels=["p1"],
    )
    leased = Issue(
        number=3,
        title="leased",
        body="## Product Spec\n\n### Intent Contract",
        url="https://example.com/3",
        labels=["p1"],
    )

    eligible, failures = tracker.collect_routable_issues(
        [ready, missing_spec, leased],
        "misty-step/bitterblossom",
        lease_warnings=lambda issue_number: ["already leased"] if issue_number == 3 else [],
    )

    assert [issue.number for issue in eligible] == [1]
    assert failures[2] == ["missing `## Product Spec` section", "missing `### Intent Contract` section"]
    assert failures[3] == ["already leased"]


def test_governance_filters_trusted_surface_state() -> None:
    payload = {
        "statusCheckRollup": [
            {"__typename": "CheckRun", "name": "Cerberus", "workflowName": "Cerberus", "status": "COMPLETED", "conclusion": "SUCCESS"},
            {"__typename": "CheckRun", "name": "CodeQL", "workflowName": "CodeQL", "status": "IN_PROGRESS"},
        ]
    }

    assert governance.trusted_surfaces_pending(payload, ["Cerberus"]) == []
    assert governance.trusted_surfaces_pending(payload, ["CodeQL"]) == ["CodeQL"]


def test_summarize_review_threads_keeps_location_and_author() -> None:
    summary = governance.summarize_review_threads(
        [
            ReviewThread(
                id="thread-1",
                path="scripts/conductor.py",
                line=59,
                author_login="coderabbitai",
                author_association="NONE",
                body="guard the stale lease check",
                url="https://example.com/thread-1",
            )
        ]
    )

    assert "scripts/conductor.py:59" in summary
    assert "@coderabbitai" in summary


def test_list_unresolved_review_threads_queries_latest_comment(monkeypatch: pytest.MonkeyPatch) -> None:
    seen: dict[str, object] = {}

    def fake_gh_graphql(_runner: object, query: str, variables: dict[str, str | int]) -> dict[str, object]:
        seen["query"] = query
        seen["variables"] = variables
        return {
            "data": {
                "repository": {
                    "pullRequest": {
                        "reviewThreads": {
                            "nodes": [
                                {
                                    "id": "thread-1",
                                    "isResolved": False,
                                    "path": "scripts/conductor.py",
                                    "line": 59,
                                    "comments": {
                                        "nodes": [
                                            {
                                                "author": {"login": "coderabbitai"},
                                                "authorAssociation": "NONE",
                                                "body": "latest comment",
                                                "url": "https://example.com/thread-1",
                                            }
                                        ]
                                    },
                                }
                            ],
                            "pageInfo": {"hasNextPage": False, "endCursor": None},
                        }
                    }
                }
            }
        }

    monkeypatch.setattr(governance, "gh_graphql", fake_gh_graphql)

    threads = governance.list_unresolved_review_threads(object(), "misty-step/bitterblossom", 42)

    assert "comments(last:1)" in str(seen["query"])
    assert threads[0].body == "latest comment"
