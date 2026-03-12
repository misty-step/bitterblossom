from __future__ import annotations

import os
import stat
import subprocess
import time
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "conductor-supervise.sh"


def write_executable(path: Path, contents: str) -> None:
    path.write_text(contents, encoding="utf-8")
    path.chmod(path.stat().st_mode | stat.S_IXUSR)


def test_install_cron_writes_reboot_entry_and_launcher(tmp_path: Path) -> None:
    fake_bin = tmp_path / "bin"
    fake_bin.mkdir()
    crontab_stdin = tmp_path / "crontab.txt"

    write_executable(
        fake_bin / "crontab",
        f"""#!/usr/bin/env bash
set -euo pipefail
cat > "{crontab_stdin}"
""",
    )

    home = tmp_path / "home"
    repo_root = tmp_path / "repo"
    repo_root.mkdir()

    env = os.environ.copy()
    env["HOME"] = str(home)
    env["PATH"] = f"{fake_bin}:{env['PATH']}"

    subprocess.run(
        [
            str(SCRIPT),
            "install-cron",
            "--repo-root",
            str(repo_root),
            "--repo",
            "misty-step/bitterblossom",
            "--label",
            "autopilot",
            "--worker",
            "noble-blue-serpent",
            "--reviewer",
            "council-fern",
        ],
        check=True,
        env=env,
        cwd=ROOT,
        text=True,
    )

    launcher = home / ".bb" / "conductor-supervisor" / "launch.sh"
    assert launcher.exists()
    assert os.access(launcher, os.X_OK)
    launcher_text = launcher.read_text(encoding="utf-8")
    assert f"cd {repo_root}" in launcher_text
    assert "scripts/conductor-supervise.sh run" in launcher_text
    assert "--repo misty-step/bitterblossom" in launcher_text
    assert "--worker noble-blue-serpent" in launcher_text

    cron_text = crontab_stdin.read_text(encoding="utf-8")
    assert "@reboot" in cron_text
    assert str(launcher) in cron_text


def test_rotate_logs_archives_current_log_and_prunes_old_entries(tmp_path: Path) -> None:
    home = tmp_path / "home"
    state_dir = home / ".bb" / "conductor-supervisor"
    state_dir.mkdir(parents=True)
    current_log = state_dir / "current.log"
    current_log.write_text("x" * 128, encoding="utf-8")

    for index in range(4):
        archived = state_dir / f"conductor-20260311-00000{index}.log"
        archived.write_text(f"old-{index}", encoding="utf-8")

    env = os.environ.copy()
    env["HOME"] = str(home)
    env["BB_CONDUCTOR_LOG_MAX_BYTES"] = "32"
    env["BB_CONDUCTOR_LOG_KEEP_FILES"] = "2"

    subprocess.run(
        [str(SCRIPT), "rotate-logs"],
        check=True,
        env=env,
        cwd=ROOT,
        text=True,
    )

    archived_logs = sorted(state_dir.glob("conductor-*.log"))
    assert len(archived_logs) == 2
    assert not current_log.exists()
    assert any(log.read_text(encoding="utf-8") == "x" * 128 for log in archived_logs)


def test_stop_terminates_supervisor_and_child(tmp_path: Path) -> None:
    fake_bin = tmp_path / "bin"
    fake_bin.mkdir()
    child_pid_file = tmp_path / "child.pid"
    terminated_file = tmp_path / "terminated.txt"

    write_executable(
        fake_bin / "python3",
        f"""#!/usr/bin/env bash
set -euo pipefail
echo "$$" > "{child_pid_file}"
trap 'echo terminated > "{terminated_file}"; exit 0' TERM
while true; do
  sleep 1
done
""",
    )

    home = tmp_path / "home"
    env = os.environ.copy()
    env["HOME"] = str(home)
    env["PATH"] = f"{fake_bin}:{env['PATH']}"
    env["BB_CONDUCTOR_RESTART_DELAY_SECONDS"] = "1"

    subprocess.run(
        [str(SCRIPT), "start", "--repo", "misty-step/bitterblossom"],
        check=True,
        env=env,
        cwd=ROOT,
        text=True,
    )

    state_dir = home / ".bb" / "conductor-supervisor"
    supervisor_pid_path = state_dir / "supervisor.pid"
    child_state_pid_path = state_dir / "child.pid"

    for _ in range(40):
        if supervisor_pid_path.exists() and child_state_pid_path.exists() and child_pid_file.exists():
            break
        time.sleep(0.1)
    else:
        raise AssertionError("supervisor did not start the child process")

    supervisor_pid = int(supervisor_pid_path.read_text(encoding="utf-8").strip())
    child_pid = int(child_pid_file.read_text(encoding="utf-8").strip())

    subprocess.run(
        [str(SCRIPT), "stop"],
        check=True,
        env=env,
        cwd=ROOT,
        text=True,
    )

    for _ in range(40):
        supervisor_alive = True
        child_alive = True
        try:
            os.kill(supervisor_pid, 0)
        except OSError:
            supervisor_alive = False
        try:
            os.kill(child_pid, 0)
        except OSError:
            child_alive = False
        if not supervisor_alive and not child_alive and terminated_file.exists():
            break
        time.sleep(0.1)
    else:
        raise AssertionError("stop did not terminate the supervised processes")
