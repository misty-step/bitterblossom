package main

import (
	"strings"
	"testing"
)

func TestKillDispatchProcessScriptUsesWorkspacePIDFile(t *testing.T) {
	t.Parallel()

	script := killDispatchProcessScriptFor("/home/sprite/workspace/repo")
	if !strings.Contains(script, `/home/sprite/workspace/repo/.bb-agent.pid`) {
		t.Fatalf("killDispatchProcessScriptFor() missing workspace pid path: %q", script)
	}
	if !strings.Contains(script, `pkill -TERM -P "$pid"`) {
		t.Fatalf("killDispatchProcessScriptFor() should terminate child processes: %q", script)
	}
	if !strings.Contains(script, `readlink -f "/proc/$1/cwd"`) {
		t.Fatalf("killDispatchProcessScriptFor() should validate workspace ownership: %q", script)
	}
	if !strings.Contains(script, `workspace_agent_pids`) {
		t.Fatalf("killDispatchProcessScriptFor() should include workspace fallback scan: %q", script)
	}
}
