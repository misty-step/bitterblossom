package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	storeevents "github.com/misty-step/bitterblossom/internal/events"
	pkgevents "github.com/misty-step/bitterblossom/pkg/events"
)

func TestEventsCommandJSONOutput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logger, err := storeevents.NewLogger(storeevents.LoggerConfig{Dir: dir})
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}

	ts := time.Date(2026, 2, 12, 16, 0, 0, 0, time.UTC)
	if err := logger.Log(pkgevents.DispatchEvent{
		Meta: pkgevents.Meta{
			TS:         ts,
			SpriteName: "bramble",
			EventKind:  pkgevents.KindDispatch,
			Issue:      13,
		},
		Task: "ship issue 13",
	}); err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	var out bytes.Buffer
	if err := run(context.Background(), []string{
		"events",
		"--dir", dir,
		"--issue", "13",
		"--json",
	}, &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("run(events --json) error = %v", err)
	}

	got := out.String()
	if !strings.Contains(got, `"type":"event"`) {
		t.Fatalf("json output missing event envelope: %q", got)
	}
	if !strings.Contains(got, `"issue":13`) {
		t.Fatalf("json output missing issue field: %q", got)
	}
}

func TestEventsCommandFiltersByTypeAndSprite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logger, err := storeevents.NewLogger(storeevents.LoggerConfig{Dir: dir})
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	base := time.Date(2026, 2, 12, 17, 0, 0, 0, time.UTC)

	for _, event := range []pkgevents.Event{
		pkgevents.ProgressEvent{
			Meta: pkgevents.Meta{
				TS:         base,
				SpriteName: "bramble",
				EventKind:  pkgevents.KindProgress,
				Issue:      13,
			},
			Activity: "tool_call",
			Detail:   "apply_patch",
		},
		pkgevents.ErrorEvent{
			Meta: pkgevents.Meta{
				TS:         base.Add(time.Minute),
				SpriteName: "thorn",
				EventKind:  pkgevents.KindError,
				Issue:      13,
			},
			Message: "failed",
		},
	} {
		if err := logger.Log(event); err != nil {
			t.Fatalf("Log() error = %v", err)
		}
	}

	var out bytes.Buffer
	if err := run(context.Background(), []string{
		"events",
		"--dir", dir,
		"--sprite", "bramble",
		"--type", "progress",
	}, &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("run(events) error = %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "bramble") || !strings.Contains(got, "progress") {
		t.Fatalf("filtered output = %q", got)
	}
	if strings.Contains(got, "thorn") || strings.Contains(got, "error") {
		t.Fatalf("unexpected non-matching events in output: %q", got)
	}
}

