package main

import (
	"bytes"
	"context"
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
