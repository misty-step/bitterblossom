from __future__ import annotations

import time
from datetime import datetime
from typing import Any, Callable

from conductorlib.common import (
    CmdError,
    FAILED_CHECK_CONCLUSIONS,
    FAILED_STATUS_CONTEXTS,
    SUCCESSFUL_CHECK_CONCLUSIONS,
    TRUSTED_REVIEW_AUTHOR_ASSOCIATIONS,
    TRUSTED_REVIEW_BOT_LOGINS,
    ReviewThread,
    Runner,
    stringify_exc,
    utc_now,
)
from conductorlib.tracker import gh_graphql, gh_json, split_repo


def rollup_item_name(item: dict[str, Any]) -> str:
    typename = str(item.get("__typename", ""))
    if typename == "CheckRun":
        return str(item.get("name", ""))
    if typename == "StatusContext":
        return str(item.get("context", ""))
    return ""


def rollup_item_workflow_name(item: dict[str, Any]) -> str:
    if str(item.get("__typename", "")) != "CheckRun":
        return ""
    return str(item.get("workflowName", ""))


def rollup_item_state(item: dict[str, Any]) -> tuple[str, bool, bool]:
    typename = str(item.get("__typename", ""))
    if typename == "CheckRun":
        status = str(item.get("status", "")).upper()
        if status != "COMPLETED":
            return status or "PENDING", False, False
        conclusion = str(item.get("conclusion", "")).upper()
        if conclusion in FAILED_CHECK_CONCLUSIONS:
            return conclusion or "FAILURE", True, True
        return conclusion or "SUCCESS", True, False
    if typename == "StatusContext":
        state = str(item.get("state", "")).upper()
        if state in FAILED_STATUS_CONTEXTS:
            return state, True, True
        if state == "SUCCESS":
            return state, True, False
        return state or "PENDING", False, False
    return "", False, False


def trusted_surface_matches(item: dict[str, Any], trusted_surface: str) -> bool:
    names = {rollup_item_name(item)}
    workflow_name = rollup_item_workflow_name(item)
    if workflow_name:
        names.add(workflow_name)
    names.discard("")
    return trusted_surface in names


def summarize_status_check_rollup(payload: dict[str, Any]) -> str:
    lines: list[str] = []
    for item in payload.get("statusCheckRollup", []):
        name = rollup_item_name(item)
        if not name:
            continue
        state, _terminal, _failed = rollup_item_state(item)
        lines.append(f"{name}: {state}")
    return "\n".join(lines) or "(no checks reported)"


def checks_complete(payload: dict[str, Any], required: set[str]) -> tuple[bool, bool]:
    required_remaining = set(required)
    all_present_terminal = True
    saw_any = False
    for item in payload.get("statusCheckRollup", []):
        name = rollup_item_name(item)
        if not name:
            continue
        saw_any = True
        state, terminal, failed = rollup_item_state(item)
        if failed and (not required or name in required):
            return False, True
        if required:
            if name in required_remaining and terminal and state in SUCCESSFUL_CHECK_CONCLUSIONS:
                required_remaining.discard(name)
            elif name in required and not terminal:
                all_present_terminal = False
            elif name in required and terminal and state not in SUCCESSFUL_CHECK_CONCLUSIONS:
                return False, True
        elif not terminal:
            all_present_terminal = False
    if required:
        return not required_remaining and all_present_terminal, False
    return saw_any and all_present_terminal, False


def wait_for_pr_checks(
    runner: Runner,
    repo: str,
    pr_number: int,
    timeout_minutes: int,
    *,
    required_status_checks: Callable[[Runner, str, str], list[str]],
    on_tick: Callable[[], None] | None = None,
) -> tuple[bool, str]:
    deadline = time.time() + max(60, timeout_minutes * 60)
    payload: dict[str, Any] = {}
    required: set[str] | None = None
    last_error = ""
    while time.time() < deadline:
        if on_tick is not None:
            on_tick()
        try:
            payload = gh_json(runner, ["pr", "view", str(pr_number), "--repo", repo, "--json", "baseRefName,statusCheckRollup"])
            if required is None:
                required = set(required_status_checks(runner, repo, str(payload.get("baseRefName", ""))))
            last_error = ""
        except CmdError as exc:
            last_error = str(exc)
            time.sleep(10)
            continue
        summary = summarize_status_check_rollup(payload)
        complete, failed = checks_complete(payload, required or set())
        if complete:
            return True, summary
        if failed:
            return False, summary
        time.sleep(10)
    detail = summarize_status_check_rollup(payload)
    if last_error and not payload:
        detail = f"{detail}\nlast polling error: {last_error}"
    return False, f"timed out waiting for PR #{pr_number} checks after {timeout_minutes}m\n{detail}"


def status_check_snapshot(payload: dict[str, Any]) -> tuple[tuple[str, str, str, str], ...]:
    snapshot: list[tuple[str, str, str, str]] = []
    for item in payload.get("statusCheckRollup", []):
        typename = str(item.get("__typename", ""))
        if typename == "CheckRun":
            name = str(item.get("name", ""))
            status = str(item.get("status", ""))
            started = str(item.get("startedAt", ""))
            completed = str(item.get("completedAt", ""))
        elif typename == "StatusContext":
            name = str(item.get("context", ""))
            status = str(item.get("state", ""))
            started = str(item.get("startedAt", ""))
            completed = ""
        else:
            continue
        snapshot.append((name, status, started, completed))
    return tuple(sorted(snapshot))


def wait_for_check_refresh(
    runner: Runner,
    repo: str,
    pr_number: int,
    before: tuple[tuple[str, str, str, str], ...],
    *,
    timeout_seconds: int = 60,
) -> None:
    deadline = time.time() + timeout_seconds
    while time.time() < deadline:
        pr = gh_json(runner, ["pr", "view", str(pr_number), "--repo", repo, "--json", "statusCheckRollup"])
        after = status_check_snapshot(pr)
        if after != before:
            return
        time.sleep(5)
    raise CmdError(f"timed out waiting for PR #{pr_number} checks to refresh after marking ready")


def list_unresolved_review_threads(runner: Runner, repo: str, pr_number: int) -> list[ReviewThread]:
    owner, name = split_repo(repo)
    query = """
query($owner:String!, $repo:String!, $number:Int!, $after:String) {
  repository(owner:$owner, name:$repo) {
    pullRequest(number:$number) {
      reviewThreads(first:100, after:$after) {
        nodes {
          id
          isResolved
          path
          line
          comments(first:1) {
            nodes {
              author { login }
              authorAssociation
              body
              url
            }
          }
        }
        pageInfo {
          hasNextPage
          endCursor
        }
      }
    }
  }
}
""".strip()
    threads: list[ReviewThread] = []
    after = ""
    while True:
        variables: dict[str, str | int] = {"owner": owner, "repo": name, "number": pr_number}
        if after:
            variables["after"] = after
        payload = gh_graphql(runner, query, variables)
        try:
            review_threads = payload["data"]["repository"]["pullRequest"]["reviewThreads"]
            nodes = review_threads["nodes"]
            page_info = review_threads["pageInfo"]
        except (KeyError, TypeError) as exc:
            raise CmdError(f"invalid review thread payload for PR #{pr_number}") from exc
        if not isinstance(nodes, list):
            raise CmdError(f"invalid review thread payload for PR #{pr_number}: nodes is not a list")
        if not isinstance(page_info, dict):
            raise CmdError(f"invalid review thread payload for PR #{pr_number}: pageInfo is not an object")
        for node in nodes:
            if not isinstance(node, dict):
                raise CmdError(f"invalid review thread payload for PR #{pr_number}: thread is not an object")
            if node.get("isResolved"):
                continue
            comments_node = node.get("comments", {})
            if not isinstance(comments_node, dict):
                raise CmdError(f"invalid review thread payload for PR #{pr_number}: comments is not an object")
            comments = comments_node.get("nodes", [])
            if not isinstance(comments, list):
                raise CmdError(f"invalid review thread payload for PR #{pr_number}: comments.nodes is not a list")
            comment = comments[0] if comments else {}
            if comment and not isinstance(comment, dict):
                raise CmdError(f"invalid review thread payload for PR #{pr_number}: comment is not an object")
            line = node.get("line")
            try:
                thread_line = int(line) if line is not None else None
            except (TypeError, ValueError):
                thread_line = None
            author_node = comment.get("author", {})
            if author_node is None:
                author_login = "unknown"
            elif isinstance(author_node, dict):
                author_login = str(author_node.get("login") or "unknown")
            else:
                raise CmdError(f"invalid review thread payload for PR #{pr_number}: author is not an object")
            author_association = str(comment.get("authorAssociation") or "").upper()
            threads.append(
                ReviewThread(
                    id=str(node.get("id", "")),
                    path=str(node.get("path") or ""),
                    line=thread_line,
                    author_login=author_login,
                    author_association=author_association,
                    body=str(comment.get("body") or ""),
                    url=str(comment.get("url") or ""),
                )
            )
        if page_info.get("hasNextPage") is not True:
            break
        after = str(page_info.get("endCursor") or "")
        if not after:
            raise CmdError(f"invalid review thread payload for PR #{pr_number}: missing endCursor")
    return [thread for thread in threads if thread.id]


def summarize_review_threads(threads: list[ReviewThread]) -> str:
    lines = [
        "Active merge-blocking review findings remain in PR threads.",
        "Address the feedback on the existing PR, push any needed updates, then resolve each addressed thread before handing back to the conductor.",
    ]
    for thread in threads:
        location = thread.path or "(unknown path)"
        if thread.line is not None:
            location = f"{location}:{thread.line}"
        body = " ".join(thread.body.split())
        if len(body) > 280:
            body = body[:277].rstrip() + "..."
        lines.append(f"- {location} by @{thread.author_login}: {body} ({thread.url})")
    return "\n".join(lines)


def resolve_review_threads(runner: Runner, thread_ids: list[str]) -> None:
    query = """
mutation($threadId:ID!) {
  resolveReviewThread(input:{threadId:$threadId}) {
    thread {
      isResolved
    }
  }
}
""".strip()
    failures: list[str] = []
    for thread_id in thread_ids:
        try:
            gh_graphql(runner, query, {"threadId": thread_id})
        except CmdError as exc:
            failures.append(f"{thread_id}: {exc}")
    if failures:
        raise CmdError("failed to resolve review threads:\n" + "\n".join(failures))


def is_trusted_review_author(thread: ReviewThread) -> bool:
    if thread.author_association in TRUSTED_REVIEW_AUTHOR_ASSOCIATIONS:
        return True
    return thread.author_login.lower() in TRUSTED_REVIEW_BOT_LOGINS


def present_pr_status_checks(runner: Runner, repo: str, pr_number: int) -> tuple[str, set[str]]:
    pr = gh_json(runner, ["pr", "view", str(pr_number), "--repo", repo, "--json", "baseRefName,statusCheckRollup"])
    names: set[str] = set()
    for item in pr.get("statusCheckRollup", []):
        typename = item.get("__typename")
        if typename == "CheckRun":
            name = item.get("name")
        elif typename == "StatusContext":
            name = item.get("context")
        else:
            name = None
        if name:
            names.add(str(name))
    return str(pr["baseRefName"]), names


def ensure_required_checks_present(
    runner: Runner,
    repo: str,
    pr_number: int,
    *,
    required_status_checks: Callable[[Runner, str, str], list[str]],
) -> None:
    base_branch, present = present_pr_status_checks(runner, repo, pr_number)
    required = required_status_checks(runner, repo, base_branch)
    missing = [context for context in required if context not in present]
    if missing:
        joined = ", ".join(sorted(missing))
        raise CmdError(f"required status checks missing for PR #{pr_number} on {base_branch}: {joined}")


SurfaceMatchSnapshot = tuple[str, str, str, str, str]
TrustedSurfaceSnapshot = tuple[tuple[str, tuple[SurfaceMatchSnapshot, ...]], ...]


def trusted_surfaces_pending(payload: dict[str, Any], trusted_surfaces: list[str]) -> list[str]:
    blocking: list[str] = []
    rollup = payload.get("statusCheckRollup", [])
    if not isinstance(rollup, list):
        return list(trusted_surfaces)
    for pattern in trusted_surfaces:
        matched = False
        for item in rollup:
            if not isinstance(item, dict):
                continue
            if not trusted_surface_matches(item, pattern):
                continue
            matched = True
            name = rollup_item_name(item)
            _state, terminal, failed = rollup_item_state(item)
            if not terminal or failed:
                blocking.append(name)
        if not matched:
            blocking.append(pattern)
    return blocking


def trusted_surface_snapshot(payload: dict[str, Any], trusted_surfaces: list[str]) -> TrustedSurfaceSnapshot:
    rollup = payload.get("statusCheckRollup", [])
    if not isinstance(rollup, list):
        return tuple((pattern, ()) for pattern in trusted_surfaces)
    snapshots: list[tuple[str, tuple[SurfaceMatchSnapshot, ...]]] = []
    for pattern in trusted_surfaces:
        matches: list[SurfaceMatchSnapshot] = []
        for item in rollup:
            if not isinstance(item, dict):
                continue
            if not trusted_surface_matches(item, pattern):
                continue
            name = rollup_item_name(item)
            workflow_name = rollup_item_workflow_name(item)
            state, _terminal, _failed = rollup_item_state(item)
            started = str(item.get("startedAt") or "")
            completed = str(item.get("completedAt") or "")
            matches.append((name, workflow_name, state, started, completed))
        snapshots.append((pattern, tuple(sorted(matches))))
    return tuple(snapshots)


def wait_for_external_reviews(
    runner: Runner,
    repo: str,
    pr_number: int,
    trusted_surfaces: list[str],
    *,
    sleep_until: Callable[[float, int], bool],
    quiet_window_seconds: int = 60,
    timeout_minutes: int = 30,
    on_tick: Callable[[], None] | None = None,
) -> tuple[bool, str]:
    if not trusted_surfaces:
        return True, ""
    deadline = time.time() + timeout_minutes * 60
    quiet_since: float | None = None
    last_payload: dict[str, Any] = {}
    last_snapshot: TrustedSurfaceSnapshot | None = None
    while time.time() < deadline:
        if on_tick is not None:
            on_tick()
        try:
            last_payload = gh_json(runner, ["pr", "view", str(pr_number), "--repo", repo, "--json", "statusCheckRollup"])
        except CmdError:
            if not sleep_until(deadline, 10):
                break
            continue
        current_snapshot = trusted_surface_snapshot(last_payload, trusted_surfaces)
        if current_snapshot != last_snapshot:
            quiet_since = None
            last_snapshot = current_snapshot
        pending = trusted_surfaces_pending(last_payload, trusted_surfaces)
        if pending:
            quiet_since = None
            if not sleep_until(deadline, 10):
                break
            continue
        if quiet_since is None:
            quiet_since = time.time()
        if time.time() - quiet_since >= quiet_window_seconds:
            return True, summarize_status_check_rollup(last_payload)
        if not sleep_until(deadline, 5):
            break
    if not last_payload:
        pending_str = "failed to fetch PR status from GitHub"
    else:
        pending = trusted_surfaces_pending(last_payload, trusted_surfaces)
        pending_str = ", ".join(pending) if pending else "(settled but quiet window did not elapse)"
    return False, f"timed out waiting for trusted external reviews to settle on PR #{pr_number} after {timeout_minutes}m: {pending_str}"


def wait_for_pr_minimum_age(
    runner: Runner,
    repo: str,
    pr_number: int,
    *,
    minimum_age_seconds: int,
    sleep_until: Callable[[float, int], bool],
    timeout_minutes: int = 30,
    on_tick: Callable[[], None] | None = None,
) -> tuple[bool, str]:
    if minimum_age_seconds <= 0:
        return True, ""
    deadline = time.time() + timeout_minutes * 60
    last_error = ""
    created: datetime | None = None
    while time.time() < deadline and created is None:
        if on_tick is not None:
            on_tick()
        try:
            pr = gh_json(runner, ["pr", "view", str(pr_number), "--repo", repo, "--json", "createdAt"])
            created_at = pr.get("createdAt")
            if not isinstance(created_at, str) or not created_at:
                raise CmdError(f"PR #{pr_number} missing or has invalid createdAt value from API: {pr!r}")
            created = datetime.fromisoformat(created_at.replace("Z", "+00:00"))
            last_error = ""
        except CmdError as exc:
            last_error = stringify_exc(exc)
            if not sleep_until(deadline, 10):
                break
        except ValueError as exc:
            raise CmdError(f"invalid PR createdAt for #{pr_number}: {created_at!r}") from exc
    if created is None:
        suffix = f"\nlast polling error: {last_error}" if last_error else ""
        return False, f"timed out waiting for PR #{pr_number} metadata before minimum-age evaluation{suffix}"
    while time.time() < deadline:
        if on_tick is not None:
            on_tick()
        last_age_seconds = max(0, int((utc_now() - created).total_seconds()))
        if last_age_seconds >= minimum_age_seconds:
            return True, f"PR #{pr_number} age {last_age_seconds}s satisfies minimum age {minimum_age_seconds}s"
        sleep_seconds = min(30, max(1, minimum_age_seconds - last_age_seconds))
        if not sleep_until(deadline, sleep_seconds):
            break
    last_age_seconds = max(0, int((utc_now() - created).total_seconds()))
    suffix = f"\nlast polling error: {last_error}" if last_error else ""
    return False, f"timed out waiting for PR #{pr_number} to reach minimum age {minimum_age_seconds}s (current age {last_age_seconds}s){suffix}"
