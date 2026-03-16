package main

import (
	"fmt"
	"path"
	"strings"
)

const (
	spriteHomeDir       = "/home/sprite"
	spriteClaudeDir     = spriteHomeDir + "/.claude"
	spriteCodexDir      = spriteHomeDir + "/.codex"
	spriteWorkspaceRoot = spriteHomeDir + "/workspace"

	spritePersonaPath = spriteWorkspaceRoot + "/PERSONA.md"

	workspaceMetadataRelPath   = ".bb/workspace.json"
	dispatchPromptFileName     = ".dispatch-prompt.md"
	agentLogFileName           = "agent.log"
	taskCompleteFileName       = "TASK_COMPLETE"
	legacyTaskCompleteFileName = "TASK_COMPLETE.md"
	blockedFileName            = "BLOCKED.md"
	agentSessionName           = "bb-agent-session"
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

func workspaceAgentLogPath(workspace string) string {
	return workspaceFilePath(workspace, agentLogFileName)
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

func workspaceStatusSignalsScript(envName string) string {
	quotedNames := make([]string, 0, len(workspaceStatusSignalFileNames))
	for _, name := range workspaceStatusSignalFileNames {
		quotedNames = append(quotedNames, fmt.Sprintf("%q", name))
	}

	return fmt.Sprintf("for f in %s; do\n  [ -f \"$%s/$f\" ] && echo \"$f: present\" || echo \"$f: absent\"\ndone", strings.Join(quotedNames, " "), envName)
}
