from __future__ import annotations

import hashlib
import json
import pathlib
import re
import tempfile
from typing import Any, Callable

from conductorlib.common import CmdError, Issue, QAFinding, QA_DEDUPE_PAGE_SIZE, ReadinessResult, Runner


def gh_json(runner: Runner, args: list[str]) -> Any:
    out = runner.run(["gh", *args], timeout=60)
    return json.loads(out)


def split_repo(repo: str) -> tuple[str, str]:
    owner, _, name = repo.partition("/")
    if not owner or not name:
        raise CmdError(f"invalid repo slug: {repo!r}")
    return owner, name


def gh_graphql(runner: Runner, query: str, variables: dict[str, str | int]) -> Any:
    argv = ["gh", "api", "graphql", "-f", f"query={query}"]
    for key, value in variables.items():
        argv.extend(["-F", f"{key}={value}"])
    out = runner.run(argv, timeout=60)
    return json.loads(out)


def issue_priority(labels: list[str]) -> tuple[int, str]:
    order = {"P0": 0, "P1": 1, "P2": 2, "P3": 3}
    best = 9
    matched = ""
    for label in labels:
        upper = label.upper()
        if upper in order and order[upper] < best:
            best = order[upper]
            matched = upper
    return best, matched


def is_qa_origin_issue(labels: list[str]) -> bool:
    return any(label.lower() == "source/qa" for label in labels)


def qa_priority_rank(issue: Issue) -> int:
    return 0 if is_qa_origin_issue(issue.labels) else 1


def qa_priority_label(severity: str) -> str:
    order = {"critical": "p0", "high": "p1", "medium": "p2", "low": "p3"}
    return order.get(severity.lower(), "p2")


def priority_label_rank(label: str) -> int | None:
    upper = label.upper()
    if not re.fullmatch(r"P[0-3]", upper):
        return None
    return int(upper[1])


def best_priority_label(labels: list[str]) -> str:
    best_rank: int | None = None
    matched = ""
    for label in labels:
        rank = priority_label_rank(label)
        if rank is None:
            continue
        if best_rank is None or rank < best_rank:
            best_rank = rank
            matched = label
    return matched


def qa_dedupe_key(title: str, summary: str, target_url: str, environment: str, repro_steps: list[str]) -> str:
    seed = "\n".join(
        [
            title.strip().lower(),
            summary.strip().lower(),
            target_url.strip().lower(),
            environment.strip().lower(),
            "\n".join(step.strip().lower() for step in repro_steps),
        ]
    )
    return hashlib.sha256(seed.encode("utf-8")).hexdigest()[:12]


def normalize_external_dedupe_key(raw_key: Any) -> str | None:
    candidate = str(raw_key or "").strip().lower()
    if re.fullmatch(r"[a-f0-9]{12}", candidate):
        return candidate
    return None


def parse_qa_intake_payload(payload: dict[str, Any]) -> list[QAFinding]:
    if not isinstance(payload, dict):
        raise CmdError("qa intake payload must be a JSON object")
    target = str(payload.get("target") or "").strip()
    environment = str(payload.get("environment") or "").strip()
    raw_findings = payload.get("findings")
    if not target:
        raise CmdError("qa intake payload missing target")
    if not environment:
        raise CmdError("qa intake payload missing environment")
    if not isinstance(raw_findings, list) or not raw_findings:
        raise CmdError("qa intake payload must include a non-empty findings list")

    findings: list[QAFinding] = []
    for item in raw_findings:
        if not isinstance(item, dict):
            raise CmdError("qa finding must be an object")
        title = str(item.get("title") or "").strip()
        summary = str(item.get("summary") or "").strip()
        severity = str(item.get("severity") or "").strip().lower()
        repro_steps = item.get("repro_steps") or []
        evidence = item.get("evidence") or []
        finding_target = str(item.get("target_url") or target).strip()
        finding_environment = str(item.get("environment") or environment).strip()
        if not title:
            raise CmdError("qa finding missing title")
        if not summary:
            raise CmdError(f"qa finding {title!r} missing summary")
        if severity not in {"critical", "high", "medium", "low"}:
            raise CmdError(f"qa finding {title!r} has unsupported severity {severity!r}")
        if not isinstance(repro_steps, list) or not repro_steps:
            raise CmdError(f"qa finding {title!r} must include repro_steps")
        if not isinstance(evidence, list):
            raise CmdError(f"qa finding {title!r} evidence must be a list")
        normalized_steps = [str(step).strip() for step in repro_steps if str(step).strip()]
        if not normalized_steps:
            raise CmdError(f"qa finding {title!r} must include non-empty repro_steps")
        normalized_evidence: list[dict[str, str]] = []
        for entry in evidence:
            if not isinstance(entry, dict):
                raise CmdError(f"qa finding {title!r} evidence entries must be objects")
            normalized_evidence.append({str(key): str(value) for key, value in entry.items()})
        priority = qa_priority_label(severity)
        dedupe_key = normalize_external_dedupe_key(item.get("dedupe_key"))
        if dedupe_key is None:
            dedupe_key = qa_dedupe_key(title, summary, finding_target, finding_environment, normalized_steps)
        findings.append(
            QAFinding(
                title=title,
                summary=summary,
                severity=severity,
                target_url=finding_target,
                environment=finding_environment,
                repro_steps=normalized_steps,
                evidence=normalized_evidence,
                dedupe_key=dedupe_key,
                priority_label=priority,
                labels=["autopilot", "bug", "domain/infra", priority, "source/qa"],
            )
        )
    return findings


def dedupe_key_from_issue_body(body: str) -> str | None:
    match = re.search(r"bitterblossom-qa-dedupe:([a-f0-9]{12})", body)
    if match is None:
        return None
    return match.group(1)


def issue_number_from_url(issue_url: str) -> int:
    try:
        return int(issue_url.rstrip("/").rsplit("/", 1)[-1])
    except ValueError as exc:
        raise CmdError(f"could not parse issue number from url: {issue_url!r}") from exc


def render_qa_evidence_lines(evidence: list[dict[str, str]], *, empty_message: str) -> str:
    lines = "\n".join(
        f"- {entry.get('kind', 'evidence')}: [{entry.get('label', entry.get('url', 'artifact'))}]({entry.get('url', '')})"
        if entry.get("url")
        else f"- {entry.get('kind', 'evidence')}: {entry.get('label', 'artifact')}"
        for entry in evidence
    )
    return lines or empty_message


def render_qa_issue_body(finding: QAFinding) -> str:
    steps = "\n".join(f"{index}. {step}" for index, step in enumerate(finding.repro_steps, start=1))
    evidence_lines = render_qa_evidence_lines(finding.evidence, empty_message="- none attached")
    return "\n".join(
        [
            "## Product Spec",
            "### Problem",
            finding.summary,
            "",
            "### Intent Contract",
            "- Intent: capture this QA-discovered regression as a GitHub issue with reproducible evidence.",
            "- Success Conditions: the issue carries severity, target, environment, evidence, and deterministic dedupe metadata.",
            "- Hard Boundaries: GitHub remains the canonical work queue.",
            "- Non-Goals: automated remediation in this intake lane.",
            "",
            "## Acceptance Criteria",
            "- [ ] [behavioral] Reproduce the reported regression on the affected target.",
            "- [ ] [behavioral] Confirm the proposed fix removes the observed failure.",
            "- [ ] [test] Preserve the QA evidence contract and dedupe marker.",
            "",
            "## QA Finding",
            f"- Severity: `{finding.severity}`",
            f"- Target: `{finding.target_url}`",
            f"- Environment: `{finding.environment}`",
            "",
            "## Reproduction",
            steps,
            "",
            "## Evidence",
            evidence_lines,
            "",
            "<!-- bitterblossom-qa-origin:true -->",
            f"<!-- bitterblossom-qa-dedupe:{finding.dedupe_key} -->",
        ]
    )


def render_qa_issue_comment(finding: QAFinding) -> str:
    evidence_lines = render_qa_evidence_lines(finding.evidence, empty_message="- no new evidence attached")
    return "\n".join(
        [
            "QA intake re-observed this finding.",
            "",
            f"- Severity: `{finding.severity}`",
            f"- Target: `{finding.target_url}`",
            f"- Environment: `{finding.environment}`",
            "",
            "### Reproduction",
            "\n".join(f"{index}. {step}" for index, step in enumerate(finding.repro_steps, start=1)),
            "",
            "### Evidence",
            evidence_lines,
        ]
    )


def list_open_qa_issues(runner: Runner, repo: str) -> list[dict[str, Any]]:
    page = 1
    issues: list[dict[str, Any]] = []
    while True:
        payload = gh_json(
            runner,
            ["api", f"repos/{repo}/issues?state=open&labels=source/qa&per_page={QA_DEDUPE_PAGE_SIZE}&page={page}"],
        )
        if not isinstance(payload, list):
            raise CmdError("unexpected GitHub issue list payload while loading source/qa issues")
        if not payload:
            return issues
        for item in payload:
            if not isinstance(item, dict) or item.get("pull_request") is not None:
                continue
            issues.append(item)
        page += 1


def existing_qa_issues_by_key(runner: Runner, repo: str) -> dict[str, Issue]:
    issues_by_key: dict[str, Issue] = {}
    for item in list_open_qa_issues(runner, repo):
        body = item.get("body") or ""
        dedupe_key = dedupe_key_from_issue_body(body)
        if dedupe_key is None:
            continue
        issues_by_key[dedupe_key] = Issue(
            number=item["number"],
            title=item["title"],
            body=body,
            url=item["url"],
            labels=[label_obj["name"] for label_obj in item.get("labels", [])],
            updated_at=item.get("updated_at") or item.get("updatedAt") or "",
        )
    return issues_by_key


def write_temp_body(prefix: str, body: str) -> str:
    with tempfile.NamedTemporaryFile("w", prefix=prefix, suffix=".md", delete=False) as handle:
        handle.write(body)
        return handle.name


def sync_qa_findings(
    runner: Runner,
    repo: str,
    findings: list[QAFinding],
    *,
    existing_issue_by_key: dict[str, Issue] | None = None,
) -> tuple[list[str], list[str]]:
    issues_by_key = existing_issue_by_key if existing_issue_by_key is not None else existing_qa_issues_by_key(runner, repo)
    created: list[str] = []
    updated: list[str] = []
    for finding in findings:
        existing = issues_by_key.get(finding.dedupe_key)
        if existing is not None:
            existing_priority = best_priority_label(existing.labels)
            body_path = write_temp_body("bb-qa-comment-", render_qa_issue_comment(finding))
            try:
                runner.run(["gh", "issue", "comment", str(existing.number), "--repo", repo, "--body-file", body_path], timeout=60)
            finally:
                pathlib.Path(body_path).unlink(missing_ok=True)
            existing_rank = priority_label_rank(existing_priority) if existing_priority else None
            finding_rank = priority_label_rank(finding.priority_label)
            if finding_rank is not None and (existing_rank is None or finding_rank < existing_rank):
                argv = ["gh", "issue", "edit", str(existing.number), "--repo", repo, "--add-label", finding.priority_label]
                if existing_priority:
                    argv.extend(["--remove-label", existing_priority])
                runner.run(argv, timeout=60)
                existing.labels = [label for label in existing.labels if label != existing_priority]
                existing.labels.append(finding.priority_label)
            updated.append(existing.url)
            continue

        body_path = write_temp_body("bb-qa-issue-", render_qa_issue_body(finding))
        try:
            argv = ["gh", "issue", "create", "--repo", repo, "--title", f"[QA][{finding.priority_label.upper()}] {finding.title}"]
            for label in finding.labels:
                argv.extend(["--label", label])
            argv.extend(["--body-file", body_path])
            issue_url = runner.run(argv, timeout=60).strip()
        finally:
            pathlib.Path(body_path).unlink(missing_ok=True)
        created.append(issue_url)
        issue_number = issue_number_from_url(issue_url)
        issues_by_key[finding.dedupe_key] = Issue(
            number=issue_number,
            title=finding.title,
            body="",
            url=issue_url,
            labels=finding.labels,
        )
    return created, updated


def list_candidate_issues(runner: Runner, repo: str, label: str, limit: int) -> list[Issue]:
    issues = gh_json(
        runner,
        ["issue", "list", "--repo", repo, "--state", "open", "--label", label, "--limit", str(limit), "--json", "number,title,body,url,labels,updatedAt"],
    )
    return [
        Issue(
            number=item["number"],
            title=item["title"],
            body=item.get("body") or "",
            url=item["url"],
            labels=[label_obj["name"] for label_obj in item.get("labels", [])],
            updated_at=item.get("updatedAt") or "",
        )
        for item in issues
    ]


def get_issue(runner: Runner, repo: str, issue_number: int) -> Issue:
    item = gh_json(
        runner,
        ["issue", "view", str(issue_number), "--repo", repo, "--json", "number,title,body,url,labels,updatedAt"],
    )
    return Issue(
        number=item["number"],
        title=item["title"],
        body=item.get("body") or "",
        url=item["url"],
        labels=[label_obj["name"] for label_obj in item.get("labels", [])],
        updated_at=item.get("updatedAt") or "",
    )


def has_markdown_heading(body: str, marker: str) -> bool:
    active_fence: str | None = None
    for raw_line in body.splitlines():
        line = raw_line.rstrip()
        stripped = line.lstrip()
        match = re.match(r"^(`{3,}|~{3,})", stripped)
        if match:
            fence_type = match.group(1)[0]
            if active_fence is None:
                active_fence = fence_type
            elif fence_type == active_fence:
                active_fence = None
            continue
        if active_fence is None and line == marker:
            return True
    return False


def validate_issue_readiness(issue: Issue) -> ReadinessResult:
    reasons: list[str] = []
    for marker in ("## Product Spec", "### Intent Contract"):
        if not has_markdown_heading(issue.body, marker):
            reasons.append(f"missing `{marker}` section")
    return ReadinessResult(ready=not reasons, reasons=reasons)


def collect_routable_issues(
    issues: list[Issue],
    repo: str,
    *,
    lease_warnings: Callable[[int], list[str]],
) -> tuple[list[Issue], dict[int, list[str]]]:
    eligible: list[Issue] = []
    readiness_failures: dict[int, list[str]] = {}
    for issue in issues:
        messages = lease_warnings(issue.number)
        if messages:
            readiness_failures[issue.number] = messages
            continue
        readiness = validate_issue_readiness(issue)
        if readiness.ready:
            eligible.append(issue)
            continue
        readiness_failures[issue.number] = readiness.reasons
    return eligible, readiness_failures


def verify_builder_pr(runner: Runner, repo: str, pr_number: int, expected_branch: str) -> tuple[int, str]:
    pr = gh_json(runner, ["pr", "view", str(pr_number), "--repo", repo, "--json", "number,url,headRefName,state"])
    if int(pr["number"]) != pr_number:
        raise CmdError(f"builder artifact PR number mismatch: expected {pr_number}, got {pr['number']}")
    if pr["headRefName"] != expected_branch:
        raise CmdError(f"builder artifact PR head mismatch: expected {expected_branch!r}, got {pr['headRefName']!r}")
    if pr["state"] != "OPEN":
        raise CmdError(f"builder artifact PR is not open: #{pr_number} state={pr['state']}")
    return int(pr["number"]), str(pr["url"])


def comment_issue(runner: Runner, repo: str, issue_number: int, body: str) -> None:
    runner.run(["gh", "issue", "comment", str(issue_number), "--repo", repo, "--body", body], timeout=60)


def comment_pr(runner: Runner, repo: str, pr_number: int, body: str) -> None:
    runner.run(["gh", "pr", "comment", str(pr_number), "--repo", repo, "--body", body], timeout=60)
