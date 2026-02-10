package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestLogsRemoteSprite(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	ctx := context.Background()

	// Test with empty sprite name
	err := runRemoteSpriteLog(ctx, "", "", 10, false, &out)
	if err == nil {
		t.Fatal("runRemoteSpriteLog() should reject empty sprite name")
	}

	// Test with mock remote that returns sample logs
	mockRemote := &mockSpriteCLIRemote{
		execFn: func(ctx context.Context, sprite, command string, stdin []byte) (string, error) {
			if strings.Contains(command, "tail") {
				return "log line 1\nlog line 2\nlog line 3\n", nil
			}
			return "", nil
		},
	}

	out.Reset()
	err = runRemoteSpriteLogWithRemote(ctx, "test-sprite", mockRemote, 3, false, &out)
	if err != nil {
		t.Fatalf("runRemoteSpriteLogWithRemote() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "log line 1") {
		t.Fatalf("expected log line 1 in output, got: %q", output)
	}
}

type mockSpriteCLIRemote struct {
	execFn func(ctx context.Context, sprite, command string, stdin []byte) (string, error)
}

func (m *mockSpriteCLIRemote) Exec(ctx context.Context, sprite, command string, stdin []byte) (string, error) {
	if m.execFn != nil {
		return m.execFn(ctx, sprite, command, stdin)
	}
	return "", nil
}

func (m *mockSpriteCLIRemote) List(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (m *mockSpriteCLIRemote) Upload(ctx context.Context, sprite, remotePath string, content []byte) error {
	return nil
}

func runRemoteSpriteLogWithRemote(ctx context.Context, spriteName string, remote *mockSpriteCLIRemote, lines int, follow bool, out io.Writer) error {
	spriteName = strings.TrimSpace(spriteName)
	if spriteName == "" {
		return fmt.Errorf("sprite name is required")
	}

	logPath := ".bb-agent/agent.log"

	if lines > 0 {
		tailCmd := fmt.Sprintf("tail -n %d %s 2>/dev/null || echo '# no logs yet'", lines, shellQuote(logPath))
		output, err := remote.Exec(ctx, spriteName, tailCmd, nil)
		if err != nil {
			return fmt.Errorf("failed to read logs from sprite %q: %w", spriteName, err)
		}
		_, _ = fmt.Fprint(out, output)
	}

	return nil
}
