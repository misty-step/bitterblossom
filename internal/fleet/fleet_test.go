package fleet

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStatusFromEvent(t *testing.T) {
	cases := []struct {
		event string
		want  string
	}{
		{"task_complete", "completed"},
		{"blocked", "blocked"},
		{"agent_shutdown", "stopped"},
		{"git_error", "error"},
		{"heartbeat", "active"},
	}
	for _, tc := range cases {
		if got := statusFromEvent(tc.event); got != tc.want {
			t.Fatalf("statusFromEvent(%q)=%q want %q", tc.event, got, tc.want)
		}
	}
}

func TestNormalizeStatus(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"COMPLETE", "completed"},
		{"BLOCKED", "blocked"},
		{"RUNNING", "running"},
		{"IDLE", "idle"},
		{"", "unknown"},
	}
	for _, tc := range cases {
		if got := normalizeStatus(tc.in); got != tc.want {
			t.Fatalf("normalizeStatus(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestLoadLatestEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.ndjson")
	content := `{"sprite":"thorn","event":"heartbeat","timestamp":"2026-01-01T00:00:00Z"}
{"sprite":"thorn","event":"blocked","timestamp":"2026-01-01T00:02:00Z"}
{"sprite":"fern","event":"heartbeat","timestamp":"2026-01-01T00:01:00Z"}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	events, err := loadLatestEvents(path)
	if err != nil {
		t.Fatalf("loadLatestEvents error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 sprites, got %d", len(events))
	}
	if events["thorn"].Event != "blocked" {
		t.Fatalf("latest thorn event mismatch: %s", events["thorn"].Event)
	}
	if !events["fern"].Time.Equal(time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)) {
		t.Fatalf("fern timestamp mismatch: %v", events["fern"].Time)
	}
}
