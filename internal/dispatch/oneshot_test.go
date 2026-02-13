package dispatch

import (
	"strings"
	"testing"
)

// TestBuildOneShotScriptExportsProxyEnvWhenRunning verifies that when the proxy
// is already running, the script still exports ANTHROPIC_BASE_URL and
// ANTHROPIC_API_KEY environment variables needed for Claude to use the proxy.
// This is a regression test for issue #294.
func TestBuildOneShotScriptExportsProxyEnvWhenRunning(t *testing.T) {
	script := buildOneShotScript("/home/sprite/workspace", "/home/sprite/workspace/.dispatch-prompt.md")

	// The script should export the proxy environment variables in the "already running" branch
	if !strings.Contains(script, "export ANTHROPIC_BASE_URL=") {
		t.Error("oneshot script missing ANTHROPIC_BASE_URL export")
	}
	if !strings.Contains(script, "export ANTHROPIC_API_KEY=proxy-mode") {
		t.Error("oneshot script missing ANTHROPIC_API_KEY export")
	}

	// Verify exports happen in the "proxy already running" branch
	// The script structure should have these exports after "proxy already running"
	lines := strings.Split(script, "\n")
	foundAlreadyRunning := false
	foundBaseURLAfter := false
	foundAPIKeyAfter := false

	for _, line := range lines {
		if strings.Contains(line, "proxy already running") {
			foundAlreadyRunning = true
		}
		if foundAlreadyRunning && strings.Contains(line, "export ANTHROPIC_BASE_URL=") {
			foundBaseURLAfter = true
		}
		if foundAlreadyRunning && strings.Contains(line, "export ANTHROPIC_API_KEY=proxy-mode") {
			foundAPIKeyAfter = true
		}
	}

	if !foundAlreadyRunning {
		t.Error("script missing 'proxy already running' message")
	}
	if !foundBaseURLAfter {
		t.Error("ANTHROPIC_BASE_URL export not found after 'proxy already running'")
	}
	if !foundAPIKeyAfter {
		t.Error("ANTHROPIC_API_KEY export not found after 'proxy already running'")
	}
}

// TestBuildOneShotScriptCapturesExitCode verifies that the oneshot script
// captures and reports the exit code of the agent process. This ensures
// dispatch can detect when the agent fails even if it produces no output.
func TestBuildOneShotScriptCapturesExitCode(t *testing.T) {
	script := buildOneShotScript("/home/sprite/workspace", "/home/sprite/workspace/.dispatch-prompt.md")

	// The script should capture the exit code
	if !strings.Contains(script, "AGENT_EXIT_CODE=") {
		t.Error("oneshot script should capture AGENT_EXIT_CODE")
	}

	// The script should output the exit code for parsing
	if !strings.Contains(script, "echo \"EXIT_CODE:$AGENT_EXIT_CODE\"") {
		t.Error("oneshot script should output EXIT_CODE for parsing")
	}
}

// TestBuildOneShotScriptLogsOutput verifies that the oneshot script
// redirects output to a log file for debugging when the agent produces
// no observable changes.
func TestBuildOneShotScriptLogsOutput(t *testing.T) {
	script := buildOneShotScript("/home/sprite/workspace", "/home/sprite/workspace/.dispatch-prompt.md")

	// The script should redirect output to a log file using tee -a
	if !strings.Contains(script, "tee -a") || !strings.Contains(script, "agent.log") {
		t.Error("oneshot script should redirect output to agent.log using tee")
	}
}
