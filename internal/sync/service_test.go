package sync

import (
	"context"
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

func setupSyncFixture(t *testing.T) (lib.Paths, string) {
	t.Helper()
	root := t.TempDir()
	paths, err := lib.NewPaths(root)
	if err != nil {
		t.Fatalf("new paths: %v", err)
	}
	for _, dir := range []string{"hooks", "skills", "commands"} {
		full := filepath.Join(paths.BaseDir, dir)
		if err := os.MkdirAll(full, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(full, "x.txt"), []byte("x"), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(paths.BaseDir, "CLAUDE.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write CLAUDE: %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.BaseDir, "settings.json"), []byte(`{"env":{}}`), 0o644); err != nil {
		t.Fatalf("write settings template: %v", err)
	}
	if err := os.MkdirAll(paths.SpritesDir, 0o755); err != nil {
		t.Fatalf("mkdir sprites: %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.SpritesDir, "thorn.md"), []byte("persona"), 0o644); err != nil {
		t.Fatalf("write sprite: %v", err)
	}
	if err := os.MkdirAll(paths.CompsDir, 0o755); err != nil {
		t.Fatalf("mkdir comps: %v", err)
	}
	comp := filepath.Join(paths.CompsDir, "v1.yaml")
	if err := os.WriteFile(comp, []byte("sprites:\n  thorn:\n    x: 1\n"), 0o644); err != nil {
		t.Fatalf("write comp: %v", err)
	}
	return paths, comp
}

func TestResolveTargetsDefaultsToComposition(t *testing.T) {
	paths, comp := setupSyncFixture(t)
	runner := &mockRunner{}
	svc := NewService(nil, lib.NewSpriteCLI(runner, "sprite", "misty-step"), paths, comp)
	targets, _, err := svc.ResolveTargets(nil)
	if err != nil {
		t.Fatalf("resolve targets: %v", err)
	}
	if len(targets) != 1 || targets[0] != "thorn" {
		t.Fatalf("unexpected targets: %v", targets)
	}
}

func TestSyncSpriteBaseOnly(t *testing.T) {
	paths, comp := setupSyncFixture(t)
	settings := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(settings, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	runner := &mockRunner{results: []lib.RunResult{{Stdout: "thorn\n"}}}
	svc := NewService(nil, lib.NewSpriteCLI(runner, "sprite", "misty-step"), paths, comp)
	if err := svc.SyncSprite(context.Background(), "thorn", settings, true); err != nil {
		t.Fatalf("sync sprite: %v", err)
	}
}

func TestSyncSpriteMissingSprite(t *testing.T) {
	paths, comp := setupSyncFixture(t)
	settings := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(settings, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	runner := &mockRunner{results: []lib.RunResult{{Stdout: "other\n"}}}
	svc := NewService(nil, lib.NewSpriteCLI(runner, "sprite", "misty-step"), paths, comp)
	if err := svc.SyncSprite(context.Background(), "thorn", settings, true); err == nil {
		t.Fatalf("expected missing sprite error")
	}
}

func TestResolveTargetsExplicit(t *testing.T) {
	paths, comp := setupSyncFixture(t)
	runner := &mockRunner{}
	svc := NewService(nil, lib.NewSpriteCLI(runner, "sprite", "misty-step"), paths, comp)
	targets, resolved, err := svc.ResolveTargets([]string{"thorn"})
	if err != nil {
		t.Fatalf("resolve explicit targets: %v", err)
	}
	if len(targets) != 1 || targets[0] != "thorn" {
		t.Fatalf("unexpected explicit targets: %v", targets)
	}
	if resolved != comp {
		t.Fatalf("expected composition path %s, got %s", comp, resolved)
	}
}

func TestSyncSpriteUploadsPersona(t *testing.T) {
	paths, comp := setupSyncFixture(t)
	settings := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(settings, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	runner := &mockRunner{results: []lib.RunResult{{Stdout: "thorn\n"}}}
	svc := NewService(nil, lib.NewSpriteCLI(runner, "sprite", "misty-step"), paths, comp)
	if err := svc.SyncSprite(context.Background(), "thorn", settings, false); err != nil {
		t.Fatalf("sync with persona failed: %v", err)
	}
	foundPersonaUpload := false
	for _, req := range runner.requests {
		if strings.Contains(strings.Join(req.Args, " "), "workspace/PERSONA.md") {
			foundPersonaUpload = true
			break
		}
	}
	if !foundPersonaUpload {
		t.Fatalf("expected persona upload command")
	}
}

func TestSyncSpriteMissingPersonaFileIsIgnored(t *testing.T) {
	paths, comp := setupSyncFixture(t)
	if err := os.Remove(filepath.Join(paths.SpritesDir, "thorn.md")); err != nil {
		t.Fatalf("remove persona: %v", err)
	}
	settings := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(settings, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	runner := &mockRunner{results: []lib.RunResult{{Stdout: "thorn\n"}}}
	svc := NewService(nil, lib.NewSpriteCLI(runner, "sprite", "misty-step"), paths, comp)
	if err := svc.SyncSprite(context.Background(), "thorn", settings, false); err != nil {
		t.Fatalf("expected missing persona to be ignored, got %v", err)
	}
}

func TestPrepareRenderedSettingsSuccessAndError(t *testing.T) {
	paths, comp := setupSyncFixture(t)
	runner := &mockRunner{}
	svc := NewService(nil, lib.NewSpriteCLI(runner, "sprite", "misty-step"), paths, comp)

	path, cleanup, err := svc.PrepareRenderedSettings("token")
	if err != nil {
		t.Fatalf("prepare settings: %v", err)
	}
	if path == "" {
		t.Fatalf("expected settings path")
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	if _, _, err := svc.PrepareRenderedSettings(""); err == nil {
		t.Fatalf("expected validation error when token missing")
	}
}
