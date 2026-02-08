package events

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcherEmitsAppendedEvents(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	watcher, err := NewWatcher(WatcherConfig{
		Paths:        []string{path},
		PollInterval: 10 * time.Millisecond,
		StartAtEnd:   true,
	})
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}
	eventsCh, cancelSub := watcher.Subscribe(1)
	defer cancelSub()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- watcher.Run(ctx) }()
	time.Sleep(30 * time.Millisecond)

	if err := appendEvent(path, DispatchEvent{
		Meta: Meta{TS: time.Now().UTC(), SpriteName: "bramble", EventKind: KindDispatch},
		Task: "Fix auth",
	}); err != nil {
		t.Fatalf("appendEvent() error = %v", err)
	}

	event := waitForEvent(t, eventsCh, time.Second)
	if event.Kind() != KindDispatch {
		t.Fatalf("kind = %q, want %q", event.Kind(), KindDispatch)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestWatcherFilterAndFanout(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	start := time.Now().UTC().Add(-time.Minute)
	end := start.Add(2 * time.Minute)
	filter := Chain(ByKind(KindError), BySprite("bramble"), ByTimeRange(start, end))

	watcher, err := NewWatcher(WatcherConfig{
		Paths:        []string{path},
		PollInterval: 10 * time.Millisecond,
		Filter:       filter,
		StartAtEnd:   true,
	})
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}
	subA, cancelA := watcher.Subscribe(2)
	defer cancelA()
	subB, cancelB := watcher.Subscribe(2)
	defer cancelB()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- watcher.Run(ctx) }()
	time.Sleep(30 * time.Millisecond)

	if err := appendEvent(path, DispatchEvent{
		Meta: Meta{TS: time.Now().UTC(), SpriteName: "bramble", EventKind: KindDispatch},
		Task: "ignored",
	}); err != nil {
		t.Fatalf("append dispatch error = %v", err)
	}
	if err := appendEvent(path, ErrorEvent{
		Meta:    Meta{TS: start.Add(time.Second), SpriteName: "bramble", EventKind: KindError},
		Code:    "boom",
		Message: "runtime failure",
	}); err != nil {
		t.Fatalf("append error event error = %v", err)
	}

	gotA := waitForEvent(t, subA, time.Second)
	gotB := waitForEvent(t, subB, time.Second)
	if gotA.Kind() != KindError || gotB.Kind() != KindError {
		t.Fatalf("fanout kind mismatch: %q / %q", gotA.Kind(), gotB.Kind())
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestWatcherInvalidLineReturnsError(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte("not-json\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	watcher, err := NewWatcher(WatcherConfig{
		Paths:        []string{path},
		PollInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}

	err = watcher.Run(context.Background())
	if err == nil {
		t.Fatal("Run() expected decode error")
	}
}

func TestWatcherSubscribeCancelClosesChannel(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	watcher, err := NewWatcher(WatcherConfig{Paths: []string{path}})
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}
	ch, cancelSub := watcher.Subscribe(1)
	cancelSub()

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("channel should be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for closed channel")
	}
}

func TestWatcherValidation(t *testing.T) {
	t.Parallel()

	_, err := NewWatcher(WatcherConfig{})
	if err == nil {
		t.Fatal("NewWatcher() expected validation error")
	}
}

func appendEvent(path string, event Event) error {
	payload, err := MarshalEvent(event)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	if _, err := file.Write(payload); err != nil {
		return err
	}
	_, err = file.Write([]byte{'\n'})
	return err
}

func waitForEvent(t *testing.T, ch <-chan Event, timeout time.Duration) Event {
	t.Helper()

	select {
	case event, ok := <-ch:
		if !ok {
			t.Fatal("channel closed before event")
		}
		return event
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for event after %s", timeout)
		return nil
	}
}

func TestWatcherRunContextCanceled(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	watcher, err := NewWatcher(WatcherConfig{
		Paths:        []string{path},
		PollInterval: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = watcher.Run(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() = %v", err)
	}
}
