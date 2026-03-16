from pathlib import Path


ROOT_DIR = Path(__file__).resolve().parents[2]
LIB_SH = ROOT_DIR / "scripts" / "lib.sh"
SCRIPTS_DIR = ROOT_DIR / "scripts"


def test_lib_defines_workspace_from_remote_home():
    content = LIB_SH.read_text(encoding="utf-8")
    assert 'REMOTE_HOME="/home/sprite"' in content
    assert 'WORKSPACE="$REMOTE_HOME/workspace"' in content


def test_scripts_directory_only_keeps_supported_shell_helpers():
    present = {path.name for path in SCRIPTS_DIR.iterdir() if path.name != "__pycache__"}
    allowed = {
        "builder-prompt-template.md",
        "glance.md",
        "lib.sh",
        "onboard.sh",
        "sentry-watcher.sh",
        "test_runtime_contract.py",
    }

    assert present == allowed
