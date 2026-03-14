"""Runtime contract verification — guards against model/provider drift.

Canonical source: base/settings.json  (ANTHROPIC_MODEL)

Surfaces validated:
  - base/settings.json       — deployed sprite env defaults
  - cmd/bb/runtime_contract.go — Go constant used by dispatch
  - scripts/lib.sh           — openrouter-claude provider default fallback
  - README.md                — operator-facing documentation

Run:
  python3 -m pytest -q scripts/test_runtime_contract.py
"""

import json
import re
from pathlib import Path

REPO_ROOT = Path(__file__).parent.parent


def _load_canonical_model() -> str:
    """Read the canonical sprite model from base/settings.json."""
    settings_path = REPO_ROOT / "base" / "settings.json"
    data = json.loads(settings_path.read_text())
    return data["env"]["ANTHROPIC_MODEL"]


CANONICAL_MODEL = _load_canonical_model()


def test_settings_json_model_vars_agree():
    """All ANTHROPIC_*_MODEL and CLAUDE_CODE_SUBAGENT_MODEL vars must match ANTHROPIC_MODEL."""
    settings_path = REPO_ROOT / "base" / "settings.json"
    data = json.loads(settings_path.read_text())
    env = data.get("env", {})

    model_keys = [
        "ANTHROPIC_MODEL",
        "ANTHROPIC_SMALL_FAST_MODEL",
        "ANTHROPIC_DEFAULT_OPUS_MODEL",
        "ANTHROPIC_DEFAULT_SONNET_MODEL",
        "ANTHROPIC_DEFAULT_HAIKU_MODEL",
        "CLAUDE_CODE_SUBAGENT_MODEL",
    ]
    mismatches = {k: env[k] for k in model_keys if k in env and env[k] != CANONICAL_MODEL}
    assert not mismatches, (
        f"base/settings.json model vars disagree with ANTHROPIC_MODEL={CANONICAL_MODEL!r}:\n"
        + "\n".join(f"  {k} = {v!r}" for k, v in mismatches.items())
    )

    print(f"\n[ok] base/settings.json: {len(model_keys)} model vars all = {CANONICAL_MODEL!r}")


def test_go_runtime_constant_matches_canonical():
    """cmd/bb/runtime_contract.go spriteModel constant must equal canonical model."""
    go_path = REPO_ROOT / "cmd" / "bb" / "runtime_contract.go"
    content = go_path.read_text()

    # Match: const spriteModel = "anthropic/claude-sonnet-4-6"
    match = re.search(r'const\s+spriteModel\s*=\s*"([^"]+)"', content)
    assert match, f"Could not find spriteModel constant in {go_path}"

    go_model = match.group(1)
    assert go_model == CANONICAL_MODEL, (
        f"cmd/bb/runtime_contract.go spriteModel={go_model!r} "
        f"!= canonical {CANONICAL_MODEL!r} from base/settings.json"
    )
    print(f"[ok] cmd/bb/runtime_contract.go: spriteModel = {go_model!r}")


def test_dispatch_go_uses_sprite_model_constant():
    """dispatch.go must not hardcode its own model string literals."""
    dispatch_path = REPO_ROOT / "cmd" / "bb" / "dispatch.go"
    content = dispatch_path.read_text()

    # Find quoted strings that look like model IDs (provider/model-id pattern).
    # These would be a sign of independent hardcoding outside runtime_contract.go.
    model_pattern = re.compile(r'"([\w-]+/[\w.-]+-\d[\w.-]*)"')
    hardcoded = [m.group(1) for m in model_pattern.finditer(content)]

    assert not hardcoded, (
        f"dispatch.go contains hardcoded model string(s): {hardcoded!r}\n"
        "Use the spriteModel constant from runtime_contract.go instead."
    )
    print(f"[ok] cmd/bb/dispatch.go: no hardcoded model strings found")


def test_lib_sh_openrouter_claude_default_matches_canonical():
    """scripts/lib.sh openrouter-claude fallback default must equal canonical model."""
    lib_path = REPO_ROOT / "scripts" / "lib.sh"
    content = lib_path.read_text()

    # Look for the openrouter-claude default assignment block:
    #   env["ANTHROPIC_MODEL"] = "anthropic/claude-..."
    # This appears inside the openrouter-claude elif branch (no model given).
    match = re.search(
        r'elif provider == "openrouter-claude":.*?env\["ANTHROPIC_MODEL"\]\s*=\s*"([^"]+)"',
        content,
        re.DOTALL,
    )
    assert match, "Could not find openrouter-claude default model in scripts/lib.sh"

    lib_default = match.group(1)
    assert lib_default == CANONICAL_MODEL, (
        f"scripts/lib.sh openrouter-claude default={lib_default!r} "
        f"!= canonical {CANONICAL_MODEL!r} from base/settings.json"
    )
    print(f"[ok] scripts/lib.sh: openrouter-claude default = {lib_default!r}")


def test_readme_documents_canonical_model():
    """README.md must document the canonical model identifier."""
    readme_path = REPO_ROOT / "README.md"
    content = readme_path.read_text()

    assert CANONICAL_MODEL in content, (
        f"README.md does not reference the canonical model {CANONICAL_MODEL!r}.\n"
        "Update the 'Runtime profile' section to match base/settings.json."
    )
    print(f"[ok] README.md: references {CANONICAL_MODEL!r}")


def test_canonical_source_is_base_settings_json():
    """Smoke test: base/settings.json must exist and have ANTHROPIC_MODEL set."""
    settings_path = REPO_ROOT / "base" / "settings.json"
    assert settings_path.exists(), "base/settings.json not found — canonical source is missing"

    data = json.loads(settings_path.read_text())
    assert "env" in data, "base/settings.json missing 'env' key"
    assert "ANTHROPIC_MODEL" in data["env"], "base/settings.json missing ANTHROPIC_MODEL in env"

    model = data["env"]["ANTHROPIC_MODEL"]
    assert model, "base/settings.json ANTHROPIC_MODEL is empty"
    print(f"\n[canonical] base/settings.json ANTHROPIC_MODEL = {model!r}")
    print(
        "Validated surfaces:"
        "\n  base/settings.json     (canonical)"
        "\n  cmd/bb/runtime_contract.go"
        "\n  scripts/lib.sh"
        "\n  README.md"
    )
