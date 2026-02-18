package main

import (
	"strings"
	"testing"
)

func TestKillAgentProcessesScriptTargetsRalphLoop(t *testing.T) {
	t.Parallel()

	// Dispatch considers the sprite busy only when the ralph loop is running.
	// `bb kill` must be able to find/terminate that loop to unblock dispatch.
	if !strings.Contains(killAgentProcessesScript, `agents='/home/sprite/workspace/\.[r]alph\.sh|[c]laude|[o]pencode'`) {
		t.Fatalf("killAgentProcessesScript does not include expected agents regex")
	}
}
