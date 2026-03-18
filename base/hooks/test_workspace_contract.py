from pathlib import Path


ROOT_DIR = Path(__file__).resolve().parents[2]
LIB_SH = ROOT_DIR / "scripts" / "lib.sh"


def test_lib_defines_workspace_from_remote_home():
    content = LIB_SH.read_text(encoding="utf-8")
    assert 'REMOTE_HOME="/home/sprite"' in content
    assert 'WORKSPACE="$REMOTE_HOME/workspace"' in content
