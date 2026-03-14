#!/usr/bin/env python3
"""Sentry-to-GitHub issue intake adapter.

Converts a Sentry incident into a deduped GitHub issue with severity,
environment, stack trace, fingerprint, and a link back to Sentry.

GitHub is the queue. Sentry is input only.

Usage (payload via stdin or --payload flag):

    echo '{...}' | python3 scripts/sentry_intake.py --repo owner/repo
    python3 scripts/sentry_intake.py --repo owner/repo --payload '{...}'

Usage (fetch from Sentry API, requires SENTRY_AUTH_TOKEN):

    python3 scripts/sentry_intake.py \\
        --repo owner/repo \\
        --sentry-org misty-step \\
        --sentry-project my-project \\
        --sentry-issue-id 123456789

Output:
    GitHub issue number (integer) written to stdout.

Environment variables:
    SENTRY_AUTH_TOKEN   Sentry API token (preferred)
    SENTRY_MASTER_TOKEN Fallback Sentry API token
"""

import argparse
import hashlib
import json
import os
import subprocess
import sys
import urllib.request
from typing import Optional


# Prefix embedded in the GitHub issue body for fingerprint-based deduplication.
# Must be stable — changing it breaks deduplication for existing issues.
DEDUPE_MARKER_PREFIX = "sentry-fp-"

SEVERITY_TO_PRIORITY = {
    "fatal": "P0",
    "error": "P1",
    "warning": "P2",
    "info": "P3",
    "debug": "P3",
}

LEVEL_TO_TITLE_PREFIX = {
    "fatal": "[P0]",
    "error": "[P1]",
    "warning": "[P2]",
    "info": "[P3]",
    "debug": "[P3]",
}


# ─── Core helpers ─────────────────────────────────────────────────────────────


def run_gh(args: list, **kwargs) -> subprocess.CompletedProcess:
    """Run a `gh` CLI command and return the result.

    Thin wrapper kept as a named function so tests can patch it cleanly.
    """
    return subprocess.run(
        ["gh"] + args,
        capture_output=True,
        text=True,
        check=True,
        **kwargs,
    )


def fingerprint_hash(incident: dict) -> str:
    """Derive a stable 16-char hex fingerprint from the Sentry incident.

    Uses the Sentry-supplied ``fingerprint`` array when present; falls back
    to the incident ``id``.  The result is deterministic for the same input.
    """
    fp = incident.get("fingerprint") or [incident.get("id", "")]
    canonical = json.dumps(fp, sort_keys=True)
    return hashlib.sha256(canonical.encode()).hexdigest()[:16]


# ─── Formatting ───────────────────────────────────────────────────────────────


def format_stack_trace(entries: list, max_frames: int = 10) -> str:
    """Extract and format the primary exception stack trace from Sentry entries."""
    for entry in entries:
        if entry.get("type") == "exception":
            values = entry.get("data", {}).get("values", [])
            if not values:
                continue
            exc = values[-1]  # last (innermost) exception
            exc_type = exc.get("type", "Unknown")
            exc_value = exc.get("value", "")
            frames = exc.get("stacktrace", {}).get("frames", [])

            lines = [f"**{exc_type}**: {exc_value}", ""]
            if frames:
                lines.append("```")
                for frame in frames[-max_frames:]:
                    filename = frame.get("filename") or frame.get("module", "?")
                    lineno = frame.get("lineno", "?")
                    fn = frame.get("function", "?")
                    lines.append(f"  {filename}:{lineno} in {fn}")
                lines.append("```")
            return "\n".join(lines)

    return "_Stack trace not available_"


def format_sentry_issue_body(incident: dict, fingerprint: str) -> str:
    """Return a GitHub issue body string built from a Sentry incident."""
    level = incident.get("level", "error")
    project = incident.get("project", {})
    project_slug = (
        project.get("slug", "") if isinstance(project, dict) else str(project)
    )

    tags = incident.get("tags", [])
    environment = next(
        (
            t["value"]
            for t in tags
            if isinstance(t, dict) and t.get("key") == "environment"
        ),
        "unknown",
    )

    count = incident.get("count", 0)
    user_count = incident.get("userCount", incident.get("user_count", 0))
    first_seen = incident.get("firstSeen", incident.get("first_seen", "unknown"))
    last_seen = incident.get("lastSeen", incident.get("last_seen", "unknown"))
    sentry_url = incident.get("permalink", incident.get("url", ""))
    sentry_id = incident.get("id", "")
    culprit = incident.get("culprit", "")

    entries = incident.get("entries", [])
    stack_trace = format_stack_trace(entries)

    rows = [
        f"| **Severity** | `{level}` |",
        f"| **Project** | `{project_slug}` |" if project_slug else None,
        f"| **Environment** | `{environment}` |",
        f"| **Culprit** | `{culprit}` |" if culprit else None,
        f"| **Event count** | {count} |",
        f"| **Users affected** | {user_count} |",
        f"| **First seen** | {first_seen} |",
        f"| **Last seen** | {last_seen} |",
        f"| **Sentry ID** | `{sentry_id}` |" if sentry_id else None,
        f"| **Sentry link** | {sentry_url} |" if sentry_url else None,
    ]
    table = "\n".join(r for r in rows if r is not None)

    repro_sentry = (
        f"1. Open the Sentry incident: {sentry_url}"
        if sentry_url
        else "1. Locate the incident in Sentry."
    )

    return "\n".join(
        [
            "## Problem",
            "",
            "A Sentry incident was detected and routed to the engineering backlog.",
            "",
            "## Context",
            "",
            "| Field | Value |",
            "|-------|-------|",
            table,
            "",
            "## Stack Trace",
            "",
            stack_trace,
            "",
            "## Intent Contract",
            "",
            "- **Problem statement**: Investigate and resolve the Sentry incident described above.",
            "- **Success conditions**: Root cause identified, fix deployed, incident resolved in Sentry.",
            "- **Hard boundaries**: Do not change unrelated behaviour while fixing this incident.",
            "",
            "### Reproduction",
            "",
            repro_sentry,
            f"2. Reproduce in `{environment}` environment.",
            "3. Identify root cause from the stack trace above.",
            "",
            f"<!-- {DEDUPE_MARKER_PREFIX}{fingerprint} -->",
        ]
    )


def format_recurrence_comment(incident: dict) -> str:
    """Return a GitHub comment body for a recurring Sentry incident."""
    level = incident.get("level", "error")
    count = incident.get("count", 0)
    last_seen = incident.get("lastSeen", incident.get("last_seen", "unknown"))
    sentry_url = incident.get("permalink", incident.get("url", ""))

    rows = [
        f"| **Severity** | `{level}` |",
        f"| **Event count** | {count} |",
        f"| **Last seen** | {last_seen} |",
        f"| **Sentry link** | {sentry_url} |" if sentry_url else None,
    ]
    table = "\n".join(r for r in rows if r is not None)

    return "\n".join(
        [
            "**Recurring Sentry incident** — new occurrence detected.",
            "",
            "| Field | Value |",
            "|-------|-------|",
            table,
        ]
    )


def build_labels(level: str) -> list:
    """Return GitHub label names for a Sentry incident severity level."""
    priority = SEVERITY_TO_PRIORITY.get(level, "P2")
    return ["bug", "source/sentry", priority]


# ─── GitHub operations ────────────────────────────────────────────────────────


def find_existing_github_issue(repo: str, fingerprint: str) -> Optional[int]:
    """Return the number of an open issue that carries the fingerprint marker.

    Lists open ``source/sentry`` issues and checks each body client-side.
    Returns ``None`` if no match is found or if the ``gh`` call fails.
    """
    marker = f"<!-- {DEDUPE_MARKER_PREFIX}{fingerprint} -->"
    try:
        result = run_gh(
            [
                "issue",
                "list",
                "--repo",
                repo,
                "--state",
                "open",
                "--label",
                "source/sentry",
                "--json",
                "number,body",
                "--limit",
                "100",
            ]
        )
        issues = json.loads(result.stdout)
        for issue in issues:
            body = issue.get("body") or ""
            if marker in body:
                return issue["number"]
    except subprocess.CalledProcessError:
        pass
    return None


def create_github_issue(
    repo: str, title: str, body: str, labels: list
) -> int:
    """Create a GitHub issue and return its number."""
    args = ["issue", "create", "--repo", repo, "--title", title, "--body", body]
    for label in labels:
        args += ["--label", label]

    result = run_gh(args)
    # gh issue create prints the URL, e.g. https://github.com/owner/repo/issues/42
    url = result.stdout.strip()
    return int(url.rstrip("/").split("/")[-1])


def comment_on_github_issue(repo: str, issue_number: int, incident: dict) -> None:
    """Add a recurrence comment to an existing GitHub issue."""
    body = format_recurrence_comment(incident)
    run_gh(
        [
            "issue",
            "comment",
            str(issue_number),
            "--repo",
            repo,
            "--body",
            body,
        ]
    )


def ensure_label(repo: str, name: str, color: str, description: str) -> None:
    """Create a GitHub label if it does not already exist."""
    try:
        run_gh(
            [
                "label",
                "create",
                name,
                "--repo",
                repo,
                "--color",
                color,
                "--description",
                description,
                "--force",
            ]
        )
    except subprocess.CalledProcessError:
        pass


# ─── Main entry point ─────────────────────────────────────────────────────────


def ingest_incident(repo: str, incident: dict) -> int:
    """Ingest a Sentry incident and return the GitHub issue number.

    - New fingerprint → creates a GitHub issue; returns the new issue number.
    - Known fingerprint → comments on the existing issue; returns its number.
    """
    fingerprint = fingerprint_hash(incident)
    level = incident.get("level", "error")

    existing = find_existing_github_issue(repo, fingerprint)
    if existing is not None:
        comment_on_github_issue(repo, existing, incident)
        return existing

    prefix = LEVEL_TO_TITLE_PREFIX.get(level, "[P2]")
    raw_title = incident.get(
        "title",
        incident.get("metadata", {}).get("value", "Untitled Sentry incident"),
    )
    title = f"{prefix} {raw_title}"

    body = format_sentry_issue_body(incident, fingerprint)
    labels = build_labels(level)

    return create_github_issue(repo, title, body, labels)


# ─── Sentry API fetch ─────────────────────────────────────────────────────────


def fetch_sentry_incident(org: str, project: str, issue_id: str) -> dict:
    """Fetch a Sentry issue from the REST API."""
    token = os.environ.get("SENTRY_AUTH_TOKEN") or os.environ.get(
        "SENTRY_MASTER_TOKEN", ""
    )
    if not token:
        raise RuntimeError(
            "SENTRY_AUTH_TOKEN or SENTRY_MASTER_TOKEN must be set to fetch from Sentry API."
        )

    url = f"https://sentry.io/api/0/projects/{org}/{project}/issues/{issue_id}/"
    req = urllib.request.Request(
        url, headers={"Authorization": f"Bearer {token}"}
    )
    with urllib.request.urlopen(req) as resp:
        return json.loads(resp.read())


# ─── CLI ──────────────────────────────────────────────────────────────────────


def _build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        description="Ingest a Sentry incident into a GitHub issue.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    p.add_argument("--repo", required=True, help="GitHub repo (owner/repo)")
    p.add_argument(
        "--payload",
        help="Sentry incident JSON string (alternative: pipe JSON via stdin)",
    )
    p.add_argument("--sentry-org", help="Sentry org slug (used with --sentry-issue-id)")
    p.add_argument(
        "--sentry-project", help="Sentry project slug (used with --sentry-issue-id)"
    )
    p.add_argument(
        "--sentry-issue-id", help="Fetch this Sentry issue ID from the API"
    )
    return p


def main(argv=None) -> None:
    args = _build_parser().parse_args(argv)

    if args.sentry_issue_id:
        if not args.sentry_org or not args.sentry_project:
            print(
                "error: --sentry-org and --sentry-project are required with --sentry-issue-id",
                file=sys.stderr,
            )
            sys.exit(1)
        incident = fetch_sentry_incident(
            args.sentry_org, args.sentry_project, args.sentry_issue_id
        )
    elif args.payload:
        incident = json.loads(args.payload)
    else:
        raw = sys.stdin.read()
        if not raw.strip():
            print(
                "error: no payload provided — pass --payload or pipe JSON via stdin.",
                file=sys.stderr,
            )
            sys.exit(1)
        incident = json.loads(raw)

    issue_number = ingest_incident(args.repo, incident)
    print(issue_number)


if __name__ == "__main__":
    main()
