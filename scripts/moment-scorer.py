#!/usr/bin/env python3
"""Flight-recorder moments: a post-run anomaly scorer (bitterblossom-914).

This is workload logic, intentionally kept out of the Rust event-plane spine
(matching hygiene-reflex.py's own convention: src/ is mechanism, not workload
judgment -- see scripts/verify.sh's LOC tripwire comment). It reads
bitterblossom's own run ledger read-only, scores newly-completed runs
against a small FIXED, deterministic 4-class taxonomy (surprise / failure /
recovery / cost_anomaly), and persists above-threshold "moment cards" -- an
excerpt plus a run link -- into its own separate SQLite store. Below-threshold
runs produce nothing. A fleet-wide ≤3/day cap on *published* cards is
enforced at insert time; capped-out cards are still recorded (never silently
dropped), just marked unpublished.

Deterministic signal set (named explicitly so it can be argued with, per
content-harness epic misty-step-912's design law #3: "newsworthiness gates
are deterministic where possible; the model never decides eligibility" --
this generator has no model in it at all):

- failure: the run's terminal state is `failure`.
- recovery: the run succeeded after more than one attempt, or after passing
  through `awaiting_recovery`.
- cost_anomaly: the run's cost is a statistical outlier against that same
  task's own trailing history (>= 5 prior completed runs required).
- surprise: a guard event (circuit breaker) fired in the run's own time
  window, or the run's duration is a statistical outlier against that task's
  own trailing history.

No LLM judgment anywhere in this scoring path -- every signal above is
already recorded in the ledger by the plane's own dispatch/recovery/guard
mechanism.

This generator has no shared cross-repo review queue to feed yet (content-
harness epic misty-step-912's "one review queue" component does not exist
as of this writing) -- `moment_cards` in the separate `moments.db` this
script manages *is* the review queue for now, until that shared component is
built. `list --json` is how an operator or the shared queue reads it back.
"""

from __future__ import annotations

import argparse
import json
import sqlite3
import statistics
import sys
from datetime import datetime, timezone
from pathlib import Path

SCHEMA_VERSION = "bb.moment_scorer_report.v1"
DAILY_PUBLISH_CAP = 3
MIN_BASELINE_SAMPLES = 5
Z_THRESHOLD = 2.0
CONSTANT_BASELINE_MULTIPLE = 3.0
EXCERPT_MAX_CHARS = 300

MOMENTS_SCHEMA = """
CREATE TABLE IF NOT EXISTS moment_cards (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id TEXT NOT NULL,
  task TEXT NOT NULL,
  class TEXT NOT NULL,
  excerpt TEXT NOT NULL,
  run_link TEXT NOT NULL,
  published INTEGER NOT NULL,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS scored_runs (
  run_id TEXT PRIMARY KEY,
  scored_at TEXT NOT NULL
);
"""


def now_iso() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%f")[:-3] + "Z"


def today_date() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%d")


def connect_ledger_ro(path: Path) -> sqlite3.Connection:
    conn = sqlite3.connect(f"file:{path}?mode=ro", uri=True)
    conn.row_factory = sqlite3.Row
    return conn


def connect_moments(path: Path) -> sqlite3.Connection:
    path.parent.mkdir(parents=True, exist_ok=True)
    conn = sqlite3.connect(path)
    conn.row_factory = sqlite3.Row
    conn.executescript(MOMENTS_SCHEMA)
    return conn


def truncate(text: str, limit: int = EXCERPT_MAX_CHARS) -> str:
    text = text.strip()
    if len(text) <= limit:
        return text
    return text[:limit].rstrip() + "..."


def fetch_unscored_completed_runs(ledger: sqlite3.Connection, moments: sqlite3.Connection):
    scored = {row["run_id"] for row in moments.execute("SELECT run_id FROM scored_runs")}
    rows = ledger.execute(
        "SELECT id, task, state, state_reason, cost_usd, duration_ms, created_at, updated_at "
        "FROM runs WHERE state IN ('success', 'failure') ORDER BY created_at ASC"
    ).fetchall()
    return [dict(r) for r in rows if r["id"] not in scored]


def fetch_attempts(ledger: sqlite3.Connection, run_id: str):
    rows = ledger.execute(
        "SELECT n, outcome, error FROM attempts WHERE run_id = ?1 ORDER BY n ASC", (run_id,)
    ).fetchall()
    return [dict(r) for r in rows]


def fetch_dead_letters(ledger: sqlite3.Connection, run_id: str):
    rows = ledger.execute(
        "SELECT error FROM dead_letters WHERE run_id = ?1 ORDER BY id ASC", (run_id,)
    ).fetchall()
    return [dict(r) for r in rows]


def fetch_events(ledger: sqlite3.Connection, run_id: str):
    rows = ledger.execute(
        "SELECT kind, data, at FROM run_events WHERE run_id = ?1 ORDER BY id ASC", (run_id,)
    ).fetchall()
    return [dict(r) for r in rows]


def fetch_guard_events_in_window(ledger: sqlite3.Connection, task: str, start: str, end: str):
    rows = ledger.execute(
        "SELECT kind, task, detail, count, at FROM guard_events "
        "WHERE (task = ?1 OR task IS NULL) AND at >= ?2 AND at <= ?3 ORDER BY id ASC",
        (task, start, end),
    ).fetchall()
    return [dict(r) for r in rows]


def task_baseline(ledger: sqlite3.Connection, task: str, exclude_run_id: str, column: str):
    """Trailing (mean, stddev, n) for `column` (always one of this module's
    own two literal column names, never external input) over this task's
    other completed runs. `None` when fewer than MIN_BASELINE_SAMPLES exist
    -- never manufacture a baseline from too little history."""
    assert column in ("cost_usd", "duration_ms"), column
    rows = ledger.execute(
        f"SELECT {column} AS v FROM runs "
        f"WHERE task = ?1 AND id != ?2 AND state IN ('success', 'failure') AND {column} IS NOT NULL",
        (task, exclude_run_id),
    ).fetchall()
    values = [r["v"] for r in rows]
    if len(values) < MIN_BASELINE_SAMPLES:
        return None
    mean = statistics.mean(values)
    stddev = statistics.pstdev(values)
    return (mean, stddev, len(values))


def is_outlier(value: float, baseline) -> tuple[bool, float, int]:
    """Returns (is_outlier, effective_mean, n) for an already-fetched
    baseline. A near-constant baseline (stddev ~ 0) falls back to a relative
    multiple floor so a real jump is still caught without a single-sample
    stddev of 0 flagging any deviation at all."""
    mean, stddev, n = baseline
    if stddev > 1e-9:
        threshold = mean + Z_THRESHOLD * stddev
    else:
        threshold = mean * CONSTANT_BASELINE_MULTIPLE if mean > 0 else 0.0
    return (value > threshold, mean, n)


def classify(ledger: sqlite3.Connection, run: dict):
    """Returns (class, excerpt) or None. Priority order when more than one
    signal could apply: failure > recovery > cost_anomaly > surprise -- each
    run yields at most one moment."""
    run_id = run["id"]
    task = run["task"]

    if run["state"] == "failure":
        dead_letters = fetch_dead_letters(ledger, run_id)
        if dead_letters:
            return ("failure", truncate(dead_letters[0]["error"]))
        attempts = fetch_attempts(ledger, run_id)
        failed = [a for a in attempts if a["outcome"] == "failed" and a["error"]]
        if failed:
            return ("failure", truncate(failed[-1]["error"]))
        if run["state_reason"]:
            return ("failure", truncate(run["state_reason"]))
        return ("failure", "(no error captured)")

    if run["state"] == "success":
        attempts = fetch_attempts(ledger, run_id)
        if len(attempts) > 1:
            last_ok = attempts[-1]
            prior = attempts[-2]
            return (
                "recovery",
                f"recovered after {len(attempts)} attempts: attempt {prior['n']} failed "
                f"({prior['error'] or 'no error recorded'}), attempt {last_ok['n']} succeeded.",
            )
        events = fetch_events(ledger, run_id)
        kinds = [e["kind"] for e in events]
        if "state:awaiting_recovery" in kinds and kinds.index("state:awaiting_recovery") < len(
            kinds
        ) - 1 and kinds[-1] == "state:success":
            reason = next(
                (e["data"] for e in events if e["kind"] == "state:awaiting_recovery" and e["data"]),
                None,
            )
            return (
                "recovery",
                f"recovered from awaiting_recovery: {reason or '(no reason recorded)'}",
            )

    if run["cost_usd"] is not None:
        baseline = task_baseline(ledger, task, run_id, "cost_usd")
        if baseline is not None:
            outlier, mean, n = is_outlier(run["cost_usd"], baseline)
            if outlier:
                multiple = run["cost_usd"] / mean if mean > 0 else float("inf")
                return (
                    "cost_anomaly",
                    f"cost ${run['cost_usd']:.2f} vs task's trailing mean ${mean:.2f} "
                    f"over {n} runs ({multiple:.1f}x)",
                )

    guard_events = fetch_guard_events_in_window(
        ledger, task, run["created_at"], run["updated_at"]
    )
    if guard_events:
        g = guard_events[0]
        return ("surprise", f"guard event {g['kind']}: {g['detail']}")

    if run["duration_ms"] is not None:
        baseline = task_baseline(ledger, task, run_id, "duration_ms")
        if baseline is not None:
            outlier, mean, n = is_outlier(run["duration_ms"], baseline)
            if outlier:
                multiple = run["duration_ms"] / mean if mean > 0 else float("inf")
                return (
                    "surprise",
                    f"duration {run['duration_ms']}ms vs task's trailing mean {mean:.0f}ms "
                    f"over {n} runs ({multiple:.1f}x)",
                )

    return None


def run_link_for(run_id: str) -> str:
    # No deployed web dashboard URL convention exists in this repo yet -- the
    # honest, always-reachable link is the CLI command that shows this run,
    # not an invented URL. Revisit if/when a dashboard base URL exists.
    return f"bb runs show {run_id} --json"


def today_published_count(moments: sqlite3.Connection) -> int:
    row = moments.execute(
        "SELECT COUNT(*) AS c FROM moment_cards WHERE published = 1 AND substr(created_at, 1, 10) = ?1",
        (today_date(),),
    ).fetchone()
    return row["c"]


def scan(ledger_path: Path, moments_path: Path) -> dict:
    ledger = connect_ledger_ro(ledger_path)
    moments = connect_moments(moments_path)
    try:
        candidates = fetch_unscored_completed_runs(ledger, moments)
        published_today = today_published_count(moments)
        new_moments = []
        for run in candidates:
            outcome = classify(ledger, run)
            if outcome is not None:
                cls, excerpt = outcome
                publish = published_today < DAILY_PUBLISH_CAP
                moments.execute(
                    "INSERT INTO moment_cards (run_id, task, class, excerpt, run_link, published, created_at) "
                    "VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)",
                    (
                        run["id"],
                        run["task"],
                        cls,
                        excerpt,
                        run_link_for(run["id"]),
                        1 if publish else 0,
                        now_iso(),
                    ),
                )
                if publish:
                    published_today += 1
                new_moments.append({"run_id": run["id"], "class": cls, "published": publish})
            moments.execute(
                "INSERT OR IGNORE INTO scored_runs (run_id, scored_at) VALUES (?1, ?2)",
                (run["id"], now_iso()),
            )
        moments.commit()
        return {
            "scanned": len(candidates),
            "new_moments": new_moments,
        }
    finally:
        ledger.close()
        moments.close()


def cmd_scan(args: argparse.Namespace) -> int:
    report = scan(Path(args.db), Path(args.moments_db))
    published = sum(1 for m in report["new_moments"] if m["published"])
    result_text = (
        f"scanned {report['scanned']} run(s), {len(report['new_moments'])} new moment(s) "
        f"({published} published)"
    )
    if args.report:
        Path(args.report).write_text(json.dumps({"schema": SCHEMA_VERSION, **report}, indent=2) + "\n")
    if args.json:
        print(json.dumps(report))
    print(
        json.dumps(
            {
                "schema_version": "bb.command_result.v1",
                "result": result_text,
                "tokens_in": 0,
                "tokens_out": 0,
                "turns": 0,
                "cost_usd": 0.0,
            }
        )
    )
    return 0


def cmd_list(args: argparse.Namespace) -> int:
    moments = connect_moments(Path(args.moments_db))
    try:
        query = "SELECT run_id, task, class, excerpt, run_link, published, created_at FROM moment_cards"
        if not args.all:
            query += " WHERE published = 1"
        query += " ORDER BY created_at DESC"
        if args.limit is not None:
            query += f" LIMIT {int(args.limit)}"
        rows = moments.execute(query).fetchall()
        cards = [
            {
                "run_id": r["run_id"],
                "task": r["task"],
                "class": r["class"],
                "excerpt": r["excerpt"],
                "run_link": r["run_link"],
                "published": bool(r["published"]),
                "created_at": r["created_at"],
            }
            for r in rows
        ]
    finally:
        moments.close()
    if args.json:
        print(json.dumps(cards))
    else:
        for card in cards:
            flag = "" if card["published"] else " [unpublished, over daily cap]"
            print(f"{card['created_at']}  {card['class']:<13} {card['task']}/{card['run_id']}{flag}")
            print(f"  {card['excerpt']}")
            print(f"  {card['run_link']}")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)

    scan_p = sub.add_parser("scan", help="score newly-completed runs and record above-threshold moments")
    scan_p.add_argument("--db", default=".bb/plane.db", help="path to bitterblossom's run ledger")
    scan_p.add_argument("--moments-db", default=".bb/moments.db", help="path to this script's own moment store")
    scan_p.add_argument("--json", action="store_true", help="also print the full scan report as JSON")
    scan_p.add_argument(
        "--report",
        default=None,
        help="optional path to also write the full scan report as REPORT.json "
        "(the command-harness attempt-artifact convention; not written unless given)",
    )
    scan_p.set_defaults(func=cmd_scan)

    list_p = sub.add_parser("list", help="list recorded moment cards (the review queue)")
    list_p.add_argument("--moments-db", default=".bb/moments.db")
    list_p.add_argument("--limit", type=int, default=None)
    list_p.add_argument("--all", action="store_true", help="include unpublished (over-cap) cards")
    list_p.add_argument("--json", action="store_true")
    list_p.set_defaults(func=cmd_list)

    args = parser.parse_args()
    return args.func(args)


if __name__ == "__main__":
    sys.exit(main())
