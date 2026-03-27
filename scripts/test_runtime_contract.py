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
    target = re.search(r"^test-conductor:\n((?:\t.*\n)+)", content, re.MULTILINE)
    assert target, "Makefile missing test-conductor target"
    body = target.group(1)

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
    shutil.rmtree(REPO_ROOT / "conductor" / "deps", ignore_errors=True)
    shutil.rmtree(REPO_ROOT / "conductor" / "_build", ignore_errors=True)

    result = subprocess.run(
        ["make", "test"],
        cwd=REPO_ROOT,
        text=True,
        capture_output=True,
        check=False,
    )

    assert result.returncode == 0, (
        "'make test' must succeed after removing conductor/deps and conductor/_build.\n"
        f"stdout:\n{result.stdout}\n"
        f"stderr:\n{result.stderr}"
    )
    print("[ok] make test: succeeds from a clean conductor checkout state")


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


def test_supported_surfaces_do_not_reference_removed_shell_entrypoints():
    """Core docs and transport surfaces should not advertise removed shell entrypoints."""
    for path in LIVE_REFERENCE_SURFACES:
        assert path.exists(), f"Expected supported surface is missing: {path}"
        content = path.read_text()

        for relative_path in REMOVED_SHELL_ENTRYPOINTS:
            basename = relative_path.replace("scripts/", "")
            assert relative_path not in content, f"{path} still references {relative_path}"
            assert basename not in content, f"{path} still references {basename}"
