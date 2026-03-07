#!/usr/bin/env python3
import argparse
import json
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[4]


class SnapshotError(RuntimeError):
    pass


def run(argv: list[str]) -> str:
    proc = subprocess.run(
        argv,
        cwd=ROOT,
        text=True,
        capture_output=True,
        check=False,
    )
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
    return run_json(
        [
            "gh",
            "api",
            "graphql",
            "-f",
            (
                "query="
                "query($owner:String!,$repo:String!,$number:Int!){"
                "repository(owner:$owner,name:$repo){"
                "pullRequest(number:$number){"
                "reviewThreads(first:100){nodes{"
                "id isResolved isOutdated path line "
                "comments(first:20){nodes{author{login}body url createdAt}}"
                "}}}}}"
            ),
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
            "python3",
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
            "python3",
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

    issue = run_json(
        [
            "gh",
            "issue",
            "view",
            str(run_row["issue_number"]),
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
