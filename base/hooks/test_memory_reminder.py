"""Tests for memory-reminder hook."""

import importlib.util
import io
import os

import pytest


_spec = importlib.util.spec_from_file_location(
    "memory_reminder",
    os.path.join(os.path.dirname(__file__), "memory-reminder.py"),
)
memory_reminder = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(memory_reminder)


def run_hook(monkeypatch, payload):
    monkeypatch.setattr(memory_reminder.sys, "stdin", io.StringIO(payload))
    with pytest.raises(SystemExit) as exc:
        memory_reminder.main()
    assert exc.value.code == 0


def test_prints_reminder_for_stop_event(monkeypatch, capsys):
    run_hook(monkeypatch, '{"event":"Stop"}')
    output = capsys.readouterr().out
    assert "[memory-reminder] Session ending." in output
    assert "MEMORY.md" in output


def test_prints_reminder_for_lowercase_stop_event(monkeypatch, capsys):
    run_hook(monkeypatch, '{"event":"stop"}')
    output = capsys.readouterr().out
    assert "Session ending" in output


def test_no_output_for_other_events(monkeypatch, capsys):
    run_hook(monkeypatch, '{"event":"PostToolUse"}')
    assert capsys.readouterr().out == ""


def test_no_output_for_invalid_json(monkeypatch, capsys):
    run_hook(monkeypatch, "not-json")
    assert capsys.readouterr().out == ""
