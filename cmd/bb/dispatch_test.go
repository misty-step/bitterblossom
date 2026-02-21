package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func TestDispatchTextMessageHandlerWritesStructuredOutput(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	handler := newDispatchTextMessageHandler(&stdout, &stderr)

	handler([]byte(`{"type":"stdout","data":"hello\n"}`))
	handler([]byte(`{"type":"stderr","data":"warn\n"}`))
	handler([]byte(`{"type":"error","error":"boom\n"}`))
	handler([]byte(`{"type":"info","data":"note\n"}`))

	if got := stdout.String(); got != "hello\nnote\n" {
		t.Fatalf("stdout = %q, want %q", got, "hello\nnote\n")
	}
	if got := stderr.String(); got != "warn\nboom\n" {
		t.Fatalf("stderr = %q, want %q", got, "warn\nboom\n")
	}
}

func TestDispatchTextMessageHandlerIgnoresControlFrames(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	handler := newDispatchTextMessageHandler(&stdout, &stderr)

	handler([]byte(`control:{"type":"op.complete","id":"1"}`))
	handler([]byte(`{"type":"session_info","session_id":"abc"}`))
	handler([]byte(`{"type":"exit","exit_code":0}`))

	if stdout.Len() != 0 {
		t.Fatalf("stdout should be empty, got %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr should be empty, got %q", stderr.String())
	}
}

func TestDispatchTextMessageHandlerWritesPlainTextFrames(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	handler := newDispatchTextMessageHandler(&stdout, &stderr)

	handler([]byte("plain frame"))

	if got := stdout.String(); got != "plain frame\n" {
		t.Fatalf("stdout = %q, want %q", got, "plain frame\n")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr should be empty, got %q", stderr.String())
	}
}

func TestActivityWriterMarksWrites(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	marks := 0
	w := &activityWriter{
		out: &out,
		mark: func() {
			marks++
		},
	}

	n, err := w.Write([]byte("ok"))
	if err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	if n != 2 {
		t.Fatalf("write length = %d, want 2", n)
	}
	if marks != 1 {
		t.Fatalf("marks = %d, want 1", marks)
	}
	if !strings.Contains(out.String(), "ok") {
		t.Fatalf("output = %q, want to contain %q", out.String(), "ok")
	}
}

func TestOffRailsDetectorEmitsWarningOnSilence(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	var out lockedBuffer
	d := newOffRailsDetector(offRailsConfig{
		SilenceAbort:  time.Hour,
		SilenceWarn:   10 * time.Millisecond,
		CheckInterval: 10 * time.Millisecond,
		Cancel:        cancel,
		Alert:         &out,
	})
	d.start()
	defer d.stop()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if strings.Contains(out.String(), "[off-rails] no output for") {
			// Ensure dispatch context was NOT cancelled (warn, not abort)
			select {
			case <-ctx.Done():
				t.Fatal("context should not be cancelled for warning")
			default:
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("expected warning line, got %q", out.String())
}

func TestGraceFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		timeout time.Duration
		want    time.Duration
	}{
		{1 * time.Second, 30 * time.Second},   // floor: 1s/4 = 250ms < 30s
		{2 * time.Minute, 30 * time.Second},   // floor: 2m/4 = 30s = 30s
		{4 * time.Minute, 1 * time.Minute},    // proportional: 4m/4 = 1m
		{20 * time.Minute, 5 * time.Minute},   // cap: 20m/4 = 5m = cap
		{2 * time.Hour, 5 * time.Minute},      // cap: 2h/4 = 30m > 5m cap
		{24 * time.Hour, 5 * time.Minute},     // cap: 24h/4 = 6h > 5m cap
	}
	for _, tt := range tests {
		got := graceFor(tt.timeout)
		if got != tt.want {
			t.Errorf("graceFor(%v) = %v, want %v", tt.timeout, got, tt.want)
		}
	}
}

type fakeSpriteScriptRunner struct {
	out         []byte
	exitCode    int
	err         error
	called      bool
	gotDeadline bool
	script      string
}

func (r *fakeSpriteScriptRunner) run(ctx context.Context, script string) ([]byte, int, error) {
	r.called = true
	_, r.gotDeadline = ctx.Deadline()
	r.script = script
	return r.out, r.exitCode, r.err
}

func TestEnsureNoActiveDispatchLoop_AllowsIdle(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{out: nil, exitCode: 0, err: nil}
	if err := ensureNoActiveDispatchLoopWithRunner(context.Background(), r.run); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !r.called {
		t.Fatal("runner should be called")
	}
	if !r.gotDeadline {
		t.Fatal("runner ctx should have a deadline (timeout)")
	}
	if !strings.Contains(r.script, "pgrep -af") {
		t.Fatalf("script = %q, want to contain %q", r.script, "pgrep -af")
	}
	if !strings.Contains(r.script, "[r]alph") {
		t.Fatalf("script = %q, want to contain %q", r.script, "[r]alph")
	}
	if strings.Contains(r.script, "claude") || strings.Contains(r.script, "opencode") {
		t.Fatalf("script = %q, want ralph-only busy check", r.script)
	}
}

func TestEnsureNoActiveDispatchLoop_BlocksWhenBusy(t *testing.T) {
	t.Parallel()

	const busy = "1234 bash /home/sprite/workspace/.ralph.sh\n"
	r := &fakeSpriteScriptRunner{out: []byte(busy), exitCode: 1, err: nil}
	err := ensureNoActiveDispatchLoopWithRunner(context.Background(), r.run)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "active dispatch loop detected") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "active dispatch loop detected")
	}
	if !strings.Contains(err.Error(), strings.TrimSpace(busy)) {
		t.Fatalf("err = %q, want to contain %q", err.Error(), strings.TrimSpace(busy))
	}
}

func TestEnsureNoActiveDispatchLoop_WrapsRunnerError(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{out: nil, exitCode: 0, err: errors.New("network")}
	err := ensureNoActiveDispatchLoopWithRunner(context.Background(), r.run)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "check dispatch loop") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "check dispatch loop")
	}
	if !strings.Contains(err.Error(), "network") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "network")
	}
}

func TestEnsureNoActiveDispatchLoop_ErrorsOnUnexpectedOutputWhenIdle(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{out: []byte("unexpected garbage"), exitCode: 0, err: nil}
	err := ensureNoActiveDispatchLoopWithRunner(context.Background(), r.run)
	if err == nil {
		t.Fatal("expected error for exit 0 with output, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected output from idle check") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "unexpected output from idle check")
	}
}

func TestEnsureNoActiveDispatchLoop_ErrorsOnUnexpectedExitCode(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{out: []byte("syntax error"), exitCode: 2, err: nil}
	err := ensureNoActiveDispatchLoopWithRunner(context.Background(), r.run)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "exited 2") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "exited 2")
	}
	if !strings.Contains(err.Error(), "syntax error") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "syntax error")
	}
}

func TestHasTaskCompleteSignalReturnsTrue(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{exitCode: 0, out: nil, err: nil}
	completed, err := hasTaskCompleteSignalWithRunner(context.Background(), r.run, "/tmp/ws")
	if err != nil {
		t.Fatalf("hasTaskCompleteSignalWithRunner() error = %v", err)
	}
	if !completed {
		t.Fatal("expected completion signal to be present")
	}
}

func TestHasTaskCompleteSignalReturnsFalseWhenMissing(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{exitCode: 1, out: nil, err: nil}
	completed, err := hasTaskCompleteSignalWithRunner(context.Background(), r.run, "/tmp/ws")
	if err != nil {
		t.Fatalf("hasTaskCompleteSignalWithRunner() error = %v", err)
	}
	if completed {
		t.Fatal("expected completion signal to be missing")
	}
}

func TestHasTaskCompleteSignalReturnsErrorOnUnexpectedExitCode(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{exitCode: 2, out: []byte("failed"), err: nil}
	_, err := hasTaskCompleteSignalWithRunner(context.Background(), r.run, "/tmp/ws")
	if err == nil {
		t.Fatal("expected error for unexpected completion check exit code")
	}
	if !strings.Contains(err.Error(), "completion signal check exited 2") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "completion signal check exited 2")
	}
}

func TestHasTaskCompleteSignalReturnsErrorOnRunnerError(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{exitCode: 0, out: nil, err: errors.New("network")}
	_, err := hasTaskCompleteSignalWithRunner(context.Background(), r.run, "/tmp/ws")
	if err == nil {
		t.Fatal("expected error for runner failure")
	}
	if !strings.Contains(err.Error(), "check completion signal command failed") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "check completion signal command failed")
	}
	if !strings.Contains(err.Error(), "network") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "network")
	}
}

func TestIsDispatchLoopActive_ReturnsFalseWhenIdle(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{out: nil, exitCode: 0, err: nil}
	busy, err := isDispatchLoopActiveWithRunner(context.Background(), r.run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if busy {
		t.Fatal("expected idle (false), got busy")
	}
}

func TestIsDispatchLoopActive_ReturnsTrueWhenBusy(t *testing.T) {
	t.Parallel()

	const busyOut = "1234 bash /home/sprite/workspace/.ralph.sh\n"
	r := &fakeSpriteScriptRunner{out: []byte(busyOut), exitCode: 1, err: nil}
	busy, err := isDispatchLoopActiveWithRunner(context.Background(), r.run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !busy {
		t.Fatal("expected busy (true), got idle")
	}
}

func TestIsDispatchLoopActive_ErrorsOnRunnerFailure(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{out: nil, exitCode: 0, err: errors.New("network")}
	_, err := isDispatchLoopActiveWithRunner(context.Background(), r.run)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "check dispatch loop") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "check dispatch loop")
	}
}

func TestIsDispatchLoopActive_ErrorsOnUnexpectedExitCode(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{out: []byte("syntax error"), exitCode: 2, err: nil}
	_, err := isDispatchLoopActiveWithRunner(context.Background(), r.run)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "exited 2") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "exited 2")
	}
}

func TestHasNewCommitsReturnsTrue(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{exitCode: 0, out: []byte("abc123 feat: something\n"), err: nil}
	hasWork, err := hasNewCommitsWithRunner(context.Background(), r.run, "/tmp/ws")
	if err != nil {
		t.Fatalf("hasNewCommitsWithRunner() error = %v", err)
	}
	if !hasWork {
		t.Fatal("expected new commits to be present")
	}
}

func TestHasNewCommitsReturnsFalseWhenNone(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{exitCode: 1, out: nil, err: nil}
	hasWork, err := hasNewCommitsWithRunner(context.Background(), r.run, "/tmp/ws")
	if err != nil {
		t.Fatalf("hasNewCommitsWithRunner() error = %v", err)
	}
	if hasWork {
		t.Fatal("expected no new commits")
	}
}

func TestHasNewCommitsReturnsErrorOnRunnerFailure(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{exitCode: 0, out: nil, err: errors.New("network")}
	_, err := hasNewCommitsWithRunner(context.Background(), r.run, "/tmp/ws")
	if err == nil {
		t.Fatal("expected error for runner failure")
	}
	if !strings.Contains(err.Error(), "check new commits") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "check new commits")
	}
	if !strings.Contains(err.Error(), "network") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "network")
	}
}

func TestHasNewCommitsReturnsErrorOnUnexpectedExitCode(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{exitCode: 2, out: []byte("fatal: not a git repo"), err: nil}
	_, err := hasNewCommitsWithRunner(context.Background(), r.run, "/tmp/ws")
	if err == nil {
		t.Fatal("expected error for unexpected exit code")
	}
	if !strings.Contains(err.Error(), "new commits check exited 2") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "new commits check exited 2")
	}
}

func TestHasNewCommitsUsesWorkspace(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{exitCode: 0, out: []byte("abc123\n"), err: nil}
	if _, err := hasNewCommitsWithRunner(context.Background(), r.run, "/home/sprite/workspace/myrepo"); err != nil {
		t.Fatalf("hasNewCommitsWithRunner() error = %v", err)
	}
	if !strings.Contains(r.script, "/home/sprite/workspace/myrepo") {
		t.Fatalf("script = %q, want to contain workspace path", r.script)
	}
}

func TestHasNewCommitsHasDeadline(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{exitCode: 0, out: []byte("abc123\n"), err: nil}
	_, _ = hasNewCommitsWithRunner(context.Background(), r.run, "/tmp/ws")
	if !r.gotDeadline {
		t.Fatal("expected context to carry a deadline (15s timeout)")
	}
}

func TestHasTaskCompleteSignalUsesWorkspace(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{exitCode: 0, out: nil, err: nil}
	if _, err := hasTaskCompleteSignalWithRunner(context.Background(), r.run, "/home/sprite/workspace/myrepo"); err != nil {
		t.Fatalf("hasTaskCompleteSignalWithRunner() error = %v", err)
	}
	if !strings.Contains(r.script, "/home/sprite/workspace/myrepo") {
		t.Fatalf("script = %q, want to contain workspace path", r.script)
	}
}

func TestDispatchCmdHasDryRunFlag(t *testing.T) {
	t.Parallel()

	cmd := newDispatchCmd()
	f := cmd.Flags().Lookup("dry-run")
	if f == nil {
		t.Fatal("--dry-run flag not registered on dispatch command")
	}
	if f.DefValue != "false" {
		t.Fatalf("--dry-run default = %q, want %q", f.DefValue, "false")
	}
}

func TestDispatchCmdDryRunFlagCanBeSet(t *testing.T) {
	t.Parallel()

	cmd := newDispatchCmd()
	if err := cmd.Flags().Set("dry-run", "true"); err != nil {
		t.Fatalf("failed to set --dry-run: %v", err)
	}
	got, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		t.Fatalf("GetBool(dry-run) error: %v", err)
	}
	if !got {
		t.Fatal("expected --dry-run to be true after Set")
	}
}

func TestDispatchCmdHasPRCheckTimeoutFlag(t *testing.T) {
	t.Parallel()

	cmd := newDispatchCmd()
	f := cmd.Flags().Lookup("pr-check-timeout")
	if f == nil {
		t.Fatal("--pr-check-timeout flag not registered on dispatch command")
	}
	if f.DefValue != "0s" {
		t.Fatalf("--pr-check-timeout default = %q, want %q", f.DefValue, "0s")
	}
}

func TestWaitForPRChecksReturnsNilWhenDisabled(t *testing.T) {
	t.Parallel()

	// When prCheckTimeout == 0, waitForPRChecks should return nil immediately
	// without calling the runner.
	r := &fakeSpriteScriptRunner{exitCode: 1, out: nil, err: nil}
	var progress bytes.Buffer
	err := waitForPRChecksWithRunner(context.Background(), r.run, "/tmp/ws", "token", 0, 30*time.Second, &progress)
	if err != nil {
		t.Fatalf("expected nil error when disabled, got %v", err)
	}
	if r.called {
		t.Fatal("runner should not be called when pr-check-timeout is 0")
	}
}

func TestWaitForPRChecksReturnsNilOnImmediatePass(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{exitCode: 0, out: []byte("all checks pass\n"), err: nil}
	var progress bytes.Buffer
	err := waitForPRChecksWithRunner(context.Background(), r.run, "/tmp/ws", "token", time.Minute, time.Second, &progress)
	if err != nil {
		t.Fatalf("expected nil error on immediate pass, got %v", err)
	}
	if !r.called {
		t.Fatal("runner should be called")
	}
}

func TestWaitForPRChecksTimesOut(t *testing.T) {
	t.Parallel()

	// Script always returns pending (exit 1).
	r := &fakeSpriteScriptRunner{exitCode: 1, out: nil, err: nil}
	var progress bytes.Buffer
	err := waitForPRChecksWithRunner(context.Background(), r.run, "/tmp/ws", "token", 50*time.Millisecond, 10*time.Millisecond, &progress)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "pr-check-timeout") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "pr-check-timeout")
	}
}

func TestWaitForPRChecksEmitsProgress(t *testing.T) {
	t.Parallel()

	calls := 0
	// First call: pending. Second call: pass.
	customRunner := func(ctx context.Context, script string) ([]byte, int, error) {
		calls++
		if calls == 1 {
			return nil, 1, nil // pending
		}
		return []byte("pass\n"), 0, nil // pass
	}

	var progress bytes.Buffer
	err := waitForPRChecksWithRunner(context.Background(), customRunner, "/tmp/ws", "token", 5*time.Second, 10*time.Millisecond, &progress)
	if err != nil {
		t.Fatalf("expected nil on second pass, got %v", err)
	}
	if !strings.Contains(progress.String(), "[dispatch]") {
		t.Fatalf("progress = %q, want to contain %q", progress.String(), "[dispatch]")
	}
}

func TestWaitForPRChecksContextCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	r := &fakeSpriteScriptRunner{exitCode: 1, out: nil, err: nil}
	var progress bytes.Buffer
	err := waitForPRChecksWithRunner(ctx, r.run, "/tmp/ws", "token", time.Minute, time.Second, &progress)
	if err == nil {
		t.Fatal("expected error on cancelled context, got nil")
	}
}

func TestWaitForPRChecksUsesWorkspaceAndToken(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{exitCode: 0, out: []byte("pass\n"), err: nil}
	var progress bytes.Buffer
	if err := waitForPRChecksWithRunner(context.Background(), r.run, "/home/sprite/workspace/myrepo", "ghtoken123", time.Minute, time.Second, &progress); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(r.script, "/home/sprite/workspace/myrepo") {
		t.Fatalf("script = %q, want to contain workspace path", r.script)
	}
	if !strings.Contains(r.script, "ghtoken123") {
		t.Fatalf("script = %q, want to contain GH token", r.script)
	}
}

func TestWaitForPRChecksFatalErrorStopsImmediately(t *testing.T) {
	t.Parallel()

	calls := 0
	runner := func(_ context.Context, _ string) ([]byte, int, error) {
		calls++
		return []byte("gh: command not found\n"), 2, nil // fatal error
	}
	var progress bytes.Buffer
	err := waitForPRChecksWithRunner(context.Background(), runner, "/tmp/ws", "token",
		time.Minute, 10*time.Millisecond, &progress)
	if err == nil {
		t.Fatal("expected error on fatal exit code 2, got nil")
	}
	if calls > 1 {
		t.Fatalf("expected only 1 call on fatal error, got %d", calls)
	}
}

func TestWaitForPRChecksRetriesOnRunnerError(t *testing.T) {
	t.Parallel()

	calls := 0
	runner := func(_ context.Context, _ string) ([]byte, int, error) {
		calls++
		if calls == 1 {
			return nil, 0, errors.New("websocket closed")
		}
		return []byte("pass\n"), 0, nil // second call succeeds
	}
	var progress bytes.Buffer
	err := waitForPRChecksWithRunner(context.Background(), runner, "/tmp/ws", "token",
		5*time.Second, 10*time.Millisecond, &progress)
	if err != nil {
		t.Fatalf("expected nil after retry success, got %v", err)
	}
	if calls < 2 {
		t.Fatalf("expected at least 2 calls (error + retry), got %d", calls)
	}
	if !strings.Contains(progress.String(), "runner error") {
		t.Fatalf("progress = %q, want to contain %q", progress.String(), "runner error")
	}
}

func TestPRChecksScriptContainsExpectedCommands(t *testing.T) {
	t.Parallel()

	if !strings.Contains(prChecksScript, "gh pr checks HEAD") {
		t.Fatalf("prChecksScript missing 'gh pr checks HEAD':\n%s", prChecksScript)
	}
	if !strings.Contains(prChecksScript, "--exit-status") {
		t.Fatalf("prChecksScript missing '--exit-status':\n%s", prChecksScript)
	}
	if !strings.Contains(prChecksScript, "exit 2") {
		t.Fatalf("prChecksScript missing fatal exit code 2:\n%s", prChecksScript)
	}
}

func TestWaitForPRChecksInitialCheckBeforeTicker(t *testing.T) {
	t.Parallel()

	// pollInterval is longer than prCheckTimeout; without an initial check
	// the function would always time out without ever calling the runner.
	r := &fakeSpriteScriptRunner{exitCode: 0, out: []byte("pass\n"), err: nil}
	var progress bytes.Buffer
	err := waitForPRChecksWithRunner(context.Background(), r.run, "/tmp/ws", "token",
		50*time.Millisecond, // prCheckTimeout
		time.Hour,           // pollInterval (ticker never fires)
		&progress)
	if err != nil {
		t.Fatalf("expected nil (immediate pass before ticker), got %v", err)
	}
	if !r.called {
		t.Fatal("runner should be called even when pollInterval > prCheckTimeout")
	}
}
