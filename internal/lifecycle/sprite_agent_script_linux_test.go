//go:build linux

package lifecycle

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func repoRootFromThisFile(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// internal/lifecycle/... -> repo root is ../..
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runSpriteAgent(t *testing.T, workspace, fakeClaudeScript string, env map[string]string) (string, int) {
	t.Helper()

	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not available")
	}

	root := repoRootFromThisFile(t)
	scriptPath := filepath.Join(root, "scripts", "sprite-agent.sh")

	binDir := filepath.Join(workspace, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	writeExecutable(t, filepath.Join(binDir, "claude"), fakeClaudeScript)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, scriptPath)
	cmd.Dir = root
	cmd.Env = append([]string{}, os.Environ()...)
	cmd.Env = append(cmd.Env, "PATH="+binDir+":"+os.Getenv("PATH"))
	cmd.Env = append(cmd.Env, "WORKSPACE="+workspace)

	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("sprite-agent timed out: %v\noutput:\n%s", ctx.Err(), string(out))
	}

	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("sprite-agent exec error: %v\noutput:\n%s", err, string(out))
		}
	}
	return string(out), code
}

func TestSpriteAgentMaxTokensTerminates(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "PROMPT.md"), []byte("do stuff\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, code := runSpriteAgent(t, workspace, "#!/usr/bin/env bash\nset -euo pipefail\ncat >/dev/null\n# usage line the parser understands\nprintf '%s\\n' '{\"usage\":{\"input_tokens\":0,\"output_tokens\":20}}'\n", map[string]string{
		"MAX_ITERATIONS":     "50",
		"MAX_TOKENS":         "10",
		"MAX_TIME_SEC":       "9999",
		"ERROR_REPEAT_COUNT": "0",
		"HEALTH_INTERVAL":    "9999",
		"HEARTBEAT_INTERVAL": "9999",
		"PROGRESS_INTERVAL":  "9999",
		"PUSH_INTERVAL":      "9999",
		"LOOP_SLEEP_SEC":     "0",
		"SHUTDOWN_GRACE_SEC": "1",
	})
	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero\noutput:\n%s", out)
	}

	eventsPath := filepath.Join(workspace, "logs", "agent.jsonl")
	eventsRaw, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("read %s: %v", eventsPath, err)
	}
	if !strings.Contains(string(eventsRaw), "\"event\":\"task_failed\"") || !strings.Contains(string(eventsRaw), "\"reason\":\"max_tokens\"") {
		t.Fatalf("expected task_failed max_tokens event, got:\n%s", string(eventsRaw))
	}
	if _, err := os.Stat(filepath.Join(workspace, "ralph.log")); err != nil {
		t.Fatalf("expected ralph.log preserved: %v", err)
	}
}

func TestSpriteAgentErrorLoopTerminates(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "PROMPT.md"), []byte("do stuff\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, code := runSpriteAgent(t, workspace, "#!/usr/bin/env bash\nset -euo pipefail\ncat >/dev/null\nprintf '%s\\n' '{\"type\":\"error\",\"message\":\"boom\"}'\n", map[string]string{
		"MAX_ITERATIONS":     "10",
		"MAX_TOKENS":         "999999",
		"MAX_TIME_SEC":       "9999",
		"ERROR_REPEAT_COUNT": "3",
		"ERROR_WINDOW_LINES": "50",
		"HEALTH_INTERVAL":    "9999",
		"HEARTBEAT_INTERVAL": "9999",
		"PROGRESS_INTERVAL":  "9999",
		"PUSH_INTERVAL":      "9999",
		"LOOP_SLEEP_SEC":     "0",
		"SHUTDOWN_GRACE_SEC": "1",
	})
	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero\noutput:\n%s", out)
	}

	eventsPath := filepath.Join(workspace, "logs", "agent.jsonl")
	eventsRaw, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("read %s: %v", eventsPath, err)
	}
	if !strings.Contains(string(eventsRaw), "\"event\":\"task_failed\"") || !strings.Contains(string(eventsRaw), "\"reason\":\"error_loop\"") {
		t.Fatalf("expected task_failed error_loop event, got:\n%s", string(eventsRaw))
	}
}

func TestSpriteAgentMaxTimeTerminates(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "PROMPT.md"), []byte("do stuff\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, code := runSpriteAgent(t, workspace, "#!/usr/bin/env bash\nset -euo pipefail\ncat >/dev/null\nsleep 5\nprintf '%s\\n' '{\"usage\":{\"input_tokens\":0,\"output_tokens\":1}}'\n", map[string]string{
		"MAX_ITERATIONS":     "50",
		"MAX_TOKENS":         "999999",
		"MAX_TIME_SEC":       "1",
		"ERROR_REPEAT_COUNT": "0",
		"HEALTH_INTERVAL":    "0",
		"HEARTBEAT_INTERVAL": "9999",
		"PROGRESS_INTERVAL":  "9999",
		"PUSH_INTERVAL":      "9999",
		"LOOP_SLEEP_SEC":     "1",
		"SHUTDOWN_GRACE_SEC": "1",
	})
	if code == 0 {
		t.Fatalf("exit code = 0, want non-zero\noutput:\n%s", out)
	}

	eventsPath := filepath.Join(workspace, "logs", "agent.jsonl")
	eventsRaw, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("read %s: %v", eventsPath, err)
	}
	if !strings.Contains(string(eventsRaw), "\"event\":\"task_failed\"") || !strings.Contains(string(eventsRaw), "\"reason\":\"max_time\"") {
		t.Fatalf("expected task_failed max_time event, got:\n%s", string(eventsRaw))
	}
}

func TestSpriteAgentPIDFileCleanup(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "PROMPT.md"), []byte("do stuff\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Claude exits immediately; ralph finishes and cleans up.
	_, _ = runSpriteAgent(t, workspace, "#!/usr/bin/env bash\ncat >/dev/null\nexit 0\n", map[string]string{
		"MAX_ITERATIONS":     "1",
		"MAX_TOKENS":         "999999",
		"MAX_TIME_SEC":       "9999",
		"ERROR_REPEAT_COUNT": "0",
		"HEALTH_INTERVAL":    "9999",
		"HEARTBEAT_INTERVAL": "9999",
		"PROGRESS_INTERVAL":  "9999",
		"PUSH_INTERVAL":      "9999",
		"LOOP_SLEEP_SEC":     "0",
		"SHUTDOWN_GRACE_SEC": "1",
	})

	pidFile := filepath.Join(workspace, ".ralph.pid")
	if _, err := os.Stat(pidFile); err == nil {
		t.Fatal("PID file should be removed after clean exit")
	}
}

func TestSpriteAgentExitTrapKillsClaude(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "PROMPT.md"), []byte("do stuff\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Fake claude that writes its PID and sleeps forever.
	// Ralph should kill it via the EXIT trap when SIGTERM arrives.
	markerFile := filepath.Join(workspace, "claude.pid")
	fakeClaudeScript := "#!/usr/bin/env bash\ncat >/dev/null\nprintf '%d' $$ > " + markerFile + "\nsleep 60\n"

	root := repoRootFromThisFile(t)
	scriptPath := filepath.Join(root, "scripts", "sprite-agent.sh")
	binDir := filepath.Join(workspace, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(binDir, "claude"), fakeClaudeScript)

	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, scriptPath)
	cmd.Dir = root
	cmd.Env = append([]string{}, os.Environ()...)
	cmd.Env = append(cmd.Env, "PATH="+binDir+":"+os.Getenv("PATH"))
	cmd.Env = append(cmd.Env, "WORKSPACE="+workspace)
	cmd.Env = append(cmd.Env, "MAX_ITERATIONS=50")
	cmd.Env = append(cmd.Env, "MAX_TOKENS=999999")
	cmd.Env = append(cmd.Env, "MAX_TIME_SEC=9999")
	cmd.Env = append(cmd.Env, "ERROR_REPEAT_COUNT=0")
	cmd.Env = append(cmd.Env, "HEALTH_INTERVAL=9999")
	cmd.Env = append(cmd.Env, "HEARTBEAT_INTERVAL=9999")
	cmd.Env = append(cmd.Env, "PROGRESS_INTERVAL=9999")
	cmd.Env = append(cmd.Env, "PUSH_INTERVAL=9999")
	cmd.Env = append(cmd.Env, "LOOP_SLEEP_SEC=1")
	cmd.Env = append(cmd.Env, "SHUTDOWN_GRACE_SEC=2")

	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Wait for Claude to start (PID marker file appears).
	var claudePID string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(markerFile)
		if err == nil && len(raw) > 0 {
			claudePID = strings.TrimSpace(string(raw))
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if claudePID == "" {
		_ = cmd.Process.Kill()
		t.Fatal("fake claude never wrote PID marker")
	}

	// Kill ralph with SIGTERM; EXIT trap should kill Claude.
	_ = cmd.Process.Signal(syscall.SIGTERM)
	_ = cmd.Wait()

	// cmd.Wait() blocks until the EXIT trap completes, so no sleep needed.

	// Verify Claude process is dead.
	checkCmd := exec.Command("kill", "-0", claudePID)
	if checkCmd.Run() == nil {
		// Process still alive — cleanup failed.
		_ = exec.Command("kill", "-9", claudePID).Run() // best-effort cleanup; test already failing
		t.Fatal("Claude process survived ralph death — EXIT trap failed to kill it")
	}

	// Verify terminal event was emitted.
	eventsPath := filepath.Join(workspace, "logs", "agent.jsonl")
	eventsRaw, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	if !strings.Contains(string(eventsRaw), "\"event\":\"task_failed\"") {
		t.Fatalf("expected task_failed event from EXIT trap, got:\n%s", string(eventsRaw))
	}

	// Verify PID file was cleaned up.
	pidFile := filepath.Join(workspace, ".ralph.pid")
	if _, err := os.Stat(pidFile); err == nil {
		t.Fatal("PID file should be removed after EXIT trap")
	}
}
