import re
from pathlib import Path


ROOT_DIR = Path(__file__).resolve().parents[2]
LIB_SH = ROOT_DIR / "scripts" / "lib.sh"
PROMPT_TEMPLATE = ROOT_DIR / "scripts" / "builder-prompt-template.md"
WORKSPACE_CONTRACT = ROOT_DIR / "cmd" / "bb" / "workspace_contract.go"


def test_lib_defines_workspace_from_remote_home():
    content = LIB_SH.read_text(encoding="utf-8")
    assert 'REMOTE_HOME="/home/sprite"' in content
    assert 'WORKSPACE="$REMOTE_HOME/workspace"' in content


def test_builder_prompt_requires_extensionless_task_complete_signal():
    content = PROMPT_TEMPLATE.read_text(encoding="utf-8")
    assert "Create a file named exactly TASK_COMPLETE" in content
    assert "Do NOT use TASK_COMPLETE.md" in content


def test_workspace_contract_keeps_dispatch_prompt_and_task_complete_names():
    content = WORKSPACE_CONTRACT.read_text(encoding="utf-8")
    assert re.search(r'\bdispatchPromptFileName\s*=\s*"\.dispatch-prompt\.md"', content)
    assert re.search(r'\btaskCompleteFileName\s*=\s*"TASK_COMPLETE"', content)
