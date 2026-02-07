from pathlib import Path


ROOT_DIR = Path(__file__).resolve().parents[2]
LIB_SH = ROOT_DIR / "scripts" / "lib.sh"
DISPATCH_SH = ROOT_DIR / "scripts" / "dispatch.sh"


def test_lib_defines_workspace_from_remote_home():
    content = LIB_SH.read_text(encoding="utf-8")
    assert 'REMOTE_HOME="/home/sprite"' in content
    assert 'WORKSPACE="$REMOTE_HOME/workspace"' in content


def test_dispatch_uses_lib_workspace_without_redefining():
    content = DISPATCH_SH.read_text(encoding="utf-8")
    assert 'source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"' in content
    assert 'WORKSPACE="$REMOTE_HOME/workspace"' not in content
    assert 'local remote_prompt="$WORKSPACE/.dispatch-prompt.md"' in content
