#!/usr/bin/env python3
import argparse
import json
import subprocess
import sys
from pathlib import Path


ROOT = next(p for p in Path(__file__).resolve().parents if (p / ".git").exists())
TIMEOUT_SECONDS = 120


def snapshot_python() -> str:
    return sys.executable


class SnapshotError(RuntimeError):
    pass


def run(argv: list[str]) -> str:
    try:
        proc = subprocess.run(
            argv,
            cwd=ROOT,
            text=True,
            capture_output=True,
            check=False,
            timeout=TIMEOUT_SECONDS,
        )
    except subprocess.TimeoutExpired as exc:
        raise SnapshotError(f"command timed out: {' '.join(argv)}") from exc
    if proc.returncode != 0:
        raise SnapshotError((proc.stderr or proc.stdout).strip() or f"command failed: {' '.join(argv)}")
    return proc.stdout


def run_json(argv: list[str]) -> object:
    return json.loads(run(argv))


def run_jsonl(argv: list[str]) -> list[dict]:
    items: list[dict] = []
    for line in run(argv).splitlines():
        line = line.strip()
        if not line:
            continue
        items.append(json.loads(line))
    return items


def graphql_review_threads(repo: str, pr_number: int) -> dict:
    owner, name = repo.split("/", 1)
    query = """
    query($owner:String!, $repo:String!, $number:Int!, $cursor:String){
        repository(owner:$owner,name:$repo){
            pullRequest(number:$number){
                reviewThreads(first:100, after:$cursor){
                    nodes{
                        id
                        isResolved
                        isOutdated
                        path
                        line
                        comments(first:100){
                            nodes{
                                author{
                                    login
                                }
                                body
                                url
                                createdAt
                            }
                        }
                    }
                    pageInfo{
                        hasNextPage
                        endCursor
                    }
                }
            }
        }
    }
    """
    oneline_query = " ".join(line.strip() for line in query.splitlines())
    all_nodes = []
    cursor: str | None = None

    while True:
        payload = run_json(
            [
                "gh",
                "api",
                "graphql",
                "-f",
                f"query={oneline_query}",
                "-F",
                f"owner={owner}",
                "-F",
                f"repo={name}",
                "-F",
                f"number={pr_number}",
                "-F",
                f"cursor={cursor or ''}",
            ]
        )
        request = payload["data"]["repository"]["pullRequest"]["reviewThreads"]
        all_nodes.extend(request["nodes"])
        page_info = request["pageInfo"]
        if not page_info["hasNextPage"]:
            break
        cursor = page_info["endCursor"]

    return {"reviewThreads": {"nodes": all_nodes}}
    return run_json(
        [
            "gh",
            "api",
            "graphql",
            "-f",
            f"query={oneline_query}",
            "-F",
            f"owner={owner}",
            "-F",
            f"repo={name}",
            "-F",
            f"number={pr_number}",
        ]
    )


def main() -> int:
    parser = argparse.ArgumentParser(description="Collect a Bitterblossom run snapshot")
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--repo", default="misty-step/bitterblossom")
    parser.add_argument("--limit", type=int, default=100)
    parser.add_argument("--out")
    args = parser.parse_args()

    runs = run_jsonl(
        [
            snapshot_python(),
            "scripts/conductor.py",
            "show-runs",
            "--limit",
            str(max(args.limit, 20)),
        ]
    )
    run_row = next((row for row in runs if row.get("run_id") == args.run_id), None)
    if run_row is None:
        raise SnapshotError(f"run not found in show-runs output: {args.run_id}")

    events = run_jsonl(
        [
            snapshot_python(),
            "scripts/conductor.py",
            "show-events",
            "--run-id",
            args.run_id,
            "--limit",
            str(args.limit),
        ]
    )

    pr = None
    review_threads = None
    pr_number = run_row.get("pr_number")
    if pr_number:
        pr = run_json(
            [
                "gh",
                "pr",
                "view",
                str(pr_number),
                "--repo",
                args.repo,
                "--json",
                (
                    "number,title,url,state,isDraft,mergeStateStatus,mergeable,"
                    "reviewDecision,createdAt,updatedAt,mergedAt,statusCheckRollup,"
                    "comments,reviews"
                ),
            ]
        )
        review_threads = graphql_review_threads(args.repo, int(pr_number))

    issue_number = run_row.get("issue_number")
    if issue_number is None:
        raise SnapshotError(f"run snapshot missing issue_number: {args.run_id}")

    issue = run_json(
        [
            "gh",
            "issue",
            "view",
            str(issue_number),
            "--repo",
            args.repo,
            "--json",
            "number,title,url,state,labels,milestone,comments",
        ]
    )

    snapshot = {
        "run": run_row,
        "events": events,
        "issue": issue,
        "pr": pr,
        "review_threads": review_threads,
    }

    payload = json.dumps(snapshot, indent=2)
    if args.out:
        Path(args.out).write_text(payload + "\n", encoding="utf-8")
    else:
        sys.stdout.write(payload + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
