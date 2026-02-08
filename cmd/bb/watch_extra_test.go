package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/misty-step/bitterblossom/pkg/events"
)

func TestWatchFormattingHelpers(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 2, 7, 20, 0, 0, 0, time.UTC)
	event := events.DispatchEvent{
		Meta: events.Meta{TS: ts, SpriteName: "bramble", EventKind: events.KindDispatch},
		Task: "fix",
	}
	line := formatEvent(event)
	if !strings.Contains(line, "bramble") || !strings.Contains(line, "dispatch") {
		t.Fatalf("formatEvent() = %q", line)
	}

	signal := events.Signal{
		At:          ts,
		Source:      "bramble",
		Severity:    events.SeverityWarning,
		Description: "stalled",
	}
	signalLine := formatSignal(signal)
	if !strings.Contains(signalLine, "warning") || !strings.Contains(signalLine, "stalled") {
		t.Fatalf("formatSignal() = %q", signalLine)
	}

	recent := appendRecent(nil, "a", 2)
	recent = appendRecent(recent, "b", 2)
	recent = appendRecent(recent, "c", 2)
	if len(recent) != 2 || recent[0] != "b" || recent[1] != "c" {
		t.Fatalf("appendRecent() = %#v", recent)
	}
}

func TestRenderWatchAndSeverityFilter(t *testing.T) {
	t.Parallel()

	snapshot := events.Snapshot{
		Start:         time.Date(2026, 2, 7, 19, 0, 0, 0, time.UTC),
		End:           time.Date(2026, 2, 7, 19, 30, 0, 0, time.UTC),
		TotalEvents:   4,
		UniqueSprites: 1,
		EventsPerMin:  2,
		ErrorRate:     0.25,
		Uptime:        0.5,
		BySprite: map[string]events.SpriteStats{
			"bramble": {
				Sprite:      "bramble",
				TotalEvents: 4,
				ErrorRate:   0.25,
				Uptime:      0.5,
				LastEventAt: time.Date(2026, 2, 7, 19, 29, 0, 0, time.UTC),
			},
		},
	}

	var out bytes.Buffer
	if err := renderWatch(&out, snapshot, []string{"entry"}); err != nil {
		t.Fatalf("renderWatch() error = %v", err)
	}
	if !strings.Contains(out.String(), "sprites:") || !strings.Contains(out.String(), "recent:") {
		t.Fatalf("render output = %q", out.String())
	}

	filter, err := buildSeverityFilter([]string{"warn,critical"})
	if err != nil {
		t.Fatalf("buildSeverityFilter() error = %v", err)
	}
	if !filter.match(events.SeverityWarning) || !filter.match(events.SeverityCritical) {
		t.Fatalf("severity matcher missing expected values")
	}
	if filter.match(events.SeverityInfo) {
		t.Fatalf("severity matcher should exclude info")
	}

	if _, err := buildSeverityFilter([]string{"invalid"}); err == nil {
		t.Fatal("buildSeverityFilter() expected error for invalid input")
	}
}

func TestDrainWatchBatchPaths(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 7, 20, 0, 0, 0, time.UTC)
	agg := events.NewAggregator(events.AggregatorConfig{
		Window:       10 * time.Minute,
		GapThreshold: time.Minute,
		Now:          func() time.Time { return now },
	})

	engine := events.NewSignalEngine(func() time.Time { return now }, testSignalDetector{
		observe: []events.Signal{{Name: "obs", Severity: events.SeverityCritical, Source: "bramble", Description: "obs", At: now}},
		tick:    []events.Signal{{Name: "tick", Severity: events.SeverityWarning, Source: "bramble", Description: "tick", At: now}},
	})

	// `drainWatchBatch` uses a non-blocking select with `default`, so receive-vs-default is nondeterministic.
	// Retry to ensure we exercise event+signal JSON envelopes at least once.
	foundEventEnvelope := false
	for i := 0; i < 30 && !foundEventEnvelope; i++ {
		ch := make(chan events.Event, 1)
		ch <- events.DispatchEvent{
			Meta: events.Meta{TS: now, SpriteName: "bramble", EventKind: events.KindDispatch},
			Task: "task",
		}

		var out bytes.Buffer
		if err := drainWatchBatch(&out, ch, agg, engine, true, severityMatcher{}); err != nil {
			t.Fatalf("drainWatchBatch(json) error = %v", err)
		}
		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		for _, line := range lines {
			var payload map[string]any
			if err := json.Unmarshal([]byte(line), &payload); err != nil {
				t.Fatalf("invalid json line %q: %v", line, err)
			}
			if payload["type"] == "event" {
				foundEventEnvelope = true
				break
			}
		}
	}
	if !foundEventEnvelope {
		t.Fatal("expected at least one event envelope after retries")
	}

	ch := make(chan events.Event)
	var out bytes.Buffer
	if err := drainWatchBatch(&out, ch, agg, engine, false, severityMatcher{events.SeverityCritical: struct{}{}}); err != nil {
		t.Fatalf("drainWatchBatch(text) error = %v", err)
	}
	if !strings.Contains(out.String(), "recent:") {
		t.Fatalf("text output = %q", out.String())
	}
}

func TestWatchCommandOnceTextAndRealtime(t *testing.T) {
	t.Parallel()

	t.Run("once text", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "events.jsonl")
		if err := writeEventsFile(path, events.DispatchEvent{
			Meta: events.Meta{TS: time.Now().UTC(), SpriteName: "bramble", EventKind: events.KindDispatch},
			Task: "once",
		}); err != nil {
			t.Fatalf("writeEventsFile() error = %v", err)
		}

		var out bytes.Buffer
		if err := run(context.Background(), []string{
			"watch",
			"--file", path,
			"--once",
			"--start-at-end=false",
		}, &out, &bytes.Buffer{}); err != nil {
			t.Fatalf("run(watch --once) error = %v", err)
		}
		if !strings.Contains(out.String(), "recent:") {
			t.Fatalf("watch once text output = %q", out.String())
		}
	})

	t.Run("realtime loop", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "events.jsonl")
		if err := writeEventsFile(path); err != nil {
			t.Fatalf("writeEventsFile() error = %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		defer cancel()

		var out bytes.Buffer
		done := make(chan error, 1)
		go func() {
			done <- run(ctx, []string{
				"watch",
				"--file", path,
				"--poll-interval", "10ms",
				"--refresh", "30ms",
			}, &out, &bytes.Buffer{})
		}()

		time.Sleep(60 * time.Millisecond)
		if err := writeEventsFile(path, events.ErrorEvent{
			Meta:    events.Meta{TS: time.Now().UTC(), SpriteName: "bramble", EventKind: events.KindError},
			Message: "boom",
		}); err != nil {
			t.Fatalf("append event error = %v", err)
		}

		if err := <-done; err != nil {
			t.Fatalf("run(watch realtime) error = %v", err)
		}
		if !strings.Contains(out.String(), "bb watch") {
			t.Fatalf("watch realtime output = %q", out.String())
		}
	})
}

type testSignalDetector struct {
	observe []events.Signal
	tick    []events.Signal
}

func (d testSignalDetector) Observe(events.Event) []events.Signal { return d.observe }
func (d testSignalDetector) Tick(time.Time) []events.Signal       { return d.tick }
