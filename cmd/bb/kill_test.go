package main

import (
	"strings"
	"testing"
)

func TestKillAgentProcessesScriptTargetsSupportedAgents(t *testing.T) {
	t.Parallel()

	if !strings.Contains(killAgentProcessesScript, `agents='[c]laude|[c]odex|[o]pencode'`) {
		t.Fatalf("killAgentProcessesScript does not include expected agents regex")
	}
}
