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
	if !strings.Contains(err.Error(), "active dispatch loop detected:") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "active dispatch loop detected:")
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
