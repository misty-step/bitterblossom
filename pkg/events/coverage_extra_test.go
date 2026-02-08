package events

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMetaGetterAliases(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 2, 7, 19, 0, 0, 0, time.UTC)
	meta := Meta{TS: ts, SpriteName: "bramble", EventKind: KindProgress}

	if !meta.GetTimestamp().Equal(ts) {
		t.Fatalf("GetTimestamp() = %v, want %v", meta.GetTimestamp(), ts)
	}
	if meta.GetSprite() != "bramble" {
		t.Fatalf("GetSprite() = %q", meta.GetSprite())
	}
	if meta.GetKind() != KindProgress {
		t.Fatalf("GetKind() = %q", meta.GetKind())
	}
}

func TestWatcherRunOncePublishesEvents(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := appendEvent(path, DispatchEvent{
		Meta: Meta{TS: time.Now().UTC(), SpriteName: "bramble", EventKind: KindDispatch},
		Task: "once",
	}); err != nil {
		t.Fatalf("appendEvent() error = %v", err)
	}

	watcher, err := NewWatcher(WatcherConfig{Paths: []string{path}})
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}
	ch, cancel := watcher.Subscribe(1)
	defer cancel()

	if err := watcher.RunOnce(); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	event := waitForEvent(t, ch, time.Second)
	if event.Kind() != KindDispatch {
		t.Fatalf("event.Kind() = %q, want %q", event.Kind(), KindDispatch)
	}
}

func TestWatcherPublishDropsOldestWhenSubscriberIsFull(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	now := time.Now().UTC()
	if err := appendEvent(path, DispatchEvent{
		Meta: Meta{TS: now, SpriteName: "bramble", EventKind: KindDispatch},
		Task: "first",
	}); err != nil {
		t.Fatalf("append first event: %v", err)
	}
	if err := appendEvent(path, DispatchEvent{
		Meta: Meta{TS: now.Add(time.Second), SpriteName: "bramble", EventKind: KindDispatch},
		Task: "second",
	}); err != nil {
		t.Fatalf("append second event: %v", err)
	}

	watcher, err := NewWatcher(WatcherConfig{Paths: []string{path}})
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}
	ch, cancel := watcher.Subscribe(1)
	defer cancel()

	if err := watcher.RunOnce(); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	got := waitForEvent(t, ch, time.Second)
	dispatch, ok := got.(*DispatchEvent)
	if !ok {
		t.Fatalf("event type = %T, want *DispatchEvent", got)
	}
	if dispatch.Task != "second" {
		t.Fatalf("Task = %q, want second (oldest dropped)", dispatch.Task)
	}
}

func TestAggregatorAddAllAndConsumeClosedChannel(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 7, 19, 0, 0, 0, time.UTC)
	current := now
	agg := NewAggregator(AggregatorConfig{
		Window:       10 * time.Minute,
		GapThreshold: time.Minute,
		Now:          func() time.Time { return current },
	})

	agg.AddAll([]Event{
		nil,
		DispatchEvent{Meta: Meta{TS: now, SpriteName: "bramble", EventKind: KindDispatch}, Task: "a"},
		ErrorEvent{Meta: Meta{TS: now.Add(time.Minute), SpriteName: "bramble", EventKind: KindError}, Message: "boom"},
	})
	current = now.Add(2 * time.Minute)
	if snapshot := agg.Snapshot(); snapshot.TotalEvents != 2 {
		t.Fatalf("Snapshot().TotalEvents = %d, want 2", snapshot.TotalEvents)
	}

	in := make(chan Event)
	close(in)
	if err := agg.Consume(context.Background(), in); err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
}

func TestSignalConstructorsDefaultValues(t *testing.T) {
	t.Parallel()

	defaults := DefaultSignalConfig()
	if defaults.Stall.Threshold <= 0 || defaults.RepeatedError.Threshold <= 0 {
		t.Fatalf("invalid defaults: %+v", defaults)
	}

	engine := NewSignalEngine(nil)
	if got := engine.Tick(); len(got) != 0 {
		t.Fatalf("empty detector engine Tick() = %v, want no signals", got)
	}

	stall := NewStallDetector(StallSignalConfig{}, nil)
	if stall.cfg.Threshold <= 0 {
		t.Fatalf("stall threshold should default, got %s", stall.cfg.Threshold)
	}
	if stall.cfg.Severity == "" {
		t.Fatalf("stall severity should default")
	}

	repeated := NewRepeatedErrorDetector(RepeatedErrorSignalConfig{})
	if repeated.cfg.Window <= 0 || repeated.cfg.Threshold <= 0 || repeated.cfg.Severity == "" {
		t.Fatalf("unexpected repeated-error defaults: %+v", repeated.cfg)
	}
}
