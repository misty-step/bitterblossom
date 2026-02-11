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

	"github.com/misty-step/bitterblossom/internal/sprite"
	"github.com/misty-step/bitterblossom/pkg/events"
)

func TestLogsHistoricalFilter(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "events.jsonl")
	now := time.Now().UTC()
	err := writeEventsFile(file,
		events.DispatchEvent{
			Meta: events.Meta{TS: now.Add(-2 * time.Hour), SpriteName: "bramble", EventKind: events.KindDispatch},
			Task: "old",
		},
		events.DispatchEvent{
			Meta: events.Meta{TS: now.Add(-10 * time.Minute), SpriteName: "bramble", EventKind: events.KindDispatch},
			Task: "new",
		},
		events.ErrorEvent{
			Meta:    events.Meta{TS: now.Add(-5 * time.Minute), SpriteName: "thorn", EventKind: events.KindError},
			Message: "boom",
		},
	)
	if err != nil {
		t.Fatalf("writeEventsFile() error = %v", err)
	}

	var out bytes.Buffer
	err = run(context.Background(), []string{
		"logs",
		"--file", file,
		"--since", "30m",
		"--sprite", "bramble",
		"--type", "dispatch",
	}, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run(logs) error = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("line count = %d, want 1", len(lines))
	}
	if !strings.Contains(lines[0], "bramble") || !strings.Contains(lines[0], "dispatch") {
		t.Fatalf("unexpected line: %q", lines[0])
	}
}

func TestLogsFollowJSON(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var out bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, []string{
			"logs",
			"--file", file,
			"--follow",
			"--json",
			"--poll-interval", "10ms",
		}, &out, &bytes.Buffer{})
	}()

	time.Sleep(30 * time.Millisecond)
	if err := writeEventsFile(file, events.ErrorEvent{
		Meta:    events.Meta{TS: time.Now().UTC(), SpriteName: "bramble", EventKind: events.KindError},
		Message: "runtime",
	}); err != nil {
		t.Fatalf("writeEventsFile() error = %v", err)
	}
	time.Sleep(80 * time.Millisecond)
	cancel()

	if err := <-done; err != nil {
		t.Fatalf("run(logs --follow) error = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatal("expected at least one JSON output line")
	}

	var envelope map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &envelope); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if envelope["type"] != "event" {
		t.Fatalf("unexpected envelope type: %#v", envelope["type"])
	}
}

func TestLogsValidationAndHelpers(t *testing.T) {
	t.Parallel()

	if _, _, err := parseTimeRange(time.Now().UTC(), "bad", ""); err == nil {
		t.Fatal("parseTimeRange() should reject bad since")
	}
	if _, err := buildEventFilter(nil, []string{"wat"}, time.Time{}, time.Time{}); err == nil {
		t.Fatal("buildEventFilter() should reject invalid type")
	}
}

func TestLogsRemoteSingleSprite(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	jsonl := marshalEventsToJSONL(t,
		events.DispatchEvent{
			Meta: events.Meta{TS: now.Add(-5 * time.Minute), SpriteName: "bramble", EventKind: events.KindDispatch},
			Task: "build-feature",
		},
		events.ErrorEvent{
			Meta:    events.Meta{TS: now.Add(-1 * time.Minute), SpriteName: "bramble", EventKind: events.KindError},
			Message: "compile failed",
		},
	)

	deps := logsDeps{
		newCLI: func(string, string) sprite.SpriteCLI {
			return &sprite.MockSpriteCLI{
				ExecFn: func(_ context.Context, name, cmd string, _ []byte) (string, error) {
					if name == "bramble" && strings.Contains(cmd, "cat") {
						return jsonl, nil
					}
					return "", nil
				},
			}
		},
	}

	var out bytes.Buffer
	cmd := newLogsCmdWithDeps(&out, &bytes.Buffer{}, deps)
	cmd.SetArgs([]string{"bramble"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("line count = %d, want 2, output=%q", len(lines), out.String())
	}
	if !strings.Contains(lines[0], "dispatch") {
		t.Fatalf("line[0] = %q, want dispatch event", lines[0])
	}
	if !strings.Contains(lines[1], "error") {
		t.Fatalf("line[1] = %q, want error event", lines[1])
	}
}

func TestLogsRemoteAllSprites(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	brambleJSONL := marshalEventsToJSONL(t,
		events.DispatchEvent{
			Meta: events.Meta{TS: now.Add(-10 * time.Minute), SpriteName: "bramble", EventKind: events.KindDispatch},
			Task: "task-a",
		},
	)
	thornJSONL := marshalEventsToJSONL(t,
		events.DispatchEvent{
			Meta: events.Meta{TS: now.Add(-5 * time.Minute), SpriteName: "thorn", EventKind: events.KindDispatch},
			Task: "task-b",
		},
	)

	deps := logsDeps{
		newCLI: func(string, string) sprite.SpriteCLI {
			return &sprite.MockSpriteCLI{
				ListFn: func(context.Context) ([]string, error) {
					return []string{"bramble", "thorn"}, nil
				},
				ExecFn: func(_ context.Context, name, cmd string, _ []byte) (string, error) {
					switch name {
					case "bramble":
						return brambleJSONL, nil
					case "thorn":
						return thornJSONL, nil
					}
					return "", nil
				},
			}
		},
	}

	var out bytes.Buffer
	cmd := newLogsCmdWithDeps(&out, &bytes.Buffer{}, deps)
	cmd.SetArgs([]string{"--all"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("line count = %d, want 2, output=%q", len(lines), out.String())
	}
	// Events sorted chronologically: bramble (-10m) then thorn (-5m)
	if !strings.Contains(lines[0], "bramble") {
		t.Fatalf("line[0] = %q, want bramble (earlier)", lines[0])
	}
	if !strings.Contains(lines[1], "thorn") {
		t.Fatalf("line[1] = %q, want thorn (later)", lines[1])
	}
}

func TestLogsRemoteJSON(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	jsonl := marshalEventsToJSONL(t,
		events.DispatchEvent{
			Meta: events.Meta{TS: now, SpriteName: "bramble", EventKind: events.KindDispatch},
			Task: "test-task",
		},
	)

	deps := logsDeps{
		newCLI: func(string, string) sprite.SpriteCLI {
			return &sprite.MockSpriteCLI{
				ExecFn: func(context.Context, string, string, []byte) (string, error) {
					return jsonl, nil
				},
			}
		},
	}

	var out bytes.Buffer
	cmd := newLogsCmdWithDeps(&out, &bytes.Buffer{}, deps)
	cmd.SetArgs([]string{"bramble", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var envelope map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &envelope); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, output=%q", err, out.String())
	}
	if envelope["type"] != "event" {
		t.Fatalf("type = %v, want event", envelope["type"])
	}
}

func TestLogsRemoteWithTypeFilter(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	jsonl := marshalEventsToJSONL(t,
		events.DispatchEvent{
			Meta: events.Meta{TS: now.Add(-5 * time.Minute), SpriteName: "bramble", EventKind: events.KindDispatch},
			Task: "filtered-out",
		},
		events.ErrorEvent{
			Meta:    events.Meta{TS: now.Add(-1 * time.Minute), SpriteName: "bramble", EventKind: events.KindError},
			Message: "kept",
		},
	)

	deps := logsDeps{
		newCLI: func(string, string) sprite.SpriteCLI {
			return &sprite.MockSpriteCLI{
				ExecFn: func(context.Context, string, string, []byte) (string, error) {
					return jsonl, nil
				},
			}
		},
	}

	var out bytes.Buffer
	cmd := newLogsCmdWithDeps(&out, &bytes.Buffer{}, deps)
	cmd.SetArgs([]string{"bramble", "--type", "error"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("line count = %d, want 1 (only error), output=%q", len(lines), out.String())
	}
	if !strings.Contains(lines[0], "error") {
		t.Fatalf("line = %q, want error event", lines[0])
	}
}

func TestLogsRemoteEmptyLog(t *testing.T) {
	t.Parallel()

	deps := logsDeps{
		newCLI: func(string, string) sprite.SpriteCLI {
			return &sprite.MockSpriteCLI{
				ExecFn: func(context.Context, string, string, []byte) (string, error) {
					return "", nil
				},
			}
		},
	}

	var out bytes.Buffer
	cmd := newLogsCmdWithDeps(&out, &bytes.Buffer{}, deps)
	cmd.SetArgs([]string{"bramble"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("expected empty output, got %q", out.String())
	}
}

func TestLogsModeValidation(t *testing.T) {
	t.Parallel()

	deps := logsDeps{
		newCLI: func(string, string) sprite.SpriteCLI {
			return &sprite.MockSpriteCLI{}
		},
	}

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "no source",
			args: nil,
			want: "provide --file paths, sprite names, or --all",
		},
		{
			name: "file and sprite args",
			args: []string{"bramble", "--file", "events.jsonl"},
			want: "cannot combine --file with sprite names or --all",
		},
		{
			name: "file and all",
			args: []string{"--file", "events.jsonl", "--all"},
			want: "cannot combine --file with sprite names or --all",
		},
		{
			name: "all and sprite args",
			args: []string{"bramble", "--all"},
			want: "cannot combine --all with explicit sprite names",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cmd := newLogsCmdWithDeps(&bytes.Buffer{}, &bytes.Buffer{}, deps)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want containing %q", err.Error(), tc.want)
			}
		})
	}
}

// marshalEventsToJSONL serializes events into JSONL format for mock CLI responses.
func marshalEventsToJSONL(t *testing.T, evts ...events.Event) string {
	t.Helper()
	var buf bytes.Buffer
	for _, event := range evts {
		data, err := events.MarshalEvent(event)
		if err != nil {
			t.Fatalf("MarshalEvent() error = %v", err)
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}
	return buf.String()
}
