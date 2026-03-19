package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestLogsRemoteCommandTailFollow verifies follow mode uses tail -f with default lines.
func TestLogsRemoteCommandTailFollow(t *testing.T) {
	t.Parallel()

	got := logsRemoteCommand("/tmp/ralph.log", true, 0)
	if !strings.Contains(got, "tail -n 50 -f") {
		t.Errorf("logsRemoteCommand follow=true lines=0: %q, want tail -n 50 -f", got)
	}
	if !strings.Contains(got, "/tmp/ralph.log") {
		t.Errorf("logsRemoteCommand: %q, want to contain log path", got)
	}
}

// TestLogsRemoteCommandTailFollowCustomLines verifies follow mode respects explicit --lines.
func TestLogsRemoteCommandTailFollowCustomLines(t *testing.T) {
	t.Parallel()

	got := logsRemoteCommand("/tmp/ralph.log", true, 20)
	if !strings.Contains(got, "tail -n 20 -f") {
		t.Errorf("logsRemoteCommand follow=true lines=20: %q, want tail -n 20 -f", got)
	}
}

// TestLogsRemoteCommandTailLines verifies non-follow with explicit --lines uses tail.
func TestLogsRemoteCommandTailLines(t *testing.T) {
	t.Parallel()

	got := logsRemoteCommand("/tmp/ralph.log", false, 30)
	if !strings.Contains(got, "tail -n 30") {
		t.Errorf("logsRemoteCommand follow=false lines=30: %q, want tail -n 30", got)
	}
	if strings.Contains(got, "-f") {
		t.Errorf("logsRemoteCommand non-follow: %q, must not contain -f", got)
	}
}

// TestLogsRemoteCommandCatAll verifies that lines=0 with no follow uses cat.
func TestLogsRemoteCommandCatAll(t *testing.T) {
	t.Parallel()

	got := logsRemoteCommand("/tmp/ralph.log", false, 0)
	if !strings.Contains(got, "cat") {
		t.Errorf("logsRemoteCommand follow=false lines=0: %q, want cat", got)
	}
	if strings.Contains(got, "tail") {
		t.Errorf("logsRemoteCommand follow=false lines=0: %q, must not use tail", got)
	}
}

// TestLogsRemoteCommandTouchesLogPath ensures the log path is always created/touched
// before reading, so commands don't fail on a missing file.
func TestLogsRemoteCommandTouchesLogPath(t *testing.T) {
	t.Parallel()

	for _, follow := range []bool{true, false} {
		for _, lines := range []int{0, 10} {
			got := logsRemoteCommand("/tmp/ralph.log", follow, lines)
			if !strings.Contains(got, "touch") {
				t.Errorf("logsRemoteCommand follow=%v lines=%d: %q, want touch", follow, lines, got)
			}
		}
	}
}

// TestLogsNoActiveTaskGoesToStderr is the regression test for #410:
// "No active task" must NOT appear on stdout in any mode, and especially
// not in --json mode where stdout must be parseable JSON only.
func TestLogsNoActiveTaskGoesToStderr(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	if err := writeLogsNoTaskMsg(&stderr, "fern"); err != nil {
		t.Fatalf("writeLogsNoTaskMsg: %v", err)
	}

	msg := stderr.String()
	if !strings.Contains(msg, `No active task on "fern".`) {
		t.Errorf("stderr = %q, want sprite-specific idle message", msg)
	}
	if !strings.Contains(msg, "dispatch log is empty") {
		t.Errorf("stderr = %q, want explanation that no logs are available", msg)
	}
	if !strings.Contains(msg, "bb status fern") {
		t.Errorf("stderr = %q, want next-step guidance", msg)
	}
}

// TestLogsCmdHasJSONFlag verifies --json flag is registered.
func TestLogsCmdHasJSONFlag(t *testing.T) {
	t.Parallel()

	cmd := newLogsCmd()
	f := cmd.Flags().Lookup("json")
	if f == nil {
		t.Fatal("--json flag not registered on logs command")
	}
	if f.DefValue != "false" {
		t.Fatalf("--json default = %q, want %q", f.DefValue, "false")
	}
}

func TestSpriteHasRunningAgentWithRunnerUsesDispatchLoopExitContract(t *testing.T) {
	t.Parallel()

	idle, err := spriteHasRunningAgentWithRunner(context.Background(), (&fakeSpriteScriptRunner{exitCode: 0}).run, "/tmp/ws")
	if err != nil {
		t.Fatalf("idle check error = %v", err)
	}
	if idle {
		t.Fatal("idle check reported active agent")
	}

	active, err := spriteHasRunningAgentWithRunner(context.Background(), (&fakeSpriteScriptRunner{exitCode: 1}).run, "/tmp/ws")
	if err != nil {
		t.Fatalf("active check error = %v", err)
	}
	if !active {
		t.Fatal("active check reported idle agent")
	}
}
