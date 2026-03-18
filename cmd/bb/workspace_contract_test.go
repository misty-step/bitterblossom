package main

import (
	"strings"
	"testing"
)

func TestBlockedSignalCheckScriptForQuotesWorkspace(t *testing.T) {
	t.Parallel()

	workspace := `/tmp/run 123; touch /tmp/pwned`
	script := blockedSignalCheckScriptFor(workspace)

	if !strings.Contains(script, `export WORKSPACE="/tmp/run 123; touch /tmp/pwned"`) {
		t.Fatalf("blockedSignalCheckScriptFor() missing quoted workspace: %q", script)
	}
	if !strings.Contains(script, `if [ -f "$WORKSPACE/BLOCKED.md" ]; then`) {
		t.Fatalf("blockedSignalCheckScriptFor() should check BLOCKED.md via WORKSPACE env var: %q", script)
	}
}
