#!/usr/bin/env python3
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


def run(argv: list[str]) -> str:
    try:
        proc = subprocess.run(
            argv,
            cwd=CONDUCTOR_DIR if argv[0] == "mix" else ROOT,
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
    output = run(argv)
    try:
        return json.loads(output)
    except json.JSONDecodeError as exc:
        raise SnapshotError(f"command produced non-JSON output: {' '.join(argv)}\n{output[:200]}") from exc


def run_jsonl(argv: list[str]) -> list[dict]:
    items: list[dict] = []
    for line in run(argv).splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            item = json.loads(line)
        except json.JSONDecodeError as exc:
            raise SnapshotError(f"command produced non-JSONL line: {line[:200]}") from exc
        if not isinstance(item, dict):
            raise SnapshotError(f"command produced non-object JSONL line: {line[:200]}")
        items.append(item)
    return items


def graphql_review_threads(repo: str, pr_number: int) -> dict:
    if "/" not in repo:
        raise SnapshotError(f"--repo must be in 'owner/name' format, got: {repo!r}")
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
        args = [
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
        if cursor is not None:
            args.extend(["-F", f"cursor={cursor}"])

        payload = run_json(args)
        if not isinstance(payload, dict):
            raise SnapshotError(f"GraphQL returned unexpected payload type for PR {pr_number}")
        if payload.get("errors"):
            errors = payload["errors"]
            if isinstance(errors, list):
                messages = "; ".join(
                    str(error.get("message") if isinstance(error, dict) else error) for error in errors
                )
            else:
                messages = str(errors)
            raise SnapshotError(f"GraphQL error fetching review threads: {messages}")
        data = payload.get("data")
        if not isinstance(data, dict):
            raise SnapshotError(f"GraphQL returned no data for PR {pr_number}")
        repository = data.get("repository")
        if not isinstance(repository, dict):
            raise SnapshotError(f"GraphQL returned no repository data for PR {pr_number}")
        pull_request = repository.get("pullRequest")
        if not isinstance(pull_request, dict):
            raise SnapshotError(f"GraphQL returned no pull request data for PR {pr_number}")
        request = pull_request.get("reviewThreads")
        if not isinstance(request, dict):
            raise SnapshotError(f"GraphQL returned no review thread data for PR {pr_number}")
        nodes = request.get("nodes")
        if not isinstance(nodes, list):
            raise SnapshotError(f"GraphQL returned no review thread nodes for PR {pr_number}")
        all_nodes.extend(nodes)
        page_info = request.get("pageInfo")
        if not isinstance(page_info, dict):
            raise SnapshotError(f"GraphQL returned no pageInfo in reviewThreads for PR {pr_number}")
        has_next_page = page_info.get("hasNextPage")
        if not isinstance(has_next_page, bool):
            raise SnapshotError(f"GraphQL returned invalid hasNextPage in reviewThreads for PR {pr_number}")
        if not has_next_page:
            break
        cursor = page_info.get("endCursor")
        if not isinstance(cursor, str) or not cursor:
            raise SnapshotError(f"GraphQL returned invalid endCursor in reviewThreads for PR {pr_number}")

    return {"reviewThreads": {"nodes": all_nodes}}


def main() -> int:
    parser = argparse.ArgumentParser(description="Collect a Bitterblossom run snapshot")
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--repo", default="misty-step/bitterblossom")
    parser.add_argument("--limit", type=int, default=100)
    parser.add_argument("--out")
    args = parser.parse_args()

    runs = run_jsonl(
        [
            "mix",
            "conductor",
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
            "mix",
            "conductor",
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
