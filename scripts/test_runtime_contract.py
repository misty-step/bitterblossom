"""Runtime contract verification — guards against model/profile drift.

Canonical sources:
  - base/settings.json — sprite-side Claude profile alias (`model`)
  - scripts/lib.sh     — exact provider/model identifier used by runtime setup

Surfaces validated:
  - base/settings.json
  - scripts/lib.sh
  - Makefile
  - README.md

Run:
  python3 -m pytest -q scripts/test_runtime_contract.py
"""

import json
import os
import pytest
import re
import shutil
import subprocess
from pathlib import Path

REPO_ROOT = Path(__file__).parent.parent
# Keep the literal split so live-surface grep checks ignore this guardrail.
REMOVED_SENTRY_WATCHER = "sentry-" "watcher.sh"
REMOVED_SHELL_ENTRYPOINTS = (
    "scripts/dispatch.sh",
    "scripts/ralph.sh",
    "scripts/sprite-agent.sh",
    "scripts/sprite-bootstrap.sh",
    f"scripts/{REMOVED_SENTRY_WATCHER}",
    "scripts/watchdog.sh",
    "scripts/watchdog-v2.sh",
    "scripts/pr-shepherd.sh",
    "scripts/fleet-status.sh",
    "scripts/refresh-dashboard.sh",
    "scripts/webhook-receiver.sh",
    "scripts/preflight.sh",
    "scripts/health-check.sh",
    "scripts/tail-logs.sh",
    "scripts/ralph-prompt-template.md",
)
LIVE_REFERENCE_SURFACES = (
    REPO_ROOT / "README.md",
    REPO_ROOT / "AGENTS.md",
    REPO_ROOT / "CLAUDE.md",
    REPO_ROOT / "docs" / "CONDUCTOR.md",
    REPO_ROOT / "docs" / "CLI-REFERENCE.md",
    REPO_ROOT / "docs" / "CODEBASE_MAP.md",
    REPO_ROOT / "docs" / "architecture" / "README.md",
    REPO_ROOT / "docs" / "architecture" / "bb-cli.md",
    REPO_ROOT / "docs" / "architecture" / "conductor.md",
    REPO_ROOT / "docs" / "architecture" / "skills.md",
    REPO_ROOT / "docs" / "adr" / "002-architecture-minimalism.md",
)


def _run(cmd: list[str], cwd: Path) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        cmd,
        cwd=cwd,
        text=True,
        capture_output=True,
        check=False,
    )


def _init_temp_repo(tmp_path: Path) -> Path:
    repo = tmp_path / "repo"
    repo.mkdir()

    for cmd in (
        ["git", "init", "-b", "main"],
        ["git", "config", "user.name", "Bitterblossom Tests"],
        ["git", "config", "user.email", "tests@example.com"],
    ):
        result = _run(cmd, repo)
        assert result.returncode == 0, result.stderr

    return repo


def _commit_file(repo: Path, relative_path: str, content: str, message: str) -> None:
    path = repo / relative_path
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content)

    for cmd in (["git", "add", relative_path], ["git", "commit", "-m", message]):
        result = _run(cmd, repo)
        assert result.returncode == 0, result.stderr


def _copy_script(repo: Path, source: Path, destination: str) -> None:
    target = repo / destination
    target.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(source, target)
    target.chmod(0o755)


def _load_settings_profile() -> str:
    """Read the canonical sprite profile alias from base/settings.json."""
    settings_path = REPO_ROOT / "base" / "settings.json"
    data = json.loads(settings_path.read_text())
    return data["model"]


SETTINGS_PROFILE = _load_settings_profile()


def _openrouter_claude_branch(content: str) -> str:
    """Extract only the openrouter-claude branch body from scripts/lib.sh."""
    branch = re.search(
        r'elif provider == "openrouter-claude":(?P<body>.*?)(?:^elif\b|^else:|\Z)',
        content,
        re.DOTALL | re.MULTILINE,
    )
    assert branch, "Could not find openrouter-claude branch in scripts/lib.sh"
    return branch.group("body")


def _load_runtime_model() -> str:
    """Read the exact runtime model identifier from scripts/lib.sh."""
    lib_path = REPO_ROOT / "scripts" / "lib.sh"
    branch = _openrouter_claude_branch(lib_path.read_text())

    match = re.search(r'env\["ANTHROPIC_MODEL"\]\s*=\s*"([^"]+)"', branch)
    assert match, f"Could not find openrouter-claude default model in {lib_path}"
    return match.group(1)


RUNTIME_MODEL = _load_runtime_model()


def test_settings_json_uses_sonnet_profile():
    """Sprite settings should use the stable Claude profile alias."""
    settings_path = REPO_ROOT / "base" / "settings.json"
    data = json.loads(settings_path.read_text())
    assert data["model"] == "sonnet"
    print(f"\n[ok] base/settings.json: model profile = {data['model']!r}")


def test_runtime_model_matches_canonical():
    """scripts/lib.sh should keep the documented exact runtime model."""
    assert RUNTIME_MODEL == "anthropic/claude-sonnet-4-6"
    print(f"[ok] scripts/lib.sh: openrouter-claude default = {RUNTIME_MODEL!r}")


def test_lib_sh_openrouter_claude_default_matches_canonical():
    """scripts/lib.sh openrouter-claude fallback default must equal canonical model."""
    lib_path = REPO_ROOT / "scripts" / "lib.sh"
    branch = _openrouter_claude_branch(lib_path.read_text())
    match = re.search(r'env\["ANTHROPIC_MODEL"\]\s*=\s*"([^"]+)"', branch)
    assert match, "Could not find openrouter-claude default model in scripts/lib.sh"

    lib_default = match.group(1)
    assert lib_default == RUNTIME_MODEL, (
        f"scripts/lib.sh openrouter-claude default={lib_default!r} "
        f"!= runtime contract {RUNTIME_MODEL!r} from scripts/lib.sh"
    )
    print(f"[ok] scripts/lib.sh: openrouter-claude default = {lib_default!r}")


def test_readme_documents_canonical_model():
    """README.md must document the canonical model identifier."""
    readme_path = REPO_ROOT / "README.md"
    content = readme_path.read_text()

    assert RUNTIME_MODEL in content, (
        f"README.md does not reference the canonical model {RUNTIME_MODEL!r}.\n"
        "Update the 'Runtime profile' section to match base/settings.json."
    )
    print(f"[ok] README.md: references {RUNTIME_MODEL!r}")


def test_makefile_test_conductor_bootstraps_dependencies():
    """The supported root test path must bootstrap Elixir deps itself."""
    makefile_path = REPO_ROOT / "Makefile"
    content = makefile_path.read_text()
    assert "ensure-mix:" in content, "Makefile must define an ensure-mix preflight target"
    assert "test-conductor: ensure-mix" in content, (
        "Makefile test-conductor target must depend on ensure-mix before running conductor tests."
    )
    conductor_rule = re.search(
        r"^conductor-check:\s+ensure-mix(?:\s|$)", content, re.MULTILINE
    )
    assert conductor_rule, (
        "Makefile conductor-check target must depend on ensure-mix before running conductor commands."
    )

    ensure_target = re.search(r"^ensure-mix:\n((?:\t.*\n)+)", content, re.MULTILINE)
    assert ensure_target, "Makefile missing ensure-mix target"
    ensure_body = ensure_target.group(1)
    assert "command -v mix" in ensure_body, "ensure-mix must check whether mix exists in PATH"

    test_rule = re.search(r"^test-conductor: ensure-mix\n((?:\t.*\n)+)", content, re.MULTILINE)
    assert test_rule, "Makefile missing test-conductor recipe"
    body = test_rule.group(1)

    assert "mix deps.get && mix test" in body, (
        "Makefile test-conductor target must run 'mix deps.get' before 'mix test' "
        "so 'make test' works from a fresh clone."
    )
    print("[ok] Makefile: test-conductor bootstraps deps before mix test")


def test_readme_documents_supported_repo_verification_command():
    """README.md should document `make test` as the root verification command."""
    readme_path = REPO_ROOT / "README.md"
    content = readme_path.read_text()

    assert "## Repo Verification" in content, "README.md must include a Repo Verification section"
    assert "Use `make test` as the supported repo-level verification command." in content, (
        "README.md must explicitly name 'make test' as the supported repo-level verification command."
    )
    print("[ok] README.md: documents make test as repo-level verification")


def test_make_test_succeeds_from_clean_checkout_state():
    """The supported root verification command must work after clearing conductor build state."""
    if shutil.which("mix") is None:
        pytest.skip("Elixir toolchain (`mix`) is not installed in this environment")

    shutil.rmtree(REPO_ROOT / "conductor" / "deps", ignore_errors=True)
    shutil.rmtree(REPO_ROOT / "conductor" / "_build", ignore_errors=True)

    try:
        result = subprocess.run(
            ["make", "test"],
            cwd=REPO_ROOT,
            text=True,
            capture_output=True,
            check=False,
            timeout=900,
        )
    except subprocess.TimeoutExpired as exc:
        raise AssertionError("'make test' timed out after 900 seconds") from exc

    assert result.returncode == 0, (
        "'make test' must succeed after removing conductor/deps and conductor/_build.\n"
        f"stdout:\n{result.stdout}\n"
        f"stderr:\n{result.stderr}"
    )
    print("[ok] make test: succeeds from a clean conductor checkout state")


def test_verdict_validate_requires_ship_by_default(tmp_path: Path):
    """verdict_validate should reject non-ship verdicts unless explicitly allowed."""
    if shutil.which("git") is None:
        pytest.skip("git is not installed in this environment")

    repo = _init_temp_repo(tmp_path)
    _commit_file(repo, "README.md", "main\n", "chore: seed repo")

    result = _run(["git", "checkout", "-b", "feature"], repo)
    assert result.returncode == 0, result.stderr
    _commit_file(repo, "README.md", "feature\n", "feat: update readme")

    verdicts = REPO_ROOT / "scripts" / "lib" / "verdicts.sh"
    head_sha = _run(["git", "rev-parse", "feature"], repo).stdout.strip()

    dont_ship = json.dumps(
        {
            "branch": "feature",
            "base": "main",
            "verdict": "dont-ship",
            "reviewers": ["gemini"],
            "scores": {"correctness": 2},
            "sha": head_sha,
            "date": "2026-04-08T23:59:00Z",
        }
    )

    result = _run(
        [
            "bash",
            "-lc",
            f"source {verdicts} && verdict_write feature '{dont_ship}' >/dev/null && verdict_validate feature",
        ],
        repo,
    )
    assert result.returncode != 0
    assert "verdict dont-ship is not allowed" in (result.stdout + result.stderr)

    ship = json.dumps(
        {
            "branch": "feature",
            "base": "main",
            "verdict": "ship",
            "reviewers": ["gemini"],
            "scores": {"correctness": 9},
            "sha": head_sha,
            "date": "2026-04-08T23:59:10Z",
        }
    )

    result = _run(
        [
            "bash",
            "-lc",
            f"source {verdicts} && verdict_write feature '{ship}' >/dev/null && verdict_validate feature",
        ],
        repo,
    )
    assert result.returncode == 0, result.stdout + result.stderr


def test_land_script_blocks_non_ship_verdicts_before_merge(tmp_path: Path):
    """scripts/land.sh must refuse to land branches with non-ship verdicts."""
    if shutil.which("git") is None:
        pytest.skip("git is not installed in this environment")

    repo = _init_temp_repo(tmp_path)
    _copy_script(repo, REPO_ROOT / "scripts" / "land.sh", "scripts/land.sh")
    _copy_script(repo, REPO_ROOT / "scripts" / "lib" / "verdicts.sh", "scripts/lib/verdicts.sh")

    dagger_stub = repo / "scripts" / "ci" / "dagger-call.sh"
    dagger_stub.parent.mkdir(parents=True, exist_ok=True)
    dagger_stub.write_text("#!/usr/bin/env bash\nexit 0\n")
    dagger_stub.chmod(0o755)

    _commit_file(repo, "README.md", "main\n", "chore: seed repo")
    _commit_file(repo, "scripts/land.sh", (repo / "scripts" / "land.sh").read_text(), "chore: add land script")
    _commit_file(
        repo,
        "scripts/lib/verdicts.sh",
        (repo / "scripts" / "lib" / "verdicts.sh").read_text(),
        "chore: add verdict helpers",
    )
    _commit_file(
        repo,
        "scripts/ci/dagger-call.sh",
        dagger_stub.read_text(),
        "chore: add dagger stub",
    )

    result = _run(["git", "checkout", "-b", "feature"], repo)
    assert result.returncode == 0, result.stderr
    _commit_file(repo, "app.txt", "feature\n", "feat: change app")

    verdicts = repo / "scripts" / "lib" / "verdicts.sh"
    head_sha = _run(["git", "rev-parse", "feature"], repo).stdout.strip()
    payload = json.dumps(
        {
            "branch": "feature",
            "base": "main",
            "verdict": "dont-ship",
            "reviewers": ["gemini"],
            "scores": {"correctness": 2},
            "sha": head_sha,
            "date": "2026-04-08T23:59:20Z",
        }
    )

    result = _run(
        [
            "bash",
            "-lc",
            f"source {verdicts} && verdict_write feature '{payload}' >/dev/null && bash scripts/land.sh feature",
        ],
        repo,
    )
    assert result.returncode != 0
    assert "verdict dont-ship is not allowed" in (result.stdout + result.stderr)


def test_verdict_validate_rejects_symbolic_revisions(tmp_path: Path):
    """verdict validation must anchor to a local branch ref, not a moving symbolic rev."""
    if shutil.which("git") is None:
        pytest.skip("git is not installed in this environment")

    repo = _init_temp_repo(tmp_path)
    _commit_file(repo, "README.md", "main\n", "chore: seed repo")

    result = _run(["git", "checkout", "-b", "feature"], repo)
    assert result.returncode == 0, result.stderr
    _commit_file(repo, "README.md", "feature\n", "feat: update readme")

    verdicts = REPO_ROOT / "scripts" / "lib" / "verdicts.sh"
    head_sha = _run(["git", "rev-parse", "feature"], repo).stdout.strip()
    payload = json.dumps(
        {
            "branch": "HEAD",
            "base": "main",
            "verdict": "ship",
            "reviewers": ["internal-bench"],
            "scores": {"correctness": 9},
            "sha": head_sha,
            "date": "2026-04-09T00:00:30Z",
        }
    )

    result = _run(
        [
            "bash",
            "-lc",
            f"source {verdicts} && verdict_write HEAD '{payload}' >/dev/null && verdict_validate HEAD",
        ],
        repo,
    )
    assert result.returncode != 0
    assert "unknown local branch: HEAD" in (result.stdout + result.stderr)


def test_land_script_verifies_the_merge_candidate(tmp_path: Path):
    """scripts/land.sh must verify the squashed merge result, not just the feature tip."""
    if shutil.which("git") is None:
        pytest.skip("git is not installed in this environment")

    repo = _init_temp_repo(tmp_path)
    _copy_script(repo, REPO_ROOT / "scripts" / "land.sh", "scripts/land.sh")
    _copy_script(repo, REPO_ROOT / "scripts" / "lib" / "verdicts.sh", "scripts/lib/verdicts.sh")

    dagger_stub = repo / "scripts" / "ci" / "dagger-call.sh"
    dagger_stub.parent.mkdir(parents=True, exist_ok=True)
    dagger_stub.write_text(
        """#!/usr/bin/env bash
set -euo pipefail
common_dir="$(git rev-parse --git-common-dir)"
if [[ "$(cat README.md)" != "main" ]]; then
  echo "expected verification to run from the default-branch base" >&2
  exit 1
fi
if [[ "$(cat app.txt)" != "feature" ]]; then
  echo "expected squashed feature content in verification worktree" >&2
  exit 1
fi
if [[ "$(cat base.txt)" != "main-base" ]]; then
  echo "expected latest default-branch content in verification worktree" >&2
  exit 1
fi
printf 'ok\n' > "$common_dir/verified-merge.txt"
"""
    )
    dagger_stub.chmod(0o755)

    _commit_file(repo, "README.md", "main\n", "chore: seed repo")
    _commit_file(repo, "scripts/land.sh", (repo / "scripts" / "land.sh").read_text(), "chore: add land script")
    _commit_file(
        repo,
        "scripts/lib/verdicts.sh",
        (repo / "scripts" / "lib" / "verdicts.sh").read_text(),
        "chore: add verdict helpers",
    )
    _commit_file(
        repo,
        "scripts/ci/dagger-call.sh",
        dagger_stub.read_text(),
        "chore: add dagger stub",
    )

    result = _run(["git", "checkout", "-b", "feature"], repo)
    assert result.returncode == 0, result.stderr
    _commit_file(repo, "app.txt", "feature\n", "feat: change app")
    result = _run(["git", "checkout", "main"], repo)
    assert result.returncode == 0, result.stderr
    _commit_file(repo, "base.txt", "main-base\n", "chore: advance base")
    result = _run(["git", "checkout", "feature"], repo)
    assert result.returncode == 0, result.stderr

    head_sha = _run(["git", "rev-parse", "feature"], repo).stdout.strip()
    payload = json.dumps(
        {
            "branch": "feature",
            "base": "main",
            "verdict": "ship",
            "reviewers": ["internal-bench"],
            "scores": {"correctness": 9},
            "sha": head_sha,
            "date": "2026-04-08T23:59:30Z",
        }
    )

    verdicts = repo / "scripts" / "lib" / "verdicts.sh"
    result = _run(["git", "checkout", "main"], repo)
    assert result.returncode == 0, result.stderr

    result = _run(
        [
            "bash",
            "-lc",
            f"source {verdicts} && verdict_write feature '{payload}' >/dev/null && bash scripts/land.sh feature",
        ],
        repo,
    )
    assert result.returncode == 0, result.stdout + result.stderr
    assert (repo / ".git" / "verified-merge.txt").read_text().strip() == "ok"
    assert _run(["git", "show", "main:app.txt"], repo).stdout.strip() == "feature"


def test_dagger_wrapper_declares_trusted_ci_override():
    """The Dagger wrapper must gate privileged CI usage behind an explicit override."""
    wrapper = (REPO_ROOT / "scripts" / "ci" / "dagger-call.sh").read_text()
    assert "BB_ALLOW_PRIVILEGED_DAGGER_IN_CI" in wrapper


def test_dagger_wrapper_includes_untracked_files_and_engine_config(tmp_path: Path):
    """The wrapper must pass untracked files and the repo engine config into dagger."""
    if shutil.which("git") is None:
        pytest.skip("git is not installed in this environment")

    repo = _init_temp_repo(tmp_path)
    _copy_script(repo, REPO_ROOT / "scripts" / "ci" / "dagger-call.sh", "scripts/ci/dagger-call.sh")
    (repo / "dagger").mkdir(parents=True, exist_ok=True)
    shutil.copy2(REPO_ROOT / "dagger" / "engine.json", repo / "dagger" / "engine.json")
    _commit_file(repo, "tracked.txt", "tracked\n", "chore: seed repo")

    (repo / "untracked.txt").write_text("untracked\n")

    bin_dir = repo / "bin"
    bin_dir.mkdir()

    (bin_dir / "docker").write_text("#!/usr/bin/env bash\nexit 0\n")
    (bin_dir / "rsync").write_text(
        """#!/usr/bin/env python3
import pathlib
import shutil
import sys

source_root = pathlib.Path(sys.argv[-2]).resolve()
dest_root = pathlib.Path(sys.argv[-1]).resolve()

data = sys.stdin.buffer.read().split(b"\\0")
for raw in data:
    if not raw:
        continue
    relative = raw.decode()
    source = source_root / relative
    destination = dest_root / relative
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(source, destination)
"""
    )
    (bin_dir / "dagger").write_text(
        """#!/usr/bin/env bash
set -euo pipefail
if [[ ! -f "$PWD/untracked.txt" ]]; then
  echo "missing untracked file in snapshot" >&2
  exit 1
fi
if [[ ! -f "$XDG_CONFIG_HOME/dagger/engine.json" ]]; then
  echo "missing engine config" >&2
  exit 1
fi
"""
    )

    for executable in ("docker", "rsync", "dagger"):
        (bin_dir / executable).chmod(0o755)

    env = dict(os.environ)
    env["PATH"] = f"{bin_dir}:{env['PATH']}"

    result = subprocess.run(
        ["bash", "scripts/ci/dagger-call.sh", "check"],
        cwd=repo,
        text=True,
        capture_output=True,
        check=False,
        env=env,
    )
    assert result.returncode == 0, result.stdout + result.stderr


def test_dagger_wrapper_falls_back_to_reachable_docker_context(tmp_path: Path):
    """The wrapper should switch contexts only when the operator opts into a fallback."""
    if shutil.which("git") is None:
        pytest.skip("git is not installed in this environment")

    repo = _init_temp_repo(tmp_path)
    _copy_script(repo, REPO_ROOT / "scripts" / "ci" / "dagger-call.sh", "scripts/ci/dagger-call.sh")
    (repo / "dagger").mkdir(parents=True, exist_ok=True)
    shutil.copy2(REPO_ROOT / "dagger" / "engine.json", repo / "dagger" / "engine.json")
    _commit_file(repo, "tracked.txt", "tracked\n", "chore: seed repo")

    bin_dir = repo / "bin"
    bin_dir.mkdir()

    (bin_dir / "docker").write_text(
        """#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "context" && "${2:-}" == "show" ]]; then
  printf 'desktop-linux\\n'
  exit 0
fi
if [[ "${1:-}" == "context" && "${2:-}" == "ls" ]]; then
  printf 'desktop-linux\\ncolima\\n'
  exit 0
fi
if [[ "${1:-}" == "version" ]]; then
  if [[ "${DOCKER_CONTEXT:-desktop-linux}" == "colima" ]]; then
    exit 0
  fi
  echo "dead context" >&2
  exit 1
fi
exit 0
"""
    )
    (bin_dir / "rsync").write_text(
        """#!/usr/bin/env python3
import pathlib
import shutil
import sys

source_root = pathlib.Path(sys.argv[-2]).resolve()
dest_root = pathlib.Path(sys.argv[-1]).resolve()

data = sys.stdin.buffer.read().split(b"\\0")
for raw in data:
    if not raw:
        continue
    relative = raw.decode()
    source = source_root / relative
    destination = dest_root / relative
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(source, destination)
"""
    )
    (bin_dir / "dagger").write_text(
        """#!/usr/bin/env bash
set -euo pipefail
if [[ "${DOCKER_CONTEXT:-}" != "colima" ]]; then
  echo "expected colima fallback, got ${DOCKER_CONTEXT:-<unset>}" >&2
  exit 1
fi
"""
    )

    for executable in ("docker", "rsync", "dagger"):
        (bin_dir / executable).chmod(0o755)

    git_dir = str(Path(shutil.which("git")).parent)
    env = {"PATH": f"{bin_dir}:{git_dir}:/usr/bin:/bin"}
    env.pop("DOCKER_CONTEXT", None)
    env.pop("DOCKER_HOST", None)
    env["BB_DOCKER_CONTEXT_FALLBACK"] = "colima"

    result = subprocess.run(
        ["bash", "scripts/ci/dagger-call.sh", "check"],
        cwd=repo,
        text=True,
        capture_output=True,
        check=False,
        env=env,
    )
    assert result.returncode == 0, result.stdout + result.stderr
    assert "using explicit fallback colima" in (result.stdout + result.stderr)


def test_dagger_wrapper_fails_closed_without_explicit_docker_context_fallback(tmp_path: Path):
    """The wrapper must not silently switch Docker contexts when the current one is down."""
    if shutil.which("git") is None:
        pytest.skip("git is not installed in this environment")

    repo = _init_temp_repo(tmp_path)
    _copy_script(repo, REPO_ROOT / "scripts" / "ci" / "dagger-call.sh", "scripts/ci/dagger-call.sh")
    (repo / "dagger").mkdir(parents=True, exist_ok=True)
    shutil.copy2(REPO_ROOT / "dagger" / "engine.json", repo / "dagger" / "engine.json")
    _commit_file(repo, "tracked.txt", "tracked\n", "chore: seed repo")

    bin_dir = repo / "bin"
    bin_dir.mkdir()

    (bin_dir / "docker").write_text(
        """#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "context" && "${2:-}" == "show" ]]; then
  printf 'desktop-linux\\n'
  exit 0
fi
if [[ "${1:-}" == "context" && "${2:-}" == "ls" ]]; then
  printf 'desktop-linux\\ncolima\\n'
  exit 0
fi
if [[ "${1:-}" == "version" ]]; then
  if [[ "${DOCKER_CONTEXT:-desktop-linux}" == "colima" ]]; then
    exit 0
  fi
  echo "dead context" >&2
  exit 1
fi
exit 0
"""
    )
    (bin_dir / "rsync").write_text(
        """#!/usr/bin/env python3
import pathlib
import shutil
import sys

source_root = pathlib.Path(sys.argv[-2]).resolve()
dest_root = pathlib.Path(sys.argv[-1]).resolve()

data = sys.stdin.buffer.read().split(b"\\0")
for raw in data:
    if not raw:
        continue
    relative = raw.decode()
    source = source_root / relative
    destination = dest_root / relative
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(source, destination)
"""
    )
    (bin_dir / "dagger").write_text("#!/usr/bin/env bash\nexit 0\n")

    for executable in ("docker", "rsync", "dagger"):
        (bin_dir / executable).chmod(0o755)

    env = dict(os.environ)
    env["PATH"] = f"{bin_dir}:{env['PATH']}"
    env.pop("DOCKER_CONTEXT", None)
    env.pop("DOCKER_HOST", None)
    env.pop("BB_DOCKER_CONTEXT_FALLBACK", None)

    result = subprocess.run(
        ["bash", "scripts/ci/dagger-call.sh", "check"],
        cwd=repo,
        text=True,
        capture_output=True,
        check=False,
        env=env,
    )
    assert result.returncode != 0
    assert "set BB_DOCKER_CONTEXT_FALLBACK=<context>" in (result.stdout + result.stderr)


def test_dagger_wrapper_uses_colima_shim_when_docker_cli_is_missing(tmp_path: Path):
    """The wrapper should synthesize a docker CLI via Colima when PATH has no docker binary."""
    if shutil.which("git") is None:
        pytest.skip("git is not installed in this environment")

    repo = _init_temp_repo(tmp_path)
    _copy_script(repo, REPO_ROOT / "scripts" / "ci" / "dagger-call.sh", "scripts/ci/dagger-call.sh")
    (repo / "dagger").mkdir(parents=True, exist_ok=True)
    shutil.copy2(REPO_ROOT / "dagger" / "engine.json", repo / "dagger" / "engine.json")
    _commit_file(repo, "tracked.txt", "tracked\n", "chore: seed repo")

    bin_dir = repo / "bin"
    bin_dir.mkdir()

    (bin_dir / "colima").write_text(
        """#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "status" ]]; then
  exit 0
fi
if [[ "${1:-}" == "ssh" && "${2:-}" == "--" && "${3:-}" == "docker" ]]; then
  exit 0
fi
exit 1
"""
    )
    (bin_dir / "rsync").write_text(
        """#!/usr/bin/env python3
import pathlib
import shutil
import sys

source_root = pathlib.Path(sys.argv[-2]).resolve()
dest_root = pathlib.Path(sys.argv[-1]).resolve()

data = sys.stdin.buffer.read().split(b"\\0")
for raw in data:
    if not raw:
        continue
    relative = raw.decode()
    source = source_root / relative
    destination = dest_root / relative
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(source, destination)
"""
    )
    (bin_dir / "dagger").write_text("#!/usr/bin/env bash\nexit 0\n")

    for executable in ("colima", "rsync", "dagger"):
        (bin_dir / executable).chmod(0o755)

    git_dir = str(Path(shutil.which("git")).parent)
    env = {"PATH": f"{bin_dir}:{git_dir}:/usr/bin:/bin"}
    env.pop("DOCKER_CONTEXT", None)
    env.pop("DOCKER_HOST", None)

    result = subprocess.run(
        ["bash", "scripts/ci/dagger-call.sh", "check"],
        cwd=repo,
        text=True,
        capture_output=True,
        check=False,
        env=env,
    )
    assert result.returncode == 0, result.stdout + result.stderr
    assert "using Colima docker shim" in (result.stdout + result.stderr)


def test_land_script_rejects_symbolic_revisions_as_branch_arguments(tmp_path: Path):
    """scripts/land.sh must only accept real local branch names for landing."""
    if shutil.which("git") is None:
        pytest.skip("git is not installed in this environment")

    repo = _init_temp_repo(tmp_path)
    _copy_script(repo, REPO_ROOT / "scripts" / "land.sh", "scripts/land.sh")
    _copy_script(repo, REPO_ROOT / "scripts" / "lib" / "verdicts.sh", "scripts/lib/verdicts.sh")

    dagger_stub = repo / "scripts" / "ci" / "dagger-call.sh"
    dagger_stub.parent.mkdir(parents=True, exist_ok=True)
    dagger_stub.write_text("#!/usr/bin/env bash\nexit 0\n")
    dagger_stub.chmod(0o755)

    _commit_file(repo, "README.md", "main\n", "chore: seed repo")
    _commit_file(repo, "scripts/land.sh", (repo / "scripts" / "land.sh").read_text(), "chore: add land script")
    _commit_file(
        repo,
        "scripts/lib/verdicts.sh",
        (repo / "scripts" / "lib" / "verdicts.sh").read_text(),
        "chore: add verdict helpers",
    )
    _commit_file(
        repo,
        "scripts/ci/dagger-call.sh",
        dagger_stub.read_text(),
        "chore: add dagger stub",
    )

    result = _run(["git", "checkout", "-b", "feature"], repo)
    assert result.returncode == 0, result.stderr
    _commit_file(repo, "app.txt", "feature\n", "feat: change app")

    head_sha = _run(["git", "rev-parse", "feature"], repo).stdout.strip()
    payload = json.dumps(
        {
            "branch": "feature",
            "base": "main",
            "verdict": "ship",
            "reviewers": ["internal-bench"],
            "scores": {"correctness": 9},
            "sha": head_sha,
            "date": "2026-04-09T00:00:40Z",
        }
    )

    verdicts = repo / "scripts" / "lib" / "verdicts.sh"
    result = _run(
        [
            "bash",
            "-lc",
            f"source {verdicts} && verdict_write feature '{payload}' >/dev/null && bash scripts/land.sh HEAD",
        ],
        repo,
    )
    assert result.returncode != 0
    assert "unknown local branch: HEAD" in (result.stdout + result.stderr)


def test_land_script_prefers_origin_head_over_stale_local_branch_names(tmp_path: Path):
    """scripts/land.sh must land into the branch named by origin/HEAD when available."""
    if shutil.which("git") is None:
        pytest.skip("git is not installed in this environment")

    repo = _init_temp_repo(tmp_path)
    _copy_script(repo, REPO_ROOT / "scripts" / "land.sh", "scripts/land.sh")
    _copy_script(repo, REPO_ROOT / "scripts" / "lib" / "verdicts.sh", "scripts/lib/verdicts.sh")

    dagger_stub = repo / "scripts" / "ci" / "dagger-call.sh"
    dagger_stub.parent.mkdir(parents=True, exist_ok=True)
    dagger_stub.write_text("#!/usr/bin/env bash\nexit 0\n")
    dagger_stub.chmod(0o755)

    _commit_file(repo, "README.md", "main\n", "chore: seed repo")
    _commit_file(repo, "scripts/land.sh", (repo / "scripts" / "land.sh").read_text(), "chore: add land script")
    _commit_file(
        repo,
        "scripts/lib/verdicts.sh",
        (repo / "scripts" / "lib" / "verdicts.sh").read_text(),
        "chore: add verdict helpers",
    )
    _commit_file(
        repo,
        "scripts/ci/dagger-call.sh",
        dagger_stub.read_text(),
        "chore: add dagger stub",
    )

    result = _run(["git", "branch", "master"], repo)
    assert result.returncode == 0, result.stderr
    result = _run(["git", "update-ref", "refs/remotes/origin/main", "main"], repo)
    assert result.returncode == 0, result.stderr
    result = _run(["git", "update-ref", "refs/remotes/origin/master", "master"], repo)
    assert result.returncode == 0, result.stderr
    result = _run(["git", "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main"], repo)
    assert result.returncode == 0, result.stderr

    result = _run(["git", "checkout", "-b", "feature"], repo)
    assert result.returncode == 0, result.stderr
    _commit_file(repo, "app.txt", "feature\n", "feat: change app")

    head_sha = _run(["git", "rev-parse", "feature"], repo).stdout.strip()
    payload = json.dumps(
        {
            "branch": "feature",
            "base": "main",
            "verdict": "ship",
            "reviewers": ["internal-bench"],
            "scores": {"correctness": 9},
            "sha": head_sha,
            "date": "2026-04-09T00:00:00Z",
        }
    )

    verdicts = repo / "scripts" / "lib" / "verdicts.sh"
    result = _run(["git", "checkout", "main"], repo)
    assert result.returncode == 0, result.stderr

    result = _run(
        [
            "bash",
            "-lc",
            f"source {verdicts} && verdict_write feature '{payload}' >/dev/null && bash scripts/land.sh feature",
        ],
        repo,
    )
    assert result.returncode == 0, result.stdout + result.stderr
    assert _run(["git", "show", "main:app.txt"], repo).stdout.strip() == "feature"
    assert _run(["git", "show", "master:app.txt"], repo).returncode != 0


def test_land_script_prefers_main_over_master_without_origin_head(tmp_path: Path):
    """scripts/land.sh must prefer local main over master when origin/HEAD is absent."""
    if shutil.which("git") is None:
        pytest.skip("git is not installed in this environment")

    repo = _init_temp_repo(tmp_path)
    _copy_script(repo, REPO_ROOT / "scripts" / "land.sh", "scripts/land.sh")
    _copy_script(repo, REPO_ROOT / "scripts" / "lib" / "verdicts.sh", "scripts/lib/verdicts.sh")

    dagger_stub = repo / "scripts" / "ci" / "dagger-call.sh"
    dagger_stub.parent.mkdir(parents=True, exist_ok=True)
    dagger_stub.write_text("#!/usr/bin/env bash\nexit 0\n")
    dagger_stub.chmod(0o755)

    _commit_file(repo, "README.md", "main\n", "chore: seed repo")
    _commit_file(repo, "scripts/land.sh", (repo / "scripts" / "land.sh").read_text(), "chore: add land script")
    _commit_file(
        repo,
        "scripts/lib/verdicts.sh",
        (repo / "scripts" / "lib" / "verdicts.sh").read_text(),
        "chore: add verdict helpers",
    )
    _commit_file(
        repo,
        "scripts/ci/dagger-call.sh",
        dagger_stub.read_text(),
        "chore: add dagger stub",
    )

    result = _run(["git", "branch", "master"], repo)
    assert result.returncode == 0, result.stderr

    result = _run(["git", "checkout", "-b", "feature"], repo)
    assert result.returncode == 0, result.stderr
    _commit_file(repo, "app.txt", "feature\n", "feat: change app")

    head_sha = _run(["git", "rev-parse", "feature"], repo).stdout.strip()
    payload = json.dumps(
        {
            "branch": "feature",
            "base": "main",
            "verdict": "ship",
            "reviewers": ["internal-bench"],
            "scores": {"correctness": 9},
            "sha": head_sha,
            "date": "2026-04-09T00:01:00Z",
        }
    )

    verdicts = repo / "scripts" / "lib" / "verdicts.sh"
    result = _run(["git", "checkout", "main"], repo)
    assert result.returncode == 0, result.stderr

    result = _run(
        [
            "bash",
            "-lc",
            f"source {verdicts} && verdict_write feature '{payload}' >/dev/null && bash scripts/land.sh feature",
        ],
        repo,
    )
    assert result.returncode == 0, result.stdout + result.stderr
    assert _run(["git", "show", "main:app.txt"], repo).stdout.strip() == "feature"
    assert _run(["git", "show", "master:app.txt"], repo).returncode != 0


def test_land_script_requires_sync_origin_before_fetching_origin(tmp_path: Path):
    """scripts/land.sh should not require a reachable origin in default local-first mode."""
    if shutil.which("git") is None:
        pytest.skip("git is not installed in this environment")

    repo = _init_temp_repo(tmp_path)
    _copy_script(repo, REPO_ROOT / "scripts" / "land.sh", "scripts/land.sh")
    _copy_script(repo, REPO_ROOT / "scripts" / "lib" / "verdicts.sh", "scripts/lib/verdicts.sh")

    dagger_stub = repo / "scripts" / "ci" / "dagger-call.sh"
    dagger_stub.parent.mkdir(parents=True, exist_ok=True)
    dagger_stub.write_text("#!/usr/bin/env bash\nexit 0\n")
    dagger_stub.chmod(0o755)

    _commit_file(repo, "README.md", "main\n", "chore: seed repo")
    _commit_file(repo, "scripts/land.sh", (repo / "scripts" / "land.sh").read_text(), "chore: add land script")
    _commit_file(
        repo,
        "scripts/lib/verdicts.sh",
        (repo / "scripts" / "lib" / "verdicts.sh").read_text(),
        "chore: add verdict helpers",
    )
    _commit_file(
        repo,
        "scripts/ci/dagger-call.sh",
        dagger_stub.read_text(),
        "chore: add dagger stub",
    )

    result = _run(["git", "remote", "add", "origin", "https://git.example.com/test/repo.git"], repo)
    assert result.returncode == 0, result.stderr

    result = _run(["git", "checkout", "-b", "feature"], repo)
    assert result.returncode == 0, result.stderr
    _commit_file(repo, "app.txt", "feature\n", "feat: change app")

    head_sha = _run(["git", "rev-parse", "feature"], repo).stdout.strip()
    payload = json.dumps(
        {
            "branch": "feature",
            "base": "main",
            "verdict": "ship",
            "reviewers": ["internal-bench"],
            "scores": {"correctness": 9},
            "sha": head_sha,
            "date": "2026-04-09T00:01:10Z",
        }
    )

    verdicts = repo / "scripts" / "lib" / "verdicts.sh"
    result = _run(["git", "checkout", "main"], repo)
    assert result.returncode == 0, result.stderr

    result = _run(
        [
            "bash",
            "-lc",
            f"source {verdicts} && verdict_write feature '{payload}' >/dev/null && bash scripts/land.sh feature",
        ],
        repo,
    )
    assert result.returncode == 0, result.stdout + result.stderr


def test_land_script_sync_origin_fails_closed_on_fetch_errors(tmp_path: Path):
    """scripts/land.sh --sync-origin must fail closed when origin fetch fails."""
    if shutil.which("git") is None:
        pytest.skip("git is not installed in this environment")

    repo = _init_temp_repo(tmp_path)
    _copy_script(repo, REPO_ROOT / "scripts" / "land.sh", "scripts/land.sh")
    _copy_script(repo, REPO_ROOT / "scripts" / "lib" / "verdicts.sh", "scripts/lib/verdicts.sh")

    dagger_stub = repo / "scripts" / "ci" / "dagger-call.sh"
    dagger_stub.parent.mkdir(parents=True, exist_ok=True)
    dagger_stub.write_text("#!/usr/bin/env bash\nexit 0\n")
    dagger_stub.chmod(0o755)

    _commit_file(repo, "README.md", "main\n", "chore: seed repo")
    _commit_file(repo, "scripts/land.sh", (repo / "scripts" / "land.sh").read_text(), "chore: add land script")
    _commit_file(
        repo,
        "scripts/lib/verdicts.sh",
        (repo / "scripts" / "lib" / "verdicts.sh").read_text(),
        "chore: add verdict helpers",
    )
    _commit_file(
        repo,
        "scripts/ci/dagger-call.sh",
        dagger_stub.read_text(),
        "chore: add dagger stub",
    )

    result = _run(["git", "remote", "add", "origin", "https://git.example.com/test/repo.git"], repo)
    assert result.returncode == 0, result.stderr

    result = _run(["git", "checkout", "-b", "feature"], repo)
    assert result.returncode == 0, result.stderr
    _commit_file(repo, "app.txt", "feature\n", "feat: change app")

    head_sha = _run(["git", "rev-parse", "feature"], repo).stdout.strip()
    payload = json.dumps(
        {
            "branch": "feature",
            "base": "main",
            "verdict": "ship",
            "reviewers": ["internal-bench"],
            "scores": {"correctness": 9},
            "sha": head_sha,
            "date": "2026-04-09T00:01:20Z",
        }
    )

    verdicts = repo / "scripts" / "lib" / "verdicts.sh"
    result = _run(["git", "checkout", "main"], repo)
    assert result.returncode == 0, result.stderr

    result = _run(
        [
            "bash",
            "-lc",
            f"source {verdicts} && verdict_write feature '{payload}' >/dev/null && bash scripts/land.sh feature --sync-origin",
        ],
        repo,
    )
    assert result.returncode != 0
    assert "failed to fetch origin before landing" in (result.stdout + result.stderr)
    assert _run(["git", "show", "main:app.txt"], repo).returncode != 0


def test_land_script_publish_fails_before_mutating_without_origin(tmp_path: Path):
    """scripts/land.sh --publish must fail before the squash commit when no origin exists."""
    if shutil.which("git") is None:
        pytest.skip("git is not installed in this environment")

    repo = _init_temp_repo(tmp_path)
    _copy_script(repo, REPO_ROOT / "scripts" / "land.sh", "scripts/land.sh")
    _copy_script(repo, REPO_ROOT / "scripts" / "lib" / "verdicts.sh", "scripts/lib/verdicts.sh")

    dagger_stub = repo / "scripts" / "ci" / "dagger-call.sh"
    dagger_stub.parent.mkdir(parents=True, exist_ok=True)
    dagger_stub.write_text("#!/usr/bin/env bash\nexit 0\n")
    dagger_stub.chmod(0o755)

    _commit_file(repo, "README.md", "main\n", "chore: seed repo")
    _commit_file(repo, "scripts/land.sh", (repo / "scripts" / "land.sh").read_text(), "chore: add land script")
    _commit_file(
        repo,
        "scripts/lib/verdicts.sh",
        (repo / "scripts" / "lib" / "verdicts.sh").read_text(),
        "chore: add verdict helpers",
    )
    _commit_file(
        repo,
        "scripts/ci/dagger-call.sh",
        dagger_stub.read_text(),
        "chore: add dagger stub",
    )

    result = _run(["git", "checkout", "-b", "feature"], repo)
    assert result.returncode == 0, result.stderr
    _commit_file(repo, "app.txt", "feature\n", "feat: change app")

    head_sha = _run(["git", "rev-parse", "feature"], repo).stdout.strip()
    payload = json.dumps(
        {
            "branch": "feature",
            "base": "main",
            "verdict": "ship",
            "reviewers": ["internal-bench"],
            "scores": {"correctness": 9},
            "sha": head_sha,
            "date": "2026-04-09T00:01:30Z",
        }
    )

    verdicts = repo / "scripts" / "lib" / "verdicts.sh"
    result = _run(["git", "checkout", "main"], repo)
    assert result.returncode == 0, result.stderr
    before = _run(["git", "rev-parse", "main"], repo).stdout.strip()

    result = _run(
        [
            "bash",
            "-lc",
            f"source {verdicts} && verdict_write feature '{payload}' >/dev/null && bash scripts/land.sh feature --publish",
        ],
        repo,
    )
    assert result.returncode != 0
    assert "--publish requires an origin remote" in (result.stdout + result.stderr)
    assert _run(["git", "rev-parse", "main"], repo).stdout.strip() == before


def test_canonical_source_is_base_settings_json():
    """Smoke test: base/settings.json must exist and define the sprite profile alias."""
    settings_path = REPO_ROOT / "base" / "settings.json"
    assert settings_path.exists(), "base/settings.json not found — canonical source is missing"

    data = json.loads(settings_path.read_text())
    assert "model" in data, "base/settings.json missing top-level 'model' key"
    model = data["model"]
    assert model, "base/settings.json model is empty"
    assert model == SETTINGS_PROFILE
    print(f"\n[canonical] base/settings.json model profile = {model!r}")
    print(
        "Validated surfaces:"
        "\n  base/settings.json     (profile alias)"
        "\n  scripts/lib.sh         (exact model)"
        "\n  README.md"
    )


def test_removed_shell_entrypoints_and_symlink_stay_deleted():
    """Dead shell entrypoints should not reappear on the supported scripts surface."""
    for relative_path in REMOVED_SHELL_ENTRYPOINTS:
        assert not (REPO_ROOT / relative_path).exists(), f"{relative_path} should stay deleted"


def test_land_script_is_checked_in_executable():
    """The documented landing entrypoint must be directly executable."""
    land_path = REPO_ROOT / "scripts" / "land.sh"
    assert land_path.exists(), "scripts/land.sh is missing"
    assert land_path.stat().st_mode & 0o111, "scripts/land.sh must keep an executable mode bit"


def test_supported_surfaces_do_not_reference_removed_shell_entrypoints():
    """Core docs and transport surfaces should not advertise removed shell entrypoints."""
    for path in LIVE_REFERENCE_SURFACES:
        assert path.exists(), f"Expected supported surface is missing: {path}"
        content = path.read_text()

        for relative_path in REMOVED_SHELL_ENTRYPOINTS:
            basename = relative_path.replace("scripts/", "")
            assert relative_path not in content, f"{path} still references {relative_path}"
            assert basename not in content, f"{path} still references {basename}"
