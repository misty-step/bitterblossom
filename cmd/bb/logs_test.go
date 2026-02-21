package main

import (
	"bytes"
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

	var stdout, stderr bytes.Buffer
	writeLogsNoTaskMsg(&stdout, &stderr)

	if stdout.Len() != 0 {
		t.Errorf("stdout must be empty, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "No active task") {
		t.Errorf("stderr = %q, want to contain %q", stderr.String(), "No active task")
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
