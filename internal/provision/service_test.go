package provision

import (
	"context"
	"fmt"
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

type funcRunner struct {
	requests []lib.RunRequest
	fn       func(req lib.RunRequest) (lib.RunResult, error)
}

func (f *funcRunner) Run(_ context.Context, req lib.RunRequest) (lib.RunResult, error) {
	f.requests = append(f.requests, req)
	if f.fn != nil {
		return f.fn(req)
	}
	return lib.RunResult{}, nil
}

func setupProvisionFixture(t *testing.T) (lib.Paths, string) {
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
			t.Fatalf("write %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(paths.BaseDir, "CLAUDE.md"), []byte("base"), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.BaseDir, "settings.json"), []byte(`{"env":{}}`), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
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
	compPath := filepath.Join(paths.CompsDir, "v1.yaml")
	if err := os.WriteFile(compPath, []byte("sprites:\n  thorn:\n    x: 1\n"), 0o644); err != nil {
		t.Fatalf("write composition: %v", err)
	}
	return paths, compPath
}

func TestResolveTargetsAll(t *testing.T) {
	paths, composition := setupProvisionFixture(t)
	runner := &mockRunner{}
	sprite := lib.NewSpriteCLI(runner, "sprite", "misty-step")
	svc := NewService(nil, sprite, runner, paths, composition, false)

	targets, resolved, err := svc.ResolveTargets(context.Background(), true, nil)
	if err != nil {
		t.Fatalf("resolve targets: %v", err)
	}
	if len(targets) != 1 || targets[0] != "thorn" {
		t.Fatalf("unexpected targets: %v", targets)
	}
	if resolved == "" {
		t.Fatalf("expected resolved composition path")
	}
}

func TestProvisionSpriteMissingDefinition(t *testing.T) {
	paths, composition := setupProvisionFixture(t)
	runner := &mockRunner{}
	sprite := lib.NewSpriteCLI(runner, "sprite", "misty-step")
	svc := NewService(nil, sprite, runner, paths, composition, false)

	err := svc.ProvisionSprite(context.Background(), "missing", paths.BaseSettingsPath(), composition, "token")
	if err == nil {
		t.Fatalf("expected error for missing definition")
	}
}

func TestBootstrapSpriteDryRunSkipsVerify(t *testing.T) {
	paths, composition := setupProvisionFixture(t)
	settings := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(settings, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	runner := &mockRunner{results: []lib.RunResult{{Stdout: ""}}}
	sprite := lib.NewSpriteCLI(runner, "sprite", "misty-step")
	svc := NewService(nil, sprite, runner, paths, composition, true)

	err := svc.BootstrapSprite(context.Background(), "thorn", filepath.Join(paths.SpritesDir, "thorn.md"), settings, "v1", "")
	if err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}
	for _, req := range runner.requests {
		joined := strings.Join(req.Args, " ")
		if strings.Contains(joined, "_git_test") {
			t.Fatalf("verify command should not run during dry-run")
		}
	}
}

func TestResolveGitHubTokenFromRunner(t *testing.T) {
	paths, composition := setupProvisionFixture(t)
	runner := &mockRunner{results: []lib.RunResult{{Stdout: "gh-token\n"}}}
	sprite := lib.NewSpriteCLI(runner, "sprite", "misty-step")
	svc := NewService(nil, sprite, runner, paths, composition, false)

	token, err := svc.resolveGitHubToken(context.Background(), "")
	if err != nil {
		t.Fatalf("resolve token: %v", err)
	}
	if token != "gh-token" {
		t.Fatalf("unexpected token: %q", token)
	}
}

func TestProvisionSpriteSuccess(t *testing.T) {
	paths, composition := setupProvisionFixture(t)
	settings := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(settings, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	runner := &funcRunner{
		fn: func(req lib.RunRequest) (lib.RunResult, error) {
			if len(req.Args) > 0 && req.Args[0] == "list" {
				return lib.RunResult{Stdout: ""}, nil
			}
			joined := strings.Join(req.Args, " ")
			if strings.Contains(joined, "_git_test") {
				return lib.RunResult{Stdout: "GIT_AUTH_OK"}, nil
			}
			return lib.RunResult{}, nil
		},
	}
	sprite := lib.NewSpriteCLI(runner, "sprite", "misty-step")
	svc := NewService(nil, sprite, runner, paths, composition, false)

	if err := svc.ProvisionSprite(context.Background(), "thorn", settings, composition, "github-token"); err != nil {
		t.Fatalf("provision sprite: %v", err)
	}
	if len(runner.requests) < 10 {
		t.Fatalf("expected many provisioning commands, got %d", len(runner.requests))
	}
}

func TestProvisionSpriteVerifyFailure(t *testing.T) {
	paths, composition := setupProvisionFixture(t)
	settings := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(settings, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	runner := &funcRunner{
		fn: func(req lib.RunRequest) (lib.RunResult, error) {
			if len(req.Args) > 0 && req.Args[0] == "list" {
				return lib.RunResult{Stdout: ""}, nil
			}
			joined := strings.Join(req.Args, " ")
			if strings.Contains(joined, "_git_test") {
				return lib.RunResult{Stdout: "GIT_AUTH_FAIL"}, nil
			}
			return lib.RunResult{}, nil
		},
	}
	sprite := lib.NewSpriteCLI(runner, "sprite", "misty-step")
	svc := NewService(nil, sprite, runner, paths, composition, false)

	err := svc.ProvisionSprite(context.Background(), "thorn", settings, composition, "github-token")
	if err == nil || !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("expected verification failure, got %v", err)
	}
}

func TestResolveTargetsValidation(t *testing.T) {
	paths, composition := setupProvisionFixture(t)
	runner := &mockRunner{}
	svc := NewService(nil, lib.NewSpriteCLI(runner, "sprite", "misty-step"), runner, paths, composition, false)
	_, _, err := svc.ResolveTargets(context.Background(), true, []string{"thorn"})
	if err == nil {
		t.Fatalf("expected validation error")
	}
	_, _, err = svc.ResolveTargets(context.Background(), false, nil)
	if err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestResolveGitHubTokenErrorsWithoutRunner(t *testing.T) {
	paths, composition := setupProvisionFixture(t)
	svc := NewService(nil, lib.NewSpriteCLI(&mockRunner{}, "sprite", "misty-step"), nil, paths, composition, false)
	_, err := svc.resolveGitHubToken(context.Background(), "")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestResolveGitHubTokenProvided(t *testing.T) {
	paths, composition := setupProvisionFixture(t)
	runner := &mockRunner{}
	svc := NewService(nil, lib.NewSpriteCLI(runner, "sprite", "misty-step"), runner, paths, composition, false)
	token, err := svc.resolveGitHubToken(context.Background(), "provided")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "provided" {
		t.Fatalf("unexpected token: %s", token)
	}
}

func TestPrepareRenderedSettingsSuccess(t *testing.T) {
	paths, composition := setupProvisionFixture(t)
	runner := &mockRunner{}
	svc := NewService(nil, lib.NewSpriteCLI(runner, "sprite", "misty-step"), runner, paths, composition, false)
	settingsPath, cleanup, err := svc.PrepareRenderedSettings("token")
	if err != nil {
		t.Fatalf("prepare settings: %v", err)
	}
	if settingsPath == "" {
		t.Fatalf("expected settings path")
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
}

func TestProvisionSpriteInvalidName(t *testing.T) {
	paths, composition := setupProvisionFixture(t)
	runner := &mockRunner{}
	svc := NewService(nil, lib.NewSpriteCLI(runner, "sprite", "misty-step"), runner, paths, composition, false)
	err := svc.ProvisionSprite(context.Background(), "BAD", "settings.json", composition, "x")
	if err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestProvisionSpriteSkipsCreateWhenExists(t *testing.T) {
	paths, composition := setupProvisionFixture(t)
	settings := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(settings, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	runner := &funcRunner{
		fn: func(req lib.RunRequest) (lib.RunResult, error) {
			if len(req.Args) > 0 && req.Args[0] == "list" {
				return lib.RunResult{Stdout: "thorn\n"}, nil
			}
			if strings.Contains(strings.Join(req.Args, " "), "_git_test") {
				return lib.RunResult{Stdout: "GIT_AUTH_OK"}, nil
			}
			return lib.RunResult{}, nil
		},
	}
	svc := NewService(nil, lib.NewSpriteCLI(runner, "sprite", "misty-step"), runner, paths, composition, false)
	if err := svc.ProvisionSprite(context.Background(), "thorn", settings, composition, "token"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, req := range runner.requests {
		if len(req.Args) > 0 && req.Args[0] == "create" {
			t.Fatalf("did not expect create command when sprite already exists: %v", req.Args)
		}
	}
}

func TestResolveGitHubTokenRunnerError(t *testing.T) {
	paths, composition := setupProvisionFixture(t)
	runner := &funcRunner{fn: func(req lib.RunRequest) (lib.RunResult, error) {
		return lib.RunResult{}, fmt.Errorf("gh failed")
	}}
	svc := NewService(nil, lib.NewSpriteCLI(runner, "sprite", "misty-step"), runner, paths, composition, false)
	_, err := svc.resolveGitHubToken(context.Background(), "")
	if err == nil {
		t.Fatalf("expected error")
	}
}
