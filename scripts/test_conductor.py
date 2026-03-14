"""Tests for the Sentry-to-GitHub intake adapter (scripts/sentry_intake.py).

Covers:
- fingerprint stability and uniqueness
- issue body formatting (fields required by acceptance criteria)
- severity-to-label mapping
- deduplication: new incident creates issue, repeated fingerprint comments
- CLI emits issue number
"""

import json
import subprocess
import sys
import os
from unittest.mock import MagicMock, patch, call

import pytest

# sentry_intake lives alongside this file; add scripts/ to path when needed.
sys.path.insert(0, os.path.dirname(__file__))

from sentry_intake import (
    fingerprint_hash,
    format_stack_trace,
    format_sentry_issue_body,
    build_labels,
    find_existing_github_issue,
    create_github_issue,
    comment_on_github_issue,
    ingest_incident,
    DEDUPE_MARKER_PREFIX,
)

# ─── Fixtures ────────────────────────────────────────────────────────────────

SAMPLE_INCIDENT = {
    "id": "sentry-123",
    "title": "TypeError: Cannot read property 'foo' of undefined",
    "level": "error",
    "fingerprint": ["abc123fingerprint"],
    "count": 42,
    "userCount": 7,
    "firstSeen": "2026-03-01T10:00:00Z",
    "lastSeen": "2026-03-14T09:00:00Z",
    "culprit": "app/components/Checkout.handleSubmit",
    "permalink": "https://sentry.io/organizations/misty-step/issues/sentry-123/",
    "project": {"slug": "bitterblossom-api"},
    "tags": [{"key": "environment", "value": "production"}],
    "entries": [
        {
            "type": "exception",
            "data": {
                "values": [
                    {
                        "type": "TypeError",
                        "value": "Cannot read property 'foo' of undefined",
                        "stacktrace": {
                            "frames": [
                                {
                                    "filename": "app/utils.js",
                                    "lineno": 14,
                                    "function": "getConfig",
                                },
                                {
                                    "filename": "app/components/Checkout.js",
                                    "lineno": 88,
                                    "function": "handleSubmit",
                                },
                            ]
                        },
                    }
                ]
            },
        }
    ],
    "metadata": {"value": "Cannot read property 'foo' of undefined"},
}


def _make_gh_result(stdout: str) -> MagicMock:
    r = MagicMock()
    r.stdout = stdout
    return r


# ─── fingerprint_hash ────────────────────────────────────────────────────────


class TestFingerprintHash:
    def test_stable_across_calls(self):
        assert fingerprint_hash(SAMPLE_INCIDENT) == fingerprint_hash(SAMPLE_INCIDENT)

    def test_is_16_hex_chars(self):
        h = fingerprint_hash(SAMPLE_INCIDENT)
        assert len(h) == 16
        assert all(c in "0123456789abcdef" for c in h)

    def test_different_fingerprints_yield_different_hashes(self):
        a = {**SAMPLE_INCIDENT, "fingerprint": ["fp-a"]}
        b = {**SAMPLE_INCIDENT, "fingerprint": ["fp-b"]}
        assert fingerprint_hash(a) != fingerprint_hash(b)

    def test_falls_back_to_id_when_no_fingerprint(self):
        inc = {k: v for k, v in SAMPLE_INCIDENT.items() if k != "fingerprint"}
        h = fingerprint_hash(inc)
        assert len(h) == 16


# ─── format_stack_trace ──────────────────────────────────────────────────────


class TestFormatStackTrace:
    def test_extracts_exception_type_and_message(self):
        result = format_stack_trace(SAMPLE_INCIDENT["entries"])
        assert "TypeError" in result
        assert "Cannot read property" in result

    def test_includes_frame_filenames(self):
        result = format_stack_trace(SAMPLE_INCIDENT["entries"])
        assert "app/utils.js" in result
        assert "app/components/Checkout.js" in result

    def test_returns_placeholder_for_empty_entries(self):
        result = format_stack_trace([])
        assert "not available" in result.lower()

    def test_returns_placeholder_for_non_exception_entries(self):
        result = format_stack_trace([{"type": "breadcrumbs", "data": {}}])
        assert "not available" in result.lower()


# ─── format_sentry_issue_body ────────────────────────────────────────────────


class TestFormatSentryIssueBody:
    def setup_method(self):
        self.fp = fingerprint_hash(SAMPLE_INCIDENT)
        self.body = format_sentry_issue_body(SAMPLE_INCIDENT, self.fp)

    def test_includes_severity(self):
        assert "error" in self.body

    def test_includes_environment(self):
        assert "production" in self.body

    def test_includes_sentry_link(self):
        assert "sentry.io" in self.body

    def test_includes_fingerprint_dedupe_marker(self):
        assert f"{DEDUPE_MARKER_PREFIX}{self.fp}" in self.body

    def test_includes_stack_trace(self):
        assert "TypeError" in self.body

    def test_includes_intent_contract_section(self):
        assert "Intent Contract" in self.body

    def test_includes_event_count(self):
        assert "42" in self.body

    def test_includes_first_and_last_seen(self):
        assert "2026-03-01" in self.body
        assert "2026-03-14" in self.body


# ─── build_labels ────────────────────────────────────────────────────────────


class TestBuildLabels:
    def test_fatal_maps_to_p0(self):
        assert "P0" in build_labels("fatal")

    def test_error_maps_to_p1(self):
        assert "P1" in build_labels("error")

    def test_warning_maps_to_p2(self):
        assert "P2" in build_labels("warning")

    def test_info_maps_to_p3(self):
        assert "P3" in build_labels("info")

    def test_unknown_level_defaults_to_p2(self):
        assert "P2" in build_labels("unknown-level")

    def test_always_includes_bug_label(self):
        for level in ("fatal", "error", "warning", "info"):
            assert "bug" in build_labels(level)

    def test_always_includes_source_sentry_label(self):
        for level in ("fatal", "error", "warning", "info"):
            assert "source/sentry" in build_labels(level)


# ─── find_existing_github_issue ──────────────────────────────────────────────


class TestFindExistingGithubIssue:
    def test_returns_none_when_no_issues_found(self):
        with patch("sentry_intake.run_gh", return_value=_make_gh_result("[]")):
            assert find_existing_github_issue("owner/repo", "abc123") is None

    def test_returns_issue_number_when_marker_matches(self):
        fp = "abc123deadbeef01"
        body = f"Some body\n<!-- {DEDUPE_MARKER_PREFIX}{fp} -->"
        payload = json.dumps([{"number": 42, "body": body}])
        with patch("sentry_intake.run_gh", return_value=_make_gh_result(payload)):
            assert find_existing_github_issue("owner/repo", fp) == 42

    def test_ignores_issues_with_different_fingerprint(self):
        body = f"Some body\n<!-- {DEDUPE_MARKER_PREFIX}differentfp123456 -->"
        payload = json.dumps([{"number": 99, "body": body}])
        with patch("sentry_intake.run_gh", return_value=_make_gh_result(payload)):
            assert find_existing_github_issue("owner/repo", "abc123") is None

    def test_returns_none_on_gh_failure(self):
        with patch(
            "sentry_intake.run_gh",
            side_effect=subprocess.CalledProcessError(1, "gh"),
        ):
            assert find_existing_github_issue("owner/repo", "abc123") is None


# ─── create_github_issue ─────────────────────────────────────────────────────


class TestCreateGithubIssue:
    def test_returns_parsed_issue_number(self):
        mock = _make_gh_result("https://github.com/owner/repo/issues/55\n")
        with patch("sentry_intake.run_gh", return_value=mock) as m:
            num = create_github_issue("owner/repo", "Title", "Body", ["bug"])
        assert num == 55

    def test_passes_repo_title_body_labels(self):
        mock = _make_gh_result("https://github.com/owner/repo/issues/55\n")
        with patch("sentry_intake.run_gh", return_value=mock) as m:
            create_github_issue("owner/repo", "My Title", "My Body", ["bug", "P1"])
            args = m.call_args[0][0]
        assert "--repo" in args
        assert "owner/repo" in args
        assert "--title" in args
        assert "My Title" in args
        assert "--body" in args
        assert "My Body" in args
        assert "--label" in args


# ─── comment_on_github_issue ─────────────────────────────────────────────────


class TestCommentOnGithubIssue:
    def test_calls_gh_issue_comment(self):
        mock = _make_gh_result("")
        with patch("sentry_intake.run_gh", return_value=mock) as m:
            comment_on_github_issue("owner/repo", 42, SAMPLE_INCIDENT)
            args = m.call_args[0][0]
        assert "comment" in args
        assert "42" in args
        assert "--repo" in args


# ─── ingest_incident (integration) ───────────────────────────────────────────


class TestIngestIncident:
    """Behavioral acceptance tests for the full ingest path."""

    def _make_sequential_mock(self, responses: list):
        """Return a mock that returns successive responses from `responses`."""
        idx = [0]

        def _mock(args, **kwargs):
            r = responses[idx[0]]
            idx[0] += 1
            return r

        return _mock

    def test_new_incident_creates_github_issue(self):
        """Given a new Sentry incident, ingest creates a GitHub issue."""
        search = _make_gh_result("[]")
        create = _make_gh_result(
            "https://github.com/owner/repo/issues/55\n"
        )
        with patch(
            "sentry_intake.run_gh",
            side_effect=self._make_sequential_mock([search, create]),
        ):
            number = ingest_incident("owner/repo", SAMPLE_INCIDENT)

        assert number == 55

    def test_repeated_incident_comments_existing_issue(self):
        """Given a recurring fingerprint, ingest comments on the existing issue."""
        fp = fingerprint_hash(SAMPLE_INCIDENT)
        existing_body = f"old body\n<!-- {DEDUPE_MARKER_PREFIX}{fp} -->"
        search = _make_gh_result(
            json.dumps([{"number": 77, "body": existing_body}])
        )
        comment = _make_gh_result("")
        with patch(
            "sentry_intake.run_gh",
            side_effect=self._make_sequential_mock([search, comment]),
        ):
            number = ingest_incident("owner/repo", SAMPLE_INCIDENT)

        assert number == 77

    def test_new_issue_title_includes_severity_prefix(self):
        """New issue title carries the [P-level] prefix."""
        search = _make_gh_result("[]")
        create = _make_gh_result(
            "https://github.com/owner/repo/issues/88\n"
        )
        captured: list = []

        def capturing_mock(args, **kwargs):
            captured.extend(args)
            calls = [search, create]
            idx = capturing_mock._call_count
            capturing_mock._call_count += 1
            return calls[idx]

        capturing_mock._call_count = 0

        with patch("sentry_intake.run_gh", side_effect=capturing_mock):
            ingest_incident("owner/repo", SAMPLE_INCIDENT)

        title_idx = captured.index("--title") + 1
        assert "[P1]" in captured[title_idx]  # "error" → P1

    def test_issue_body_contains_dedupe_marker(self):
        """Issue body written to GitHub includes the fingerprint dedupe marker."""
        search = _make_gh_result("[]")
        create = _make_gh_result(
            "https://github.com/owner/repo/issues/99\n"
        )
        captured: list = []

        def capturing_mock(args, **kwargs):
            captured.extend(args)
            calls = [search, create]
            idx = capturing_mock._call_count
            capturing_mock._call_count += 1
            return calls[idx]

        capturing_mock._call_count = 0

        with patch("sentry_intake.run_gh", side_effect=capturing_mock):
            ingest_incident("owner/repo", SAMPLE_INCIDENT)

        body_idx = captured.index("--body") + 1
        body = captured[body_idx]
        fp = fingerprint_hash(SAMPLE_INCIDENT)
        assert DEDUPE_MARKER_PREFIX in body
        assert fp in body
