from pathlib import Path


ROOT_DIR = Path(__file__).resolve().parents[2]
LIB_SH = ROOT_DIR / "scripts" / "lib.sh"
PROMPT_TEMPLATE = ROOT_DIR / "scripts" / "builder-prompt-template.md"
SPRITE_IMPL = ROOT_DIR / "conductor" / "lib" / "conductor" / "sprite.ex"


def test_lib_defines_workspace_from_remote_home():
    content = LIB_SH.read_text(encoding="utf-8")
    assert 'REMOTE_HOME="/home/sprite"' in content
    assert 'WORKSPACE="$REMOTE_HOME/workspace"' in content


def test_builder_prompt_requires_extensionless_task_complete_signal():
    content = PROMPT_TEMPLATE.read_text(encoding="utf-8")
    assert "Create a file named exactly TASK_COMPLETE" in content
    assert "Do NOT use TASK_COMPLETE.md" in content


def test_sprite_contract_keeps_prompt_and_log_names():
    content = SPRITE_IMPL.read_text(encoding="utf-8")
    assert 'Path.join(workspace, "PROMPT.md")' in content
    assert '@log_file "ralph.log"' in content
