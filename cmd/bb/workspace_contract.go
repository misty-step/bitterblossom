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

func activeDispatchCheckScriptFor(workspace string) string {
	pidFile := workspaceDispatchPIDPath(workspace)

	return fmt.Sprintf(`PID_FILE=%q
if [ ! -f "$PID_FILE" ]; then
  exit 0
fi

pid="$(cat "$PID_FILE" 2>/dev/null || true)"
case "$pid" in
  ''|*[!0-9]*)
    rm -f "$PID_FILE"
    exit 0
    ;;
esac

if kill -0 "$pid" 2>/dev/null; then
  ps -p "$pid" -o pid= -o args= 2>/dev/null || echo "$pid"
  exit 1
fi

rm -f "$PID_FILE"
exit 0`, pidFile)
}

func killDispatchProcessScriptFor(workspace string) string {
	pidFile := workspaceDispatchPIDPath(workspace)

	return fmt.Sprintf(`PID_FILE=%q
if [ ! -f "$PID_FILE" ]; then
  echo "no stale agent processes found"
  exit 0
fi

pid="$(cat "$PID_FILE" 2>/dev/null || true)"
case "$pid" in
  ''|*[!0-9]*)
    rm -f "$PID_FILE"
    echo "no stale agent processes found"
    exit 0
    ;;
esac

if ! kill -0 "$pid" 2>/dev/null; then
  rm -f "$PID_FILE"
  echo "no stale agent processes found"
  exit 0
fi

echo "found agent process:"
ps -p "$pid" -o pid= -o args= 2>/dev/null || echo "$pid"

pkill -TERM -P "$pid" 2>/dev/null || true
kill -TERM "$pid" 2>/dev/null || true
sleep 1
pkill -KILL -P "$pid" 2>/dev/null || true
kill -KILL "$pid" 2>/dev/null || true

rm -f "$PID_FILE"
echo "stale agent processes terminated"`, pidFile)
}

func workspaceStatusSignalsScript(envName string) string {
	quotedNames := make([]string, 0, len(workspaceStatusSignalFileNames))
	for _, name := range workspaceStatusSignalFileNames {
		quotedNames = append(quotedNames, fmt.Sprintf("%q", name))
	}

	return fmt.Sprintf("for f in %s; do\n  [ -f \"$%s/$f\" ] && echo \"$f: present\" || echo \"$f: absent\"\ndone", strings.Join(quotedNames, " "), envName)
}
