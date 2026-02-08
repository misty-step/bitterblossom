package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/misty-step/bitterblossom/pkg/events"
)

func TestWriteLogEventVariants(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 7, 20, 0, 0, 0, time.UTC)
	cases := []events.Event{
		events.DispatchEvent{Meta: events.Meta{TS: now, SpriteName: "bramble", EventKind: events.KindDispatch}, Task: "task"},
		&events.DispatchEvent{Meta: events.Meta{TS: now, SpriteName: "bramble", EventKind: events.KindDispatch}, Task: "task"},
		events.BlockedEvent{Meta: events.Meta{TS: now, SpriteName: "bramble", EventKind: events.KindBlocked}, Reason: "blocked"},
		&events.BlockedEvent{Meta: events.Meta{TS: now, SpriteName: "bramble", EventKind: events.KindBlocked}, Reason: "blocked"},
		events.ErrorEvent{Meta: events.Meta{TS: now, SpriteName: "bramble", EventKind: events.KindError}, Message: "boom"},
		&events.ErrorEvent{Meta: events.Meta{TS: now, SpriteName: "bramble", EventKind: events.KindError}, Message: "boom"},
	}

	var out bytes.Buffer
	for _, event := range cases {
		if err := writeLogEvent(&out, event, false); err != nil {
			t.Fatalf("writeLogEvent(%T) error = %v", event, err)
		}
	}
	text := out.String()
	for _, needle := range []string{"task=task", "reason=blocked", "message=boom"} {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in output %q", needle, text)
		}
	}

	out.Reset()
	if err := writeLogEvent(&out, cases[0], true); err != nil {
		t.Fatalf("writeLogEvent(json) error = %v", err)
	}
	if !strings.Contains(out.String(), `"type":"event"`) {
		t.Fatalf("json output = %q", out.String())
	}
}

func TestParseTimeRangeAdditionalCases(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 7, 20, 0, 0, 0, time.UTC)
	start, end, err := parseTimeRange(now, "10m", "2026-02-07T20:05:00Z")
	if err != nil {
		t.Fatalf("parseTimeRange() error = %v", err)
	}
	if start.IsZero() || end.IsZero() {
		t.Fatalf("expected non-zero range, got start=%v end=%v", start, end)
	}

	if _, _, err := parseTimeRange(now, "2026-02-07T20:10:00Z", "2026-02-07T20:05:00Z"); err == nil {
		t.Fatal("parseTimeRange() expected until-before-since error")
	}
	if _, _, err := parseTimeRange(now, "", "bad"); err == nil {
		t.Fatal("parseTimeRange() expected invalid until error")
	}
}

func TestReadHistoricalEventsErrorPath(t *testing.T) {
	t.Parallel()

	dirPath := t.TempDir()
	_, err := readHistoricalEvents([]string{dirPath}, nil)
	if err == nil {
		t.Fatal("readHistoricalEvents() expected read error for directory input")
	}
}
