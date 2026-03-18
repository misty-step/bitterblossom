package main

import (
	"strings"
	"testing"
)

func TestKillAgentProcessesScriptTargetsSupportedAgents(t *testing.T) {
	t.Parallel()

	for _, want := range []string{
		`agents='`,
		`/home/sprite/workspace/`,
		`\\.[r]alph\\.sh`,
		`[c]laude`,
		`[c]odex`,
		`[o]pencode`,
	} {
		if !strings.Contains(killAgentProcessesScript, want) {
			t.Fatalf("killAgentProcessesScript missing %q in agent targets", want)
		}
	}
}

func TestKillAgentProcessesScriptRequiresPgrepAndPkill(t *testing.T) {
	t.Parallel()

	for _, want := range []string{"command -v pgrep", "command -v pkill"} {
		if !strings.Contains(killAgentProcessesScript, want) {
			t.Fatalf("killAgentProcessesScript = %q, want to contain %q", killAgentProcessesScript, want)
		}
	}
}
