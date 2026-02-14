package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/misty-step/bitterblossom/internal/sprite"
	"github.com/misty-step/bitterblossom/pkg/events"
)

func TestWriteLogEventVariants(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 7, 20, 0, 0, 0, time.UTC)
	success := true
	cases := []events.Event{
		events.DispatchEvent{Meta: events.Meta{TS: now, SpriteName: "bramble", EventKind: events.KindDispatch}, Task: "task"},
		&events.DispatchEvent{Meta: events.Meta{TS: now, SpriteName: "bramble", EventKind: events.KindDispatch}, Task: "task"},
		events.ProgressEvent{
			Meta:     events.Meta{TS: now, SpriteName: "bramble", EventKind: events.KindProgress},
			Activity: "tool_call",
			Detail:   "exec_command",
			Success:  &success,
		},
		&events.ProgressEvent{
			Meta:     events.Meta{TS: now, SpriteName: "bramble", EventKind: events.KindProgress},
			Activity: "command_run",
			Detail:   "git status",
		},
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
	for _, needle := range []string{"task=task", "activity=tool_call", "detail=exec_command", "reason=blocked", "message=boom"} {
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

func TestFetchRemoteEvents(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	jsonl := marshalEventsToJSONL(t,
		events.DispatchEvent{
			Meta: events.Meta{TS: now.Add(-10 * time.Minute), SpriteName: "bramble", EventKind: events.KindDispatch},
			Task: "task-a",
		},
		events.ErrorEvent{
			Meta:    events.Meta{TS: now.Add(-5 * time.Minute), SpriteName: "bramble", EventKind: events.KindError},
			Message: "boom",
		},
	)

	cli := &sprite.MockSpriteCLI{
		ExecFn: func(_ context.Context, name, cmd string, _ []byte) (string, error) {
			if name == "bramble" {
				return jsonl, nil
			}
			return "", nil
		},
	}

	evts, offsets, err := fetchRemoteEvents(context.Background(), cli, []string{"bramble"}, nil)
	if err != nil {
		t.Fatalf("fetchRemoteEvents() error = %v", err)
	}
	if len(evts) != 2 {
		t.Fatalf("len(evts) = %d, want 2", len(evts))
	}
	if offsets["bramble"] != 2 {
		t.Fatalf("offsets[bramble] = %d, want 2", offsets["bramble"])
	}
}

func TestFetchRemoteEventsWithFilter(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	jsonl := marshalEventsToJSONL(t,
		events.DispatchEvent{
			Meta: events.Meta{TS: now, SpriteName: "bramble", EventKind: events.KindDispatch},
			Task: "task-a",
		},
		events.ErrorEvent{
			Meta:    events.Meta{TS: now, SpriteName: "bramble", EventKind: events.KindError},
			Message: "boom",
		},
	)

	cli := &sprite.MockSpriteCLI{
		ExecFn: func(context.Context, string, string, []byte) (string, error) {
			return jsonl, nil
		},
	}

	filter := events.ByKind(events.KindError)
	evts, offsets, err := fetchRemoteEvents(context.Background(), cli, []string{"bramble"}, filter)
	if err != nil {
		t.Fatalf("fetchRemoteEvents() error = %v", err)
	}
	// Filter reduces returned events but offsets count all lines
	if len(evts) != 1 {
		t.Fatalf("len(evts) = %d, want 1 (filtered)", len(evts))
	}
	if offsets["bramble"] != 2 {
		t.Fatalf("offsets[bramble] = %d, want 2 (total lines)", offsets["bramble"])
	}
}

func TestFetchRemoteEventsExecError(t *testing.T) {
	t.Parallel()

	cli := &sprite.MockSpriteCLI{
		ExecFn: func(context.Context, string, string, []byte) (string, error) {
			return "", fmt.Errorf("connection refused")
		},
	}

	_, _, err := fetchRemoteEvents(context.Background(), cli, []string{"bramble"}, nil)
	if err == nil {
		t.Fatal("expected error from exec failure")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("error = %q, want containing 'connection refused'", err.Error())
	}
}

func TestFollowRemoteEventsReceivesNewEvents(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	newJSONL := marshalEventsToJSONL(t,
		events.DispatchEvent{
			Meta: events.Meta{TS: now, SpriteName: "bramble", EventKind: events.KindDispatch},
			Task: "new-task",
		},
	)

	calls := 0
	cli := &sprite.MockSpriteCLI{
		ExecFn: func(context.Context, string, string, []byte) (string, error) {
			calls++
			if calls == 1 {
				return newJSONL, nil
			}
			return "", nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	var out bytes.Buffer
	var errBuf bytes.Buffer
	offsets := map[string]int{"bramble": 0}

	done := make(chan error, 1)
	go func() {
		done <- followRemoteEvents(ctx, &out, &errBuf, cli, []string{"bramble"}, nil, false, 10*time.Millisecond, offsets)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	if err := <-done; err != nil {
		t.Fatalf("followRemoteEvents() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "new-task") {
		t.Fatalf("output = %q, want containing new-task", output)
	}
	if offsets["bramble"] != 1 {
		t.Fatalf("offsets[bramble] = %d, want 1", offsets["bramble"])
	}
}

func TestFollowRemoteEventsLogsErrorsToStderr(t *testing.T) {
	t.Parallel()

	cli := &sprite.MockSpriteCLI{
		ExecFn: func(context.Context, string, string, []byte) (string, error) {
			return "", fmt.Errorf("connection timeout")
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	var out bytes.Buffer
	var errBuf bytes.Buffer
	offsets := map[string]int{"bramble": 0}

	done := make(chan error, 1)
	go func() {
		done <- followRemoteEvents(ctx, &out, &errBuf, cli, []string{"bramble"}, nil, false, 10*time.Millisecond, offsets)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	if err := <-done; err != nil {
		t.Fatalf("followRemoteEvents() error = %v", err)
	}

	stderr := errBuf.String()
	if !strings.Contains(stderr, "connection timeout") {
		t.Fatalf("stderr = %q, want containing 'connection timeout'", stderr)
	}
}

func TestFollowRemoteEventsClampsZeroInterval(t *testing.T) {
	t.Parallel()

	cli := &sprite.MockSpriteCLI{
		ExecFn: func(context.Context, string, string, []byte) (string, error) {
			return "", nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	var out bytes.Buffer
	var errBuf bytes.Buffer
	offsets := map[string]int{"bramble": 0}

	done := make(chan error, 1)
	go func() {
		// Zero interval should be clamped to default, not panic
		done <- followRemoteEvents(ctx, &out, &errBuf, cli, []string{"bramble"}, nil, false, 0, offsets)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	if err := <-done; err != nil {
		t.Fatalf("followRemoteEvents(interval=0) error = %v", err)
	}
}

func TestDefaultLogsDeps(t *testing.T) {
	t.Parallel()

	deps := defaultLogsDeps()
	cli := deps.newCLI("sprite", "test-org")
	if _, ok := cli.(sprite.CLI); !ok {
		t.Fatalf("newCLI returned %T, want sprite.CLI", cli)
	}
}
