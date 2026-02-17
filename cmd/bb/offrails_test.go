package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestOffRailsDetector_SilenceAbort(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	var alert bytes.Buffer
	d := newOffRailsDetector(offRailsConfig{
		SilenceAbort:  50 * time.Millisecond,
		SilenceWarn:   20 * time.Millisecond,
		CheckInterval: 10 * time.Millisecond,
		Cancel:        cancel,
		Alert:         &alert,
	})
	d.start()
	defer d.stop()

	select {
	case <-ctx.Done():
		cause := context.Cause(ctx)
		if !errors.Is(cause, errOffRails) {
			t.Fatalf("expected errOffRails, got: %v", cause)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for silence abort")
	}
}

func TestOffRailsDetector_ActivityPreventsAbort(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	var alert bytes.Buffer
	d := newOffRailsDetector(offRailsConfig{
		SilenceAbort:  100 * time.Millisecond,
		SilenceWarn:   50 * time.Millisecond,
		CheckInterval: 10 * time.Millisecond,
		Cancel:        cancel,
		Alert:         &alert,
	})
	d.start()
	defer d.stop()

	// Keep marking activity for 150ms (exceeds the 100ms abort threshold)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 20; i++ {
			d.markActivity()
			time.Sleep(10 * time.Millisecond)
		}
	}()
	<-done

	select {
	case <-ctx.Done():
		t.Fatal("context was cancelled despite continuous activity")
	default:
		// good
	}
}

func TestOffRailsDetector_ErrorLoop(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	var alert bytes.Buffer
	d := newOffRailsDetector(offRailsConfig{
		SilenceAbort:  time.Hour,
		ErrorRepeatN:  3,
		CheckInterval: time.Hour,
		Cancel:        cancel,
		Alert:         &alert,
	})

	d.recordError("Error: command not found: foo")
	d.recordError("Error: command not found: foo")
	d.recordError("Error: command not found: foo")

	select {
	case <-ctx.Done():
		cause := context.Cause(ctx)
		if !errors.Is(cause, errOffRails) {
			t.Fatalf("expected errOffRails, got: %v", cause)
		}
	default:
		t.Fatal("expected context to be cancelled after 3 repeated errors")
	}
}

func TestOffRailsDetector_DifferentErrorsNoAbort(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	var alert bytes.Buffer
	d := newOffRailsDetector(offRailsConfig{
		SilenceAbort:  time.Hour,
		ErrorRepeatN:  3,
		CheckInterval: time.Hour,
		Cancel:        cancel,
		Alert:         &alert,
	})

	d.recordError("Error: file not found")
	d.recordError("Error: permission denied")
	d.recordError("Error: timeout exceeded")

	select {
	case <-ctx.Done():
		t.Fatal("context should not be cancelled for different errors")
	default:
		// good
	}
}

func TestOffRailsDetector_WrapMarksActivity(t *testing.T) {
	_, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	var alert bytes.Buffer
	d := newOffRailsDetector(offRailsConfig{
		SilenceAbort:  time.Hour,
		CheckInterval: time.Hour,
		Cancel:        cancel,
		Alert:         &alert,
	})

	before := d.lastActivityNano.Load()
	time.Sleep(time.Millisecond)

	var buf bytes.Buffer
	w := d.wrap(&buf)
	_, _ = w.Write([]byte("hello"))

	after := d.lastActivityNano.Load()
	if after <= before {
		t.Fatal("wrap should update lastActivityNano on write")
	}
	if buf.String() != "hello" {
		t.Fatalf("expected %q, got %q", "hello", buf.String())
	}
}

func TestOffRailsDetector_SilenceDisabled(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	var alert bytes.Buffer
	d := newOffRailsDetector(offRailsConfig{
		SilenceAbort:  0, // disabled
		CheckInterval: 10 * time.Millisecond,
		Cancel:        cancel,
		Alert:         &alert,
	})
	d.start()
	defer d.stop()

	time.Sleep(50 * time.Millisecond)

	select {
	case <-ctx.Done():
		t.Fatal("context should not be cancelled when silence abort is disabled")
	default:
		// good
	}
}

func TestTruncateError(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"  ", ""},
		{"Error: foo", "Error: foo"},
		{strings.Repeat("x", 300), strings.Repeat("x", 200)},
	}
	for _, tt := range tests {
		got := truncateError(tt.input)
		if got != tt.want {
			t.Errorf("truncateError(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTruncateStr(t *testing.T) {
	if got := truncateStr("hello", 10); got != "hello" {
		t.Errorf("truncateStr short: got %q", got)
	}
	if got := truncateStr("hello world", 5); got != "hello..." {
		t.Errorf("truncateStr long: got %q", got)
	}
}
