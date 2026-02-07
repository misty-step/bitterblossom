"""Tests for fast-feedback hook."""

import importlib.util
import io
import os
from types import SimpleNamespace

import pytest


_spec = importlib.util.spec_from_file_location(
    "fast_feedback",
    os.path.join(os.path.dirname(__file__), "fast-feedback.py"),
)
fast_feedback = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(fast_feedback)


def test_get_cwd_reads_hook_payload(monkeypatch):
    monkeypatch.setattr(fast_feedback.sys, "stdin", io.StringIO('{"cwd":"/tmp/repo"}'))
    assert fast_feedback.get_cwd() == "/tmp/repo"


def test_get_cwd_falls_back_on_invalid_json(monkeypatch):
    monkeypatch.setattr(fast_feedback.sys, "stdin", io.StringIO("not-json"))
    monkeypatch.setattr(fast_feedback.os, "getcwd", lambda: "/fallback")
    assert fast_feedback.get_cwd() == "/fallback"


def test_detect_project_returns_none_without_markers(tmp_path):
    assert fast_feedback.detect_project(str(tmp_path)) is None


def test_detect_project_prefers_typescript_marker(tmp_path):
    (tmp_path / "tsconfig.json").write_text("{}", encoding="utf-8")
    (tmp_path / "pyproject.toml").write_text("", encoding="utf-8")
    assert fast_feedback.detect_project(str(tmp_path)) == "typescript"


@pytest.mark.parametrize(
    ("marker", "project_type"),
    [
        ("pyproject.toml", "python"),
        ("setup.py", "python"),
        ("Cargo.toml", "rust"),
        ("go.mod", "go"),
    ],
)
def test_detect_project_for_each_supported_marker(tmp_path, marker, project_type):
    (tmp_path / marker).write_text("", encoding="utf-8")
    assert fast_feedback.detect_project(str(tmp_path)) == project_type


@pytest.mark.parametrize(
    ("project_type", "expected_cmd", "expected_timeout"),
    [
        ("typescript", ["npx", "tsc", "--noEmit", "--pretty"], 30),
        ("python", ["ruff", "check", "."], 15),
        ("rust", ["cargo", "check", "--message-format=short"], 60),
        ("go", ["go", "vet", "./..."], 30),
    ],
)
def test_run_check_uses_expected_command(monkeypatch, project_type, expected_cmd, expected_timeout):
    captured = {}

    def fake_run(cmd, capture_output, text, timeout, cwd):
        captured["cmd"] = cmd
        captured["capture_output"] = capture_output
        captured["text"] = text
        captured["timeout"] = timeout
        captured["cwd"] = cwd
        return SimpleNamespace(returncode=0, stdout="", stderr="")

    monkeypatch.setattr(fast_feedback.subprocess, "run", fake_run)

    result = fast_feedback.run_check(project_type, "/repo")
    assert result.returncode == 0
    assert captured["cmd"] == expected_cmd
    assert captured["capture_output"] is True
    assert captured["text"] is True
    assert captured["timeout"] == expected_timeout
    assert captured["cwd"] == "/repo"


def test_run_check_returns_none_for_unknown_project():
    assert fast_feedback.run_check("unknown", "/repo") is None


def test_run_check_returns_none_on_missing_binary(monkeypatch):
    def fake_run(*_args, **_kwargs):
        raise FileNotFoundError

    monkeypatch.setattr(fast_feedback.subprocess, "run", fake_run)
    assert fast_feedback.run_check("python", "/repo") is None


def test_run_check_returns_none_on_timeout(monkeypatch):
    def fake_run(*_args, **_kwargs):
        raise fast_feedback.subprocess.TimeoutExpired(cmd=["go", "vet", "./..."], timeout=30)

    monkeypatch.setattr(fast_feedback.subprocess, "run", fake_run)
    assert fast_feedback.run_check("go", "/repo") is None


def test_main_exits_without_running_checks_when_project_undetected(monkeypatch, capsys):
    monkeypatch.setattr(fast_feedback, "get_cwd", lambda: "/repo")
    monkeypatch.setattr(fast_feedback, "detect_project", lambda _cwd: None)
    monkeypatch.setattr(
        fast_feedback,
        "run_check",
        lambda *_args, **_kwargs: (_ for _ in ()).throw(AssertionError("run_check should not be called")),
    )

    with pytest.raises(SystemExit) as exc:
        fast_feedback.main()

    assert exc.value.code == 0
    assert capsys.readouterr().out == ""


def test_main_prints_issues_when_check_fails(monkeypatch, capsys):
    monkeypatch.setattr(fast_feedback, "get_cwd", lambda: "/repo")
    monkeypatch.setattr(fast_feedback, "detect_project", lambda _cwd: "python")
    monkeypatch.setattr(
        fast_feedback,
        "run_check",
        lambda *_args, **_kwargs: SimpleNamespace(returncode=1, stdout="ruff error\n", stderr=""),
    )

    with pytest.raises(SystemExit) as exc:
        fast_feedback.main()

    assert exc.value.code == 0
    output = capsys.readouterr().out
    assert "[fast-feedback] python issues:" in output
    assert "ruff error" in output


def test_main_stays_silent_on_successful_check(monkeypatch, capsys):
    monkeypatch.setattr(fast_feedback, "get_cwd", lambda: "/repo")
    monkeypatch.setattr(fast_feedback, "detect_project", lambda _cwd: "go")
    monkeypatch.setattr(
        fast_feedback,
        "run_check",
        lambda *_args, **_kwargs: SimpleNamespace(returncode=0, stdout="ignored", stderr="ignored"),
    )

    with pytest.raises(SystemExit) as exc:
        fast_feedback.main()

    assert exc.value.code == 0
    assert capsys.readouterr().out == ""
