package agent

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNDJSONSinkEmit(t *testing.T) {
	buf := &bytes.Buffer{}
	sink := NewNDJSONSink(buf, "thorn")
	sink.now = func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }

	if err := sink.Emit("heartbeat", map[string]any{"ok": true}); err != nil {
		t.Fatalf("emit returned error: %v", err)
	}

	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("expected output line")
	}

	var ev Event
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if ev.Sprite != "thorn" {
		t.Fatalf("sprite mismatch: got %q", ev.Sprite)
	}
	if ev.Event != "heartbeat" {
		t.Fatalf("event mismatch: got %q", ev.Event)
	}
	if ev.Timestamp != "2026-01-02T03:04:05Z" {
		t.Fatalf("timestamp mismatch: got %q", ev.Timestamp)
	}
}

func TestNDJSONSinkEmit_EmptyEvent(t *testing.T) {
	sink := NewNDJSONSink(&bytes.Buffer{}, "fern")
	if err := sink.Emit("", nil); err == nil {
		t.Fatal("expected error for empty event")
	}
}
