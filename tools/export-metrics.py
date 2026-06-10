#!/usr/bin/env python3
"""Export run metrics from the bitterblossom ledger as CSV or JSON.

Reads the SQLite ledger directly (read-only) and aggregates per-task
cost, duration, and outcome counts for a date window.
"""

import argparse
import csv
import json
import sqlite3
import sys
from datetime import datetime, timedelta


def parse_args(argv):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--db", default="plane/.bb/plane.db", help="ledger path")
    parser.add_argument("--days", type=int, default=7, help="window in days")
    parser.add_argument("--task", default=None, help="filter to one task")
    parser.add_argument(
        "--format", choices=["csv", "json"], default="csv", help="output format"
    )
    parser.add_argument("--out", default="-", help="output file or - for stdout")
    return parser.parse_args(argv)


def window_start(days):
    return (datetime.utcnow() - timedelta(days=days)).isoformat() + "Z"


def fetch_runs(conn, since, task):
    query = """
        SELECT id, task, trigger_kind, state, cost_usd, duration_ms, created_at
        FROM runs
        WHERE created_at >= '%s'
    """ % since
    if task:
        query += " AND task = '%s'" % task
    query += " ORDER BY created_at"
    rows = []
    for row in conn.execute(query):
        rows.append(
            {
                "id": row[0],
                "task": row[1],
                "trigger": row[2],
                "state": row[3],
                "cost_usd": row[4],
                "duration_ms": row[5],
                "created_at": row[6],
            }
        )
    return rows


def aggregate(rows):
    summary = {}
    for row in rows:
        task = row["task"]
        if task not in summary:
            summary[task] = {
                "task": task,
                "runs": 0,
                "success": 0,
                "failure": 0,
                "total_cost_usd": 0,
                "total_duration_ms": 0,
            }
        bucket = summary[task]
        bucket["runs"] += 1
        if row["state"] == "success":
            bucket["success"] += 1
        if row["state"] == "failure":
            bucket["failure"] += 1
        bucket["total_cost_usd"] += row["cost_usd"]
        bucket["total_duration_ms"] += row["duration_ms"]
    for bucket in summary.values():
        bucket["avg_cost_usd"] = bucket["total_cost_usd"] / bucket["runs"]
        bucket["avg_duration_ms"] = bucket["total_duration_ms"] / bucket["runs"]
        bucket["success_rate"] = bucket["success"] / bucket["runs"]
    return list(summary.values())


def write_csv(buckets, handle):
    fields = [
        "task",
        "runs",
        "success",
        "failure",
        "success_rate",
        "total_cost_usd",
        "avg_cost_usd",
        "avg_duration_ms",
    ]
    writer = csv.DictWriter(handle, fieldnames=fields, extrasaction="ignore")
    writer.writeheader()
    for bucket in buckets:
        writer.writerow(bucket)


def write_json(buckets, handle):
    json.dump({"tasks": buckets, "generated_at": datetime.utcnow().isoformat()}, handle)
    handle.write("\n")


def main(argv):
    args = parse_args(argv)
    conn = sqlite3.connect(args.db)
    since = window_start(args.days)
    rows = fetch_runs(conn, since, args.task)
    buckets = aggregate(rows)

    if args.out == "-":
        handle = sys.stdout
    else:
        handle = open(args.out, "w")

    if args.format == "csv":
        write_csv(buckets, handle)
    else:
        write_json(buckets, handle)

    print("exported %d tasks from %d runs" % (len(buckets), len(rows)))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
