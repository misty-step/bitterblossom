package main

import (
	"fmt"
	"path"
	"strings"
)

const (
	spriteHomeDir        = "/home/sprite"
	spriteClaudeDir      = spriteHomeDir + "/.claude"
	spriteCodexDir       = spriteHomeDir + "/.codex"
	spriteRuntimeDir     = spriteHomeDir + "/.bitterblossom"
	spriteRuntimeEnvPath = spriteRuntimeDir + "/runtime.env"
	spriteWorkspaceRoot  = spriteHomeDir + "/workspace"

	spritePersonaPath        = spriteWorkspaceRoot + "/PERSONA.md"
	spritePromptTemplatePath = spriteWorkspaceRoot + "/.builder-prompt-template.md"

	workspaceMetadataRelPath   = ".bb/workspace.json"
	dispatchPromptFileName     = ".dispatch-prompt.md"
	dispatchPIDFileName        = ".bb-agent.pid"
	ralphLogFileName           = "ralph.log"
	taskCompleteFileName       = "TASK_COMPLETE"
	legacyTaskCompleteFileName = "TASK_COMPLETE.md"
	blockedFileName            = "BLOCKED.md"
)

var (
	taskCompleteSignalFileNames = []string{
		taskCompleteFileName,
		legacyTaskCompleteFileName,
	}
	workspaceStatusSignalFileNames = []string{
		taskCompleteFileName,
		legacyTaskCompleteFileName,
		blockedFileName,
	}
)

func spriteRepoWorkspace(repo string) string {
	return path.Join(spriteWorkspaceRoot, path.Base(repo))
}

func workspaceFilePath(workspace, name string) string {
	return path.Join(workspace, name)
}

func workspaceDispatchPromptPath(workspace string) string {
	return workspaceFilePath(workspace, dispatchPromptFileName)
}

func workspaceDispatchPIDPath(workspace string) string {
	return workspaceFilePath(workspace, dispatchPIDFileName)
}

func workspaceRalphLogPath(workspace string) string {
	return workspaceFilePath(workspace, ralphLogFileName)
}

func cleanSignalsScriptFor(workspace string) string {
	targets := make([]string, 0, len(workspaceStatusSignalFileNames))
	for _, name := range workspaceStatusSignalFileNames {
		targets = append(targets, fmt.Sprintf(`"$WORKSPACE"/%s`, name))
	}

	return fmt.Sprintf(`export WORKSPACE=%q; rm -f %s`, workspace, strings.Join(targets, " "))
}

func taskCompleteSignalCheckScriptFor(workspace string) string {
	checks := make([]string, 0, len(taskCompleteSignalFileNames))
	for _, name := range taskCompleteSignalFileNames {
		checks = append(checks, fmt.Sprintf(`[ -f "$WORKSPACE/%s" ]`, name))
	}

	return fmt.Sprintf("export WORKSPACE=%q\nif %s; then\n  exit 0\nfi\nexit 1", workspace, strings.Join(checks, " || "))
}

func workspaceAgentProcessHelpersScript(workspace string) string {
	return fmt.Sprintf(`WORKSPACE=%q

pid_cwd() {
  readlink -f "/proc/$1/cwd" 2>/dev/null || true
}

pid_in_workspace() {
  local pid="$1"
  [ -n "$pid" ] || return 1
  [ "$(pid_cwd "$pid")" = "$WORKSPACE" ]
}

print_proc() {
  ps -p "$1" -o pid= -o args= 2>/dev/null || echo "$1"
}

workspace_agent_pids() {
  if ! command -v pgrep >/dev/null 2>&1; then
    return 0
  fi

  {
    pgrep -x claude 2>/dev/null || true
    pgrep -x codex 2>/dev/null || true
    pgrep -x opencode 2>/dev/null || true
  } | while IFS= read -r pid; do
    [ -n "$pid" ] || continue
    if pid_in_workspace "$pid"; then
      printf '%%s\n' "$pid"
    fi
  done | sort -u
}

workspace_agent_parent_pids() {
  printf '%%s\n' "$@" | while IFS= read -r pid; do
    [ -n "$pid" ] || continue
    ppid="$(ps -o ppid= -p "$pid" 2>/dev/null | tr -d '[:space:]')"
    [ -n "$ppid" ] || continue
    if pid_in_workspace "$ppid"; then
      printf '%%s\n' "$ppid"
    fi
  done | sort -u
}
`, workspace)
}

func activeDispatchCheckScriptFor(workspace string) string {
	pidFile := workspaceDispatchPIDPath(workspace)
	helpers := workspaceAgentProcessHelpersScript(workspace)

	return fmt.Sprintf(`%s
PID_FILE=%q
if [ -f "$PID_FILE" ]; then
  pid="$(cat "$PID_FILE" 2>/dev/null || true)"
  case "$pid" in
    ''|*[!0-9]*)
      rm -f "$PID_FILE"
      ;;
    *)
      if kill -0 "$pid" 2>/dev/null; then
        if pid_in_workspace "$pid"; then
          print_proc "$pid"
          exit 1
        fi
        rm -f "$PID_FILE"
      else
        rm -f "$PID_FILE"
      fi
      ;;
  esac
fi

mapfile -t fallback_pids < <(workspace_agent_pids)
if [ "${#fallback_pids[@]}" -eq 0 ]; then
  exit 0
fi

for pid in "${fallback_pids[@]}"; do
  print_proc "$pid"
done
exit 1`, helpers, pidFile)
}

func killDispatchProcessScriptFor(workspace string) string {
	pidFile := workspaceDispatchPIDPath(workspace)
	helpers := workspaceAgentProcessHelpersScript(workspace)

	return fmt.Sprintf(`%s
PID_FILE=%q
if [ -f "$PID_FILE" ]; then
  pid="$(cat "$PID_FILE" 2>/dev/null || true)"
  case "$pid" in
    ''|*[!0-9]*)
      rm -f "$PID_FILE"
      ;;
    *)
      if kill -0 "$pid" 2>/dev/null; then
        if pid_in_workspace "$pid"; then
          echo "found agent process:"
          print_proc "$pid"

          pkill -TERM -P "$pid" 2>/dev/null || true
          kill -TERM "$pid" 2>/dev/null || true
          sleep 1
          pkill -KILL -P "$pid" 2>/dev/null || true
          kill -KILL "$pid" 2>/dev/null || true

          rm -f "$PID_FILE"
          echo "stale agent processes terminated"
          exit 0
        fi
        rm -f "$PID_FILE"
      else
        rm -f "$PID_FILE"
      fi
      ;;
  esac
fi

mapfile -t fallback_pids < <(workspace_agent_pids)
if [ "${#fallback_pids[@]}" -eq 0 ]; then
  echo "no stale agent processes found"
  exit 0
fi

echo "found workspace-scoped agent processes:"
for pid in "${fallback_pids[@]}"; do
  print_proc "$pid"
done

mapfile -t fallback_parents < <(workspace_agent_parent_pids "${fallback_pids[@]}")
for pid in "${fallback_parents[@]}"; do
  pkill -TERM -P "$pid" 2>/dev/null || true
  kill -TERM "$pid" 2>/dev/null || true
done
for pid in "${fallback_pids[@]}"; do
  kill -TERM "$pid" 2>/dev/null || true
done

sleep 1

for pid in "${fallback_parents[@]}"; do
  pkill -KILL -P "$pid" 2>/dev/null || true
  kill -KILL "$pid" 2>/dev/null || true
done
for pid in "${fallback_pids[@]}"; do
  kill -KILL "$pid" 2>/dev/null || true
done

rm -f "$PID_FILE"
echo "stale agent processes terminated"`, helpers, pidFile)
}

func workspaceStatusSignalsScript(envName string) string {
	quotedNames := make([]string, 0, len(workspaceStatusSignalFileNames))
	for _, name := range workspaceStatusSignalFileNames {
		quotedNames = append(quotedNames, fmt.Sprintf("%q", name))
	}

	return fmt.Sprintf("for f in %s; do\n  [ -f \"$%s/$f\" ] && echo \"$f: present\" || echo \"$f: absent\"\ndone", strings.Join(quotedNames, " "), envName)
}
