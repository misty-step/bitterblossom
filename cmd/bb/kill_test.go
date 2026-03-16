package main

import (
	"strings"
	"testing"
)

func TestKillAgentProcessesScriptTargetsManagedAgentSession(t *testing.T) {
	t.Parallel()

	if !strings.Contains(killAgentProcessesScript, `agents='[b]b-agent-session|[c]laude|[c]odex'`) {
		t.Fatalf("killAgentProcessesScript does not include expected agents regex")
	}
}
