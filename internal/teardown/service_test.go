package teardown

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/misty-step/bitterblossom/internal/lib"
)

type mockRunner struct {
	requests []lib.RunRequest
	results  []lib.RunResult
	errors   []error
}

func (m *mockRunner) Run(_ context.Context, req lib.RunRequest) (lib.RunResult, error) {
	m.requests = append(m.requests, req)
	idx := len(m.requests) - 1
	if idx < len(m.errors) && m.errors[idx] != nil {
		var result lib.RunResult
		if idx < len(m.results) {
			result = m.results[idx]
		}
		return result, m.errors[idx]
	}
	if idx < len(m.results) {
		return m.results[idx], nil
	}
	return lib.RunResult{}, nil
}

func TestRedactSettingsJSON(t *testing.T) {
	raw := []byte(`{"env":{"ANTHROPIC_AUTH_TOKEN":"secret","OTHER":"x"}}`)
	redacted, err := RedactSettingsJSON(raw)
	if err != nil {
		t.Fatalf("redact failed: %v", err)
	}
	if strings.Contains(string(redacted), "secret") {
		t.Fatalf("token should be redacted")
	}
	if !strings.Contains(string(redacted), "__REDACTED__") {
		t.Fatalf("expected redacted marker")
	}
}

func TestTeardownAborted(t *testing.T) {
	paths, err := lib.NewPaths(t.TempDir())
	if err != nil {
		t.Fatalf("new paths: %v", err)
	}
	runner := &mockRunner{results: []lib.RunResult{{Stdout: "thorn\n"}}}
	svc := NewService(nil, lib.NewSpriteCLI(runner, "sprite", "misty-step"), paths, false)

	_, err = svc.TeardownSprite(context.Background(), "thorn", false, func(string) (bool, error) {
		return false, nil
	})
	if !errors.Is(err, ErrAborted) {
		t.Fatalf("expected ErrAborted, got %v", err)
	}
}

func TestTeardownExportsAndDestroys(t *testing.T) {
	root := t.TempDir()
	paths, err := lib.NewPaths(root)
	if err != nil {
		t.Fatalf("new paths: %v", err)
	}
	if err := os.MkdirAll(paths.ArchivesDir, 0o755); err != nil {
		t.Fatalf("mkdir archives: %v", err)
	}

	runner := &mockRunner{results: []lib.RunResult{
		{Stdout: "thorn\n"}, // exists list
		{Stdout: "memory"},
		{Stdout: "claude"},
		{Stdout: `{"env":{"ANTHROPIC_AUTH_TOKEN":"secret"}}`},
		{}, // checkpoint
		{}, // destroy
	}}
	svc := NewService(nil, lib.NewSpriteCLI(runner, "sprite", "misty-step"), paths, false)
	archivePath, err := svc.TeardownSprite(context.Background(), "thorn", true, nil)
	if err != nil {
		t.Fatalf("teardown failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(archivePath, "MEMORY.md")); err != nil {
		t.Fatalf("expected MEMORY.md archive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(archivePath, "CLAUDE.md")); err != nil {
		t.Fatalf("expected CLAUDE.md archive: %v", err)
	}
	settings, err := os.ReadFile(filepath.Join(archivePath, "settings.json"))
	if err != nil {
		t.Fatalf("read archived settings: %v", err)
	}
	if !strings.Contains(string(settings), "__REDACTED__") {
		t.Fatalf("expected redacted token in settings archive")
	}
}

func TestRedactSettingsJSONInvalid(t *testing.T) {
	if _, err := RedactSettingsJSON([]byte("not-json")); err == nil {
		t.Fatalf("expected parse error")
	}
}

func TestTeardownRequiresConfirmHandlerWhenNotForced(t *testing.T) {
	paths, err := lib.NewPaths(t.TempDir())
	if err != nil {
		t.Fatalf("new paths: %v", err)
	}
	runner := &mockRunner{results: []lib.RunResult{{Stdout: "thorn\n"}}}
	svc := NewService(nil, lib.NewSpriteCLI(runner, "sprite", "misty-step"), paths, false)
	_, err = svc.TeardownSprite(context.Background(), "thorn", false, nil)
	if err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestTeardownMissingSprite(t *testing.T) {
	paths, err := lib.NewPaths(t.TempDir())
	if err != nil {
		t.Fatalf("new paths: %v", err)
	}
	runner := &mockRunner{results: []lib.RunResult{{Stdout: ""}}}
	svc := NewService(nil, lib.NewSpriteCLI(runner, "sprite", "misty-step"), paths, false)
	_, err = svc.TeardownSprite(context.Background(), "thorn", true, nil)
	if err == nil {
		t.Fatalf("expected missing sprite error")
	}
}
