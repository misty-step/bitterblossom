package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/misty-step/bitterblossom/pkg/events"
)

func TestWatchOnceJSON(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "events.jsonl")
	if err := writeEventsFile(file,
		events.DispatchEvent{
			Meta: events.Meta{TS: time.Now().UTC(), SpriteName: "bramble", EventKind: events.KindDispatch},
			Task: "Fix auth",
		},
		events.ErrorEvent{
			Meta:    events.Meta{TS: time.Now().UTC(), SpriteName: "bramble", EventKind: events.KindError},
			Code:    "build_failed",
			Message: "build failed",
		},
	); err != nil {
		t.Fatalf("writeEventsFile() error = %v", err)
	}

	var out bytes.Buffer
	err := run(context.Background(), []string{
		"watch",
		"--file", file,
		"--once",
		"--start-at-end=false",
		"--json",
		"--sprite", "bramble",
		"--type", "dispatch,error",
		"--severity", "critical",
	}, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(watch) error = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 JSON lines, got %d", len(lines))
	}

	eventCount := 0
	signalCount := 0
	for _, line := range lines {
		var envelope map[string]any
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			t.Fatalf("invalid JSON line %q: %v", line, err)
		}
		switch envelope["type"] {
		case "event":
			eventCount++
		case "signal":
			signalCount++
		}
	}
	if eventCount != 2 {
		t.Fatalf("event count = %d, want 2", eventCount)
	}
	if signalCount == 0 {
		t.Fatal("expected at least one signal")
	}
}

func TestWatchValidation(t *testing.T) {
	t.Parallel()

	err := run(context.Background(), []string{"watch", "--once"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("watch without --file should fail")
	}
}

func writeEventsFile(path string, input ...events.Event) (err error) {
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	emitter, err := events.NewEmitter(file)
	if err != nil {
		return err
	}
	for _, event := range input {
		if err := emitter.Emit(event); err != nil {
			return err
		}
	}
	return nil
}
