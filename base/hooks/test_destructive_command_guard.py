"""Tests for destructive-command-guard hook.

Run: python -m pytest base/hooks/test_destructive_command_guard.py -v
"""
import importlib.util
import os
import pytest

# Import the module from its file path (no package structure)
_spec = importlib.util.spec_from_file_location(
    "guard",
    os.path.join(os.path.dirname(__file__), "destructive-command-guard.py"),
)
guard = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(guard)

check = guard.check_command
split = guard.split_shell_commands


# --- Push protection ---

class TestPushProtection:
    """Block direct pushes to main/master."""

    @pytest.mark.parametrize("cmd", [
        "git push origin main",
        "git push origin master",
        "git push upstream main",
        "git push origin refs/heads/main",
        "git push origin refs/heads/master",
        "git push origin 'main'",
        'git push origin "master"',
        'git push origin "refs/heads/main"',
    ])
    def test_blocks_direct_push(self, cmd):
        blocked, _ = check(cmd)
        assert blocked, f"should block: {cmd}"

    @pytest.mark.parametrize("cmd", [
        "git push origin feature-branch",
        "git push origin main-dev",
        "git push origin fix/main-thing",
        "git push -u origin my-branch",
    ])
    def test_allows_feature_branch_push(self, cmd):
        blocked, _ = check(cmd)
        assert not blocked, f"should allow: {cmd}"

    def test_blocks_plus_refspec_force(self):
        blocked, _ = check("git push origin +main")
        assert blocked

    def test_blocks_plus_refspec_master(self):
        blocked, _ = check("git push origin +master")
        assert blocked

    def test_blocks_plus_refs_heads(self):
        blocked, _ = check("git push origin +refs/heads/main")
        assert blocked

    @pytest.mark.parametrize("cmd", [
        "git push origin master --delete",
        "git push origin main --force",
        "git push origin master --quiet",
    ])
    def test_blocks_trailing_args_after_branch(self, cmd):
        blocked, _ = check(cmd)
        assert blocked, f"should block: {cmd}"


class TestRefspecProtection:
    """Block refspecs targeting main/master."""

    @pytest.mark.parametrize("cmd", [
        "git push origin HEAD:main",
        "git push origin HEAD:master",
        "git push origin feature:main",
        "git push origin HEAD:refs/heads/main",
        "git push origin feature:refs/heads/master",
        "git push origin 'HEAD:main'",
        'git push origin "feature:refs/heads/master"',
    ])
    def test_blocks_refspec_to_protected(self, cmd):
        blocked, _ = check(cmd)
        assert blocked, f"should block: {cmd}"

    @pytest.mark.parametrize("cmd", [
        "git push origin HEAD:feature",
        "git push origin HEAD:refs/for/main",
        "git push origin HEAD:refs/tags/main",
    ])
    def test_allows_refspec_to_other(self, cmd):
        blocked, _ = check(cmd)
        assert not blocked, f"should allow: {cmd}"


# --- Force push protection ---

class TestForcePush:

    @pytest.mark.parametrize("cmd", [
        "git push --force origin feature",
        "git push -f origin feature",
        "git push origin +feature",
        "git push origin +refs/heads/feature",
        "git push origin '+feature'",
    ])
    def test_blocks_force_push(self, cmd):
        blocked, _ = check(cmd)
        assert blocked, f"should block: {cmd}"

    @pytest.mark.parametrize("cmd", [
        "git push --force-with-lease origin feature",
        "git push --force-if-includes origin feature",
    ])
    def test_allows_safe_force(self, cmd):
        blocked, _ = check(cmd)
        assert not blocked, f"should allow: {cmd}"


# --- Rebase protection ---

class TestRebase:

    def test_blocks_rebase(self):
        blocked, _ = check("git rebase main")
        assert blocked

    def test_blocks_filter_branch(self):
        blocked, _ = check("git filter-branch --all")
        assert blocked


# --- Clean protection ---

class TestClean:

    def test_blocks_clean(self):
        blocked, _ = check("git clean -fd")
        assert blocked

    def test_allows_dry_run_separate_flag(self):
        blocked, _ = check("git clean -n -d")
        assert not blocked

    def test_allows_dry_run_long_flag(self):
        blocked, _ = check("git clean --dry-run -d")
        assert not blocked


# --- Destructive commands ---

class TestDestructiveGit:

    @pytest.mark.parametrize("cmd", [
        "git reset --hard HEAD~1",
        "git stash drop",
        "git stash clear",
        "gh pr merge 42",
        "gh repo delete my-repo",
    ])
    def test_blocks_destructive(self, cmd):
        blocked, _ = check(cmd)
        assert blocked, f"should block: {cmd}"


# --- Dangerous flags ---

class TestDangerousFlags:

    def test_blocks_no_verify(self):
        blocked, _ = check("git commit --no-verify -m 'test'")
        assert blocked

    def test_allows_verify_as_substring(self):
        """--no-verify-something should not match --no-verify."""
        blocked, _ = check("echo --no-verify-something")
        assert not blocked


# --- Subshell bypass ---

class TestSubshellExtraction:
    """Subshell contents must be checked, not discarded."""

    def test_blocks_simple_dollar_subshell(self):
        blocked, _ = check("$(git push --force origin master)")
        assert blocked

    def test_blocks_backtick_subshell(self):
        blocked, _ = check("`git push origin master`")
        assert blocked

    def test_blocks_nested_subshell(self):
        blocked, _ = check("echo $(echo $(git push origin master))")
        assert blocked

    def test_blocks_deeply_nested(self):
        blocked, _ = check("echo $(a $(b $(git rebase main)))")
        assert blocked

    def test_blocks_subshell_in_compound(self):
        blocked, _ = check("echo hello && $(git push --force origin feature)")
        assert blocked


# --- Shell grouping bypass ---

class TestShellGrouping:
    """Bare () subshells and { } brace groups must be checked."""

    def test_blocks_bare_parens(self):
        blocked, _ = check("(git push origin main)")
        assert blocked

    def test_blocks_brace_group(self):
        blocked, _ = check("{ git push origin main; }")
        assert blocked

    def test_blocks_brace_group_no_space(self):
        blocked, _ = check("{git rebase main;}")
        assert blocked


# --- Compound commands ---

class TestCompoundCommands:

    def test_blocks_destructive_in_chain(self):
        blocked, _ = check("echo hello && git push origin main")
        assert blocked

    def test_blocks_destructive_after_pipe(self):
        blocked, _ = check("echo test | git rebase main")
        assert blocked

    def test_blocks_destructive_after_semicolon(self):
        blocked, _ = check("echo test; git reset --hard")
        assert blocked

    def test_allows_safe_chain(self):
        blocked, _ = check("git add . && git commit -m 'test'")
        assert not blocked


# --- split_shell_commands ---

class TestSplitShellCommands:

    def test_simple_command(self):
        assert split("git status") == ["git status"]

    def test_chain(self):
        parts = split("echo a && echo b || echo c")
        assert "echo a" in parts
        assert "echo b" in parts
        assert "echo c" in parts

    def test_extracts_subshell(self):
        parts = split("echo $(git push origin main)")
        assert any("git push origin main" in p for p in parts)

    def test_extracts_nested_subshell(self):
        parts = split("echo $(echo $(git push origin main))")
        assert any("git push origin main" in p for p in parts)

    def test_strips_bare_parens(self):
        parts = split("(git status)")
        assert "git status" in parts

    def test_strips_brace_group(self):
        parts = split("{ git status; }")
        assert any("git status" in p for p in parts)
